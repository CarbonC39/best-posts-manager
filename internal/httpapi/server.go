package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"best-post-manager/internal/app"
	"best-post-manager/internal/bridge"
	"best-post-manager/internal/platform"
	"best-post-manager/internal/service"
)

type Server struct {
	Svc      *service.Service
	Static   http.Handler
	Bridge   *bridge.Manager
	bridgeMu sync.Mutex
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
			var v app.Account
			if e := decode(r, &v); e != nil {
				fail(w, 400, e)
				return
			}
			v, e := s.Svc.Store.CreateAccount(ctx, v)
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
	case path == "topics":
		if r.Method == http.MethodGet {
			v, e := s.Svc.Store.ListTopics(ctx, r.URL.Query().Get("profile_id"))
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
			v, e := s.Svc.Store.ListPublications(ctx)
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
				writeJSON(w, 400, map[string]any{"error": e.Error(), "publication": v})
				return
			}
			writeJSON(w, 201, v)
			return
		}
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
			fail(w, 400, e)
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
			fail(w, 400, e)
		} else {
			writeJSON(w, 201, v)
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
		v, e := s.Svc.Store.ListMedia(ctx, r.URL.Query().Get("topic_id"))
		if e != nil {
			fail(w, 500, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	case path == "media" && r.Method == http.MethodPost:
		s.upload(w, r)
		return
	}
	fail(w, 404, fmt.Errorf("unknown API route"))
}
func (s *Server) topic(w http.ResponseWriter, r *http.Request, id string) {
	if !strings.HasSuffix(r.URL.Path, "/draft") {
		fail(w, 404, app.ErrNotFound)
		return
	}
	ctx := r.Context()
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
			fail(w, 400, e)
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
			fail(w, 400, e)
		} else {
			writeJSON(w, 200, v)
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
	if e := r.ParseMultipartForm(16 << 20); e != nil {
		fail(w, 400, e)
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
