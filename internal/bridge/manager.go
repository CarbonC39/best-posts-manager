package bridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"best-post-manager/internal/app"
)

const (
	ProtocolVersion = "v1"
	PairingTTL      = 10 * time.Minute
	TokenTTL        = 90 * 24 * time.Hour
	HeartbeatTTL    = 30 * time.Second
	RequestTTL      = 15 * time.Minute
	MaxTextBytes    = 32 << 10
	MaxMedia        = 16
	MaxBodyBytes    = 1 << 20
)

var supported = map[string]bool{"x": true, "weibo": true, "xiaohongshu": true, "qzone": true}

type Manager struct {
	Store    *app.Store
	Now      func() time.Time
	mu       sync.Mutex
	pairs    map[string]*pairing
	requests map[string]*Request
}
type pairing struct {
	ID, Platform, CodeHash, TokenHash, PageURL string
	CodeExpiresAt, TokenExpiresAt              time.Time
	LastSeenAt                                 time.Time
	CodeUsed, Revoked                          bool
}
type Pairing struct {
	ID         string    `json:"id"`
	Version    string    `json:"version"`
	Platform   string    `json:"platform"`
	Code       string    `json:"code,omitempty"`
	Token      string    `json:"token,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
	CodeUsed   bool      `json:"code_used"`
	Revoked    bool      `json:"revoked"`
	PageURL    string    `json:"page_url,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
	Connected  bool      `json:"connected"`
}
type Media struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	AltText  string `json:"alt_text"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
}
type Request struct {
	ID               string    `json:"id"`
	RequestID        string    `json:"request_id"`
	Version          string    `json:"version"`
	Type             string    `json:"type"`
	Kind             string    `json:"kind"`
	PairingID        string    `json:"pairing_id"`
	Platform         string    `json:"platform"`
	AccountID        string    `json:"account_id"`
	TopicID          string    `json:"topic_id"`
	VariantID        string    `json:"variant_id"`
	PublicationID    string    `json:"publication_id,omitempty"`
	PageURL          string    `json:"page_url"`
	Text             string    `json:"text,omitempty"`
	Media            []Media   `json:"media,omitempty"`
	AutoPublish      bool      `json:"auto_publish"`
	Status           string    `json:"status"`
	ResultURL        string    `json:"result_url,omitempty"`
	RemoteID         string    `json:"remote_id,omitempty"`
	ExternalPostID   string    `json:"external_post_id,omitempty"`
	Error            string    `json:"error,omitempty"`
	Warning          string    `json:"warning,omitempty"`
	Incomplete       bool      `json:"incomplete,omitempty"`
	IncompleteReason string    `json:"incomplete_reason,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
}

func NewManager(s *app.Store) *Manager {
	m := &Manager{Store: s, Now: time.Now, pairs: make(map[string]*pairing), requests: make(map[string]*Request)}
	if s != nil {
		if persisted, err := s.ListBridgePairings(context.Background()); err == nil {
			for _, saved := range persisted {
				m.pairs[saved.ID] = &pairing{ID: saved.ID, Platform: saved.Platform, TokenHash: saved.TokenHash, TokenExpiresAt: saved.TokenExpiresAt, Revoked: saved.Revoked, CodeUsed: saved.TokenHash != ""}
			}
		}
	}
	return m
}
func randomValue(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func hash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func (m *Manager) CreatePairing(platform string) (Pairing, error) {
	if !supported[platform] {
		return Pairing{}, fmt.Errorf("unsupported bridge platform %q", platform)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.Now()
	code := randomValue(4)
	p := &pairing{ID: randomValue(8), Platform: platform, CodeHash: hash(code), CodeExpiresAt: now.Add(PairingTTL)}
	m.pairs[p.ID] = p
	return Pairing{ID: p.ID, Version: ProtocolVersion, Platform: platform, Code: code, ExpiresAt: p.CodeExpiresAt}, nil
}
func pairingView(p *pairing, now time.Time) Pairing {
	expires := p.CodeExpiresAt
	if p.CodeUsed {
		expires = p.TokenExpiresAt
	}
	return Pairing{ID: p.ID, Version: ProtocolVersion, Platform: p.Platform, ExpiresAt: expires,
		CodeUsed: p.CodeUsed, Revoked: p.Revoked, PageURL: diagnosticPageURL(p.PageURL), LastSeenAt: p.LastSeenAt,
		Connected: p.CodeUsed && !p.Revoked && now.Before(p.TokenExpiresAt) && !p.LastSeenAt.IsZero() && now.Before(p.LastSeenAt.Add(HeartbeatTTL))}
}

func diagnosticPageURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
func (m *Manager) ListPairings() []Pairing {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Pairing, 0, len(m.pairs))
	for _, p := range m.pairs {
		out = append(out, pairingView(p, m.Now()))
	}
	return out
}
func (m *Manager) Revoke(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pairs[id]
	if !ok {
		return app.ErrNotFound
	}
	if m.Store != nil && p.CodeUsed {
		if err := m.Store.RevokeBridgePairing(context.Background(), id); err != nil {
			return err
		}
	}
	p.Revoked = true
	return nil
}
func (m *Manager) PairingPlatform(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pairs[id]
	if !ok || p.Revoked || !p.CodeUsed || !m.Now().Before(p.TokenExpiresAt) {
		return "", errors.New("pairing is not connected, expired, or revoked")
	}
	return p.Platform, nil
}
func (m *Manager) findToken(token string) (*pairing, error) {
	h := hash(token)
	now := m.Now()
	for _, p := range m.pairs {
		if p.TokenHash != "" && subtle.ConstantTimeCompare([]byte(p.TokenHash), []byte(h)) == 1 {
			if p.Revoked || !p.CodeUsed || !now.Before(p.TokenExpiresAt) {
				return nil, errors.New("bridge token expired or revoked")
			}
			return p, nil
		}
	}
	return nil, errors.New("invalid bridge token")
}
func (m *Manager) Hello(code, platform, pageURL, origin string) (Pairing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !supported[platform] || !AllowedPage(platform, pageURL) || (origin != "" && !AllowedPage(platform, origin)) {
		return Pairing{}, errors.New("bridge platform or page origin is not allowed")
	}
	codeHash := hash(code)
	var p *pairing
	for _, candidate := range m.pairs {
		if candidate.Platform == platform && !candidate.CodeUsed && !candidate.Revoked && m.Now().Before(candidate.CodeExpiresAt) && subtle.ConstantTimeCompare([]byte(candidate.CodeHash), []byte(codeHash)) == 1 {
			p = candidate
			break
		}
	}
	if p == nil {
		return Pairing{}, errors.New("pairing code is invalid, expired, or already used")
	}
	token := randomValue(24)
	now := m.Now()
	tokenHash := hash(token)
	tokenExpiresAt := now.Add(TokenTTL)
	if m.Store != nil {
		if err := m.Store.SaveBridgePairing(context.Background(), app.BridgePairing{ID: p.ID, Platform: p.Platform, TokenHash: tokenHash, TokenExpiresAt: tokenExpiresAt, CreatedAt: now}); err != nil {
			return Pairing{}, err
		}
	}
	for _, existing := range m.pairs {
		if existing != p && existing.Platform == platform && existing.CodeUsed {
			existing.Revoked = true
		}
	}
	p.CodeUsed = true
	p.CodeHash = ""
	p.TokenHash = tokenHash
	p.TokenExpiresAt = tokenExpiresAt
	p.PageURL = pageURL
	p.LastSeenAt = now
	view := pairingView(p, now)
	view.Token = token
	return view, nil
}

func (m *Manager) resolvePageURL(pairingID, requested string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pairs[pairingID]
	if !ok || p.Revoked || !p.CodeUsed || !m.Now().Before(p.TokenExpiresAt) {
		return "", errors.New("pairing is not connected, expired, or revoked")
	}
	pageURL := strings.TrimSpace(requested)
	if pageURL == "" {
		pageURL = p.PageURL
	}
	if !AllowedPage(p.Platform, pageURL) {
		return "", errors.New("a connected platform page URL is required")
	}
	return pageURL, nil
}
func (m *Manager) authenticate(token, pairingID, platform, pageURL, origin string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := m.findToken(token)
	if err != nil {
		return err
	}
	if p.ID != pairingID || !p.CodeUsed || p.Platform != platform || !AllowedPage(platform, pageURL) || (origin != "" && !AllowedPage(platform, origin)) {
		return errors.New("bridge authentication or page origin is invalid")
	}
	p.PageURL = pageURL
	p.LastSeenAt = m.Now()
	return nil
}
func (m *Manager) AuthenticateForHTTP(token string, r Request, pageURL, origin string) error {
	return m.authenticate(token, r.PairingID, r.Platform, pageURL, origin)
}
func (m *Manager) AuthenticatePairing(token, pairingID, platform, pageURL, origin string) error {
	return m.authenticate(token, pairingID, platform, pageURL, origin)
}
func (m *Manager) CreateFill(ctx context.Context, pairingID, variantID, pageURL string) (Request, error) {
	return m.CreateFillWithOptions(ctx, pairingID, variantID, pageURL, false)
}

func (m *Manager) CreateFillWithOptions(ctx context.Context, pairingID, variantID, pageURL string, autoPublish bool) (Request, error) {
	pairPlatform, err := m.PairingPlatform(pairingID)
	if err != nil {
		return Request{}, err
	}
	v, err := m.Store.GetVariant(ctx, variantID)
	if err != nil {
		return Request{}, err
	}
	a, err := m.Store.GetAccount(ctx, v.AccountID)
	if err != nil {
		return Request{}, err
	}
	if a.Disabled {
		return Request{}, errors.New("account is deactivated; add an active account before browser fill")
	}
	pageURL, err = m.resolvePageURL(pairingID, pageURL)
	if err != nil || pairPlatform != a.Platform || !supported[a.Platform] || !AllowedPage(a.Platform, pageURL) {
		return Request{}, errors.New("account platform or page URL is not bridge-supported")
	}
	media, err := m.Store.ListMedia(ctx, v.TopicID)
	if err != nil {
		return Request{}, err
	}
	var selected []Media
	for _, x := range media {
		if x.VariantID == "" || x.VariantID == v.ID {
			if len(selected) >= MaxMedia {
				return Request{}, errors.New("too many bridge media items")
			}
			selected = append(selected, Media{ID: x.ID, Filename: x.Filename, MIME: x.MIME, AltText: x.AltText, Size: x.Size, URL: "/media/" + x.ID})
		}
	}
	if len([]byte(v.Text)) > MaxTextBytes {
		return Request{}, errors.New("bridge text exceeds request limit")
	}
	p, err := m.Store.CreatePublication(ctx, app.Publication{VariantID: v.ID, AccountID: a.ID, Status: "needs_confirmation", Source: "bridge"})
	if err != nil {
		return Request{}, err
	}
	return m.saveRequest(Request{Kind: "fill", PairingID: pairingID, Platform: a.Platform, AccountID: a.ID, TopicID: v.TopicID, VariantID: v.ID, PublicationID: p.ID, PageURL: pageURL, Text: v.Text, Media: selected, AutoPublish: autoPublish, Status: "pending"})
}
func (m *Manager) CreateCapture(ctx context.Context, pairingID, topicID, accountID, platform, pageURL string) (Request, error) {
	pairPlatform, err := m.PairingPlatform(pairingID)
	if err != nil {
		return Request{}, err
	}
	a, err := m.Store.GetAccount(ctx, accountID)
	if err != nil {
		return Request{}, err
	}
	if a.Disabled {
		return Request{}, errors.New("account is deactivated; add an active account before capture")
	}
	pageURL, err = m.resolvePageURL(pairingID, pageURL)
	if err != nil || pairPlatform != platform || a.Platform != platform || !supported[platform] || !AllowedPage(platform, pageURL) {
		return Request{}, errors.New("account platform or page URL is not bridge-supported")
	}
	topic, err := m.Store.GetTopic(ctx, topicID)
	if err != nil {
		return Request{}, err
	}
	if topic.ProfileID != a.ProfileID {
		return Request{}, errors.New("capture account must belong to the topic profile")
	}
	return m.saveRequest(Request{Kind: "capture", PairingID: pairingID, Platform: platform, AccountID: accountID, TopicID: topicID, PageURL: pageURL, Status: "pending"})
}
func (m *Manager) saveRequest(r Request) (Request, error) {
	if r.ID == "" {
		r.ID = randomValue(12)
	}
	r.RequestID = r.ID
	if r.Version == "" {
		r.Version = ProtocolVersion
	}
	if r.Type == "" {
		r.Type = r.Kind
	}
	now := m.Now()
	r.CreatedAt = now
	r.ExpiresAt = now.Add(RequestTTL)
	m.mu.Lock()
	m.requests[r.ID] = &r
	m.mu.Unlock()
	return r, nil
}
func (m *Manager) Request(id string) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.requests[id]
	if !ok {
		return Request{}, app.ErrNotFound
	}
	if r.Status == "pending" && !m.Now().Before(r.ExpiresAt) {
		if r.Kind == "fill" {
			r.Status = "needs_confirmation"
		} else {
			r.Status = "expired"
		}
	}
	return *r, nil
}
func (m *Manager) Pending(pairingID string) []Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.pairs[pairingID]; p != nil {
		p.LastSeenAt = m.Now()
	}
	out := []Request{}
	for _, r := range m.requests {
		if r.PairingID == pairingID && r.Status == "pending" {
			if !m.Now().Before(r.ExpiresAt) {
				if r.Kind == "fill" {
					r.Status = "needs_confirmation"
				} else {
					r.Status = "expired"
				}
				continue
			}
			out = append(out, *r)
		}
	}
	return out
}

// Filled records that the userscript successfully put content into the visible
// editor. It does not publish; the publication remains needs_confirmation until
// the user confirms the post and the script returns a result.
func (m *Manager) Filled(id, warning string) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.requests[id]
	if !ok {
		return Request{}, app.ErrNotFound
	}
	if r.Kind != "fill" {
		return Request{}, errors.New("filled is only valid for fill requests")
	}
	if r.Status == "filled" {
		if r.Warning == warning {
			return *r, nil
		}
		return Request{}, errors.New("filled request warning conflicts with existing result")
	}
	if r.Status != "pending" {
		return *r, errors.New("fill request is no longer awaiting the editor")
	}
	r.Status = "filled"
	r.Warning = strings.TrimSpace(warning)
	return *r, nil
}
func (m *Manager) Result(ctx context.Context, id, resultURL, remoteID string) (Request, error) {
	if strings.TrimSpace(resultURL) == "" && strings.TrimSpace(remoteID) == "" {
		return Request{}, errors.New("published result requires URL or remote ID")
	}
	m.mu.Lock()
	r, ok := m.requests[id]
	if !ok {
		m.mu.Unlock()
		return Request{}, app.ErrNotFound
	}
	if r.Kind != "fill" {
		m.mu.Unlock()
		return Request{}, errors.New("result is only valid for fill requests")
	}
	if resultURL != "" && !AllowedPage(r.Platform, resultURL) {
		m.mu.Unlock()
		return Request{}, errors.New("published result URL is not allowed for this platform")
	}
	if r.Status == "published" {
		same := r.ResultURL == resultURL && r.RemoteID == remoteID
		out := *r
		m.mu.Unlock()
		if !same {
			return Request{}, errors.New("request result conflicts with existing result")
		}
		return out, nil
	}
	if r.Status != "pending" && r.Status != "filled" && r.Status != "needs_confirmation" {
		out := *r
		m.mu.Unlock()
		return out, errors.New("request is no longer accepting a result")
	}
	p, err := m.Store.GetPublication(ctx, r.PublicationID)
	if err != nil {
		m.mu.Unlock()
		return Request{}, err
	}
	p.Status, p.RemoteURL, p.RemoteID, p.ErrorCategory, p.ErrorMessage = "published", resultURL, remoteID, "", ""
	p.PublishedAt = m.Now().UTC()
	if err = m.Store.UpdatePublication(ctx, p); err != nil {
		m.mu.Unlock()
		return Request{}, err
	}
	r.Status, r.ResultURL, r.RemoteID = "published", resultURL, remoteID
	out := *r
	m.mu.Unlock()
	return out, nil
}
func (m *Manager) Error(ctx context.Context, id, message string) (Request, error) {
	if strings.TrimSpace(message) == "" {
		return Request{}, errors.New("bridge error message is required")
	}
	m.mu.Lock()
	r, ok := m.requests[id]
	if !ok {
		m.mu.Unlock()
		return Request{}, app.ErrNotFound
	}
	if r.Status == "error" && r.Error == message {
		out := *r
		m.mu.Unlock()
		return out, nil
	}
	if r.Status != "pending" && r.Status != "filled" {
		out := *r
		m.mu.Unlock()
		return out, errors.New("request is no longer accepting an error")
	}
	if r.PublicationID != "" {
		p, err := m.Store.GetPublication(ctx, r.PublicationID)
		if err != nil {
			m.mu.Unlock()
			return Request{}, err
		}
		p.Status, p.ErrorCategory, p.ErrorMessage = "failed", "bridge", message
		if err = m.Store.UpdatePublication(ctx, p); err != nil {
			m.mu.Unlock()
			return Request{}, err
		}
	}
	r.Status, r.Error = "error", message
	out := *r
	m.mu.Unlock()
	return out, nil
}
func (m *Manager) Capture(ctx context.Context, id, textSnapshot, pageURL string, media []app.ExternalMedia, incomplete bool, reason string) (Request, error) {
	m.mu.Lock()
	r, ok := m.requests[id]
	if !ok {
		m.mu.Unlock()
		return Request{}, app.ErrNotFound
	}
	if r.Kind != "capture" {
		m.mu.Unlock()
		return Request{}, errors.New("capture payload requires a capture request")
	}
	if r.Status == "captured" {
		out := *r
		m.mu.Unlock()
		return out, nil
	}
	if r.Status != "pending" {
		out := *r
		m.mu.Unlock()
		return out, errors.New("capture request is no longer pending")
	}
	if pageURL == "" {
		pageURL = r.PageURL
	}
	if !AllowedPage(r.Platform, pageURL) {
		m.mu.Unlock()
		return Request{}, errors.New("capture page URL is not allowed")
	}
	if len(media) > MaxMedia {
		m.mu.Unlock()
		return Request{}, errors.New("too many captured media items")
	}
	if len([]byte(textSnapshot)) > MaxTextBytes {
		m.mu.Unlock()
		return Request{}, errors.New("capture text exceeds request limit")
	}
	post, err := m.Store.CreateExternalPost(ctx, app.ExternalPost{TopicID: r.TopicID, AccountID: r.AccountID, Platform: r.Platform, URL: pageURL, TextSnapshot: textSnapshot, MediaSnapshot: media, CapturedBy: "browser", Incomplete: incomplete, IncompleteReason: reason})
	if err != nil {
		m.mu.Unlock()
		return Request{}, err
	}
	r.Status = "captured"
	r.Incomplete = incomplete
	r.IncompleteReason = reason
	r.ExternalPostID = post.ID
	out := *r
	m.mu.Unlock()
	return out, nil
}

func AllowedPage(platform, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	h := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return allowedHost(platform, h)
}
func allowedHost(platform, h string) bool {
	for _, suffix := range map[string][]string{"x": {"x.com", "twitter.com"}, "weibo": {"weibo.com"}, "xiaohongshu": {"xiaohongshu.com"}, "qzone": {"qzone.qq.com"}}[platform] {
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return true
		}
	}
	return false
}
