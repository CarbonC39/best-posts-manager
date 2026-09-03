package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	"best-post-manager/internal/credential"
)

type Error struct {
	Category       string
	Stage          string
	RemoteStatus   int
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
type Result struct{ ID, URL, CID string }

type MastodonPermissionSignal struct {
	Endpoint string `json:"endpoint"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

// MastodonDiagnostic is safe for the WebUI: it contains identity metadata but
// never tokens or remote response bodies. Permission probes use OPTIONS only.
type MastodonDiagnostic struct {
	OK          bool                     `json:"ok"`
	Stage       string                   `json:"stage"`
	Category    string                   `json:"category,omitempty"`
	Message     string                   `json:"message"`
	Hint        string                   `json:"hint,omitempty"`
	Username    string                   `json:"username,omitempty"`
	AccountID   string                   `json:"account_id,omitempty"`
	StatusWrite MastodonPermissionSignal `json:"status_write"`
	MediaWrite  MastodonPermissionSignal `json:"media_write"`
	ElapsedMS   int64                    `json:"elapsed_ms"`
}
type Capability struct {
	MaxRunes      int
	MaxMedia      int
	MaxMediaBytes int64
}
type Client struct {
	HTTP        *http.Client
	Credentials credential.Store
	mu          sync.Mutex
	emoji       map[string][]Emoji
	caps        map[string]Capability
}

func (c *Client) token(account app.Account, stage string) (string, error) {
	if account.CredentialRef != "" {
		if c.Credentials == nil {
			return "", &Error{Category: "config", Stage: stage, Err: errors.New("credential store is unavailable")}
		}
		token, err := c.Credentials.Get(account.CredentialRef)
		if err != nil {
			return "", &Error{Category: "config", Stage: stage, Err: errors.New("saved credential is unavailable; reconnect the account")}
		}
		return token, nil
	}
	if strings.TrimSpace(account.TokenEnv) == "" {
		return "", &Error{Category: "config", Stage: stage, Err: errors.New("token_env is required")}
	}
	token := strings.TrimSpace(os.Getenv(account.TokenEnv))
	if token == "" {
		return "", &Error{Category: "config", Stage: stage, Err: fmt.Errorf("token environment variable %q is empty", account.TokenEnv)}
	}
	return token, nil
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, emoji: make(map[string][]Emoji), caps: make(map[string]Capability)}
}

// CheckConnection checks instance discovery, token authentication and the
// authenticated identity. It then sends OPTIONS probes to the status/media
// endpoints as a permission signal; it never sends a status or media body.
func (c *Client) CheckConnection(ctx context.Context, account app.Account) MastodonDiagnostic {
	started := time.Now()
	d := MastodonDiagnostic{Stage: "validate", Message: "正在检查 Mastodon-compatible 配置", StatusWrite: permissionSignal("/api/v1/statuses"), MediaWrite: permissionSignal("/api/v2/media")}
	finish := func() MastodonDiagnostic {
		d.ElapsedMS = time.Since(started).Milliseconds()
		if d.OK {
			log.Printf("mastodon diagnostic stage=%s result=ok account=%s status_write=%s media_write=%s", d.Stage, account.ID, d.StatusWrite.Status, d.MediaWrite.Status)
		} else {
			log.Printf("mastodon diagnostic stage=%s category=%s result=failed account=%s", d.Stage, d.Category, account.ID)
		}
		return d
	}
	base := strings.TrimSpace(account.BaseURL)
	u, err := url.ParseRequestURI(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		d.Category, d.Message, d.Hint = "config", "实例地址无效", "填写完整的 http(s)://实例地址，不要包含账号密码、查询参数或页面路径"
		return finish()
	}
	token, err := c.token(account, "auth")
	if err != nil {
		d.Category, d.Message, d.Hint = mastodonDiagnosticError(err)
		return finish()
	}
	d.Stage = "instance"
	resp, err := c.request(ctx, account, http.MethodGet, "/api/v1/instance", nil, "", "instance", "")
	if err != nil {
		d.Category, d.Message, d.Hint = mastodonDiagnosticError(err)
		return finish()
	}
	resp.Body.Close()
	d.Stage = "auth"
	resp, err = c.request(ctx, account, http.MethodGet, "/api/v1/accounts/verify_credentials", nil, "", "auth", "")
	if err != nil {
		if pe, ok := err.(*Error); ok && pe.Category == "auth" && pe.RemoteStatus == http.StatusForbidden {
			d.OK, d.Stage, d.Category = true, "permissions", "实例连通，但账号身份无法验证"
			d.Hint = "token 可能缺少 read:accounts，也可能是账号受限；重新授权后再测。OPTIONS 结果仅是端点兼容信号"
			d.StatusWrite = c.probePermission(ctx, account, token, "/api/v1/statuses")
			d.MediaWrite = c.probePermission(ctx, account, token, "/api/v2/media")
			return finish()
		}
		d.Category, d.Message, d.Hint = mastodonDiagnosticError(err)
		return finish()
	}
	var identity struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&identity)
	resp.Body.Close()
	if decodeErr != nil || identity.ID == "" || identity.Username == "" {
		d.Category, d.Message, d.Hint = "protocol", "身份端点返回的信息不完整", "确认实例地址指向 Mastodon-compatible API，而不是前端页面"
		return finish()
	}
	d.OK, d.Stage, d.Message = true, "permissions", "实例连通且账号身份验证成功"
	d.Username, d.AccountID = identity.Username, identity.ID
	d.StatusWrite = c.probePermission(ctx, account, token, "/api/v1/statuses")
	d.MediaWrite = c.probePermission(ctx, account, token, "/api/v2/media")
	if d.StatusWrite.Status == "denied" || d.MediaWrite.Status == "denied" {
		d.Category, d.Message, d.Hint = "compatibility", "账号身份有效，但实例拒绝了 OPTIONS 探测", "这不能单独证明缺少写权限；若实际发布失败，请按发布记录中的媒体上传/正文提交阶段排查"
	} else if d.StatusWrite.Status == "unknown" || d.MediaWrite.Status == "unknown" {
		d.Category, d.Message, d.Hint = "permission", "账号已连接，但实例未明确返回写权限信号", "确认 OAuth scope 包含写入动态/媒体；可先用无媒体草稿做人工发布测试"
	}
	return finish()
}

func permissionSignal(endpoint string) MastodonPermissionSignal {
	return MastodonPermissionSignal{Endpoint: endpoint, Status: "unknown", Message: "尚未测试"}
}

func (c *Client) probePermission(ctx context.Context, account app.Account, token, endpoint string) MastodonPermissionSignal {
	signal := permissionSignal(endpoint)
	u := strings.TrimRight(account.BaseURL, "/") + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, u, nil)
	if err != nil {
		signal.Message = "无法构造权限探测请求"
		return signal
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		signal.Status, signal.Message = "unknown", "无法连接权限探测端点"
		return signal
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		signal.Status, signal.Message = "available", "端点响应 OPTIONS（仅兼容性信号，不等于 scope 已确认）"
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		signal.Status, signal.Message = "denied", "实例拒绝访问，可能缺少对应写权限"
	case resp.StatusCode == http.StatusNotFound:
		signal.Status, signal.Message = "missing", "实例没有这个标准端点"
	case resp.StatusCode == http.StatusMethodNotAllowed:
		signal.Status, signal.Message = "unknown", "实例不支持 OPTIONS，无法仅凭探测确认 scope"
	default:
		signal.Status, signal.Message = "unknown", "实例返回了无法判断权限的状态"
	}
	return signal
}

func mastodonDiagnosticError(err error) (category, message, hint string) {
	pe, ok := err.(*Error)
	if !ok {
		return "unknown", "连接测试失败", "查看 bpm 进程日志后检查实例地址和网络"
	}
	switch pe.Category {
	case "config":
		return "config", "本地 token 配置不可用", "检查 token 环境变量名和值，或重新进行 OAuth 授权"
	case "auth":
		return "auth", "token 无效或权限不足", "重新 OAuth 授权，并确认授予写入动态和媒体权限"
	case "network":
		return "network", "无法连接到实例", "检查实例地址、代理/VPN 和网络连通性"
	case "retryable":
		return "retryable", "实例暂时不可用或限流", "稍后重试并检查实例状态"
	case "protocol":
		return "protocol", "实例返回格式不符合预期", "确认地址是 Mastodon-compatible API，而不是前端页面"
	default:
		return pe.Category, "连接测试失败", "检查实例地址和 token 后重试"
	}
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
	token, tokenErr := c.token(account, stage)
	if tokenErr != nil {
		return nil, tokenErr
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
		detail := remoteErrorDetail(resp.Body)
		resp.Body.Close()
		return nil, &Error{Category: "auth", Stage: stage, RemoteStatus: resp.StatusCode, Err: remoteStatusError(resp.Status, detail)}
	}
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		detail := remoteErrorDetail(resp.Body)
		resp.Body.Close()
		return nil, &Error{Category: "retryable", Stage: stage, RemoteStatus: resp.StatusCode, Retryable: true, Err: remoteStatusError(resp.Status, detail)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := remoteErrorDetail(resp.Body)
		resp.Body.Close()
		return nil, &Error{Category: "validation", Stage: stage, RemoteStatus: resp.StatusCode, Err: remoteStatusError(resp.Status, detail)}
	}
	return resp, nil
}

func remoteErrorDetail(body io.Reader) string {
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if json.NewDecoder(io.LimitReader(body, 64<<10)).Decode(&payload) != nil {
		return ""
	}
	detail := strings.TrimSpace(payload.Error)
	if detail == "" {
		detail = strings.TrimSpace(payload.ErrorDescription)
	}
	clean := strings.Map(func(r rune) rune {
		if r < ' ' || r == '\u007f' {
			return ' '
		}
		return r
	}, detail)
	clean = strings.Join(strings.Fields(clean), " ")
	runes := []rune(clean)
	if len(runes) > 300 {
		clean = string(runes[:300]) + "…"
	}
	return clean
}

func remoteStatusError(status, detail string) error {
	if detail == "" {
		return fmt.Errorf("instance returned %s", status)
	}
	return fmt.Errorf("instance returned %s: %s", status, detail)
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
	resp, e := c.request(ctx, account, http.MethodPost, "/api/v1/statuses", bytes.NewReader(raw), "application/json", "submit", idempotencyKey)
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
