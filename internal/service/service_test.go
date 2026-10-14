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

type recordingCredentials struct {
	deleted string
}

func (r *recordingCredentials) Get(string) (string, error) { return "", errors.New("not used") }
func (r *recordingCredentials) Set(string, string) error   { return nil }
func (r *recordingCredentials) Delete(ref string) error {
	r.deleted = ref
	return nil
}

func TestPublishApplicationFlow(t *testing.T) {
	t.Setenv("BPM_TEST_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/statuses" {
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

func TestImportRejectsCrossProfileBeforeRemoteRequest(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer ts.Close()
	t.Setenv("MISSKEY_IMPORT_TOKEN", "secret")
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p1, _ := st.CreateProfile(ctx, app.Profile{Name: "one"})
	p2, _ := st.CreateProfile(ctx, app.Profile{Name: "two"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: p1.ID, Platform: "misskey", Name: "mk", BaseURL: ts.URL, TokenEnv: "MISSKEY_IMPORT_TOKEN"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p2.ID, Title: "other"})
	if _, err = New(st).ImportExternal(ctx, topic.ID, a.ID, "note1"); err == nil {
		t.Fatal("cross-profile import must fail")
	}
	if called {
		t.Fatal("cross-profile import must not call a remote API")
	}
}

func TestDeactivatedAccountCannotPublishOrImport(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	account, _ := st.CreateAccount(ctx, app.Account{ProfileID: profile.ID, Platform: "misskey", Name: "old"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: profile.ID, Title: "t"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "hello", Format: "plain"})
	variant, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	if _, err = st.DeactivateAccount(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	svc := New(st)
	if _, err = svc.Publish(ctx, variant.ID); err == nil {
		t.Fatal("deactivated account must not publish")
	}
	if _, err = svc.ImportExternal(ctx, topic.ID, account.ID, "note"); err == nil {
		t.Fatal("deactivated account must not import")
	}
}

func TestDeactivateAccountDeletesCredentialAndClearsReference(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	account, _ := st.CreateAccount(ctx, app.Account{ProfileID: profile.ID, Platform: "mastodon", Name: "old", CredentialRef: "cred-test"})
	credentials := &recordingCredentials{}
	svc := New(st)
	svc.Credentials = credentials
	got, err := svc.DeactivateAccount(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.deleted != "cred-test" || !got.Disabled || got.CredentialRef != "" {
		t.Fatalf("credential cleanup incomplete: deleted=%q account=%+v", credentials.deleted, got)
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

func TestConfirmPublicationValidatesURLAndState(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	account, _ := st.CreateAccount(ctx, app.Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	pending, _ := st.CreatePublication(ctx, app.Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation", ErrorCategory: "unknown", ErrorMessage: "unknown outcome"})
	svc := New(st)
	for _, remoteURL := range []string{"", "/relative", "ftp://example.test/post", "https:///missing-host"} {
		if _, err = svc.ConfirmPublication(ctx, pending.ID, remoteURL, ""); err == nil {
			t.Fatalf("invalid URL accepted: %q", remoteURL)
		}
	}
	confirmed, err := svc.ConfirmPublication(ctx, pending.ID, "https://example.test/post/1", " optional-id ")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "published" || confirmed.RemoteID != "optional-id" || confirmed.PublishedAt.IsZero() {
		t.Fatalf("unexpected confirmation: %+v", confirmed)
	}
	if _, err = svc.ConfirmPublication(ctx, pending.ID, "https://example.test/post/1", ""); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("published record should conflict, got %v", err)
	}
	if _, err = svc.ConfirmPublication(ctx, "missing", "https://example.test/post/1", ""); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing publication should be not found, got %v", err)
	}
	toCancel, err := st.CreatePublication(ctx, app.Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation", RemoteURL: "https://example.test/uncertain", ErrorCategory: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.CancelPublication(ctx, toCancel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" || cancelled.ErrorMessage != "用户确认未发布" || cancelled.RemoteURL != "" {
		t.Fatalf("unexpected cancellation: %+v", cancelled)
	}
	if _, err = svc.CancelPublication(ctx, toCancel.ID); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("cancelled record should conflict, got %v", err)
	}
}

func TestPublishRejectsAnUnresolvedPublication(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	account, _ := st.CreateAccount(ctx, app.Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	_, _ = st.CreatePublication(ctx, app.Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation"})
	if _, err = New(st).Publish(ctx, variant.ID); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("unresolved duplicate publication should conflict, got %v", err)
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
	latePath := filepath.Join(st.MediaDir, "late-image")
	if err = os.WriteFile(latePath, []byte("late"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddMedia(ctx, app.Media{TopicID: topic.ID, Filename: "late.png", MIME: "image/png", Path: latePath}); err != nil {
		t.Fatal(err)
	}
	retried, err := svc.Retry(ctx, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID != failed.ID || retried.Status != "published" || len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("retry did not preserve identity/key: failed=%+v retried=%+v keys=%v", failed, retried, keys)
	}
}

func TestOfficialImportDispatchesWithoutWebScrapeFallback(t *testing.T) {
	t.Setenv("MISSKEY_TOKEN", "secret")
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/api/notes/show" {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "n1", "text": "MFM snapshot"})
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
	owner, _ := st.CreateProfile(ctx, app.Profile{Name: "owner"})
	workspace, _ := st.CreateProfile(ctx, app.Profile{Name: "workspace"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: owner.ID, Platform: "misskey", Name: "m", BaseURL: ts.URL, TokenEnv: "MISSKEY_TOKEN"})
	if _, err = st.AttachAccount(ctx, workspace.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: workspace.ID, Title: "t"})
	post, err := New(st).ImportExternal(ctx, topic.ID, a.ID, ts.URL+"/notes/n1")
	if err != nil || post.CapturedBy != "official_api" || post.TextSnapshot != "MFM snapshot" {
		t.Fatalf("official import failed: %+v %v", post, err)
	}
	if len(paths) != 1 || paths[0] != "/api/notes/show" {
		t.Fatalf("import must only call official single-note endpoint: %v", paths)
	}
}

func TestTranslateCreatesTraceableDetachedVariantWithoutChangingSource(t *testing.T) {
	t.Setenv("TRANSLATE_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "translated"})
	}))
	defer ts.Close()
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	workspace, _ := st.CreateProfile(ctx, app.Profile{Name: "workspace"})
	other, _ := st.CreateProfile(ctx, app.Profile{Name: "other"})
	sourceAccount, _ := st.CreateAccount(ctx, app.Account{ProfileID: workspace.ID, Platform: "mastodon", Name: "source"})
	targetAccount, _ := st.CreateAccount(ctx, app.Account{ProfileID: other.ID, Platform: "mastodon", Name: "target"})
	if _, err = st.AttachAccount(ctx, workspace.ID, targetAccount.ID); err != nil {
		t.Fatal(err)
	}
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: workspace.ID, Title: "translation"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "original", Language: "en"})
	source, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: sourceAccount.ID}, "")
	config, err := st.CreateTranslationConfig(ctx, app.TranslationConfig{Provider: "openai", BaseURL: ts.URL, TokenEnv: "TRANSLATE_TOKEN", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	variant, record, err := New(st).Translate(ctx, TranslationRequest{SourceVariantID: source.ID, TargetAccountID: targetAccount.ID, TargetLanguage: "zh", ConfigID: config.ID})
	if err != nil {
		t.Fatal(err)
	}
	if variant.Text != "translated" || variant.Relation != "detached" || record.TargetVariantID != variant.ID || record.Status != "succeeded" {
		t.Fatalf("unexpected translation result: %+v %+v", variant, record)
	}
	unchanged, _ := st.GetVariant(ctx, source.ID)
	if unchanged.Text != source.Text || unchanged.ID == variant.ID {
		t.Fatalf("source variant was changed: %+v", unchanged)
	}
	if records, err := st.ListTranslations(ctx); err != nil || len(records) != 1 {
		t.Fatalf("translation record was not persisted: %v %v", records, err)
	}
}

func TestTranslateRejectsUnattachedTargetBeforeProviderCall(t *testing.T) {
	t.Setenv("TRANSLATE_TOKEN", "secret")
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer ts.Close()
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	owner, _ := st.CreateProfile(ctx, app.Profile{Name: "owner"})
	workspace, _ := st.CreateProfile(ctx, app.Profile{Name: "workspace"})
	sourceAccount, _ := st.CreateAccount(ctx, app.Account{ProfileID: owner.ID, Platform: "mastodon", Name: "source"})
	targetAccount, _ := st.CreateAccount(ctx, app.Account{ProfileID: owner.ID, Platform: "mastodon", Name: "target"})
	if _, err = st.AttachAccount(ctx, workspace.ID, sourceAccount.ID); err != nil {
		t.Fatal(err)
	}
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: workspace.ID, Title: "translation"})
	source, err := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: sourceAccount.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	config, err := st.CreateTranslationConfig(ctx, app.TranslationConfig{Provider: "openai", BaseURL: ts.URL, TokenEnv: "TRANSLATE_TOKEN", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = New(st).Translate(ctx, TranslationRequest{SourceVariantID: source.ID, TargetAccountID: targetAccount.ID, TargetLanguage: "zh", ConfigID: config.ID}); err == nil {
		t.Fatal("unattached translation target must fail")
	}
	if called {
		t.Fatal("unattached translation target must be rejected before calling the provider")
	}
}

func TestTranslateProviderFailureDoesNotCreateVariantOrRecord(t *testing.T) {
	t.Setenv("TRANSLATE_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{}})
	}))
	defer ts.Close()
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "mastodon", Name: "a"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "translation failure"})
	source, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	config, _ := st.CreateTranslationConfig(ctx, app.TranslationConfig{Provider: "openai", BaseURL: ts.URL, TokenEnv: "TRANSLATE_TOKEN"})
	if _, _, err = New(st).Translate(ctx, TranslationRequest{SourceVariantID: source.ID, TargetAccountID: a.ID, TargetLanguage: "zh", ConfigID: config.ID}); err == nil {
		t.Fatal("partial provider response must fail")
	}
	variants, variantsErr := st.ListVariants(ctx, topic.ID)
	records, recordsErr := st.ListTranslations(ctx)
	if variantsErr != nil || recordsErr != nil || len(variants) != 1 || len(records) != 0 {
		t.Fatalf("failed translation changed local state: variants=%d records=%d errors=%v/%v", len(variants), len(records), variantsErr, recordsErr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
