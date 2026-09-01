package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"best-post-manager/internal/app"
)

func TestMastodonEmojiCacheAndPublishContract(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	var emojiCalls, mediaCalls, statusCalls int
	var statusBody map[string]any
	var idempotency string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/custom_emojis":
			emojiCalls++
			_ = json.NewEncoder(w).Encode([]Emoji{{Shortcode: "wave", URL: "https://cdn.test/wave.png"}})
		case "/api/v2/media":
			mediaCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "media-1"})
		case "/api/statuses":
			statusCalls++
			idempotency = r.Header.Get("Idempotency-Key")
			_ = json.NewDecoder(r.Body).Decode(&statusBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "post-1", "url": "https://mastodon.test/@me/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	c := NewClient()
	account := app.Account{BaseURL: ts.URL, Platform: "mastodon", TokenEnv: "BPM_TEST_TOKEN"}
	if _, err := c.CustomEmoji(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CustomEmoji(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	if emojiCalls != 1 {
		t.Fatalf("expected one cached emoji request, got %d", emojiCalls)
	}
	path := filepath.Join(t.TempDir(), "photo.txt")
	if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := c.Publish(context.Background(), account, app.Variant{Text: "hello :wave:", Visibility: "unlisted"}, []app.Media{{Path: path, MIME: "image/png", AltText: "photo"}}, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "post-1" || mediaCalls != 1 || statusCalls != 1 {
		t.Fatalf("unexpected result calls=%d/%d/%d result=%+v", emojiCalls, mediaCalls, statusCalls, r)
	}
	if statusBody["visibility"] != "unlisted" || !strings.Contains(statusBody["status"].(string), ":wave:") {
		t.Fatalf("native status payload not preserved: %#v", statusBody)
	}
	if idempotency != "key-1" {
		t.Fatalf("idempotency header missing: %q", idempotency)
	}
}

func TestMastodonValidationClassifiesInput(t *testing.T) {
	c := NewClient()
	err := c.Validate(app.Account{BaseURL: "http://example.test"}, app.Variant{Text: strings.Repeat("x", 501)}, nil)
	if e, ok := err.(*Error); !ok || e.Category != "validation" {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRequestErrorCategoriesAndTokenSafety(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			w.WriteHeader(http.StatusUnauthorized)
		case "/rate":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/bad":
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()
	c := NewClient()
	a := app.Account{BaseURL: ts.URL, TokenEnv: "BPM_TEST_TOKEN"}
	for _, tc := range []struct{ path, category string }{{"/auth", "auth"}, {"/rate", "retryable"}, {"/bad", "validation"}} {
		_, err := c.request(context.Background(), a, http.MethodGet, tc.path, nil, "", "test", "")
		e, ok := err.(*Error)
		if !ok || e.Category != tc.category {
			t.Fatalf("%s: %#v", tc.path, err)
		}
	}
	called := false
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, nil })}
	a.TokenEnv = "MISSING_TOKEN"
	_, err := c.request(context.Background(), a, http.MethodGet, "/never", nil, "", "test", "")
	if called || err == nil {
		t.Fatalf("empty token must fail before request: called=%v err=%v", called, err)
	}
}

func TestSubmitProtocolErrorHasUnknownOutcome(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer ts.Close()
	c := NewClient()
	_, err := c.Publish(context.Background(), app.Account{BaseURL: ts.URL, TokenEnv: "BPM_TEST_TOKEN"}, app.Variant{Text: "hello"}, nil, "key")
	platformErr, ok := err.(*Error)
	if !ok || !platformErr.OutcomeUnknown || platformErr.Stage != "submit" {
		t.Fatalf("submit protocol error must require confirmation: %#v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
