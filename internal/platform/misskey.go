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

	"best-post-manager/internal/app"
	"best-post-manager/internal/credential"
)

type MisskeyClient struct {
	HTTP        *http.Client
	Credentials credential.Store
	emoji       map[string][]Emoji
	mu          sync.Mutex
}

func (c *MisskeyClient) token(a app.Account, stage string) (string, error) {
	if a.CredentialRef != "" {
		if c.Credentials == nil {
			return "", &Error{Category: "config", Stage: stage, Err: errors.New("credential store is unavailable")}
		}
		value, err := c.Credentials.Get(a.CredentialRef)
		if err != nil {
			return "", &Error{Category: "config", Stage: stage, Err: errors.New("saved credential is unavailable; reconnect the account")}
		}
		return value, nil
	}
	value := strings.TrimSpace(os.Getenv(a.TokenEnv))
	if a.TokenEnv == "" || value == "" {
		return "", &Error{Category: "config", Stage: stage, Err: errors.New("Misskey token_env is empty")}
	}
	return value, nil
}

func NewMisskeyClient() *MisskeyClient {
	return &MisskeyClient{HTTP: &http.Client{Timeout: 30 * time.Second}, emoji: map[string][]Emoji{}}
}

func (c *MisskeyClient) request(ctx context.Context, a app.Account, endpoint string, payload any, stage string) (*http.Response, error) {
	token, tokenErr := c.token(a, stage)
	if tokenErr != nil {
		return nil, tokenErr
	}
	raw, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.BaseURL, "/")+"/api/"+endpoint, bytes.NewReader(raw))
	if e != nil {
		return nil, &Error{Category: "network", Stage: stage, Err: e}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, &Error{Category: "network", Stage: stage, OutcomeUnknown: stage == "submit", Err: e}
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, &Error{Category: "auth", Stage: stage, Err: fmt.Errorf("Misskey returned %s", resp.Status)}
	}
	if resp.StatusCode == 429 {
		resp.Body.Close()
		return nil, &Error{Category: "retryable", Stage: stage, Retryable: true, Err: fmt.Errorf("Misskey returned %s", resp.Status)}
	}
	if resp.StatusCode >= 500 {
		resp.Body.Close()
		return nil, &Error{Category: "retryable", Stage: stage, Retryable: stage != "submit", OutcomeUnknown: stage == "submit", Err: fmt.Errorf("Misskey returned %s", resp.Status)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, &Error{Category: "validation", Stage: stage, Err: fmt.Errorf("Misskey returned %s", resp.Status)}
	}
	return resp, nil
}
func (c *MisskeyClient) CustomEmoji(ctx context.Context, a app.Account) ([]Emoji, error) {
	key := strings.TrimRight(a.BaseURL, "/")
	c.mu.Lock()
	if v, ok := c.emoji[key]; ok {
		c.mu.Unlock()
		return append([]Emoji(nil), v...), nil
	}
	c.mu.Unlock()
	token, tokenErr := c.token(a, "emoji")
	if tokenErr != nil {
		return nil, tokenErr
	}
	resp, e := c.request(ctx, a, "emojis", map[string]any{"i": token}, "emoji")
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	var body struct {
		Emojis []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"emojis"`
	}
	if e = json.NewDecoder(resp.Body).Decode(&body); e != nil {
		return nil, &Error{Category: "protocol", Stage: "emoji", Err: e}
	}
	out := make([]Emoji, 0, len(body.Emojis))
	for _, x := range body.Emojis {
		name := x.Name
		if name == "" {
			continue
		}
		u := x.URL
		if u == "" {
			u = strings.TrimRight(a.BaseURL, "/") + "/emoji/" + url.PathEscape(name) + ".webp"
		}
		out = append(out, Emoji{Shortcode: name, URL: u, StaticURL: u})
	}
	c.mu.Lock()
	c.emoji[key] = append([]Emoji(nil), out...)
	c.mu.Unlock()
	return out, nil
}
func (c *MisskeyClient) Publish(ctx context.Context, a app.Account, v app.Variant, media []app.Media) (Result, error) {
	token, tokenErr := c.token(a, "validate")
	if tokenErr != nil {
		return Result{}, tokenErr
	}
	if v.Text == "" {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("text is empty")}
	}
	if v.Format != "" && v.Format != "mfm" {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("Misskey expects original MFM text")}
	}
	if v.Visibility != "" && !contains([]string{"public", "home", "followers"}, v.Visibility) {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("invalid Misskey visibility")}
	}
	if len(media) > 4 {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("at most 4 media items are supported")}
	}
	var ids []string
	for _, m := range media {
		if !strings.HasPrefix(strings.ToLower(m.MIME), "image/") {
			return Result{}, &Error{Category: "validation", Stage: "media", Err: fmt.Errorf("media %q is not an image", m.Filename)}
		}
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
		if e == nil {
			e = mw.WriteField("i", token)
		}
		ct := mw.FormDataContentType()
		if closeErr := mw.Close(); e == nil {
			e = closeErr
		}
		if e != nil {
			return Result{}, &Error{Category: "media", Stage: "media", Err: e}
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.BaseURL, "/")+"/api/drive/files/create", &b)
		if e != nil {
			return Result{}, &Error{Category: "network", Stage: "media", Err: e}
		}
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, e := c.HTTP.Do(req)
		if e != nil {
			return Result{}, &Error{Category: "network", Stage: "media", Err: e}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			category, retryable := "validation", false
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				category = "auth"
			} else if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				category, retryable = "retryable", true
			}
			return Result{}, &Error{Category: category, Stage: "media", Retryable: retryable, Err: fmt.Errorf("Misskey media returned %s", resp.Status)}
		}
		var file struct {
			ID string `json:"id"`
		}
		e = json.NewDecoder(resp.Body).Decode(&file)
		resp.Body.Close()
		if e != nil {
			return Result{}, &Error{Category: "protocol", Stage: "media", Err: e}
		}
		ids = append(ids, file.ID)
	}
	payload := map[string]any{"i": token, "text": v.Text, "visibility": v.Visibility, "cw": v.SpoilerText, "localOnly": v.LocalOnly}
	if payload["visibility"] == "" {
		payload["visibility"] = "public"
	}
	if len(ids) > 0 {
		payload["fileIds"] = ids
	}
	resp, e := c.request(ctx, a, "notes/create", payload, "submit")
	if e != nil {
		return Result{}, e
	}
	defer resp.Body.Close()
	var out struct {
		CreatedNote struct {
			ID string `json:"id"`
		} `json:"createdNote"`
	}
	if e = json.NewDecoder(resp.Body).Decode(&out); e != nil {
		return Result{}, &Error{Category: "protocol", Stage: "submit", OutcomeUnknown: true, Err: e}
	}
	return Result{ID: out.CreatedNote.ID, URL: strings.TrimRight(a.BaseURL, "/") + "/notes/" + out.CreatedNote.ID}, nil
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func (c *MisskeyClient) Import(ctx context.Context, a app.Account, noteID string) (app.ExternalPost, error) {
	if parsed, err := url.Parse(noteID); err == nil && parsed.Host != "" {
		base, baseErr := url.Parse(a.BaseURL)
		if baseErr != nil || !strings.EqualFold(parsed.Host, base.Host) {
			return app.ExternalPost{}, &Error{Category: "validation", Stage: "import", Err: errors.New("Misskey note URL must belong to the configured instance")}
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			noteID = parts[len(parts)-1]
		}
	}
	if strings.TrimSpace(noteID) == "" {
		return app.ExternalPost{}, &Error{Category: "validation", Stage: "import", Err: errors.New("Misskey note ID or URL is required")}
	}
	token, tokenErr := c.token(a, "import")
	if tokenErr != nil {
		return app.ExternalPost{}, tokenErr
	}
	resp, e := c.request(ctx, a, "notes/show", map[string]any{"i": token, "noteId": noteID}, "import")
	if e != nil {
		return app.ExternalPost{}, e
	}
	defer resp.Body.Close()
	var n struct {
		ID, Text  string
		CreatedAt string `json:"createdAt"`
	}
	if e = json.NewDecoder(resp.Body).Decode(&n); e != nil {
		return app.ExternalPost{}, &Error{Category: "protocol", Stage: "import", Err: e}
	}
	return app.ExternalPost{Platform: "misskey", AccountID: a.ID, RemoteID: n.ID, URL: strings.TrimRight(a.BaseURL, "/") + "/notes/" + n.ID, TextSnapshot: n.Text, CapturedBy: "official_api"}, nil
}
