package translation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/platform"
)

func TestProviderContractsAndSecretFreeErrors(t *testing.T) {
	secret := "do-not-leak-translation-secret"
	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		if r.URL.Path == "/v3/projects/project:translateText" {
			if r.Header.Get("Authorization") != "Bearer "+secret {
				t.Error("Google bearer was not sent")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"translations": []map[string]string{{"translatedText": "你好"}}})
			return
		}
		if r.URL.Path == "/v2/translate" {
			if r.Header.Get("Authorization") != "DeepL-Auth-Key "+secret {
				t.Error("DeepL key was not sent")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("Content-Type") != "application/json" || body["target_lang"] != "ZH" {
				t.Errorf("unexpected DeepL request: header=%q body=%v", r.Header.Get("Content-Type"), body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"translations": []map[string]string{{"text": "你好"}}})
			return
		}
		if r.URL.Path == "/v1/responses" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["store"] != false {
				t.Error("OpenAI translation must set store:false")
			}
			if instructions, _ := body["instructions"].(string); !strings.Contains(instructions, "from en into zh") {
				t.Errorf("OpenAI request lacks language direction: %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{map[string]any{"content": []any{map[string]string{"type": "output_text", "text": "你好"}}}}})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()
	t.Setenv("TRANSLATION_SECRET", secret)
	ctx := context.Background()
	providers := []app.TranslationConfig{
		{Provider: "google", BaseURL: ts.URL, ProjectID: "project", TokenEnv: "TRANSLATION_SECRET"},
		{Provider: "deepl", BaseURL: ts.URL, KeyEnv: "TRANSLATION_SECRET"},
		{Provider: "openai", BaseURL: ts.URL, TokenEnv: "TRANSLATION_SECRET", Model: "fake"},
	}
	for _, config := range providers {
		provider, err := NewProvider(config)
		if err != nil {
			t.Fatal(err)
		}
		got, err := provider.Translate(ctx, Request{Text: "hello", SourceLanguage: "en", TargetLanguage: "zh"})
		if err != nil || got != "你好" {
			t.Fatalf("%s translation: %q %v", config.Provider, got, err)
		}
	}
	if len(seen) != 3 || strings.Join(seen, ",") != "/v3/projects/project:translateText,/v2/translate,/v1/responses" {
		t.Fatalf("unexpected provider paths: %v", seen)
	}

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(secret))
	}))
	defer badServer.Close()
	provider, _ := NewProvider(app.TranslationConfig{Provider: "openai", BaseURL: badServer.URL, TokenEnv: "TRANSLATION_SECRET"})
	_, err := provider.Translate(ctx, Request{Text: "hello", TargetLanguage: "zh"})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("provider error leaked secret: %v", err)
	}
	var pe *platform.Error
	if !errorsAs(err, &pe) || pe.Category != "auth" {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestProviderRejectsPartialResponseWithoutCreatingText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{map[string]any{"content": []any{}}}})
	}))
	defer ts.Close()
	t.Setenv("TRANSLATION_SECRET", "never-return-this-secret")
	provider, err := NewProvider(app.TranslationConfig{Provider: "openai", BaseURL: ts.URL, TokenEnv: "TRANSLATION_SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	text, err := provider.Translate(context.Background(), Request{Text: "Unicode 🌍", TargetLanguage: "zh"})
	if err == nil || text != "" || strings.Contains(err.Error(), "never-return-this-secret") {
		t.Fatalf("partial response must fail safely: text=%q err=%v", text, err)
	}
	var pe *platform.Error
	if !errors.As(err, &pe) || pe.Category != "protocol" {
		t.Fatalf("expected protocol error, got %v", err)
	}
}

func TestProviderTimeoutIsActionableAndDoesNotLeakResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer ts.Close()
	t.Setenv("TRANSLATION_SECRET", "secret-timeout")
	p, err := NewProvider(app.TranslationConfig{Provider: "openai", BaseURL: ts.URL, TokenEnv: "TRANSLATION_SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	httpProvider := p.(*HTTPProvider)
	httpProvider.HTTP.Timeout = 10 * time.Millisecond
	_, err = p.Translate(context.Background(), Request{Text: "hello", TargetLanguage: "zh"})
	if err == nil || !strings.Contains(err.Error(), "translation request failed") || strings.Contains(err.Error(), "secret-timeout") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
}

func errorsAs(err error, target any) bool {
	// Keep this helper local to make the assertion explicit without exposing
	// provider implementation details in the public API.
	return errors.As(err, target)
}
