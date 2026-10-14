package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"best-post-manager/internal/app"
	"best-post-manager/internal/bridge"
	"best-post-manager/internal/service"
)

func mustBridgeManager(t *testing.T, store *app.Store) *bridge.Manager {
	t.Helper()
	manager, err := bridge.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestBridgeLazyInitializationFailureReturnsInternalServerError(t *testing.T) {
	st, err := app.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.DB.Exec(`INSERT INTO bridge_pairings(id,platform,token_hash,token_expires_at,revoked,created_at) VALUES('broken','x','invalid','2026-01-02T00:00:00Z',0,'2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	server := &Server{Svc: service.New(st)}
	for _, path := range []string{"/api/bridge/pairings", "/bridge/v1/requests"} {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestBridgeHTTPPairFillResultAndCapture(t *testing.T) {
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, _ := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "x", Name: "x"})
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "t"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "hello", Format: "plain"})
	v, _ := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	s := &Server{Svc: service.New(st), Bridge: mustBridgeManager(t, st)}
	ts := httptest.NewServer(s)
	defer ts.Close()
	post := func(path string, body any, out any) *http.Response {
		raw, _ := json.Marshal(body)
		res, e := http.Post(ts.URL+path, "application/json", bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		if out != nil {
			defer res.Body.Close()
			if e = json.NewDecoder(res.Body).Decode(out); e != nil {
				t.Fatal(e)
			}
		}
		return res
	}
	var pair bridge.Pairing
	res := post("/api/bridge/pairings", map[string]string{"platform": "x"}, &pair)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("pairing: %s", res.Status)
	}
	var hello bridge.Pairing
	res = postBridge(t, ts.URL+"/bridge/v1/hello", "", "", "x", map[string]string{"code": pair.Code, "platform": "x", "page_url": "https://x.com/compose"}, &hello)
	if res.StatusCode != 200 {
		t.Fatalf("hello: %s", res.Status)
	}
	if pair.Token != "" || hello.Token == "" {
		t.Fatalf("hello must exchange code for token: pair=%+v hello=%+v", pair, hello)
	}
	var request bridge.Request
	res = post("/api/bridge/requests/fill", map[string]string{"pairing_id": pair.ID, "variant_id": v.ID, "page_url": "https://x.com/compose"}, &request)
	if res.StatusCode != 201 {
		t.Fatalf("fill: %s", res.Status)
	}
	var pending []bridge.Request
	res = getBridge(t, ts.URL+"/bridge/v1/requests", hello.Token, pair.ID, "x", "https://x.com/compose", &pending)
	if res.StatusCode != 200 || len(pending) != 1 {
		t.Fatalf("pending: %s %#v", res.Status, pending)
	}
	res = postBridge(t, ts.URL+"/bridge/v1/requests/"+request.ID+"/filled", hello.Token, request.ID, "x", map[string]string{"warning": "图片需人工检查"}, &request)
	if res.StatusCode != 200 || request.Status != "filled" || request.Warning != "图片需人工检查" {
		t.Fatalf("filled: %s %#v", res.Status, request)
	}
	var done bridge.Request
	res = postBridge(t, ts.URL+"/bridge/v1/requests/"+request.ID+"/result", hello.Token, request.ID, "x", map[string]string{"url": "https://x.com/user/status/1", "remote_id": "1"}, &done)
	if res.StatusCode != 200 || done.Status != "published" {
		t.Fatalf("result: %s %+v", res.Status, done)
	}
	pub, _ := st.GetPublication(ctx, request.PublicationID)
	if pub.Status != "published" || pub.RemoteID != "1" {
		t.Fatalf("publication: %+v", pub)
	}
	var capture bridge.Request
	res = post("/api/bridge/requests/capture", map[string]string{"pairing_id": pair.ID, "topic_id": topic.ID, "account_id": a.ID, "platform": "x", "page_url": "https://x.com/post/1"}, &capture)
	if res.StatusCode != 201 {
		t.Fatalf("capture request: %s", res.Status)
	}
	res = postBridge(t, ts.URL+"/bridge/v1/requests/"+capture.ID+"/capture", hello.Token, capture.ID, "x", map[string]any{"text": "visible", "url": "https://x.com/post/1", "incomplete": false}, &capture)
	if res.StatusCode != 200 || capture.Status != "captured" || capture.ExternalPostID == "" {
		t.Fatalf("capture: %s %+v", res.Status, capture)
	}
	posts, err := st.ListExternalPosts(ctx, topic.ID)
	if err != nil || len(posts) != 1 || posts[0].CapturedBy != "browser" || posts[0].ID != capture.ExternalPostID {
		t.Fatalf("snapshot: %+v %v", posts, err)
	}
}

func postBridge(t *testing.T, endpoint, token, pairingID, platform string, body any, out any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BPM-Pairing-ID", pairingID)
	req.Header.Set("X-BPM-Platform", platform)
	req.Header.Set("X-BPM-Page-URL", "https://x.com/compose")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err = json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return res
}
func getBridge(t *testing.T, endpoint, token, pairingID, platform, page string, out any) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-BPM-Pairing-ID", pairingID)
	req.Header.Set("X-BPM-Platform", platform)
	req.Header.Set("X-BPM-Page-URL", page)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err = json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return res
}

func TestBridgeRejectsOversizedBody(t *testing.T) {
	st, err := app.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ts := httptest.NewServer(&Server{Svc: service.New(st), Bridge: mustBridgeManager(t, st)})
	defer ts.Close()
	res, err := http.Post(ts.URL+"/bridge/v1/hello", "application/json", strings.NewReader(strings.Repeat("x", bridge.MaxBodyBytes+1)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized bridge body status=%s", res.Status)
	}
}

func TestBridgeRejectsChunkedOversizeAndCrossOriginAdmin(t *testing.T) {
	st, err := app.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ts := httptest.NewServer(&Server{Svc: service.New(st), Bridge: mustBridgeManager(t, st)})
	defer ts.Close()

	oversized := "{\"code\":\"" + strings.Repeat("x", bridge.MaxBodyBytes+1) + "\",\"platform\":\"x\",\"page_url\":\"https://x.com\"}"
	request, _ := http.NewRequest(http.MethodPost, ts.URL+"/bridge/v1/hello", io.NopCloser(strings.NewReader(oversized)))
	request.ContentLength = -1
	request.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked oversized bridge body status=%s", res.Status)
	}

	request, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/bridge/pairings", strings.NewReader(`{"platform":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://x.com")
	res, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin bridge admin status=%s", res.Status)
	}
}
