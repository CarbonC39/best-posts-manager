package connection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/credential"
)

func TestMastodonOAuthStoresOnlyReferenceAndRejectsReplay(t *testing.T) {
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"scopes_supported": []string{"write:statuses", "write:media"}})
		case "/api/v1/apps":
			_ = json.NewEncoder(w).Encode(map[string]string{"client_id": "client", "client_secret": "private"})
		case "/oauth/token":
			tokenCalls++
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "top-secret"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	store, err := app.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p, err := store.CreateProfile(context.Background(), app.Profile{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := credential.NewFileStore(filepath.Join(dir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(store, secrets)
	start, err := m.StartMastodon(context.Background(), StartRequest{ProfileID: p.ID, Name: "main", BaseURL: srv.URL}, "http://127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(start.AuthorizationURL)
	if err != nil || u.Query().Get("state") == "" || u.Query().Get("scope") != "write:statuses write:media" {
		t.Fatalf("bad auth URL: %s", start.AuthorizationURL)
	}
	a, err := m.CallbackMastodon(context.Background(), start.State, "one-time-code")
	if err != nil {
		t.Fatal(err)
	}
	if a.CredentialRef == "" || a.TokenEnv != "" {
		t.Fatalf("credential fields: %+v", a)
	}
	if got, _ := secrets.Get(a.CredentialRef); got != "top-secret" {
		t.Fatalf("secret=%q", got)
	}
	if _, err = m.CallbackMastodon(context.Background(), start.State, "again"); err == nil || tokenCalls != 1 {
		t.Fatalf("replay err=%v calls=%d", err, tokenCalls)
	}
	if m.get(start.State) != nil {
		t.Fatal("completed OAuth state was not removed")
	}
}

func TestMisskeyMiAuthCreatesAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/miauth/") && strings.HasSuffix(r.URL.Path, "/check") {
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "mk-secret", "user": map[string]string{"id": "u1", "username": "alice"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	store, err := app.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p, _ := store.CreateProfile(context.Background(), app.Profile{Name: "p"})
	secrets, _ := credential.NewFileStore(filepath.Join(store.DataDir, "secrets"))
	m := NewManager(store, secrets)
	start, err := m.StartMisskey(context.Background(), StartRequest{ProfileID: p.ID, BaseURL: srv.URL}, "http://localhost:8787")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(start.AuthorizationURL, "/miauth/") {
		t.Fatal(start.AuthorizationURL)
	}
	a, err := m.CallbackMisskey(context.Background(), start.Session)
	if err != nil {
		t.Fatal(err)
	}
	if a.RemoteID != "u1" || a.CredentialRef == "" {
		t.Fatalf("account=%+v", a)
	}
}

func TestExpiredMiAuthStateIsRemoved(t *testing.T) {
	ctx := context.Background()
	store, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, err := store.CreateProfile(ctx, app.Profile{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := credential.NewFileStore(filepath.Join(store.DataDir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	manager := NewManager(store, secrets)
	manager.Now = func() time.Time { return now }
	start, err := manager.StartMisskey(ctx, StartRequest{ProfileID: profile.ID, BaseURL: "https://misskey.example"}, "127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	if _, err = manager.CallbackMisskey(ctx, start.Session); err == nil {
		t.Fatal("expired state was accepted")
	}
	if manager.get(start.Session) != nil {
		t.Fatal("expired state was not removed")
	}
}

func TestLocalHostAcceptsIPv4AndIPv6Loopback(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "127.0.0.1:8787", "::1", "[::1]:8787"} {
		if !localHost(host) {
			t.Errorf("loopback host rejected: %s", host)
		}
	}
	if localHost("example.com:8787") {
		t.Fatal("non-loopback host accepted")
	}
}
