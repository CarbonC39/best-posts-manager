package platform

import (
	"best-post-manager/internal/app"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

func TestBlueskyFacetBlobRecordAndImport(t *testing.T) {
	t.Setenv("BSKY_APP_PASSWORD", "app-password")
	var record map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			if r.Method != http.MethodPost {
				t.Errorf("createSession must use POST, got %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"did": "did:plc:test", "handle": "me.test", "accessJwt": "jwt"})
		case "/xrpc/com.atproto.repo.uploadBlob":
			if r.Method != http.MethodPost {
				t.Errorf("uploadBlob must use POST, got %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"blob": map[string]any{"$type": "blob", "ref": map[string]string{"$link": "cid1"}, "mimeType": "image/png", "size": 3}})
		case "/xrpc/com.atproto.identity.resolveHandle":
			if r.Method != http.MethodGet {
				t.Errorf("resolveHandle must use GET, got %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"did": "did:plc:alice"})
		case "/xrpc/com.atproto.repo.createRecord":
			if r.Method != http.MethodPost {
				t.Errorf("createRecord must use POST, got %s", r.Method)
			}
			_ = json.NewDecoder(r.Body).Decode(&record)
			_ = json.NewEncoder(w).Encode(map[string]string{"uri": "at://did:plc:test/app.bsky.feed.post/r1", "cid": "cid1"})
		case "/xrpc/com.atproto.repo.getRecord":
			if r.Method != http.MethodGet {
				t.Errorf("getRecord must use GET, got %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"uri": "at://did:plc:test/app.bsky.feed.post/r1", "cid": "cid1", "value": map[string]string{"text": "imported"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	c := NewBlueskyClient()
	a := app.Account{BaseURL: ts.URL, TokenEnv: "BSKY_APP_PASSWORD", RemoteID: "me.test", ID: "a"}
	p := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(p, []byte("png"), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := c.Publish(context.Background(), a, app.Variant{Text: "你好 @alice.test https://x.test", Format: "plain"}, []app.Media{{Path: p, MIME: "image/png", Size: 3}})
	if err != nil || res.ID != "at://did:plc:test/app.bsky.feed.post/r1" || res.URL != "https://bsky.app/profile/did:plc:test/post/r1" || res.CID != "cid1" {
		t.Fatalf("publish: %+v %v", res, err)
	}
	r := record["record"].(map[string]any)
	facets := r["facets"].([]any)
	if len(facets) < 2 {
		t.Fatalf("facets: %#v", facets)
	}
	prefix := len([]byte("你好 @alice.test "))
	found := false
	for _, raw := range facets {
		idx := raw.(map[string]any)["index"].(map[string]any)
		if int(idx["byteStart"].(float64)) == prefix {
			if int(idx["byteEnd"].(float64)) != prefix+len([]byte("https://x.test")) {
				t.Fatalf("facet bytes: %#v", idx)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("unicode facet bytes absent: %#v", facets)
	}
	if !utf8.ValidString(r["text"].(string)) {
		t.Fatal("invalid text")
	}
	post, err := c.Import(context.Background(), a, "https://bsky.app/profile/me.test/post/r1")
	if err != nil || post.TextSnapshot != "imported" || post.URL == "" {
		t.Fatalf("import: %+v %v", post, err)
	}
}

func TestBlueskyUsesRecordByteLimitBeforeNetwork(t *testing.T) {
	c := NewBlueskyClient()
	a := app.Account{BaseURL: "http://invalid.test", TokenEnv: "MISSING"}
	_, err := c.Publish(context.Background(), a, app.Variant{Text: strings.Repeat("x", 3001), Format: "plain"}, nil)
	if pe, ok := err.(*Error); !ok || pe.Category != "validation" || pe.Stage != "validate" {
		t.Fatalf("expected byte-limit validation, got %v", err)
	}
}

func TestBlueskyUsesGraphemeLimitBeforeNetwork(t *testing.T) {
	c := NewBlueskyClient()
	a := app.Account{BaseURL: "http://invalid.test", TokenEnv: "MISSING"}
	_, err := c.Publish(context.Background(), a, app.Variant{Text: strings.Repeat("x", 301), Format: "plain"}, nil)
	if pe, ok := err.(*Error); !ok || pe.Category != "validation" || !strings.Contains(pe.Error(), "grapheme") {
		t.Fatalf("expected grapheme-limit validation, got %v", err)
	}
	// A combined glyph is one grapheme even when it contains several code points.
	if got := uniseg.GraphemeClusterCount(strings.Repeat("🏳️‍🌈", 300)); got != 300 {
		t.Fatalf("combined grapheme fixture count=%d", got)
	}
}

func TestBlueskyImportRefRequiresPostCollection(t *testing.T) {
	if _, _, err := parseRef("at://did:plc:test/app.bsky.feed.like/r1"); err == nil {
		t.Fatal("non-post AT URI must be rejected")
	}
	repo, rkey, err := parseRef("at://did:plc:test/app.bsky.feed.post/r1")
	if err != nil || repo != "did:plc:test" || rkey != "r1" {
		t.Fatalf("valid post ref: repo=%q rkey=%q err=%v", repo, rkey, err)
	}
}

func TestBlueskyResponseClassificationAndFacetOverlap(t *testing.T) {
	for _, tc := range []struct {
		status   int
		category string
		retry    bool
		unknown  bool
	}{
		{http.StatusUnauthorized, "auth", false, false},
		{http.StatusTooManyRequests, "retryable", true, false},
		{http.StatusServiceUnavailable, "retryable", false, true},
	} {
		resp := &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status), Body: io.NopCloser(strings.NewReader(""))}
		_, err := decodeXRPCResponse(resp, "submit")
		pe, ok := err.(*Error)
		if !ok || pe.Category != tc.category || pe.Retryable != tc.retry || pe.OutcomeUnknown != tc.unknown {
			t.Fatalf("status %d: %#v", tc.status, err)
		}
	}
	text := "https://example.test/@alice.test"
	got := facets(context.Background(), NewBlueskyClient(), app.Account{}, text, bskySession{})
	if len(got) != 1 || got[0].Features[0]["$type"] != "app.bsky.richtext.facet#link" {
		t.Fatalf("overlapping link/mention facets: %#v", got)
	}
}
