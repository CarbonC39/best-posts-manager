package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedFrontendIsPresent(t *testing.T) {
	data, err := fs.ReadFile(web, "dist/index.html")
	if err != nil || len(data) == 0 {
		t.Fatalf("embedded frontend missing: %v", err)
	}
}

func TestWebHandlerServesHistoryRoutesAndKeepsAssetsStrict(t *testing.T) {
	handler, err := newWebHandler(fstest.MapFS{
		"index.html":    {Data: []byte("<main>app shell</main>")},
		"assets/app.js": {Data: []byte("console.log('asset')")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/topics", "/topics/topic-1", "/history", "/settings"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "app shell") {
			t.Fatalf("GET %s: status=%d body=%q", path, recorder.Code, recorder.Body.String())
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://localhost/assets/app.js", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "asset") {
		t.Fatalf("asset: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "http://localhost/settings", nil))
	if recorder.Code == http.StatusOK || strings.Contains(recorder.Body.String(), "app shell") {
		t.Fatalf("POST /settings unexpectedly received SPA fallback: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestServeAddressMustBeLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8787", "[::1]:8787", "localhost:8787"} {
		if !loopbackListenAddress(addr) {
			t.Fatalf("loopback address rejected: %s", addr)
		}
	}
	for _, addr := range []string{"0.0.0.0:8787", ":8787", "192.168.1.2:8787"} {
		if loopbackListenAddress(addr) {
			t.Fatalf("non-loopback address accepted: %s", addr)
		}
	}
}

func TestHTTPLogOptInFromEnvironment(t *testing.T) {
	for _, value := range []string{"1", "true", "yes", "on", "TRUE"} {
		t.Setenv("BPM_HTTP_LOG", value)
		if !envHTTPLog() {
			t.Fatalf("BPM_HTTP_LOG=%q was not enabled", value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "random"} {
		t.Setenv("BPM_HTTP_LOG", value)
		if envHTTPLog() {
			t.Fatalf("BPM_HTTP_LOG=%q was unexpectedly enabled", value)
		}
	}
}

func TestCLIApplicationFlow(t *testing.T) {
	dir := t.TempDir()
	goCache := filepath.Join(t.TempDir(), "cache")
	run := func(args ...string) string {
		cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
		cmd.Dir = "."
		cmd.Env = append(os.Environ(), "BPM_DATA_DIR="+dir, "GOCACHE="+goCache)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	profile := run("profile", "add", "cli-profile")
	var p struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(profile), &p) != nil || p.ID == "" {
		t.Fatalf("profile output: %s", profile)
	}
	account := run("account", "add", p.ID, "mastodon", "cli-account", "http://fake", "TOKEN")
	var a struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(account), &a) != nil || a.ID == "" {
		t.Fatalf("account output: %s", account)
	}
	topic := run("topic", "add", p.ID, "cli-topic")
	var tp struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(topic), &tp) != nil || tp.ID == "" {
		t.Fatalf("topic output: %s", topic)
	}
	run("draft", "set", tp.ID, "hello from cli")
	draft := run("draft", "show", tp.ID)
	if !strings.Contains(draft, "hello from cli") {
		t.Fatalf("draft output: %s", draft)
	}
	variant := run("variant", "add", tp.ID, a.ID)
	var v struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(variant), &v) != nil || v.ID == "" {
		t.Fatalf("variant output: %s", variant)
	}
	run("variant", "edit", v.ID, "edited variant")
	shown := run("variant", "show", v.ID)
	if !strings.Contains(shown, "edited variant") {
		t.Fatalf("variant show output: %s", shown)
	}
	run("external", "mark", tp.ID, "x", "https://x.test/1")
	listed := run("external", "list", tp.ID)
	if !strings.Contains(listed, "https://x.test/1") {
		t.Fatalf("external list output: %s", listed)
	}
}
