package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/backup"
	"best-post-manager/internal/bridge"
	"best-post-manager/internal/connection"
	"best-post-manager/internal/httpapi"
	"best-post-manager/internal/service"
)

//go:embed dist
var web embed.FS

func main() {
	ctx := context.Background()
	data := os.Getenv("BPM_DATA_DIR")
	if data == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			fatal(err)
		}
		data = filepath.Join(config, "best-post-manager")
	}
	if len(os.Args) < 2 {
		usage()
		return
	}
	store, err := app.Open(ctx, data)
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	svc := service.New(store)
	switch os.Args[1] {
	case "serve":
		serve(svc, os.Args[2:])
	case "profile":
		profile(ctx, store, os.Args[2:])
	case "account":
		account(ctx, store, os.Args[2:])
	case "topic":
		topic(ctx, store, os.Args[2:])
	case "variant":
		variant(ctx, store, os.Args[2:])
	case "draft":
		draft(ctx, store, os.Args[2:])
	case "media":
		media(ctx, store, os.Args[2:])
	case "publish":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("publish requires variant id"))
		}
		p, e := svc.Publish(ctx, os.Args[2])
		printResult(p, e)
	case "retry":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("retry requires publication id"))
		}
		p, e := svc.Retry(ctx, os.Args[2])
		printResult(p, e)
	case "history":
		p, e := store.ListPublications(ctx)
		printResult(p, e)
	case "external":
		external(ctx, store, os.Args[2:])
	case "import":
		if len(os.Args) < 5 {
			fatal(fmt.Errorf("import requires topic-id account-id reference"))
		}
		post, e := svc.ImportExternal(ctx, os.Args[2], os.Args[3], os.Args[4])
		printResult(post, e)
	case "translation-config":
		translationConfig(ctx, store, os.Args[2:])
	case "translate":
		if len(os.Args) < 6 {
			fatal(fmt.Errorf("translate requires source-variant target-account target-language config-id"))
		}
		sourceLanguage := ""
		if len(os.Args) > 6 {
			sourceLanguage = os.Args[6]
		}
		v, record, e := svc.Translate(ctx, service.TranslationRequest{SourceVariantID: os.Args[2], TargetAccountID: os.Args[3], SourceLanguage: sourceLanguage, TargetLanguage: os.Args[4], ConfigID: os.Args[5]})
		printResult(map[string]any{"variant": v, "translation": record}, e)
	case "backup":
		backupCommand(ctx, store, os.Args[2:])
	default:
		usage()
	}
}
func usage() {
	fmt.Println("bpm serve [-addr 127.0.0.1:8787]\nbpm profile add <name> | list\nbpm account add <profile-id> <platform> <name> [base-url] [token-env] [remote-id] | list [profile-id]\nbpm topic add <profile-id> <title> | list [profile-id]\nbpm draft show|set <topic-id> [text]\nbpm variant add|show|edit|apply|list ...\nbpm media add|list <topic-id> [file] [variant-id]\nbpm publish <variant-id> | retry <publication-id>\nbpm history\nbpm import <topic-id> <account-id> <URL-or-remote-id>\nbpm external mark|list <topic-id> <platform> <url> [remote-id]\nbpm translation-config add <provider> <token-or-key-env> [base-url] [project-or-model] | list\nbpm translate <source-variant> <target-account> <target-language> <config-id> [source-language]\nbpm backup create <archive.zip> | restore <archive.zip> <empty-directory>")
}

func translationConfig(ctx context.Context, s *app.Store, args []string) {
	if len(args) > 0 && args[0] == "list" {
		v, e := s.ListTranslationConfigs(ctx)
		printResult(v, e)
		return
	}
	if len(args) < 2 || args[0] != "add" {
		fatal(fmt.Errorf("translation-config add requires provider token-or-key-env [base-url] [project-or-model]"))
	}
	c := app.TranslationConfig{Provider: args[1]}
	if c.Provider == "google-gtx" {
		if len(args) > 2 {
			c.BaseURL = args[2]
		}
		v, e := s.CreateTranslationConfig(ctx, c)
		printResult(v, e)
		return
	}
	if len(args) < 3 {
		fatal(fmt.Errorf("translation-config add requires provider token-or-key-env [base-url] [project-or-model]"))
	}
	if c.Provider == "deepl" {
		c.KeyEnv = args[2]
	} else {
		c.TokenEnv = args[2]
	}
	if len(args) > 3 {
		c.BaseURL = args[3]
	}
	if len(args) > 4 {
		if c.Provider == "google" || c.Provider == "google-cloud" {
			c.ProjectID = args[4]
		} else {
			c.Model = args[4]
		}
	}
	v, e := s.CreateTranslationConfig(ctx, c)
	printResult(v, e)
}

func backupCommand(ctx context.Context, s *app.Store, args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("backup create <archive.zip> | restore <archive.zip> <empty-directory>"))
	}
	switch args[0] {
	case "create":
		printResult(map[string]string{"archive": args[1]}, backup.Create(ctx, s.DataDir, args[1]))
	case "restore":
		if len(args) < 3 {
			fatal(fmt.Errorf("backup restore requires archive and empty directory"))
		}
		printResult(map[string]string{"directory": args[2]}, backup.Restore(ctx, args[1], args[2]))
	default:
		fatal(fmt.Errorf("unknown backup command"))
	}
}
func serve(svc *service.Service, args []string) {
	f := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := f.String("addr", "127.0.0.1:8787", "listen address")
	httpLog := f.Bool("http-log", envHTTPLog(), "log HTTP routes (also enabled by BPM_HTTP_LOG=1)")
	_ = f.Parse(args)
	if !loopbackListenAddress(*addr) {
		fatal(fmt.Errorf("serve address must use localhost or a loopback IP; remote access is not supported"))
	}
	static, _ := fs.Sub(web, "dist")
	userScript, err := fs.ReadFile(static, "bpm-bridge.user.js")
	if err != nil {
		fatal(fmt.Errorf("embedded userscript: %w", err))
	}
	log.Printf("startup address=http://%s http_request_log=%s", *addr, enabledLabel(*httpLog))
	webHandler, err := newWebHandler(static)
	if err != nil {
		fatal(fmt.Errorf("embedded web index: %w", err))
	}
	if e := http.ListenAndServe(*addr, &httpapi.Server{Svc: svc, Static: webHandler, UserScript: userScript, Bridge: bridge.NewManager(svc.Store), Connections: connection.NewManager(svc.Store, svc.Credentials), LogRequests: *httpLog}); e != nil {
		fatal(e)
	}
}

func newWebHandler(static fs.FS) (http.Handler, error) {
	indexHTML, err := fs.ReadFile(static, "index.html")
	if err != nil {
		return nil, err
	}
	staticHandler := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && (r.URL.Path == "/" || r.URL.Path == "/topics" || strings.HasPrefix(r.URL.Path, "/topics/") || r.URL.Path == "/history" || r.URL.Path == "/settings") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(indexHTML))
			return
		}
		staticHandler.ServeHTTP(w, r)
	}), nil
}

func envHTTPLog() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("BPM_HTTP_LOG")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func enabledLabel(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}

func loopbackListenAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	host = strings.Trim(host, "[]")
	return strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}
func profile(ctx context.Context, s *app.Store, args []string) {
	if len(args) > 0 && args[0] == "add" {
		name := strings.Join(args[1:], " ")
		p, e := s.CreateProfile(ctx, app.Profile{Name: name})
		printResult(p, e)
		return
	}
	p, e := s.ListProfiles(ctx)
	printResult(p, e)
}
func account(ctx context.Context, s *app.Store, args []string) {
	if len(args) > 0 && args[0] == "add" {
		if len(args) < 4 {
			fatal(fmt.Errorf("account add requires profile platform name [base-url] [token-env]"))
		}
		bridgePlatform := args[2] == "x" || args[2] == "weibo" || args[2] == "xiaohongshu" || args[2] == "qzone"
		if !bridgePlatform && len(args) < 5 {
			fatal(fmt.Errorf("API platform accounts require base-url"))
		}
		a := app.Account{ProfileID: args[1], Platform: args[2], Name: args[3]}
		if len(args) > 4 {
			a.BaseURL = args[4]
		}
		if len(args) > 5 {
			a.TokenEnv = args[5]
		}
		if len(args) > 6 {
			a.RemoteID = args[6]
		}
		v, e := s.CreateAccount(ctx, a)
		printResult(v, e)
		return
	}
	profileID := ""
	if len(args) > 1 {
		profileID = args[1]
	}
	v, e := s.ListAccounts(ctx, profileID)
	printResult(v, e)
}
func topic(ctx context.Context, s *app.Store, args []string) {
	if len(args) > 0 && args[0] == "add" {
		if len(args) < 3 {
			fatal(fmt.Errorf("topic add requires profile-id title"))
		}
		v, e := s.CreateTopic(ctx, app.Topic{ProfileID: args[1], Title: strings.Join(args[2:], " ")})
		printResult(v, e)
		return
	}
	profileID := ""
	if len(args) > 1 {
		profileID = args[1]
	}
	v, e := s.ListTopics(ctx, profileID)
	printResult(v, e)
}
func variant(ctx context.Context, s *app.Store, args []string) {
	if len(args) > 0 && args[0] == "add" {
		if len(args) < 3 {
			fatal(fmt.Errorf("variant add requires topic-id account-id [source-variant-id]"))
		}
		source := ""
		if len(args) > 3 {
			source = args[3]
		}
		v, e := s.CreateVariant(ctx, app.Variant{TopicID: args[1], AccountID: args[2]}, source)
		printResult(v, e)
		return
	}
	if len(args) < 2 {
		fatal(fmt.Errorf("variant list requires topic-id"))
	}
	if args[0] == "show" {
		v, e := s.GetVariant(ctx, args[1])
		printResult(v, e)
		return
	}
	if args[0] == "edit" {
		if len(args) < 3 {
			fatal(fmt.Errorf("variant edit requires id text"))
		}
		v, e := s.GetVariant(ctx, args[1])
		if e != nil {
			fatal(e)
		}
		v.Text = strings.Join(args[2:], " ")
		v, e = s.UpdateVariant(ctx, v)
		printResult(v, e)
		return
	}
	if args[0] == "apply" {
		v, e := s.ApplyVariantUpdate(ctx, args[1])
		printResult(v, e)
		return
	}
	v, e := s.ListVariants(ctx, args[1])
	printResult(v, e)
}
func draft(ctx context.Context, s *app.Store, args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("draft show|set requires topic-id"))
	}
	if args[0] == "show" {
		v, e := s.GetBaseDraft(ctx, args[1])
		printResult(v, e)
		return
	}
	if args[0] != "set" || len(args) < 3 {
		fatal(fmt.Errorf("draft set requires topic-id text"))
	}
	v, e := s.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: args[1], Text: strings.Join(args[2:], " "), Format: "plain"})
	printResult(v, e)
}
func media(ctx context.Context, s *app.Store, args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("media add|list requires topic-id"))
	}
	if args[0] == "list" {
		v, e := s.ListMedia(ctx, args[1])
		printResult(v, e)
		return
	}
	if args[0] != "add" || len(args) < 3 {
		fatal(fmt.Errorf("media add requires topic-id file [variant-id]"))
	}
	data, e := os.ReadFile(args[2])
	if e != nil {
		fatal(e)
	}
	if len(data) > 32<<20 {
		fatal(fmt.Errorf("image exceeds 32 MiB"))
	}
	mimeType := mime.TypeByExtension(filepath.Ext(args[2]))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		fatal(fmt.Errorf("only image uploads are supported"))
	}
	var randomID [16]byte
	if _, e = rand.Read(randomID[:]); e != nil {
		fatal(e)
	}
	dst := filepath.Join(s.MediaDir, fmt.Sprintf("%x", randomID[:]))
	if e = os.WriteFile(dst, data, 0o600); e != nil {
		fatal(e)
	}
	variantID := ""
	if len(args) > 3 {
		variantID = args[3]
	}
	v, e := s.AddMedia(ctx, app.Media{TopicID: args[1], VariantID: variantID, Filename: filepath.Base(args[2]), MIME: mimeType, Path: dst, Size: int64(len(data))})
	if e != nil {
		_ = os.Remove(dst)
	}
	printResult(v, e)
}
func external(ctx context.Context, s *app.Store, args []string) {
	if len(args) > 0 && args[0] == "list" {
		if len(args) < 2 {
			fatal(fmt.Errorf("external list requires topic-id"))
		}
		v, e := s.ListExternalPosts(ctx, args[1])
		printResult(v, e)
		return
	}
	if len(args) < 4 || args[0] != "mark" {
		fatal(fmt.Errorf("external mark requires topic-id platform url [remote-id]"))
	}
	p := app.ExternalPost{TopicID: args[1], Platform: args[2], URL: args[3], CapturedBy: "manual"}
	if len(args) > 4 {
		p.RemoteID = args[4]
	}
	v, e := s.CreateExternalPost(ctx, p)
	printResult(v, e)
}
func printResult(v any, e error) {
	if e != nil {
		fatal(e)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
func fatal(e error) { fmt.Fprintln(os.Stderr, "error:", e); os.Exit(1) }
