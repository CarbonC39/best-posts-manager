package translation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"best-post-manager/internal/app"
)

func TestGoogleGTXSegmentsAndUnicodeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/translate_a/single" || r.URL.Query().Get("client") != "gtx" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[[["你好","hello",null,null,1],["世界","world",null,null,1]],null,"en",null,null,null,1,[],[["en"],null,[1],["en"]]]`))
	}))
	defer server.Close()
	p, _ := NewProvider(app.TranslationConfig{Provider: "google-gtx", BaseURL: server.URL})
	got, err := p.Translate(context.Background(), Request{Text: "hello", TargetLanguage: "zh"})
	if err != nil || got != "你好世界" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err = p.Translate(context.Background(), Request{Text: strings.Repeat("界", 5001), TargetLanguage: "zh"}); err == nil {
		t.Fatal("accepted 5001 Unicode characters")
	}
}
