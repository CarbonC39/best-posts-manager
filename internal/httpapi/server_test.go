package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"best-post-manager/internal/app"
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
	post("/api/accounts", map[string]any{"profile_id": p.ID, "platform": "mastodon", "name": "a", "base_url": "http://fake", "token_env": "TOKEN"}, &a)
	if a.ProfileID != p.ID {
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
