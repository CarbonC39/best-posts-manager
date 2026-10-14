package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/bridge"
	"best-post-manager/internal/connection"
	"best-post-manager/internal/platform"
	"best-post-manager/internal/service"
)

type Server struct {
	Svc         *service.Service
	Static      http.Handler
	UserScript  []byte
	Bridge      *bridge.Manager
	Connections *connection.Manager
	// LogRequests is opt-in because this is a local app and route logs add
	// noise during normal use. Diagnostic and startup logs are independent.
	LogRequests bool
	bridgeMu    sync.Mutex
}

type responseLogger struct {
	http.ResponseWriter
	status int
}

func apiBridgePlatform(platform string) bool {
	switch platform {
	case "x", "weibo", "xiaohongshu", "qzone":
		return true
	}
	return false
}

func (w *responseLogger) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseLogger) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
func (w *responseLogger) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var logged *responseLogger
	if s.LogRequests {
		logged = &responseLogger{ResponseWriter: w}
		w = logged
	}
	started := time.Now()
	defer func() {
		if !s.LogRequests {
			return
		}
		status := logged.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf("http method=%s path=%s status=%d duration_ms=%d", r.Method, r.URL.Path, status, time.Since(started).Milliseconds())
	}()
	if !localRequestHost(r.Host) {
		fail(w, http.StatusForbidden, fmt.Errorf("local service only accepts loopback hosts"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/bridge/v1/") {
		s.bridgeScript(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.api(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/media/") {
		s.media(w, r)
		return
	}
	if r.URL.Path == "/userscript/bpm-bridge.user.js" {
		if len(s.UserScript) == 0 {
			http.NotFound(w, r)
			return
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		content := bytes.ReplaceAll(s.UserScript, []byte("__BPM_ORIGIN__"), []byte(scheme+"://"+r.Host))
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Content-Disposition", `inline; filename="bpm-bridge.user.js"`)
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "bpm-bridge.user.js", time.Time{}, bytes.NewReader(content))
		return
	}
	if s.Static != nil {
		s.Static.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	writeJSON(w, status, map[string]string{"error": e.Error()})
}
func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(v)
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path == "health" {
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	if strings.HasPrefix(path, "bridge/") {
		s.bridgeAdmin(w, r)
		return
	}
	if strings.HasPrefix(path, "connections/") {
		s.connections(w, r)
		return
	}
	switch {
	case path == "profiles":
		if r.Method == http.MethodGet {
			v, e := s.Svc.Store.ListProfiles(ctx)
			if e != nil {
				fail(w, 500, e)
			} else {
				writeJSON(w, 200, v)
			}
			return
		}
		if r.Method == http.MethodPost {
			var v app.Profile
			if e := decode(r, &v); e != nil {
				fail(w, 400, e)
				return
			}
			v, e := s.Svc.Store.CreateProfile(ctx, v)
			if e != nil {
				fail(w, 400, e)
			} else {
				writeJSON(w, 201, v)
			}
			return
		}
	case path == "accounts":
		if r.Method == http.MethodGet {
			v, e := s.Svc.Store.ListAccounts(ctx, r.URL.Query().Get("profile_id"))
			if e != nil {
				fail(w, 500, e)
			} else {
				writeJSON(w, 200, v)
			}
			return
		}
		if r.Method == http.MethodPost {
			var req struct {
				app.Account
				Secret string `json:"secret"`
			}
			if e := decode(r, &req); e != nil {
				fail(w, 400, e)
				return
			}
			// A manual secret is written to the local credential store by the
			// service. Neither the secret nor its internal reference is returned.
			if strings.TrimSpace(req.Secret) != "" && strings.TrimSpace(req.TokenEnv) != "" {
				fail(w, http.StatusBadRequest, errors.New("choose either a saved credential or an environment variable, not both"))
				return
			}
			if req.Secret == "" && req.TokenEnv == "" && !apiBridgePlatform(req.Platform) {
				fail(w, http.StatusBadRequest, errors.New("enter an access token/app password or an environment variable name"))
				return
			}
			v, e := s.Svc.CreateAccount(ctx, req.Account, req.Secret)
			if e != nil {
				status := 400
				if errors.Is(e, app.ErrNotFound) {
					status = 404
				} else if errors.Is(e, app.ErrConflict) {
					status = http.StatusConflict
				}
				fail(w, status, e)
			} else {
				writeJSON(w, 201, v)
			}
			return
		}
	case path == "profile-accounts":
		if r.Method != http.MethodPost {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var req struct {
			ProfileID string `json:"profile_id"`
			AccountID string `json:"account_id"`
		}
		if e := decode(r, &req); e != nil {
			fail(w, http.StatusBadRequest, e)
			return
		}
		v, e := s.Svc.Store.AttachAccount(ctx, req.ProfileID, req.AccountID)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusCreated, v)
		}
		return
	case path == "topics":
		if r.Method == http.MethodGet {
			v, e := s.Svc.Store.ListTopicSummaries(ctx, r.URL.Query().Get("profile_id"))
			if e != nil {
				fail(w, 500, e)
			} else {
				writeJSON(w, 200, v)
			}
			return
		}
		if r.Method == http.MethodPost {
			var v app.Topic
			if e := decode(r, &v); e != nil {
				fail(w, 400, e)
				return
			}
			v, e := s.Svc.Store.CreateTopic(ctx, v)
			if e != nil {
				status := 400
				if errors.Is(e, app.ErrNotFound) {
					status = 404
				}
				fail(w, status, e)
			} else {
				writeJSON(w, 201, v)
			}
			return
		}
	case path == "variants":
		if r.Method == http.MethodGet {
			v, e := s.Svc.Store.ListVariants(ctx, r.URL.Query().Get("topic_id"))
			if e != nil {
				fail(w, 500, e)
			} else {
				writeJSON(w, 200, v)
			}
			return
		}
		if r.Method == http.MethodPost {
			var req struct {
				TopicID         string `json:"topic_id"`
				AccountID       string `json:"account_id"`
				SourceVariantID string `json:"source_variant_id"`
			}
			if e := decode(r, &req); e != nil {
				fail(w, 400, e)
				return
			}
			v, e := s.Svc.Store.CreateVariant(ctx, app.Variant{TopicID: req.TopicID, AccountID: req.AccountID}, req.SourceVariantID)
			if e != nil {
				status := 400
				if errors.Is(e, app.ErrNotFound) {
					status = 404
				}
				fail(w, status, e)
			} else {
				writeJSON(w, 201, v)
			}
			return
		}
	case path == "publications":
		if r.Method == http.MethodGet {
			v, e := s.Svc.Store.ListPublicationSummariesForProfile(ctx, strings.TrimSpace(r.URL.Query().Get("profile_id")))
			if e != nil {
				fail(w, 500, e)
			} else {
				writeJSON(w, 200, v)
			}
			return
		}
		if r.Method == http.MethodPost {
			var req struct {
				VariantID string `json:"variant_id"`
			}
			if e := decode(r, &req); e != nil {
				fail(w, 400, e)
				return
			}
			v, e := s.Svc.Publish(ctx, req.VariantID)
			if e != nil {
				status := http.StatusBadRequest
				if errors.Is(e, app.ErrNotFound) {
					status = http.StatusNotFound
				} else if errors.Is(e, app.ErrConflict) {
					status = http.StatusConflict
				}
				writeJSON(w, status, map[string]any{"error": e.Error(), "publication": v})
				return
			}
			writeJSON(w, 201, v)
			return
		}
	case path == "publications/summary":
		if r.Method != http.MethodGet {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		profileID := strings.TrimSpace(r.URL.Query().Get("profile_id"))
		if profileID == "" {
			fail(w, http.StatusBadRequest, fmt.Errorf("profile_id is required"))
			return
		}
		v, e := s.Svc.Store.PublicationStatusSummary(ctx, profileID)
		if e != nil {
			fail(w, http.StatusInternalServerError, e)
		} else {
			writeJSON(w, http.StatusOK, v)
		}
		return
	case path == "publications/retry" && r.Method == http.MethodPost:
		var req struct {
			PublicationID string `json:"publication_id"`
		}
		if e := decode(r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		v, e := s.Svc.Retry(ctx, req.PublicationID)
		if e != nil {
			writeJSON(w, 400, map[string]any{"error": e.Error(), "publication": v})
		} else {
			writeJSON(w, 200, v)
		}
		return
	case strings.HasPrefix(path, "publications/") && strings.HasSuffix(path, "/confirm"):
		if r.Method != http.MethodPost {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		publicationID := strings.TrimSuffix(strings.TrimPrefix(path, "publications/"), "/confirm")
		if publicationID == "" || strings.Contains(publicationID, "/") {
			fail(w, http.StatusNotFound, app.ErrNotFound)
			return
		}
		var req struct {
			RemoteURL string `json:"remote_url"`
			RemoteID  string `json:"remote_id"`
		}
		if e := decode(r, &req); e != nil {
			fail(w, http.StatusBadRequest, e)
			return
		}
		v, e := s.Svc.ConfirmPublication(ctx, publicationID, req.RemoteURL, req.RemoteID)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, v)
		}
		return
	case strings.HasPrefix(path, "publications/") && r.Method == http.MethodDelete:
		publicationID := strings.TrimPrefix(path, "publications/")
		if publicationID == "" || strings.Contains(publicationID, "/") {
			fail(w, http.StatusNotFound, app.ErrNotFound)
			return
		}
		if e := s.Svc.DeletePublication(ctx, publicationID); e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "remote": "untouched"})
		}
		return
	case strings.HasPrefix(path, "publications/") && strings.HasSuffix(path, "/cancel"):
		if r.Method != http.MethodPost {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		publicationID := strings.TrimSuffix(strings.TrimPrefix(path, "publications/"), "/cancel")
		if publicationID == "" || strings.Contains(publicationID, "/") {
			fail(w, http.StatusNotFound, app.ErrNotFound)
			return
		}
		v, e := s.Svc.CancelPublication(ctx, publicationID)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, v)
		}
		return
	case path == "external-posts" && r.Method == http.MethodGet:
		v, e := s.Svc.Store.ListExternalPosts(ctx, r.URL.Query().Get("topic_id"))
		if e != nil {
			fail(w, 500, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	case path == "external-posts/import" && r.Method == http.MethodPost:
		var req struct {
			TopicID   string `json:"topic_id"`
			AccountID string `json:"account_id"`
			Reference string `json:"reference"`
		}
		if e := decode(r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		v, e := s.Svc.ImportExternal(ctx, req.TopicID, req.AccountID, req.Reference)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			}
			fail(w, status, e)
		} else {
			writeJSON(w, 201, v)
		}
		return
	case path == "external-posts" && r.Method == http.MethodPost:
		var v app.ExternalPost
		if e := decode(r, &v); e != nil {
			fail(w, 400, e)
			return
		}
		if v.CapturedBy == "" {
			v.CapturedBy = "manual"
		}
		v, e := s.Svc.Store.CreateExternalPost(ctx, v)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			}
			fail(w, status, e)
		} else {
			writeJSON(w, 201, v)
		}
		return
	case strings.HasPrefix(path, "external-posts/"):
		if r.Method != http.MethodDelete {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		externalPostID := strings.TrimPrefix(path, "external-posts/")
		if externalPostID == "" || strings.Contains(externalPostID, "/") {
			fail(w, http.StatusNotFound, app.ErrNotFound)
			return
		}
		if e := s.Svc.Store.DeleteExternalPost(ctx, externalPostID); e != nil {
			status := http.StatusInternalServerError
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		}
		return
	case path == "translation-configs" && r.Method == http.MethodGet:
		v, e := s.Svc.Store.ListTranslationConfigs(ctx)
		if e != nil {
			fail(w, 500, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	case path == "translation-configs" && r.Method == http.MethodPost:
		var v app.TranslationConfig
		if e := decode(r, &v); e != nil {
			fail(w, 400, e)
			return
		}
		// There is no public endpoint for attaching an arbitrary secret reference.
		v.CredentialRef = ""
		v, e := s.Svc.Store.CreateTranslationConfig(ctx, v)
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 201, v)
		}
		return
	case path == "translations" && r.Method == http.MethodGet:
		v, e := s.Svc.Store.ListTranslations(ctx)
		if e != nil {
			fail(w, 500, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	case path == "translations" && r.Method == http.MethodPost:
		var req struct {
			SourceVariantID string `json:"source_variant_id"`
			TargetAccountID string `json:"target_account_id"`
			SourceLanguage  string `json:"source_language"`
			TargetLanguage  string `json:"target_language"`
			ConfigID        string `json:"config_id"`
		}
		if e := decode(r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		v, record, e := s.Svc.Translate(ctx, service.TranslationRequest{SourceVariantID: req.SourceVariantID, TargetAccountID: req.TargetAccountID, SourceLanguage: req.SourceLanguage, TargetLanguage: req.TargetLanguage, ConfigID: req.ConfigID})
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 201, map[string]any{"variant": v, "translation": record})
		}
		return
	case strings.HasPrefix(path, "topics/"):
		s.topic(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "topics/"), "/draft"))
		return
	case strings.HasPrefix(path, "accounts/") && strings.HasSuffix(path, "/emojis") && r.Method == http.MethodGet:
		aid := strings.TrimSuffix(strings.TrimPrefix(path, "accounts/"), "/emojis")
		a, e := s.Svc.Store.GetAccount(ctx, aid)
		if e != nil {
			fail(w, 404, e)
			return
		}
		var v []platform.Emoji
		if a.Platform == "mastodon" || a.Platform == "mastodon-compatible" {
			v, e = s.Svc.Mastodon.CustomEmoji(ctx, a)
		} else if a.Platform == "misskey" {
			v, e = s.Svc.Misskey.CustomEmoji(ctx, a)
		} else {
			fail(w, 400, fmt.Errorf("this platform has no custom emoji endpoint"))
			return
		}
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	case strings.HasPrefix(path, "accounts/") && strings.HasSuffix(path, "/diagnostic") && r.Method == http.MethodPost:
		aid := strings.TrimSuffix(strings.TrimPrefix(path, "accounts/"), "/diagnostic")
		account, e := s.Svc.Store.GetAccount(ctx, aid)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			}
			fail(w, status, e)
			return
		}
		if account.Platform == "bluesky" {
			diagnostic, diagnosticErr := s.Svc.DiagnoseBluesky(ctx, aid)
			if diagnosticErr != nil {
				fail(w, http.StatusBadRequest, diagnosticErr)
			} else {
				writeJSON(w, http.StatusOK, diagnostic)
			}
			return
		}
		if account.Platform == "mastodon" || account.Platform == "mastodon-compatible" {
			diagnostic, diagnosticErr := s.Svc.DiagnoseMastodon(ctx, aid)
			if diagnosticErr != nil {
				fail(w, http.StatusBadRequest, diagnosticErr)
			} else {
				writeJSON(w, http.StatusOK, diagnostic)
			}
			return
		}
		fail(w, http.StatusBadRequest, fmt.Errorf("connection diagnostics are not supported for %s", account.Platform))
		return
	case strings.HasPrefix(path, "accounts/") && strings.HasSuffix(path, "/connection") && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		aid := strings.TrimSuffix(strings.TrimPrefix(path, "accounts/"), "/connection")
		current, e := s.Svc.Store.GetAccount(ctx, aid)
		if e != nil {
			status := http.StatusInternalServerError
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			}
			fail(w, status, e)
			return
		}
		var req struct {
			Name     string `json:"name"`
			RemoteID string `json:"remote_id"`
			BaseURL  string `json:"base_url"`
			Secret   string `json:"secret"`
			TokenEnv string `json:"token_env"`
		}
		if e = decode(r, &req); e != nil {
			fail(w, http.StatusBadRequest, e)
			return
		}
		updated, e := s.Svc.UpdateAccount(ctx, app.Account{ID: current.ID, ProfileID: current.ProfileID, Platform: current.Platform, Name: req.Name, RemoteID: req.RemoteID, BaseURL: req.BaseURL}, req.Secret, req.TokenEnv)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, updated)
		}
		return
	case strings.HasPrefix(path, "accounts/") && strings.HasSuffix(path, "/capability") && r.Method == http.MethodGet:
		aid := strings.TrimSuffix(strings.TrimPrefix(path, "accounts/"), "/capability")
		a, e := s.Svc.Store.GetAccount(ctx, aid)
		if e != nil {
			fail(w, 404, e)
			return
		}
		writeJSON(w, 200, platform.CapabilityFor(a.Platform))
		return
	case strings.HasPrefix(path, "variants/"):
		s.variant(w, r, strings.TrimPrefix(path, "variants/"))
		return
	case path == "media" && r.Method == http.MethodGet:
		topicID := r.URL.Query().Get("topic_id")
		var v []app.Media
		var e error
		if _, scoped := r.URL.Query()["variant_id"]; scoped {
			v, e = s.Svc.Store.ListMediaForVariant(ctx, topicID, r.URL.Query().Get("variant_id"))
		} else {
			v, e = s.Svc.Store.ListMedia(ctx, topicID)
		}
		if e != nil {
			status := http.StatusInternalServerError
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if strings.Contains(e.Error(), "same topic") {
				status = http.StatusBadRequest
			}
			fail(w, status, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	case path == "media/order" && r.Method == http.MethodPut:
		var req struct {
			TopicID   string   `json:"topic_id"`
			VariantID string   `json:"variant_id"`
			MediaIDs  []string `json:"media_ids"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
		decoder.DisallowUnknownFields()
		if e := decoder.Decode(&req); e != nil {
			fail(w, http.StatusBadRequest, e)
			return
		}
		if req.TopicID == "" || req.MediaIDs == nil {
			fail(w, http.StatusBadRequest, errors.New("topic_id and media_ids are required"))
			return
		}
		if e := s.Svc.Store.ReorderMedia(ctx, req.TopicID, req.VariantID, req.MediaIDs); e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, map[string][]string{"media_ids": req.MediaIDs})
		}
		return
	case path == "media" && r.Method == http.MethodPost:
		s.upload(w, r)
		return
	case strings.HasPrefix(path, "media/"):
		id := strings.TrimPrefix(path, "media/")
		if r.Method == http.MethodPatch {
			var req struct {
				AltText *string `json:"alt_text"`
			}
			decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if e := decoder.Decode(&req); e != nil {
				fail(w, http.StatusBadRequest, e)
				return
			}
			if req.AltText == nil {
				fail(w, http.StatusBadRequest, errors.New("alt_text is required"))
				return
			}
			updated, e := s.Svc.Store.UpdateMediaAltText(ctx, id, *req.AltText)
			if e != nil {
				status := http.StatusBadRequest
				if errors.Is(e, app.ErrNotFound) {
					status = http.StatusNotFound
				}
				fail(w, status, e)
			} else {
				writeJSON(w, http.StatusOK, updated)
			}
			return
		}
		if r.Method != http.MethodDelete {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		if err := s.Svc.Store.DeleteMedia(ctx, id); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(err, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, err)
		} else {
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		}
		return
	case strings.HasPrefix(path, "accounts/") && strings.HasSuffix(path, "/deactivate"):
		if r.Method != http.MethodDelete {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		aid := strings.TrimSuffix(strings.TrimPrefix(path, "accounts/"), "/deactivate")
		account, err := s.Svc.DeactivateAccount(ctx, aid)
		if err != nil {
			fail(w, 400, err)
		} else {
			writeJSON(w, http.StatusOK, account)
		}
		return
	case strings.HasPrefix(path, "accounts/") && r.Method == http.MethodDelete:
		aid := strings.TrimPrefix(path, "accounts/")
		if aid == "" || strings.Contains(aid, "/") {
			fail(w, http.StatusNotFound, app.ErrNotFound)
			return
		}
		account, err := s.Svc.DeleteAccountPermanently(ctx, aid)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(err, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, err)
		} else {
			writeJSON(w, http.StatusOK, account)
		}
		return
	}
	fail(w, 404, fmt.Errorf("unknown API route"))
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	if s.Connections == nil {
		fail(w, 503, fmt.Errorf("connections are unavailable"))
		return
	}
	path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/connections/"), "/")
	if path == "status" && r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]string{"status": "ready"})
		return
	}
	if path == "mastodon/start" || path == "misskey/start" {
		if r.Method != http.MethodPost {
			fail(w, 405, fmt.Errorf("method not allowed"))
			return
		}
		if !localRequestHost(r.Host) {
			fail(w, 400, fmt.Errorf("server must be accessed through a loopback host"))
			return
		}
		if !sameOriginOrNoOrigin(r) {
			fail(w, http.StatusForbidden, fmt.Errorf("connection requests must be same-origin"))
			return
		}
		if !bridgeJSONRequest(r) {
			fail(w, http.StatusUnsupportedMediaType, fmt.Errorf("connection requests require application/json"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var req connection.StartRequest
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		var out connection.StartResult
		var err error
		callbackBase := r.Host
		if path == "mastodon/start" {
			out, err = s.Connections.StartMastodon(r.Context(), req, callbackBase)
		} else {
			out, err = s.Connections.StartMisskey(r.Context(), req, callbackBase)
		}
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if path == "mastodon/callback" {
		if r.Method != http.MethodGet {
			fail(w, 405, fmt.Errorf("method not allowed"))
			return
		}
		if !localRequestHost(r.Host) {
			fail(w, 400, fmt.Errorf("callback must use a loopback host"))
			return
		}
		_, err := s.Connections.CallbackMastodon(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		http.Redirect(w, r, "/?connection=mastodon", http.StatusFound)
		return
	}
	if path == "misskey/callback" {
		if r.Method != http.MethodGet {
			fail(w, 405, fmt.Errorf("method not allowed"))
			return
		}
		if !localRequestHost(r.Host) {
			fail(w, 400, fmt.Errorf("callback must use a loopback host"))
			return
		}
		_, err := s.Connections.CallbackMisskey(r.Context(), r.URL.Query().Get("session"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		http.Redirect(w, r, "/?connection=misskey", http.StatusFound)
		return
	}
	fail(w, 404, fmt.Errorf("unknown connection route"))
}
func localRequestHost(host string) bool {
	h := host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		h = parsed
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
func (s *Server) topic(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	if !strings.HasSuffix(r.URL.Path, "/draft") {
		if r.Method != http.MethodPut {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var v app.Topic
		if e := decode(r, &v); e != nil {
			fail(w, http.StatusBadRequest, e)
			return
		}
		v.ID = id
		v, e := s.Svc.Store.UpdateTopic(ctx, v)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, v)
		}
		return
	}
	if r.Method == http.MethodGet {
		v, e := s.Svc.Store.GetBaseDraft(ctx, id)
		if e != nil {
			fail(w, 404, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if r.Method == http.MethodPut {
		var v app.BaseDraft
		if e := decode(r, &v); e != nil {
			fail(w, 400, e)
			return
		}
		v.TopicID = id
		v, e := s.Svc.Store.UpdateBaseDraft(ctx, v)
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	fail(w, 405, fmt.Errorf("method not allowed"))
}
func (s *Server) variant(w http.ResponseWriter, r *http.Request, id string) {
	id = strings.TrimSuffix(id, "/")
	if strings.HasSuffix(id, "/apply") {
		if r.Method != http.MethodPost {
			fail(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		id = strings.TrimSuffix(id, "/apply")
		v, e := s.Svc.Store.ApplyVariantUpdate(r.Context(), id)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if r.Method == http.MethodGet {
		v, e := s.Svc.Store.GetVariant(r.Context(), id)
		if e != nil {
			fail(w, 404, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if r.Method == http.MethodPut {
		var v app.Variant
		if e := decode(r, &v); e != nil {
			fail(w, 400, e)
			return
		}
		v.ID = id
		v, e := s.Svc.Store.UpdateVariant(r.Context(), v)
		if e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if r.Method == http.MethodDelete {
		if e := s.Svc.Store.DeleteVariant(r.Context(), id); e != nil {
			status := http.StatusBadRequest
			if errors.Is(e, app.ErrNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(e, app.ErrConflict) {
				status = http.StatusConflict
			}
			fail(w, status, e)
		} else {
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		}
		return
	}
	fail(w, 405, fmt.Errorf("method not allowed"))
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 32<<20 {
		fail(w, 413, fmt.Errorf("image exceeds 32 MiB"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 33<<20)
	if e := r.ParseMultipartForm(16 << 20); e != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(e, &tooLarge) {
			fail(w, http.StatusRequestEntityTooLarge, fmt.Errorf("image exceeds 32 MiB"))
		} else {
			fail(w, 400, e)
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	topicID := r.FormValue("topic_id")
	variantID := r.FormValue("variant_id")
	file, head, e := r.FormFile("file")
	if e != nil {
		fail(w, 400, e)
		return
	}
	defer file.Close()
	if !strings.HasPrefix(strings.ToLower(head.Header.Get("Content-Type")), "image/") {
		fail(w, 400, fmt.Errorf("only image uploads are supported"))
		return
	}
	var randomID [16]byte
	if _, e := rand.Read(randomID[:]); e != nil {
		fail(w, 500, e)
		return
	}
	mid := fmt.Sprintf("%x", randomID[:])
	path := filepath.Join(s.Svc.Store.MediaDir, mid)
	out, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if e != nil {
		fail(w, 500, e)
		return
	}
	n, e := io.Copy(out, file)
	_ = out.Close()
	if e != nil {
		_ = os.Remove(path)
		fail(w, 500, e)
		return
	}
	if n > 32<<20 {
		_ = os.Remove(path)
		fail(w, 413, fmt.Errorf("image exceeds 32 MiB"))
		return
	}
	m, e := s.Svc.Store.AddMedia(r.Context(), app.Media{TopicID: topicID, VariantID: variantID, Filename: head.Filename, MIME: head.Header.Get("Content-Type"), Path: path, AltText: r.FormValue("alt_text"), Size: n})
	if e != nil {
		_ = os.Remove(path)
		fail(w, 400, e)
		return
	}
	writeJSON(w, 201, m)
}
func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	id := filepath.Base(strings.TrimPrefix(r.URL.Path, "/media/"))
	rows, e := s.Svc.Store.DB.QueryContext(r.Context(), `SELECT path FROM media WHERE id=?`, id)
	if e != nil {
		fail(w, 500, e)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		http.NotFound(w, r)
		return
	}
	var p string
	if e = rows.Scan(&p); e != nil {
		fail(w, 500, e)
		return
	}
	if filepath.Dir(p) != s.Svc.Store.MediaDir {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}
