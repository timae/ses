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
	tmpl := template.New("").Funcs(tmplFuncs)
	if _, err := tmpl.New("snapshot").Parse(snapshotPageHTML); err != nil {
		return nil, fmt.Errorf("parsing snapshot template: %w", err)
	}
	if _, err := tmpl.New("handoff").Parse(handoffPageHTML); err != nil {
		return nil, fmt.Errorf("parsing handoff template: %w", err)
	}
	return &Server{cfg: cfg, store: store, tmpl: tmpl}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/shares", s.handleShares)     // POST
	mux.HandleFunc("/v1/shares/", s.handleShareByID) // DELETE /v1/shares/{id}, POST /v1/shares/{id}/consume
	mux.HandleFunc("/s/", s.handleView)              // GET /s/{id}[/raw.json.gz]
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// --- write: POST /v1/shares ----------------------------------------------

type uploadRequest struct {
	Name            string    `json:"name,omitempty"`
	Kind            ShareKind `json:"kind,omitempty"`
	ExpiresInSecond int64     `json:"expires_in_seconds"`
	SingleUse       bool      `json:"single_use,omitempty"`

	// Handoff-only
	HandoffNote    string          `json:"handoff_note,omitempty"`
	LinkedSessions []LinkedSession `json:"linked_sessions,omitempty"`
	Files          []ShareFile     `json:"files,omitempty"`

	Session  ShareSession `json:"session"`
	Messages []ShareMsg   `json:"messages"`
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

	kind := req.Kind
	if kind == "" {
		kind = KindSnapshot
	}
	if kind != KindSnapshot && kind != KindHandoff {
		s.replyError(w, http.StatusBadRequest, "invalid kind %q", kind)
		return
	}
	share := Share{
		ID:             id,
		Name:           req.Name,
		Kind:           kind,
		CreatedAt:      now,
		ExpiresAt:      expiresAt,
		SingleUse:      req.SingleUse,
		HandoffNote:    req.HandoffNote,
		LinkedSessions: req.LinkedSessions,
		Files:          req.Files,
		Session:        req.Session,
		Messages:       req.Messages,
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
	rest := strings.TrimPrefix(r.URL.Path, "/v1/shares/")
	// /v1/shares/{id}/consume → routed to handleConsume; anything else is a
	// direct DELETE on the id.
	if id, tail, ok := splitOnce(rest, "/"); ok {
		if tail == "consume" {
			s.handleConsume(w, r, id)
			return
		}
		http.NotFound(w, r)
		return
	}
	id := rest
	if id == "" {
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

// handleConsume is the endpoint a handoff recipient's CLI calls. It returns
// the full Share as decoded JSON and, for single-use shares, deletes the
// stored object. Consume is unauthenticated: the share URL itself is the
// capability, and handoff URLs are single-use by design.
//
// Race note: under concurrent calls, two clients can both receive the
// payload before Delete lands. That's acceptable — a handoff URL in
// practice goes to one person, and a forked claim isn't worse than the
// sender continuing on their own machine.
func (s *Server) handleConsume(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gz, _, err := s.store.Get(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, ErrExpired):
		_ = s.store.Delete(context.Background(), id)
		http.Error(w, "this share has expired", http.StatusGone)
		return
	case err != nil:
		s.cfg.Logger.Printf("consume get %s: %v", id, err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	share, err := decodeShare(gz)
	if err != nil {
		s.cfg.Logger.Printf("consume decode %s: %v", id, err)
		http.Error(w, "malformed share", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(share)
	if share.SingleUse {
		if err := s.store.Delete(context.Background(), id); err != nil {
			s.cfg.Logger.Printf("consume delete %s: %v", id, err)
		}
	}
}

// splitOnce returns (head, tail, true) if s contains sep, otherwise ("", "", false).
func splitOnce(s, sep string) (string, string, bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+len(sep):], true
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

	// Handoff shares never leak their transcript via the HTML or raw paths.
	// The only way to get the body is POST /v1/shares/{id}/consume from the
	// CLI. That's what makes them safe to share without a bearer token.
	share, decodeErr := decodeShare(gz)
	if decodeErr != nil {
		s.cfg.Logger.Printf("decoding share %s: %v", id, decodeErr)
		http.Error(w, "malformed share", http.StatusInternalServerError)
		return
	}
	isHandoff := share.Kind == KindHandoff

	switch tail {
	case "":
		if isHandoff {
			s.renderHandoffClaim(w, share, exp)
			return
		}
		s.renderHTML(w, share, exp)
	case "raw.json.gz":
		if isHandoff {
			http.Error(w, "handoff shares can only be claimed via `ses resume --from <url>`", http.StatusForbidden)
			return
		}
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

func (s *Server) renderHTML(w http.ResponseWriter, share Share, exp time.Time) {
	remaining := time.Until(exp).Round(time.Minute)
	data := map[string]any{
		"Share":     share,
		"Expires":   exp,
		"Remaining": remaining.String(),
		"RawURL":    path.Join("/s", share.ID, "raw.json.gz"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.ExecuteTemplate(w, "snapshot", data); err != nil {
		s.cfg.Logger.Printf("template exec: %v", err)
	}
}

func (s *Server) renderHandoffClaim(w http.ResponseWriter, share Share, exp time.Time) {
	remaining := time.Until(exp).Round(time.Minute)
	claimURL := fmt.Sprintf("%s/s/%s", s.cfg.PublicURL, share.ID)
	data := map[string]any{
		"Share":     share,
		"Expires":   exp,
		"Remaining": remaining.String(),
		"ClaimURL":  claimURL,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.ExecuteTemplate(w, "handoff", data); err != nil {
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

const snapshotPageHTML = `<!doctype html>
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

const handoffPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Handoff · {{if .Share.Name}}{{.Share.Name}}{{else}}{{.Share.Session.ShortID}}{{end}}</title>
<meta name="robots" content="noindex,nofollow">
<style>
  :root { color-scheme: light dark; }
  body { font: 15px/1.5 -apple-system, system-ui, sans-serif; max-width: 760px; margin: 2rem auto; padding: 0 1rem; }
  header { border-bottom: 1px solid #8883; padding-bottom: 1rem; margin-bottom: 1.5rem; }
  h1 { font-size: 1.4rem; margin: 0 0 .25rem; }
  .meta { color: #888; font-size: .85rem; }
  .meta span { margin-right: 1rem; }
  .note { background: #fffbe6; color: #5a4400; padding: 1rem; border-radius: 6px; border-left: 3px solid #e1b945; margin: 1rem 0; white-space: pre-wrap; }
  @media (prefers-color-scheme: dark) { .note { background: #3a2f0f; color: #f0d78f; border-color: #c2a13a; } }
  pre.cmd { background: #f5f5f7; padding: .75rem 1rem; border-radius: 4px; overflow-x: auto; font: 13px/1.4 ui-monospace, "SF Mono", monospace; }
  @media (prefers-color-scheme: dark) { pre.cmd { background: #1f1f21; } }
  .files { font-size: .9rem; }
  .files li { font-family: ui-monospace, "SF Mono", monospace; }
  .action { display: inline-block; font-size: .7rem; text-transform: uppercase; color: #888; margin-right: .5em; min-width: 3em; }
  .expiry { color: #888; font-size: .85rem; margin-top: 2rem; }
</style>
</head>
<body>
<header>
  <h1>Session handoff</h1>
  <div class="meta">
    <span>{{.Share.Session.Source}}{{if .Share.Session.Model}} · {{.Share.Session.Model}}{{end}}</span>
    <span>{{.Share.Session.Project}}</span>
    {{if .Share.Session.GitBranch}}<span>{{.Share.Session.GitBranch}}</span>{{end}}
    <span>{{.Share.Session.MessageCount}} messages</span>
  </div>
</header>

{{if .Share.HandoffNote}}
<div class="note">{{.Share.HandoffNote}}</div>
{{end}}

<p>To continue this session on your machine:</p>
<pre class="cmd">cd /path/to/your/checkout
ses resume --from {{.ClaimURL}}</pre>

<p>The CLI will download the full context and launch Claude Code with the transcript, linked sessions, and this note pre-loaded. <strong>This link is single-use</strong> — it is deleted after the first successful <code>ses resume --from</code>.</p>

{{if .Share.Files}}
<h2 style="font-size: 1rem; margin-top: 2rem;">Files touched in the original session</h2>
<ul class="files">
{{range .Share.Files}}  <li><span class="action">{{.Action}}</span>{{.Path}}</li>
{{end}}</ul>
{{end}}

{{if .Share.LinkedSessions}}
<h2 style="font-size: 1rem; margin-top: 2rem;">Linked prior sessions ({{len .Share.LinkedSessions}})</h2>
<p style="color: #888; font-size: .85rem;">The full briefs are included in the context blob when you claim the handoff.</p>
<ul>
{{range .Share.LinkedSessions}}  <li><code>{{.ShortID}}</code>{{if .FirstPrompt}} — {{.FirstPrompt}}{{end}}{{if .Reason}} <em>({{.Reason}})</em>{{end}}</li>
{{end}}</ul>
{{end}}

<p class="expiry">Expires {{fmtTime .Expires}} · {{.Remaining}} remaining · id <code>{{.Share.ID}}</code></p>
</body>
</html>
`
