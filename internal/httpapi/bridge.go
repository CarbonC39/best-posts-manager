package httpapi

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"best-post-manager/internal/app"
	"best-post-manager/internal/bridge"
)

func (s *Server) bridgeManager() *bridge.Manager {
	s.bridgeMu.Lock()
	defer s.bridgeMu.Unlock()
	if s.Bridge == nil {
		s.Bridge = bridge.NewManager(s.Svc.Store)
	}
	return s.Bridge
}

func (s *Server) bridgeAdmin(w http.ResponseWriter, r *http.Request) {
	if !sameOriginOrNoOrigin(r) {
		fail(w, http.StatusForbidden, errors.New("bridge admin requests must be same-origin"))
		return
	}
	if r.ContentLength > bridge.MaxBodyBytes {
		fail(w, http.StatusRequestEntityTooLarge, errors.New("bridge request body is too large"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, bridge.MaxBodyBytes)
	if r.Method == http.MethodPost && !bridgeJSONRequest(r) {
		fail(w, http.StatusUnsupportedMediaType, errors.New("bridge requests require application/json"))
		return
	}
	b := s.bridgeManager()
	path := strings.TrimPrefix(r.URL.Path, "/api/bridge/")
	if path == "pairings" {
		if r.Method == http.MethodGet {
			writeJSON(w, 200, b.ListPairings())
			return
		}
		if r.Method == http.MethodPost {
			var req struct {
				Platform string `json:"platform"`
			}
			if !decodeBridge(w, r, &req) {
				return
			}
			p, e := b.CreatePairing(req.Platform)
			if e != nil {
				fail(w, 400, e)
			} else {
				w.Header().Set("Cache-Control", "no-store")
				writeJSON(w, 201, p)
			}
			return
		}
		fail(w, 405, errors.New("method not allowed"))
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[0] == "pairings" && parts[1] == "revoke" {
		fail(w, 400, errors.New("pairing id is required"))
		return
	}
	if len(parts) == 3 && parts[0] == "pairings" && parts[2] == "revoke" && r.Method == http.MethodPost {
		if e := b.Revoke(parts[1]); e != nil {
			fail(w, 404, e)
		} else {
			writeJSON(w, 200, map[string]string{"status": "revoked"})
		}
		return
	}
	if len(parts) == 2 && parts[0] == "requests" && r.Method == http.MethodGet {
		v, e := b.Request(parts[1])
		if e != nil {
			fail(w, 404, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if path == "requests/fill" && r.Method == http.MethodPost {
		var req struct {
			PairingID   string `json:"pairing_id"`
			VariantID   string `json:"variant_id"`
			PageURL     string `json:"page_url"`
			AutoPublish bool   `json:"auto_publish"`
		}
		if !decodeBridge(w, r, &req) {
			return
		}
		if _, e := b.PairingPlatform(req.PairingID); e != nil {
			fail(w, 400, e)
			return
		}
		v, e := b.CreateFillWithOptions(r.Context(), req.PairingID, req.VariantID, req.PageURL, req.AutoPublish)
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 201, v)
		}
		return
	}
	if path == "requests/capture" && r.Method == http.MethodPost {
		var req struct {
			PairingID string `json:"pairing_id"`
			TopicID   string `json:"topic_id"`
			AccountID string `json:"account_id"`
			Platform  string `json:"platform"`
			PageURL   string `json:"page_url"`
		}
		if !decodeBridge(w, r, &req) {
			return
		}
		if _, e := b.PairingPlatform(req.PairingID); e != nil {
			fail(w, 400, e)
			return
		}
		v, e := b.CreateCapture(r.Context(), req.PairingID, req.TopicID, req.AccountID, req.Platform, req.PageURL)
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 201, v)
		}
		return
	}
	fail(w, 404, errors.New("unknown bridge admin route"))
}

func sameOriginOrNoOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, r.Host)
}

func bridgeToken(r *http.Request) string {
	const prefix = "Bearer "
	a := r.Header.Get("Authorization")
	if !strings.HasPrefix(a, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(a, prefix))
}
func bridgePage(r *http.Request) string { return r.Header.Get("X-BPM-Page-URL") }
func bridgeJSONRequest(r *http.Request) bool {
	value, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && value == "application/json"
}
func decodeBridge(w http.ResponseWriter, r *http.Request, value any) bool {
	err := json.NewDecoder(r.Body).Decode(value)
	if err == nil {
		return true
	}
	status := http.StatusBadRequest
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	fail(w, status, err)
	return false
}
func (s *Server) bridgeScript(w http.ResponseWriter, r *http.Request) {
	b := s.bridgeManager()
	if r.ContentLength > bridge.MaxBodyBytes {
		fail(w, 413, errors.New("bridge request body is too large"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, bridge.MaxBodyBytes)
	if r.Method == http.MethodPost && !bridgeJSONRequest(r) {
		fail(w, http.StatusUnsupportedMediaType, errors.New("bridge requests require application/json"))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/bridge/v1/")
	if path == "hello" && r.Method == http.MethodPost {
		var raw struct {
			Code     string `json:"code"`
			Platform string `json:"platform"`
			PageURL  string `json:"page_url"`
		}
		if !decodeBridge(w, r, &raw) {
			return
		}
		p, e := b.Hello(raw.Code, raw.Platform, raw.PageURL, r.Header.Get("Origin"))
		if e != nil {
			fail(w, 401, e)
		} else {
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, p)
		}
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "requests" && r.Method == http.MethodGet {
		pairingID := r.Header.Get("X-BPM-Pairing-ID")
		platform := r.Header.Get("X-BPM-Platform")
		if pairingID == "" || platform == "" {
			fail(w, 400, errors.New("pairing and platform headers are required"))
			return
		}
		if e := b.AuthenticatePairing(bridgeToken(r), pairingID, platform, bridgePage(r), r.Header.Get("Origin")); e != nil {
			fail(w, 401, e)
			return
		}
		writeJSON(w, 200, b.Pending(pairingID))
		return
	}
	if len(parts) < 2 || parts[0] != "requests" {
		fail(w, 404, errors.New("unknown bridge route"))
		return
	}
	id := parts[1]
	request, e := b.Request(id)
	if e != nil {
		fail(w, 404, e)
		return
	}
	if e = b.AuthenticateForHTTP(bridgeToken(r), request, bridgePage(r), r.Header.Get("Origin")); e != nil {
		fail(w, 401, e)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		writeJSON(w, 200, request)
		return
	}
	if len(parts) == 3 && parts[2] == "result" && r.Method == http.MethodPost {
		var req struct {
			URL      string `json:"url"`
			RemoteID string `json:"remote_id"`
		}
		if !decodeBridge(w, r, &req) {
			return
		}
		v, e := b.Result(r.Context(), id, req.URL, req.RemoteID)
		if e != nil {
			fail(w, 409, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if len(parts) == 3 && parts[2] == "filled" && r.Method == http.MethodPost {
		var req struct {
			Warning string `json:"warning"`
		}
		if !decodeBridge(w, r, &req) {
			return
		}
		v, e := b.Filled(id, req.Warning)
		if e != nil {
			fail(w, 409, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if len(parts) == 3 && parts[2] == "error" && r.Method == http.MethodPost {
		var req struct {
			Error string `json:"error"`
		}
		if !decodeBridge(w, r, &req) {
			return
		}
		v, e := b.Error(r.Context(), id, req.Error)
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	if len(parts) == 3 && parts[2] == "capture" && r.Method == http.MethodPost {
		var req struct {
			Text       string              `json:"text"`
			URL        string              `json:"url"`
			Media      []app.ExternalMedia `json:"media"`
			Incomplete bool                `json:"incomplete"`
			Reason     string              `json:"incomplete_reason"`
		}
		if !decodeBridge(w, r, &req) {
			return
		}
		v, e := b.Capture(r.Context(), id, req.Text, req.URL, req.Media, req.Incomplete, req.Reason)
		if e != nil {
			fail(w, 400, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	fail(w, 405, errors.New("method not allowed"))
}
