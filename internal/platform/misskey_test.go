package platform

import (
	"best-post-manager/internal/app"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMisskeyMFMEmojiMediaAndImport(t *testing.T) {
	t.Setenv("MISSKEY_TOKEN", "secret")
	var note map[string]any
	emojiCalls := 0
	var fileIDs []any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/emojis":
			emojiCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"emojis": []map[string]any{{"name": "party", "url": ""}}})
		case "/api/drive/files/create":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "file1"})
		case "/api/notes/create":
			_ = json.NewDecoder(r.Body).Decode(&note)
			_ = json.NewEncoder(w).Encode(map[string]any{"createdNote": map[string]string{"id": "note1"}})
		case "/api/notes/show":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "note1", "text": "old MFM"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	c := NewMisskeyClient()
	a := app.Account{BaseURL: ts.URL, TokenEnv: "MISSKEY_TOKEN", ID: "a"}
	e, e2 := c.CustomEmoji(context.Background(), a)
	if e2 != nil || len(e) != 1 || !strings.Contains(e[0].URL, "/emoji/party.webp") {
		t.Fatalf("emoji: %+v %v", e, e2)
	}
	if _, e2 = c.CustomEmoji(context.Background(), a); e2 != nil || emojiCalls != 1 {
		t.Fatalf("emoji cache calls=%d err=%v", emojiCalls, e2)
	}
	p := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(p, []byte("png"), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := c.Publish(context.Background(), a, app.Variant{Text: "$[tada MFM]", Format: "mfm", Visibility: "home", SpoilerText: "cw", LocalOnly: true}, []app.Media{{Path: p, MIME: "image/png", Size: 3}})
	if err != nil || res.ID != "note1" {
		t.Fatalf("publish: %+v %v", res, err)
	}
	if note["visibility"] != "home" || note["cw"] != "cw" || note["localOnly"] != true {
		t.Fatalf("note options: %#v", note)
	}
	fileIDs, _ = note["fileIds"].([]any)
	if len(fileIDs) != 1 {
		t.Fatalf("file ids: %#v", note)
	}
	post, err := c.Import(context.Background(), a, "note1")
	if err != nil || post.TextSnapshot != "old MFM" || post.CapturedBy != "official_api" {
		t.Fatalf("import: %+v %v", post, err)
	}
}

func TestMisskeyErrorsAreClassified(t *testing.T) {
	t.Setenv("MISSKEY_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			w.WriteHeader(http.StatusUnauthorized)
		case "/api/rate":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()
	c := NewMisskeyClient()
	a := app.Account{BaseURL: ts.URL, TokenEnv: "MISSKEY_TOKEN"}
	for _, tc := range []struct {
		endpoint, category string
		retry              bool
	}{{"auth", "auth", false}, {"rate", "retryable", true}, {"bad", "validation", false}} {
		resp, err := c.request(context.Background(), a, tc.endpoint, map[string]string{}, "test")
		if resp != nil {
			resp.Body.Close()
		}
		pe, ok := err.(*Error)
		if !ok || pe.Category != tc.category || pe.Retryable != tc.retry {
			t.Fatalf("%s: %#v", tc.endpoint, err)
		}
	}
}

func TestMisskeyRejectsCrossInstanceImportAndClassifiesMediaErrors(t *testing.T) {
	t.Setenv("MISSKEY_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()
	c := NewMisskeyClient()
	a := app.Account{BaseURL: ts.URL, TokenEnv: "MISSKEY_TOKEN"}
	if _, err := c.Import(context.Background(), a, "https://other.example/notes/n1"); err == nil {
		t.Fatal("cross-instance note URL must be rejected before fetching")
	}
	p := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(p, []byte("png"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := c.Publish(context.Background(), a, app.Variant{Text: "hello", Format: "mfm"}, []app.Media{{Path: p, MIME: "image/png"}})
	pe, ok := err.(*Error)
	if !ok || pe.Category != "validation" || pe.Retryable {
		t.Fatalf("media 400 classification: %#v", err)
	}
}
