package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"best-post-manager/internal/app"
)

type Error struct {
	Category       string
	Stage          string
	OutcomeUnknown bool
	Retryable      bool
	Err            error
}

func (e *Error) Error() string { return e.Category + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type Emoji struct {
	Shortcode string `json:"shortcode"`
	URL       string `json:"url"`
	StaticURL string `json:"static_url"`
}
type Result struct{ ID, URL string }
type Capability struct {
	MaxRunes      int
	MaxMedia      int
	MaxMediaBytes int64
}
type Client struct {
	HTTP  *http.Client
	mu    sync.Mutex
	emoji map[string][]Emoji
	caps  map[string]Capability
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, emoji: make(map[string][]Emoji), caps: make(map[string]Capability)}
}

func (c *Client) SetCapability(instance string, cap Capability) {
	c.mu.Lock()
	c.caps[strings.TrimRight(instance, "/")] = cap
	c.mu.Unlock()
}
func (c *Client) capability(instance string) Capability {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cap, ok := c.caps[strings.TrimRight(instance, "/")]; ok {
		return cap
	}
	return Capability{MaxRunes: 500, MaxMedia: 4, MaxMediaBytes: 32 << 20}
}

func (c *Client) request(ctx context.Context, account app.Account, method, endpoint string, body io.Reader, contentType, stage, idempotencyKey string) (*http.Response, error) {
	if strings.TrimSpace(account.TokenEnv) == "" {
		return nil, &Error{Category: "config", Stage: stage, Err: errors.New("token_env is required")}
	}
	token := strings.TrimSpace(os.Getenv(account.TokenEnv))
	if token == "" {
		return nil, &Error{Category: "config", Stage: stage, Err: fmt.Errorf("token environment variable %q is empty", account.TokenEnv)}
	}
	base, err := url.Parse(strings.TrimRight(account.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, &Error{Category: "config", Stage: stage, Err: errors.New("invalid instance URL")}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(account.BaseURL, "/")+endpoint, body)
	if err != nil {
		return nil, &Error{Category: "network", Stage: stage, OutcomeUnknown: stage == "submit", Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &Error{Category: "network", Stage: stage, OutcomeUnknown: stage == "submit", Err: err}
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, &Error{Category: "auth", Stage: stage, Err: fmt.Errorf("instance returned %s", resp.Status)}
	}
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		resp.Body.Close()
		return nil, &Error{Category: "retryable", Stage: stage, Retryable: true, Err: fmt.Errorf("instance returned %s", resp.Status)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, &Error{Category: "validation", Stage: stage, Err: fmt.Errorf("instance returned %s", resp.Status)}
	}
	return resp, nil
}

func (c *Client) CustomEmoji(ctx context.Context, account app.Account) ([]Emoji, error) {
	key := strings.TrimRight(account.BaseURL, "/")
	c.mu.Lock()
	if e, ok := c.emoji[key]; ok {
		out := append([]Emoji(nil), e...)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()
	resp, err := c.request(ctx, account, http.MethodGet, "/api/v1/custom_emojis", nil, "", "emoji", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var values []Emoji
	if err = json.NewDecoder(resp.Body).Decode(&values); err != nil {
		return nil, &Error{Category: "protocol", Stage: "emoji", Err: err}
	}
	c.mu.Lock()
	c.emoji[key] = append([]Emoji(nil), values...)
	c.mu.Unlock()
	return values, nil
}

func (c *Client) Validate(account app.Account, v app.Variant, media []app.Media) error {
	cap := c.capability(account.BaseURL)
	if v.Text == "" {
		return &Error{Category: "validation", Stage: "validate", Err: errors.New("text is empty")}
	}
	if v.Visibility != "" && v.Visibility != "public" && v.Visibility != "unlisted" && v.Visibility != "private" && v.Visibility != "direct" {
		return &Error{Category: "validation", Stage: "validate", Err: errors.New("invalid visibility")}
	}
	if cap.MaxRunes > 0 && utf8.RuneCountInString(v.Text) > cap.MaxRunes {
		return &Error{Category: "validation", Stage: "validate", Err: fmt.Errorf("text exceeds %d characters", cap.MaxRunes)}
	}
	if cap.MaxMedia > 0 && len(media) > cap.MaxMedia {
		return &Error{Category: "validation", Stage: "validate", Err: fmt.Errorf("at most %d media items are supported", cap.MaxMedia)}
	}
	var total int64
	for _, m := range media {
		if !strings.HasPrefix(strings.ToLower(m.MIME), "image/") {
			return &Error{Category: "validation", Stage: "media", Err: fmt.Errorf("media %q is not an image", m.Filename)}
		}
		info, e := os.Stat(m.Path)
		if e != nil {
			return &Error{Category: "media", Stage: "media", Err: e}
		}
		if m.Size > 0 && m.Size != info.Size() {
			return &Error{Category: "validation", Stage: "media", Err: fmt.Errorf("media %q size changed", m.Filename)}
		}
		total += info.Size()
	}
	if cap.MaxMediaBytes > 0 && total > cap.MaxMediaBytes {
		return &Error{Category: "validation", Stage: "media", Err: fmt.Errorf("media exceeds %d bytes", cap.MaxMediaBytes)}
	}
	return nil
}

func (c *Client) Publish(ctx context.Context, account app.Account, v app.Variant, media []app.Media, idempotencyKey string) (Result, error) {
	if err := c.Validate(account, v, media); err != nil {
		return Result{}, err
	}
	visibility := v.Visibility
	if visibility == "" {
		visibility = "public"
	}
	spoiler := v.SpoilerText
	var ids []string
	for _, m := range media {
		f, e := os.Open(m.Path)
		if e != nil {
			return Result{}, &Error{Category: "media", Stage: "media", Err: e}
		}
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		part, e := mw.CreateFormFile("file", filepath.Base(m.Path))
		if e == nil {
			_, e = io.Copy(part, f)
		}
		f.Close()
		if e == nil && m.AltText != "" {
			e = mw.WriteField("description", m.AltText)
		}
		contentType := mw.FormDataContentType()
		if ce := mw.Close(); e == nil {
			e = ce
		}
		if e != nil {
			return Result{}, &Error{Category: "media", Stage: "media", Err: e}
		}
		resp, e := c.request(ctx, account, http.MethodPost, "/api/v2/media", &b, contentType, "media", "")
		if e != nil {
			return Result{}, e
		}
		var result struct {
			ID string `json:"id"`
		}
		de := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if de != nil {
			return Result{}, &Error{Category: "protocol", Stage: "media", Err: de}
		}
		ids = append(ids, result.ID)
	}
	payload := map[string]any{"status": v.Text, "visibility": visibility, "spoiler_text": spoiler}
	if len(ids) > 0 {
		payload["media_ids"] = ids
	}
	raw, _ := json.Marshal(payload)
	resp, e := c.request(ctx, account, http.MethodPost, "/api/statuses", bytes.NewReader(raw), "application/json", "submit", idempotencyKey)
	if e != nil {
		return Result{}, e
	}
	defer resp.Body.Close()
	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if e = json.NewDecoder(resp.Body).Decode(&result); e != nil {
		return Result{}, &Error{Category: "protocol", Stage: "submit", OutcomeUnknown: true, Err: e}
	}
	return Result{ID: result.ID, URL: result.URL}, nil
}
