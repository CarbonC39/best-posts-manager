package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"best-post-manager/internal/app"
	"best-post-manager/internal/credential"
	"github.com/rivo/uniseg"
)

type BlueskyClient struct {
	HTTP        *http.Client
	Credentials credential.Store
}

// BlueskyDiagnostic is deliberately safe to show in the WebUI. It contains
// no access/refresh token and its message is selected from a fixed category,
// rather than exposing a remote response body or URL-bearing network error.
type BlueskyDiagnostic struct {
	OK        bool   `json:"ok"`
	Stage     string `json:"stage"`
	Category  string `json:"category,omitempty"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	Handle    string `json:"handle,omitempty"`
	DID       string `json:"did,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

func NewBlueskyClient() *BlueskyClient {
	return &BlueskyClient{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// CheckConnection validates the local configuration and performs a harmless
// createSession call. It never publishes, reads a post, or logs credentials.
func (c *BlueskyClient) CheckConnection(ctx context.Context, a app.Account) BlueskyDiagnostic {
	started := time.Now()
	d := BlueskyDiagnostic{Stage: "validate", Message: "正在检查 Bluesky 配置"}
	finish := func() BlueskyDiagnostic {
		d.ElapsedMS = time.Since(started).Milliseconds()
		if d.OK {
			log.Printf("bluesky diagnostic stage=%s result=ok account=%s", d.Stage, a.ID)
		} else {
			log.Printf("bluesky diagnostic stage=%s category=%s result=failed account=%s", d.Stage, d.Category, a.ID)
		}
		return d
	}
	base := strings.TrimSpace(a.BaseURL)
	u, err := url.ParseRequestURI(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		d.Category, d.Message, d.Hint = "config", "PDS 地址无效", "填写完整的 http(s)://PDS 地址，不要包含账号密码或发布页面路径"
		return finish()
	}
	identity := strings.TrimSpace(a.RemoteID)
	if identity == "" || strings.ContainsAny(identity, " \t\r\n") {
		d.Category, d.Message, d.Hint = "config", "Bluesky 身份为空", "填写 handle（例如 alice.bsky.social）或 DID"
		return finish()
	}
	if a.CredentialRef == "" && strings.TrimSpace(a.TokenEnv) == "" {
		d.Category, d.Message, d.Hint = "config", "未配置 app password 环境变量", "填写环境变量名，并在启动 bpm 前导出 app password；不要把密码写入配置或日志"
		return finish()
	}
	d.Stage = "auth"
	s, err := c.session(ctx, a)
	if err != nil {
		d.Category, d.Message, d.Hint = diagnosticError(err)
		return finish()
	}
	if s.DID == "" || s.Handle == "" {
		d.Category, d.Message, d.Hint = "protocol", "PDS 返回的身份信息不完整", "确认 PDS 地址指向 Bluesky 服务端，并重新测试"
		return finish()
	}
	d.OK = true
	d.Stage = "auth"
	d.Message = "Bluesky 连接成功"
	d.Handle, d.DID = s.Handle, s.DID
	return finish()
}

func diagnosticError(err error) (category, message, hint string) {
	pe, ok := err.(*Error)
	if !ok {
		return "unknown", "连接测试失败", "查看 bpm 进程日志后检查 PDS 地址和网络"
	}
	switch pe.Category {
	case "config":
		return "config", "本地凭据配置不可用", "检查环境变量名、环境变量值，或重新添加账号"
	case "auth":
		return "auth", "app password 无效或已过期", "在 Bluesky 设置中创建新的 app password；不要使用主密码"
	case "network":
		return "network", "无法连接到 PDS", "检查 PDS 地址、代理/VPN 和网络连通性"
	case "retryable":
		return "retryable", "PDS 暂时不可用或限流", "稍后重试；确认 PDS 地址没有被网关拦截"
	case "protocol":
		return "protocol", "PDS 返回格式不符合预期", "确认地址是 Bluesky PDS，而不是 Web 前端地址"
	default:
		return pe.Category, "Bluesky 连接测试失败", "检查 PDS 地址和 app password 后重试"
	}
}

type bskySession struct {
	DID        string `json:"did"`
	Handle     string `json:"handle"`
	AccessJWT  string `json:"accessJwt"`
	RefreshJWT string `json:"refreshJwt"`
}

func (c *BlueskyClient) call(ctx context.Context, a app.Account, method string, payload any, stage, token string) (json.RawMessage, error) {
	raw, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.BaseURL, "/")+"/xrpc/"+method, bytes.NewReader(raw))
	if e != nil {
		return nil, &Error{Category: "network", Stage: stage, Err: e}
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, &Error{Category: "network", Stage: stage, OutcomeUnknown: stage == "submit", Err: e}
	}
	return decodeXRPCResponse(resp, stage)
}

// query handles XRPC read methods, which use GET query parameters rather than
// POST JSON (notably resolveHandle and repo.getRecord).
func (c *BlueskyClient) query(ctx context.Context, a app.Account, method string, params url.Values, stage, token string) (json.RawMessage, error) {
	u := strings.TrimRight(a.BaseURL, "/") + "/xrpc/" + method
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return nil, &Error{Category: "network", Stage: stage, Err: e}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, &Error{Category: "network", Stage: stage, Err: e}
	}
	return decodeXRPCResponse(resp, stage)
}

func decodeXRPCResponse(resp *http.Response, stage string) (json.RawMessage, error) {
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, &Error{Category: "auth", Stage: stage, Err: errors.New("Bluesky session expired; sign in again")}
	}
	if resp.StatusCode == 429 {
		resp.Body.Close()
		return nil, &Error{Category: "retryable", Stage: stage, Retryable: true, Err: fmt.Errorf("Bluesky returned %s", resp.Status)}
	}
	if resp.StatusCode >= 500 {
		resp.Body.Close()
		return nil, &Error{Category: "retryable", Stage: stage, Retryable: stage != "submit", OutcomeUnknown: stage == "submit", Err: fmt.Errorf("Bluesky returned %s", resp.Status)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, &Error{Category: "validation", Stage: stage, Err: fmt.Errorf("Bluesky returned %s", resp.Status)}
	}
	defer resp.Body.Close()
	out, e := io.ReadAll(resp.Body)
	if e != nil {
		return nil, &Error{Category: "network", Stage: stage, OutcomeUnknown: stage == "submit", Err: e}
	}
	return out, nil
}
func (c *BlueskyClient) session(ctx context.Context, a app.Account) (bskySession, error) {
	password := ""
	if a.CredentialRef != "" {
		if c.Credentials == nil {
			return bskySession{}, &Error{Category: "config", Stage: "auth", Err: errors.New("credential store is unavailable")}
		}
		var err error
		password, err = c.Credentials.Get(a.CredentialRef)
		if err != nil {
			return bskySession{}, &Error{Category: "config", Stage: "auth", Err: errors.New("saved credential is unavailable; reconnect the account")}
		}
	} else {
		password = strings.TrimSpace(os.Getenv(a.TokenEnv))
		if a.TokenEnv == "" || password == "" {
			return bskySession{}, &Error{Category: "config", Stage: "auth", Err: errors.New("Bluesky app password token_env is empty")}
		}
	}
	raw, e := c.call(ctx, a, "com.atproto.server.createSession", map[string]string{"identifier": a.RemoteID, "password": password}, "auth", "")
	if e != nil {
		return bskySession{}, e
	}
	var s bskySession
	if e = json.Unmarshal(raw, &s); e != nil {
		return s, &Error{Category: "protocol", Stage: "auth", Err: e}
	}
	if s.AccessJWT == "" || s.DID == "" {
		return s, &Error{Category: "auth", Stage: "auth", Err: errors.New("Bluesky session response missing access token")}
	}
	return s, nil
}

var linkRE = regexp.MustCompile(`https?://[^\s]+`)
var mentionRE = regexp.MustCompile(`@[a-zA-Z0-9._-]+(?:\.[a-zA-Z0-9._-]+)+`)

type facet struct {
	Index struct {
		ByteStart int `json:"byteStart"`
		ByteEnd   int `json:"byteEnd"`
	} `json:"index"`
	Features []map[string]string `json:"features"`
}

func facets(ctx context.Context, c *BlueskyClient, a app.Account, s string, session bskySession) []facet {
	var out []facet
	links := linkRE.FindAllStringIndex(s, -1)
	for _, m := range links {
		var f facet
		f.Index.ByteStart, f.Index.ByteEnd = m[0], m[1]
		f.Features = []map[string]string{{"$type": "app.bsky.richtext.facet#link", "uri": s[m[0]:m[1]]}}
		out = append(out, f)
	}
	for _, m := range mentionRE.FindAllStringIndex(s, -1) {
		if overlapsAny(m, links) {
			continue
		}
		handle := strings.TrimPrefix(s[m[0]:m[1]], "@")
		raw, e := c.query(ctx, a, "com.atproto.identity.resolveHandle", url.Values{"handle": []string{handle}}, "resolve", session.AccessJWT)
		if e != nil {
			continue
		}
		var v struct {
			DID string `json:"did"`
		}
		if json.Unmarshal(raw, &v) != nil || v.DID == "" {
			continue
		}
		var f facet
		f.Index.ByteStart, f.Index.ByteEnd = m[0], m[1]
		f.Features = []map[string]string{{"$type": "app.bsky.richtext.facet#mention", "did": v.DID}}
		out = append(out, f)
	}
	return out
}

func overlapsAny(candidate []int, ranges [][]int) bool {
	for _, current := range ranges {
		if candidate[0] < current[1] && current[0] < candidate[1] {
			return true
		}
	}
	return false
}
func (c *BlueskyClient) Publish(ctx context.Context, a app.Account, v app.Variant, media []app.Media) (Result, error) {
	if v.Format != "" && v.Format != "plain" {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("Bluesky supports plain text/facets only")}
	}
	if !utf8.ValidString(v.Text) || len([]byte(v.Text)) == 0 {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("text is empty")}
	}
	// app.bsky.feed.post declares maxLength 3000 bytes and maxGraphemes 300.
	// A Unicode segmenter is required because rune count is not grapheme count.
	if len([]byte(v.Text)) > 3000 {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("Bluesky text exceeds the 3000-byte record limit")}
	}
	if uniseg.GraphemeClusterCount(v.Text) > 300 {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("Bluesky text exceeds the 300-grapheme post limit")}
	}
	if len(media) > 4 {
		return Result{}, &Error{Category: "validation", Stage: "validate", Err: errors.New("at most 4 images are supported")}
	}
	s, e := c.session(ctx, a)
	if e != nil {
		return Result{}, e
	}
	var embeds []map[string]any
	for _, m := range media {
		if !strings.HasPrefix(strings.ToLower(m.MIME), "image/") {
			return Result{}, &Error{Category: "validation", Stage: "media", Err: errors.New("Bluesky media must be images")}
		}
		data, e := os.ReadFile(m.Path)
		if e != nil {
			return Result{}, &Error{Category: "media", Stage: "media", Err: e}
		}
		if len(data) > 1_000_000 {
			return Result{}, &Error{Category: "validation", Stage: "media", Err: errors.New("Bluesky image exceeds the 1,000,000-byte blob limit")}
		}
		raw, e := c.uploadBlob(ctx, a, data, m.MIME, s.AccessJWT)
		if e != nil {
			return Result{}, e
		}
		var b struct {
			Blob map[string]any `json:"blob"`
		}
		if e = json.Unmarshal(raw, &b); e != nil {
			return Result{}, &Error{Category: "protocol", Stage: "media", Err: e}
		}
		embeds = append(embeds, map[string]any{"alt": m.AltText, "image": b.Blob})
	}
	record := map[string]any{"$type": "app.bsky.feed.post", "text": v.Text, "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}
	if fs := facets(ctx, c, a, v.Text, s); len(fs) > 0 {
		record["facets"] = fs
	}
	if len(embeds) > 0 {
		record["embed"] = map[string]any{"$type": "app.bsky.embed.images", "images": embeds}
	}
	raw, _ := json.Marshal(map[string]any{"repo": s.DID, "collection": "app.bsky.feed.post", "record": record})
	out, e := c.call(ctx, a, "com.atproto.repo.createRecord", json.RawMessage(raw), "submit", s.AccessJWT)
	if e != nil {
		return Result{}, e
	}
	var result struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if e = json.Unmarshal(out, &result); e != nil {
		return Result{}, &Error{Category: "protocol", Stage: "submit", OutcomeUnknown: true, Err: e}
	}
	return Result{ID: result.URI, URL: bskyURL(result.URI), CID: result.CID}, nil
}
func (c *BlueskyClient) uploadBlob(ctx context.Context, a app.Account, data []byte, mime, token string) (json.RawMessage, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.BaseURL, "/")+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(data))
	if e != nil {
		return nil, &Error{Category: "network", Stage: "media", Err: e}
	}
	req.Header.Set("Content-Type", mime)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, &Error{Category: "network", Stage: "media", Err: e}
	}
	if resp.StatusCode == 401 {
		resp.Body.Close()
		return nil, &Error{Category: "auth", Stage: "media", Err: errors.New("Bluesky session expired; sign in again")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		category, retryable := "validation", false
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			category, retryable = "retryable", true
		}
		return nil, &Error{Category: category, Stage: "media", Retryable: retryable, Err: fmt.Errorf("Bluesky blob upload returned %s", resp.Status)}
	}
	defer resp.Body.Close()
	out, e := io.ReadAll(resp.Body)
	if e != nil {
		return nil, &Error{Category: "network", Stage: "media", Err: e}
	}
	return out, nil
}
func (c *BlueskyClient) Import(ctx context.Context, a app.Account, ref string) (app.ExternalPost, error) {
	s, e := c.session(ctx, a)
	if e != nil {
		return app.ExternalPost{}, e
	}
	repo, rkey, e := parseRef(ref)
	if e != nil {
		return app.ExternalPost{}, e
	}
	raw, e := c.query(ctx, a, "com.atproto.repo.getRecord", url.Values{"repo": []string{repo}, "collection": []string{"app.bsky.feed.post"}, "rkey": []string{rkey}}, "import", s.AccessJWT)
	if e != nil {
		return app.ExternalPost{}, e
	}
	var rec struct {
		URI   string `json:"uri"`
		CID   string `json:"cid"`
		Value struct {
			Text      string `json:"text"`
			CreatedAt string `json:"createdAt"`
		} `json:"value"`
	}
	if e = json.Unmarshal(raw, &rec); e != nil {
		return app.ExternalPost{}, &Error{Category: "protocol", Stage: "import", Err: e}
	}
	return app.ExternalPost{Platform: "bluesky", AccountID: a.ID, RemoteID: rec.URI, URL: bskyURL(rec.URI), TextSnapshot: rec.Value.Text, CapturedBy: "official_api"}, nil
}
func parseRef(ref string) (string, string, error) {
	if strings.HasPrefix(ref, "at://") {
		p := strings.Split(strings.TrimPrefix(ref, "at://"), "/")
		if len(p) == 3 && p[1] == "app.bsky.feed.post" && p[0] != "" && p[2] != "" {
			return p[0], p[2], nil
		}
	}
	u, e := url.Parse(ref)
	if e == nil && u.Host == "bsky.app" {
		p := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(p) >= 4 && p[0] == "profile" && p[2] == "post" {
			return p[1], p[3], nil
		}
	}
	return "", "", &Error{Category: "validation", Stage: "import", Err: errors.New("invalid Bluesky at:// or bsky.app post URL")}
}
func bskyURL(uri string) string {
	p := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if len(p) >= 3 {
		return "https://bsky.app/profile/" + p[0] + "/post/" + p[2]
	}
	return uri
}
