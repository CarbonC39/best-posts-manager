// Package connection implements the deliberately small, local-only account
// connection flows. Secrets live in credential.Store and pending OAuth state
// is process memory only.
package connection

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/credential"
)

type Manager struct {
	Store       *app.Store
	Credentials credential.Store
	HTTP        *http.Client
	Now         func() time.Time
	mu          sync.Mutex
	pending     map[string]*pending
}
type pending struct {
	platform, key, profileID, name, baseURL, callback, clientID, clientSecret, scopes string
	expires                                                                           time.Time
	used                                                                              bool
}
type StartRequest struct {
	ProfileID string `json:"profile_id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
}
type StartResult struct {
	AuthorizationURL string    `json:"authorization_url"`
	State            string    `json:"state,omitempty"`
	Session          string    `json:"session,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
}

func NewManager(store *app.Store, secrets credential.Store) *Manager {
	return &Manager{Store: store, Credentials: secrets, HTTP: &http.Client{Timeout: 20 * time.Second}, Now: time.Now, pending: make(map[string]*pending)}
}
func randomString() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func localHost(host string) bool {
	h := host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		h = parsed
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
func instance(raw string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || u.Host == "" || u.Scheme == "" {
		return "", errors.New("base_url must be an absolute URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("base_url must not contain credentials or query parameters")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && localHost(u.Hostname())) {
		return "", errors.New("base_url must use HTTPS")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func (m *Manager) callback(base, platform string) (string, error) {
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if !localHost(u.Hostname()) {
		return "", errors.New("callback host must be loopback")
	}
	return "http://" + u.Host + "/api/connections/" + platform + "/callback", nil
}
func (m *Manager) start(ctx context.Context, req StartRequest, platform string, callbackBase string) (StartResult, error) {
	if m.Store == nil || m.Credentials == nil {
		return StartResult{}, errors.New("connection storage is unavailable")
	}
	profiles, err := m.Store.ListProfiles(ctx)
	if err != nil {
		return StartResult{}, err
	}
	found := false
	for _, p := range profiles {
		if p.ID == req.ProfileID {
			found = true
			break
		}
	}
	if !found {
		return StartResult{}, app.ErrNotFound
	}
	base, err := instance(req.BaseURL)
	if err != nil {
		return StartResult{}, err
	}
	callback, err := m.callback(callbackBase, platform)
	if err != nil {
		return StartResult{}, err
	}
	key, err := randomString()
	if err != nil {
		return StartResult{}, err
	}
	now := m.Now().UTC()
	expires := now.Add(10 * time.Minute)
	m.mu.Lock()
	for pendingKey, value := range m.pending {
		if value.used || now.After(value.expires) {
			delete(m.pending, pendingKey)
		}
	}
	m.pending[key] = &pending{platform: platform, key: key, profileID: req.ProfileID, name: strings.TrimSpace(req.Name), baseURL: base, callback: callback, expires: expires}
	m.mu.Unlock()
	return StartResult{State: key, ExpiresAt: expires}, nil
}

func (m *Manager) StartMastodon(ctx context.Context, req StartRequest, callbackBase string) (StartResult, error) {
	log.Printf("connection stage=start platform=mastodon")
	result, err := m.start(ctx, req, "mastodon", callbackBase)
	if err != nil {
		return result, err
	}
	p := m.get(result.State)
	if p == nil {
		return StartResult{}, errors.New("connection state is unavailable")
	}
	scopes := "write" // old instances accept the coarse scope
	// OAuth metadata is advisory: granular scopes are requested only when advertised.
	metadataRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/.well-known/oauth-authorization-server", nil)
	if resp, e := m.HTTP.Do(metadataRequest); e == nil {
		var meta struct {
			Scopes []string `json:"scopes_supported"`
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&meta) == nil {
			for _, s := range meta.Scopes {
				if s == "write:statuses" {
					for _, x := range meta.Scopes {
						if x == "write:media" {
							scopes = "write:statuses write:media"
						}
					}
				}
			}
		}
		resp.Body.Close()
	}
	var out struct {
		ID           string `json:"id"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	registration := url.Values{"client_name": {"Best Post Manager"}, "redirect_uris": {p.callback}, "scopes": {scopes}, "website": {"http://127.0.0.1"}}
	registerRequest, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/v1/apps", strings.NewReader(registration.Encode()))
	registerRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.HTTP.Do(registerRequest)
	if err != nil {
		m.remove(result.State)
		return StartResult{}, errors.New("could not register the local application")
	}
	de := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || de != nil || (out.ClientID == "" && out.ID == "") || out.ClientSecret == "" {
		m.remove(result.State)
		return StartResult{}, errors.New("instance did not return OAuth application credentials")
	}
	if out.ClientID == "" {
		out.ClientID = out.ID
	}
	if !m.update(result.State, func(value *pending) {
		value.scopes, value.clientID, value.clientSecret = scopes, out.ClientID, out.ClientSecret
	}) {
		return StartResult{}, errors.New("connection state is unavailable")
	}
	query := url.Values{"response_type": {"code"}, "client_id": {out.ClientID}, "redirect_uri": {p.callback}, "scope": {scopes}, "state": {result.State}}
	result.AuthorizationURL = p.baseURL + "/oauth/authorize?" + query.Encode()
	log.Printf("connection stage=authorization_ready platform=mastodon")
	return result, nil
}

func (m *Manager) StartMisskey(ctx context.Context, req StartRequest, callbackBase string) (StartResult, error) {
	log.Printf("connection stage=start platform=misskey")
	result, err := m.start(ctx, req, "misskey", callbackBase)
	if err != nil {
		return result, err
	}
	p := m.get(result.State)
	if p == nil {
		return StartResult{}, errors.New("connection state is unavailable")
	}
	query := url.Values{"name": {"Best Post Manager"}, "callback": {p.callback}, "permission": {"write:notes,write:drive"}}
	result.Session = result.State
	result.AuthorizationURL = p.baseURL + "/miauth/" + result.State + "?" + query.Encode()
	log.Printf("connection stage=authorization_ready platform=misskey")
	return result, nil
}

func (m *Manager) get(key string) *pending {
	m.mu.Lock()
	defer m.mu.Unlock()
	if value := m.pending[key]; value != nil {
		copy := *value
		return &copy
	}
	return nil
}
func (m *Manager) update(key string, change func(*pending)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	value := m.pending[key]
	if value == nil {
		return false
	}
	change(value)
	return true
}
func (m *Manager) take(key string) (*pending, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pending[key]
	if p == nil {
		return nil, errors.New("connection state is expired or already used")
	}
	if p.used || m.Now().After(p.expires) {
		delete(m.pending, key)
		return nil, errors.New("connection state is expired or already used")
	}
	p.used = true
	return p, nil
}
func (m *Manager) remove(key string) { m.mu.Lock(); delete(m.pending, key); m.mu.Unlock() }

func (m *Manager) CallbackMastodon(ctx context.Context, state, code string) (app.Account, error) {
	log.Printf("connection stage=callback platform=mastodon")
	if strings.TrimSpace(code) == "" {
		return app.Account{}, errors.New("authorization code is required")
	}
	p, err := m.take(state)
	if err != nil {
		return app.Account{}, err
	}
	defer m.remove(state)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {p.clientID}, "client_secret": {p.clientSecret}, "redirect_uri": {p.callback}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return app.Account{}, errors.New("token exchange failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return app.Account{}, errors.New("token exchange was rejected")
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.AccessToken == "" {
		return app.Account{}, errors.New("token exchange returned no access token")
	}
	return m.saveAccount(ctx, p, out.AccessToken, "", "")
}
func (m *Manager) CallbackMisskey(ctx context.Context, session string) (app.Account, error) {
	log.Printf("connection stage=callback platform=misskey")
	p, err := m.take(session)
	if err != nil {
		return app.Account{}, err
	}
	defer m.remove(session)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/miauth/"+url.PathEscape(session)+"/check", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return app.Account{}, errors.New("MiAuth check failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return app.Account{}, errors.New("MiAuth check was rejected")
	}
	var out struct {
		Token string `json:"token"`
		User  struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.Token == "" {
		return app.Account{}, errors.New("MiAuth returned no token")
	}
	name := out.User.Username
	if name == "" {
		name = out.User.Name
	}
	return m.saveAccount(ctx, p, out.Token, out.User.ID, name)
}
func (m *Manager) saveAccount(ctx context.Context, p *pending, secret, remoteID, remoteName string) (app.Account, error) {
	ref, err := credential.NewRef()
	if err != nil {
		return app.Account{}, errors.New("could not create credential reference")
	}
	if err = m.Credentials.Set(ref, secret); err != nil {
		return app.Account{}, errors.New("could not save credential")
	}
	name := p.name
	if name == "" {
		name = remoteName
	}
	if name == "" {
		name = p.platform + " account"
	}
	a, err := m.Store.CreateAccount(ctx, app.Account{ProfileID: p.profileID, Platform: p.platform, Name: name, RemoteID: remoteID, BaseURL: p.baseURL, CredentialRef: ref})
	if err != nil {
		_ = m.Credentials.Delete(ref)
		return app.Account{}, err
	}
	log.Printf("connection stage=account_saved platform=%s", p.platform)
	return a, nil
}
