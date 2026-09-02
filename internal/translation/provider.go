// Package translation contains small HTTP clients for the supported translation
// providers. Secrets are read from the environment for each request and are
// never included in returned errors.
package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/platform"
)

type Request struct {
	Text           string
	SourceLanguage string
	TargetLanguage string
}

type Provider interface {
	Translate(context.Context, Request) (string, error)
}

type HTTPProvider struct {
	Config app.TranslationConfig
	HTTP   *http.Client
}

func NewProvider(c app.TranslationConfig) (Provider, error) {
	switch c.Provider {
	case "google", "google-cloud", "deepl", "openai", "openai-compatible":
		return &HTTPProvider{Config: c, HTTP: &http.Client{Timeout: 45 * time.Second}}, nil
	default:
		return nil, &platform.Error{Category: "config", Stage: "translate", Err: fmt.Errorf("unsupported translation provider %q", c.Provider)}
	}
}

func (p *HTTPProvider) Translate(ctx context.Context, in Request) (string, error) {
	if strings.TrimSpace(in.Text) == "" || strings.TrimSpace(in.TargetLanguage) == "" {
		return "", &platform.Error{Category: "validation", Stage: "translate", Err: errors.New("text and target language are required")}
	}
	switch p.Config.Provider {
	case "google", "google-cloud":
		return p.google(ctx, in)
	case "deepl":
		return p.deepl(ctx, in)
	case "openai", "openai-compatible":
		return p.openai(ctx, in)
	default:
		return "", &platform.Error{Category: "config", Stage: "translate", Err: errors.New("unsupported translation provider")}
	}
}

func (p *HTTPProvider) credential(env, label string) (string, error) {
	if strings.TrimSpace(env) == "" {
		return "", &platform.Error{Category: "config", Stage: "auth", Err: fmt.Errorf("%s environment variable is required", label)}
	}
	value := strings.TrimSpace(os.Getenv(env))
	if value == "" {
		return "", &platform.Error{Category: "config", Stage: "auth", Err: fmt.Errorf("%s environment variable %q is empty", label, env)}
	}
	return value, nil
}

func (p *HTTPProvider) endpoint(defaultBase, path string) string {
	base := strings.TrimRight(strings.TrimSpace(p.Config.BaseURL), "/")
	if base == "" {
		base = defaultBase
	}
	return base + path
}

func (p *HTTPProvider) do(ctx context.Context, method, endpoint string, body io.Reader, contentType string, headers map[string]string, stage string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, &platform.Error{Category: "network", Stage: stage, Err: err}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		category := "network"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			category = "network"
		}
		return nil, &platform.Error{Category: category, Stage: stage, Err: errors.New("translation request failed")}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &platform.Error{Category: "auth", Stage: stage, Err: fmt.Errorf("translation provider returned %s", resp.Status)}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &platform.Error{Category: "retryable", Stage: stage, Retryable: true, Err: errors.New("translation provider rate limit; retry later")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &platform.Error{Category: "validation", Stage: stage, Err: fmt.Errorf("translation provider returned %s", resp.Status)}
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, &platform.Error{Category: "network", Stage: stage, Err: errors.New("could not read translation response")}
	}
	return out, nil
}

func (p *HTTPProvider) google(ctx context.Context, in Request) (string, error) {
	token, err := p.credential(p.Config.TokenEnv, "Google access token")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Config.ProjectID) == "" {
		return "", &platform.Error{Category: "config", Stage: "translate", Err: errors.New("Google project_id is required")}
	}
	body := map[string]any{"targetLanguageCode": in.TargetLanguage, "contents": []string{in.Text}, "mimeType": "text/plain"}
	if strings.TrimSpace(in.SourceLanguage) != "" {
		body["sourceLanguageCode"] = in.SourceLanguage
	}
	raw, _ := json.Marshal(body)
	path := "/v3/projects/" + url.PathEscape(p.Config.ProjectID) + ":translateText"
	if strings.TrimSpace(p.Config.Location) != "" {
		path = "/v3/projects/" + url.PathEscape(p.Config.ProjectID) + "/locations/" + url.PathEscape(p.Config.Location) + ":translateText"
	}
	response, err := p.do(ctx, http.MethodPost, p.endpoint("https://translation.googleapis.com", path), bytes.NewReader(raw), "application/json", map[string]string{"Authorization": "Bearer " + token}, "translate")
	if err != nil {
		return "", err
	}
	var result struct {
		Translations []struct {
			Text string `json:"translatedText"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(response, &result); err != nil || len(result.Translations) == 0 || result.Translations[0].Text == "" {
		return "", &platform.Error{Category: "protocol", Stage: "translate", Err: errors.New("Google translation response was missing translated text")}
	}
	return result.Translations[0].Text, nil
}

func (p *HTTPProvider) deepl(ctx context.Context, in Request) (string, error) {
	key, err := p.credential(p.Config.KeyEnv, "DeepL API key")
	if err != nil {
		return "", err
	}
	body := map[string]any{"text": []string{in.Text}, "target_lang": strings.ToUpper(in.TargetLanguage)}
	if strings.TrimSpace(in.SourceLanguage) != "" {
		body["source_lang"] = strings.ToUpper(in.SourceLanguage)
	}
	raw, _ := json.Marshal(body)
	response, err := p.do(ctx, http.MethodPost, p.endpoint("https://api-free.deepl.com", "/v2/translate"), bytes.NewReader(raw), "application/json", map[string]string{"Authorization": "DeepL-Auth-Key " + key}, "translate")
	if err != nil {
		return "", err
	}
	var result struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(response, &result); err != nil || len(result.Translations) == 0 || result.Translations[0].Text == "" {
		return "", &platform.Error{Category: "protocol", Stage: "translate", Err: errors.New("DeepL response was missing translated text")}
	}
	return result.Translations[0].Text, nil
}

func (p *HTTPProvider) openai(ctx context.Context, in Request) (string, error) {
	token, err := p.credential(p.Config.TokenEnv, "OpenAI API token")
	if err != nil {
		return "", err
	}
	model := p.Config.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	languageDirection := "Translate the input into " + in.TargetLanguage + "."
	if strings.TrimSpace(in.SourceLanguage) != "" {
		languageDirection = "Translate the input from " + in.SourceLanguage + " into " + in.TargetLanguage + "."
	}
	body := map[string]any{
		"model":        model,
		"instructions": languageDirection + " Return only the translated text and preserve meaning, paragraph breaks, URLs, and platform shortcodes.",
		"input":        in.Text,
		"store":        false,
	}
	raw, _ := json.Marshal(body)
	response, err := p.do(ctx, http.MethodPost, p.endpoint("https://api.openai.com", "/v1/responses"), bytes.NewReader(raw), "application/json", map[string]string{"Authorization": "Bearer " + token}, "translate")
	if err != nil {
		return "", err
	}
	var result struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return "", &platform.Error{Category: "protocol", Stage: "translate", Err: errors.New("OpenAI response was invalid")}
	}
	if result.OutputText != "" {
		return result.OutputText, nil
	}
	for _, item := range result.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" || content.Text != "" {
				if content.Text != "" {
					return content.Text, nil
				}
			}
		}
	}
	return "", &platform.Error{Category: "protocol", Stage: "translate", Err: errors.New("OpenAI response was missing output text")}
}
