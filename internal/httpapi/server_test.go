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
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/api/health?code=do-not-log&state=secret", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	logged := output.String()
	if !strings.Contains(logged, "path=/api/health") || strings.Contains(logged, "do-not-log") || strings.Contains(logged, "state") {
		t.Fatalf("unsafe request log: %s", logged)
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
