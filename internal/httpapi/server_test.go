package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"best-post-manager/internal/app"
	"best-post-manager/internal/connection"
	"best-post-manager/internal/credential"
	"best-post-manager/internal/service"
)

func TestJSONCoreChain(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := httptest.NewServer(&Server{Svc: service.New(st)})
	defer server.Close()
	post := func(path string, value any, out any) {
		raw, _ := json.Marshal(value)
		res, e := http.Post(server.URL+path, "application/json", bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode >= 300 {
			t.Fatalf("POST %s: %s", path, res.Status)
		}
		if e = json.NewDecoder(res.Body).Decode(out); e != nil {
			t.Fatal(e)
		}
	}
	var p app.Profile
	post("/api/profiles", map[string]any{"name": "p", "default_language": "zh-CN"}, &p)
	if p.ID == "" || p.Name != "p" {
		t.Fatalf("profile JSON not decoded: %+v", p)
	}
	var a app.Account
	post("/api/accounts", map[string]any{"profile_id": p.ID, "platform": "mastodon", "name": "a", "base_url": "http://fake", "token_env": "TOKEN", "credential_ref": "client-controlled"}, &a)
	if a.ProfileID != p.ID || a.CredentialRef != "" {
		t.Fatalf("account profile_id lost: %+v", a)
	}
	var second app.Profile
	post("/api/profiles", map[string]any{"name": "second"}, &second)
	var attached app.Account
	post("/api/profile-accounts", map[string]any{"profile_id": second.ID, "account_id": a.ID}, &attached)
	if len(attached.ProfileIDs) != 2 || attached.ID != a.ID {
		t.Fatalf("account was not shared across profiles: %+v", attached)
	}
	var topic app.Topic
	post("/api/topics", map[string]any{"profile_id": p.ID, "title": "topic"}, &topic)
	if topic.ProfileID != p.ID {
		t.Fatalf("topic profile_id lost: %+v", topic)
	}
	var draft app.BaseDraft
	rawDraft, _ := json.Marshal(map[string]any{"text": "hello", "format": "plain"})
	draftRes, e := http.NewRequest(http.MethodPut, server.URL+"/api/topics/"+topic.ID+"/draft", bytes.NewReader(rawDraft))
	if e != nil {
		t.Fatal(e)
	}
	draftHTTP, e := http.DefaultClient.Do(draftRes)
	if e != nil {
		t.Fatal(e)
	}
	defer draftHTTP.Body.Close()
	if draftHTTP.StatusCode >= 300 {
		t.Fatalf("draft update: %s", draftHTTP.Status)
	}
	if e = json.NewDecoder(draftHTTP.Body).Decode(&draft); e != nil {
		t.Fatal(e)
	}
	if draft.TopicID != topic.ID || draft.Text != "hello" {
		t.Fatalf("draft chain failed: %+v", draft)
	}
	var v app.Variant
	post("/api/variants", map[string]any{"topic_id": topic.ID, "account_id": a.ID}, &v)
	if v.TopicID != topic.ID || v.AccountID != a.ID || v.Relation != "linked" {
		t.Fatalf("variant chain failed: %+v", v)
	}
	res, e := http.Get(server.URL + "/api/variants/" + v.ID)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var got app.Variant
	if e = json.NewDecoder(res.Body).Decode(&got); e != nil {
		t.Fatal(e)
	}
	if got.ID != v.ID {
		t.Fatalf("variant response id lost: %+v", got)
	}
}

func TestManualAccountSecretIsStoredLocallyAndCanBeReplaced(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := service.New(st)
	server := httptest.NewServer(&Server{Svc: svc})
	defer server.Close()
	postJSON := func(method, path string, value any) (*http.Response, map[string]any) {
		raw, _ := json.Marshal(value)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		return res, body
	}
	_, profile := postJSON(http.MethodPost, "/api/profiles", map[string]any{"name": "p"})
	res, account := postJSON(http.MethodPost, "/api/accounts", map[string]any{"profile_id": profile["id"], "platform": "bluesky", "name": "b", "remote_id": "me.test", "secret": "first-secret"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%v", res.StatusCode, account)
	}
	if _, ok := account["secret"]; ok {
		t.Fatal("secret returned in account JSON")
	}
	if _, ok := account["credential_ref"]; ok {
		t.Fatal("credential ref returned in account JSON")
	}
	id := account["id"].(string)
	a, err := svc.Store.GetAccount(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if a.CredentialRef == "" {
		t.Fatal("managed credential ref was not assigned")
	}
	if got, err := svc.Credentials.Get(a.CredentialRef); err != nil || got != "first-secret" {
		t.Fatalf("stored secret=%q err=%v", got, err)
	}
	res, _ = postJSON(http.MethodPut, "/api/accounts/"+id+"/connection", map[string]any{"name": "b2", "remote_id": "me.test", "base_url": "", "secret": "second-secret"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", res.StatusCode)
	}
	updated, err := svc.Store.GetAccount(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Credentials.Get(updated.CredentialRef); err != nil || got != "second-secret" {
		t.Fatalf("replacement secret=%q err=%v", got, err)
	}
	if _, err := svc.Credentials.Get(a.CredentialRef); err == nil {
		t.Fatal("old credential was not deleted")
	}
	secondRef := updated.CredentialRef
	res, _ = postJSON(http.MethodPut, "/api/accounts/"+id+"/connection", map[string]any{"name": "renamed only", "remote_id": "me.test", "base_url": "bsky.social/"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("metadata-only update status=%d", res.StatusCode)
	}
	updated, err = svc.Store.GetAccount(ctx, id)
	if err != nil || updated.Name != "renamed only" || updated.BaseURL != "https://bsky.social" || updated.CredentialRef != secondRef {
		t.Fatalf("metadata-only update changed connection unexpectedly: account=%+v err=%v", updated, err)
	}
	if got, getErr := svc.Credentials.Get(secondRef); getErr != nil || got != "second-secret" {
		t.Fatalf("metadata-only update lost secret=%q err=%v", got, getErr)
	}
	res, body := postJSON(http.MethodPut, "/api/accounts/"+id+"/connection", map[string]any{"name": "ambiguous", "remote_id": "me.test", "base_url": "https://bsky.social", "secret": "third-secret", "token_env": "BSKY_PASSWORD"})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(fmt.Sprint(body["error"]), "either") {
		t.Fatalf("ambiguous credential update status=%d body=%v", res.StatusCode, body)
	}
	if after, getErr := svc.Store.GetAccount(ctx, id); getErr != nil || after.CredentialRef != secondRef || after.Name != "renamed only" {
		t.Fatalf("ambiguous update mutated account: account=%+v err=%v", after, getErr)
	}
	if _, err := credential.NewFileStore(st.DataDir); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionStartRequiresLoopbackAndSameOrigin(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := service.New(st)
	server := &Server{Svc: svc, Connections: connection.NewManager(st, svc.Credentials)}

	for _, host := range []string{"127.0.0.1:8787", "[::1]:8787", "localhost:8787"} {
		if !localRequestHost(host) {
			t.Fatalf("loopback host rejected: %s", host)
		}
	}
	if localRequestHost("example.com:8787") {
		t.Fatal("non-loopback host accepted")
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/connections/misskey/start", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUserscriptIsServedSameOriginWithInstallHeaders(t *testing.T) {
	server := &Server{UserScript: []byte("// @updateURL __BPM_ORIGIN__/userscript/bpm-bridge.user.js")}
	req := httptest.NewRequest(http.MethodGet, "http://localhost:9191/userscript/bpm-bridge.user.js", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || !strings.Contains(rec.Header().Get("Content-Disposition"), "bpm-bridge.user.js") {
		t.Fatalf("code=%d headers=%v", rec.Code, rec.Header())
	}
	if !strings.Contains(rec.Body.String(), "@updateURL http://localhost:9191/userscript/bpm-bridge.user.js") || strings.Contains(rec.Body.String(), "__BPM_ORIGIN__") {
		t.Fatalf("userscript update URL was not made same-origin: %s", rec.Body.String())
	}
}

func TestRequestLogDoesNotIncludeQueryValues(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	server := &Server{LogRequests: true}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/api/health?code=do-not-log&state=secret", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	logged := output.String()
	if !strings.Contains(logged, "path=/api/health") || strings.Contains(logged, "do-not-log") || strings.Contains(logged, "state") {
		t.Fatalf("unsafe request log: %s", logged)
	}
}

func TestRequestLogIsDisabledByDefault(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/api/health", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if output.Len() != 0 {
		t.Fatalf("request route was logged by default: %s", output.String())
	}
}

func TestBlueskyDiagnosticEndpointRedactsFailure(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	account, _ := st.CreateAccount(ctx, app.Account{ProfileID: profile.ID, Platform: "bluesky", Name: "bsky", BaseURL: "http://127.0.0.1:1", RemoteID: "me.test", TokenEnv: "BPM_DIAGNOSTIC_SECRET"})
	t.Setenv("BPM_DIAGNOSTIC_SECRET", "super-secret-app-password")
	s := service.New(st)
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/accounts/"+account.ID+"/diagnostic", nil)
	rec := httptest.NewRecorder()
	(&Server{Svc: s}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostic status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != false || result["category"] != "network" || result["message"] != "无法连接到 PDS" {
		t.Fatalf("unexpected diagnostic: %#v", result)
	}
	if strings.Contains(rec.Body.String(), "super-secret") || strings.Contains(rec.Body.String(), "127.0.0.1:1") {
		t.Fatalf("diagnostic leaked secret or endpoint: %s", rec.Body.String())
	}
}

func TestExternalMarkerDoesNotCallRemote(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "t"})
	server := httptest.NewServer(&Server{Svc: service.New(st)})
	defer server.Close()
	raw, _ := json.Marshal(app.ExternalPost{TopicID: topic.ID, Platform: "x", URL: "https://remote.invalid/post"})
	res, e := http.Post(server.URL+"/api/external-posts", "application/json", bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("marker failed: %s", res.Status)
	}
}

func TestMetadataDeletionAndConfirmationEndpoints(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	account, _ := st.CreateAccount(ctx, app.Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: profile.ID, Title: "before"})
	variant, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	publication, _ := st.CreatePublication(ctx, app.Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation", ErrorCategory: "unknown", ErrorMessage: "unknown"})
	external, _ := st.CreateExternalPost(ctx, app.ExternalPost{TopicID: topic.ID, Platform: "x", URL: "https://example.test/external"})
	server := httptest.NewServer(&Server{Svc: service.New(st)})
	defer server.Close()

	requestJSON := func(method, path string, body any) *http.Response {
		raw, _ := json.Marshal(body)
		req, requestErr := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		req.Header.Set("Content-Type", "application/json")
		res, requestErr := http.DefaultClient.Do(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		return res
	}

	res := requestJSON(http.MethodPut, "/api/topics/"+topic.ID, map[string]string{"title": "renamed", "status": "archived"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("topic update status=%d", res.StatusCode)
	}
	var updated app.Topic
	_ = json.NewDecoder(res.Body).Decode(&updated)
	_ = res.Body.Close()
	if updated.Title != "renamed" || updated.Status != "archived" {
		t.Fatalf("unexpected topic: %+v", updated)
	}
	res = requestJSON(http.MethodPut, "/api/topics/"+topic.ID, map[string]string{"title": "renamed", "status": "invalid"})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid topic status=%d", res.StatusCode)
	}

	res = requestJSON(http.MethodPost, "/api/publications/"+publication.ID+"/confirm", map[string]string{"remote_url": "/relative"})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("relative confirmation URL status=%d", res.StatusCode)
	}
	res = requestJSON(http.MethodPost, "/api/publications/"+publication.ID+"/confirm", map[string]string{"remote_url": "https://example.test/post/1", "remote_id": "one"})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("confirmation status=%d", res.StatusCode)
	}
	res = requestJSON(http.MethodPost, "/api/publications/"+publication.ID+"/confirm", map[string]string{"remote_url": "https://example.test/post/1"})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("repeated confirmation status=%d", res.StatusCode)
	}
	res = requestJSON(http.MethodPost, "/api/publications/missing/confirm", map[string]string{"remote_url": "https://example.test/post/1"})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing confirmation status=%d", res.StatusCode)
	}
	toCancel, err := st.CreatePublication(ctx, app.Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation", RemoteURL: "https://example.test/uncertain", ErrorCategory: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	res = requestJSON(http.MethodPost, "/api/publications/"+toCancel.ID+"/cancel", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("cancellation status=%d", res.StatusCode)
	}
	var cancelled app.Publication
	_ = json.NewDecoder(res.Body).Decode(&cancelled)
	_ = res.Body.Close()
	if cancelled.Status != "cancelled" || cancelled.ErrorMessage != "用户确认未发布" || cancelled.RemoteURL != "" {
		t.Fatalf("unexpected cancelled publication: %+v", cancelled)
	}
	res = requestJSON(http.MethodPost, "/api/publications/"+toCancel.ID+"/cancel", nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("repeated cancellation status=%d", res.StatusCode)
	}

	res = requestJSON(http.MethodDelete, "/api/external-posts/"+external.ID, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("external delete status=%d", res.StatusCode)
	}
	res = requestJSON(http.MethodDelete, "/api/external-posts/"+external.ID, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing external delete status=%d", res.StatusCode)
	}
}

func TestCapabilityIsSelectedFromAccountPlatform(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, err := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "bluesky", Name: "bsky", BaseURL: "https://pds.example"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(&Server{Svc: service.New(st)})
	defer server.Close()
	res, err := http.Get(server.URL + "/api/accounts/" + a.ID + "/capability")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var capability map[string]any
	if err = json.NewDecoder(res.Body).Decode(&capability); err != nil {
		t.Fatal(err)
	}
	if capability["platform"] != "bluesky" || capability["format"] != "plain" || capability["text_max_bytes"] != float64(3000) {
		t.Fatalf("unexpected capability: %#v", capability)
	}
}

func TestEmptyListResponsesAreJSONArrays(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := httptest.NewServer(&Server{Svc: service.New(st)})
	defer server.Close()
	paths := []string{"profiles", "accounts", "topics", "variants?topic_id=missing", "media?topic_id=missing", "publications", "external-posts?topic_id=missing", "translation-configs", "bridge/pairings"}
	for _, path := range paths {
		res, requestErr := http.Get(fmt.Sprintf("%s/api/%s", server.URL, path))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		var value []any
		decodeErr := json.NewDecoder(res.Body).Decode(&value)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK || decodeErr != nil || value == nil {
			t.Errorf("%s: status=%d value=%#v decode=%v", path, res.StatusCode, value, decodeErr)
		}
	}
}

func TestPublicationEndpointsAreProfileScoped(t *testing.T) {
	ctx := context.Background()
	store, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createPublication := func(name, status string) (app.Profile, app.Publication) {
		profile, createErr := store.CreateProfile(ctx, app.Profile{Name: name})
		if createErr != nil {
			t.Fatal(createErr)
		}
		account, _ := store.CreateAccount(ctx, app.Account{ProfileID: profile.ID, Platform: "mastodon", Name: name})
		topic, _ := store.CreateTopic(ctx, app.Topic{ProfileID: profile.ID, Title: name})
		variant, _ := store.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: account.ID}, "")
		publication, createErr := store.CreatePublication(ctx, app.Publication{VariantID: variant.ID, AccountID: account.ID, Status: status})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return profile, publication
	}
	first, firstPublication := createPublication("first", app.PublicationNeedsConfirmation)
	second, _ := createPublication("second", app.PublicationFailed)
	server := httptest.NewServer(&Server{Svc: service.New(store)})
	defer server.Close()

	res, err := http.Get(server.URL + "/api/publications?profile_id=" + first.ID)
	if err != nil {
		t.Fatal(err)
	}
	var rows []app.PublicationSummary
	decodeErr := json.NewDecoder(res.Body).Decode(&rows)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || decodeErr != nil || len(rows) != 1 || rows[0].ID != firstPublication.ID {
		t.Fatalf("scoped list status=%d rows=%+v decode=%v", res.StatusCode, rows, decodeErr)
	}

	res, err = http.Get(server.URL + "/api/publications")
	if err != nil {
		t.Fatal(err)
	}
	rows = nil
	decodeErr = json.NewDecoder(res.Body).Decode(&rows)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || decodeErr != nil || len(rows) != 2 {
		t.Fatalf("compatible unscoped list status=%d rows=%+v decode=%v", res.StatusCode, rows, decodeErr)
	}

	res, err = http.Get(server.URL + "/api/publications/summary?profile_id=" + second.ID)
	if err != nil {
		t.Fatal(err)
	}
	var summary app.PublicationStatusSummary
	decodeErr = json.NewDecoder(res.Body).Decode(&summary)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || decodeErr != nil || summary.ActionCount != 1 || summary.FailedCount != 1 {
		t.Fatalf("summary status=%d body=%+v decode=%v", res.StatusCode, summary, decodeErr)
	}

	res, err = http.Get(server.URL + "/api/publications/summary?profile_id=missing")
	if err != nil {
		t.Fatal(err)
	}
	summary = app.PublicationStatusSummary{}
	decodeErr = json.NewDecoder(res.Body).Decode(&summary)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || decodeErr != nil || summary.ActionCount != 0 || summary.FailedCount != 0 {
		t.Fatalf("unknown summary status=%d body=%+v decode=%v", res.StatusCode, summary, decodeErr)
	}

	res, err = http.Get(server.URL + "/api/publications/summary")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing profile summary status=%d", res.StatusCode)
	}
}

func TestRejectsNonLoopbackHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://attacker.example/api/profiles", nil)
	recorder := httptest.NewRecorder()
	(&Server{}).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("non-loopback Host status=%d", recorder.Code)
	}
}
