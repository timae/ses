package shareserver

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	defaultMaxBody = 10 << 20 // 10 MB — mirrors the CLI-side cap
	maxLifetime    = 30 * 24 * time.Hour
	minLifetime    = 5 * time.Minute
)

type Config struct {
	// BearerToken is required on write endpoints (POST/DELETE). Read endpoints
	// are public so the share URLs work for recipients without auth.
	BearerToken string
	// PublicURL is the externally-visible base (e.g. https://share.example.com)
	// used when building URLs returned to the uploader.
	PublicURL string
	// MaxBodyBytes overrides the default 10 MB upload cap. Zero uses the default.
	MaxBodyBytes int64
	// Logger is optional; defaults to log.Default().
	Logger *log.Logger
}

type Server struct {
	cfg   Config
	store Store
	tmpl  *template.Template
}

func NewServer(cfg Config, store Store) (*Server, error) {
	if cfg.BearerToken == "" {
		return nil, errors.New("BearerToken is required")
	}
	if cfg.PublicURL == "" {
		return nil, errors.New("PublicURL is required")
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = defaultMaxBody
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	tmpl, err := template.New("share").Funcs(tmplFuncs).Parse(sharePageHTML)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}
	return &Server{cfg: cfg, store: store, tmpl: tmpl}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/shares", s.handleShares)      // POST
	mux.HandleFunc("/v1/shares/", s.handleShareByID)  // DELETE /v1/shares/{id}
	mux.HandleFunc("/s/", s.handleView)               // GET /s/{id}[/raw.json.gz]
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// --- write: POST /v1/shares ----------------------------------------------

type uploadRequest struct {
	Name            string       `json:"name,omitempty"`
	ExpiresInSecond int64        `json:"expires_in_seconds"`
	Session         ShareSession `json:"session"`
	Messages        []ShareMsg   `json:"messages"`
}

type uploadResponse struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) handleShares(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
	var req uploadRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		s.replyError(w, http.StatusBadRequest, "invalid json: %v", err)
		return
	}
	if len(req.Messages) == 0 {
		s.replyError(w, http.StatusBadRequest, "messages must not be empty")
		return
	}
	lifetime := time.Duration(req.ExpiresInSecond) * time.Second
	if lifetime < minLifetime || lifetime > maxLifetime {
		s.replyError(w, http.StatusBadRequest, "expires_in_seconds must be between %d and %d", int(minLifetime.Seconds()), int(maxLifetime.Seconds()))
		return
	}

	id, err := NewID()
	if err != nil {
		s.replyError(w, http.StatusInternalServerError, "generating id: %v", err)
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(lifetime).UTC()

	share := Share{
		ID:        id,
		Name:      req.Name,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Session:   req.Session,
		Messages:  req.Messages,
	}
	gz, err := gzipJSON(share)
	if err != nil {
		s.replyError(w, http.StatusInternalServerError, "encoding: %v", err)
		return
	}
	if err := s.store.Put(r.Context(), id, expiresAt, gz); err != nil {
		s.cfg.Logger.Printf("store put %s: %v", id, err)
		s.replyError(w, http.StatusInternalServerError, "storage error")
		return
	}

	resp := uploadResponse{
		ID:        id,
		URL:       fmt.Sprintf("%s/s/%s", s.cfg.PublicURL, id),
		ExpiresAt: expiresAt,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// --- write: DELETE /v1/shares/{id} ---------------------------------------

func (s *Server) handleShareByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/shares/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := s.store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.cfg.Logger.Printf("store delete %s: %v", id, err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- read: GET /s/{id} and /s/{id}/raw.json.gz ---------------------------

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	id, tail := rest, ""
	if i := strings.Index(rest, "/"); i >= 0 {
		id, tail = rest[:i], rest[i+1:]
	}
	if id == "" {
		http.NotFound(w, r)
		return
	}

	gz, exp, err := s.store.Get(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, ErrExpired):
		// Best-effort cleanup; sweeper is the backstop.
		_ = s.store.Delete(context.Background(), id)
		http.Error(w, "this share link has expired", http.StatusGone)
		return
	case err != nil:
		s.cfg.Logger.Printf("store get %s: %v", id, err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	switch tail {
	case "":
		s.renderHTML(w, r, gz, exp)
	case "raw.json.gz":
		s.serveRaw(w, gz, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveRaw(w http.ResponseWriter, gz []byte, id string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="share-%s.json.gz"`, id))
	_, _ = w.Write(gz)
}

func (s *Server) renderHTML(w http.ResponseWriter, r *http.Request, gz []byte, exp time.Time) {
	share, err := decodeShare(gz)
	if err != nil {
		s.cfg.Logger.Printf("decoding share: %v", err)
		http.Error(w, "malformed share", http.StatusInternalServerError)
		return
	}
	remaining := time.Until(exp).Round(time.Minute)
	data := map[string]any{
		"Share":     share,
		"Expires":   exp,
		"Remaining": remaining.String(),
		"RawURL":    path.Join("/s", share.ID, "raw.json.gz"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.Execute(w, data); err != nil {
		s.cfg.Logger.Printf("template exec: %v", err)
	}
}

// --- helpers --------------------------------------------------------------

func (s *Server) checkAuth(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	got := h[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.BearerToken)) == 1
}

func (s *Server) replyError(w http.ResponseWriter, code int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf(format, args...)})
}

func gzipJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := json.NewEncoder(gz).Encode(v); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeShare(gz []byte) (Share, error) {
	var s Share
	gr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return s, err
	}
	defer gr.Close()
	body, err := io.ReadAll(gr)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return s, err
	}
	return s, nil
}

// --- template -------------------------------------------------------------

var tmplFuncs = template.FuncMap{
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("2006-01-02 15:04 UTC")
	},
}

const sharePageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{if .Share.Name}}{{.Share.Name}}{{else}}Shared session {{.Share.ID}}{{end}}</title>
<meta name="robots" content="noindex,nofollow">
<style>
  :root { color-scheme: light dark; }
  body { font: 15px/1.5 -apple-system, system-ui, sans-serif; max-width: 860px; margin: 2rem auto; padding: 0 1rem; }
  header { border-bottom: 1px solid #8883; padding-bottom: 1rem; margin-bottom: 1rem; }
  h1 { font-size: 1.4rem; margin: 0 0 .25rem; }
  .meta { color: #888; font-size: .85rem; }
  .meta span { margin-right: 1rem; }
  .banner { background: #fce8e8; color: #7a1f1f; padding: .5rem .75rem; border-radius: 4px; font-size: .9rem; margin-bottom: 1rem; }
  @media (prefers-color-scheme: dark) { .banner { background: #4a1e1e; color: #f4b4b4; } }
  .msg { border-left: 3px solid #8884; padding: .25rem .75rem; margin: 1rem 0; }
  .msg.user { border-color: #4a8; }
  .msg.assistant { border-color: #68a; }
  .msg .role { font-weight: 600; font-size: .8rem; text-transform: uppercase; letter-spacing: .05em; color: #888; margin-bottom: .25rem; }
  .msg pre { white-space: pre-wrap; word-wrap: break-word; margin: 0; font: inherit; }
  footer { margin-top: 2rem; border-top: 1px solid #8883; padding-top: 1rem; font-size: .85rem; color: #888; }
  a { color: inherit; }
</style>
</head>
<body>
<header>
  <h1>{{if .Share.Name}}{{.Share.Name}}{{else}}{{.Share.Session.FirstPrompt}}{{end}}</h1>
  <div class="meta">
    <span>{{.Share.Session.Source}}{{if .Share.Session.Model}} · {{.Share.Session.Model}}{{end}}</span>
    <span>{{.Share.Session.Project}}</span>
    {{if .Share.Session.GitBranch}}<span>{{.Share.Session.GitBranch}}</span>{{end}}
    <span>{{.Share.Session.MessageCount}} messages</span>
    <span>started {{fmtTime .Share.Session.StartedAt}}</span>
  </div>
</header>
<div class="banner">This link expires at <strong>{{fmtTime .Expires}}</strong> ({{.Remaining}} remaining). The transcript was scrubbed before upload, but review carefully before forwarding further.</div>
{{range .Share.Messages}}
  <div class="msg {{.Role}}">
    <div class="role">{{.Role}}{{if .ToolName}} · {{.ToolName}}{{end}}{{if .FilePath}} · {{.FilePath}}{{end}}</div>
    <pre>{{.Content}}</pre>
  </div>
{{end}}
<footer>
  Share id <code>{{.Share.ID}}</code> · <a href="{{.RawURL}}">download raw (gzip)</a>
</footer>
</body>
</html>
`
