package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
