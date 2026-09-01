package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"best-post-manager/internal/app"
)

func TestPublishApplicationFlow(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/statuses" {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "1", "url": "http://fake/1"})
			return
		}
		if r.URL.Path == "/api/v2/media" {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "m"})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "mastodon", BaseURL: ts.URL, Name: "a", TokenEnv: "BPM_TEST_TOKEN"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "t"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "hello", Format: "plain"})
	v, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	got, err := New(st).Publish(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "published" || got.RemoteID != "1" {
		t.Fatalf("unexpected publication: %+v", got)
	}
}

func TestPublishNetworkFailureNeedsConfirmation(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "mastodon", BaseURL: "http://fake", Name: "a", TokenEnv: "BPM_TEST_TOKEN"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "t"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "hello", Format: "plain"})
	v, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	svc := New(st)
	svc.Mastodon.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	got, err := svc.Publish(ctx, v.ID)
	if err == nil {
		t.Fatal("expected network error")
	}
	if got.Status != "needs_confirmation" {
		t.Fatalf("unexpected status: %s", got.Status)
	}
}

func TestMediaNetworkFailureIsRetryableNotUnknown(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "mastodon", BaseURL: "http://fake", Name: "a", TokenEnv: "BPM_TEST_TOKEN"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "t"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "hello", Format: "plain"})
	v, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	path := filepath.Join(st.MediaDir, "image")
	if err = os.WriteFile(path, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddMedia(ctx, app.Media{TopicID: topic.ID, VariantID: v.ID, Filename: "image.png", MIME: "image/png", Path: path, Size: 5}); err != nil {
		t.Fatal(err)
	}
	svc := New(st)
	svc.Mastodon.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	got, err := svc.Publish(ctx, v.ID)
	if err == nil {
		t.Fatal("expected media network error")
	}
	if got.Status != "failed" || got.ErrorCategory != "retryable" {
		t.Fatalf("media failure must be retryable but not unknown: %+v", got)
	}
}

func TestRetryReusesPublicationAndIdempotencyKey(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	var calls int
	var keys []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "1", "url": "http://fake/1"})
	}))
	defer ts.Close()
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "mastodon", BaseURL: ts.URL, Name: "a", TokenEnv: "BPM_TEST_TOKEN"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "t"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "hello", Format: "plain"})
	v, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	svc := New(st)
	failed, err := svc.Publish(ctx, v.ID)
	if err == nil || failed.Status != "failed" || failed.ErrorCategory != "retryable" {
		t.Fatalf("expected retryable failure: %+v %v", failed, err)
	}
	retried, err := svc.Retry(ctx, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID != failed.ID || retried.Status != "published" || len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("retry did not preserve identity/key: failed=%+v retried=%+v keys=%v", failed, retried, keys)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
