package gateway

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/isBioku/kubetre/internal/access"
)

const (
	sessionCookie = "kubetre_gateway"
	loginCookie   = "kubetre_gateway_login"
	sessionTTL    = 8 * time.Hour
	loginTTL      = 10 * time.Minute
	// HandoffTTL is how long the encrypted Guacamole payload is valid. It is also single-use.
	HandoffTTL = 60 * time.Second
)

// Server is the gateway broker's HTTP front end.
type Server struct {
	Resolver      *Resolver
	Login         Login
	Sealer        *Sealer
	GuacKey       GuacKey
	ExternalURL   *url.URL // e.g. https://tre.example.org/gateway; its path is the base path
	GuacamolePath string   // e.g. /guacamole/
	Log           *slog.Logger
	Now           func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler returns the routes, served under the external URL's path.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /login", s.login)
	mux.HandleFunc("GET /callback", s.callback)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("POST /connect/{workspace}/{name}", s.connect)
	mux.HandleFunc("GET /connect/{workspace}/{name}", s.connectLink)
	mux.HandleFunc("GET /handoff.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = io.WriteString(w, handoffScript)
	})
	base := s.basePath()
	if base == "" {
		return securityHeaders(mux)
	}
	outer := http.NewServeMux()
	outer.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	outer.Handle(base+"/", http.StripPrefix(base, mux))
	outer.Handle("GET "+base, http.RedirectHandler(base+"/", http.StatusFound))
	return securityHeaders(outer)
}

// basePath is the path the gateway is served under, for example /gateway, or "".
func (s *Server) basePath() string {
	if s.ExternalURL == nil {
		return ""
	}
	return strings.TrimSuffix(s.ExternalURL.Path, "/")
}

// path turns a route into the browser-visible path.
func (s *Server) path(p string) string { return s.basePath() + p }

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: s.path("/"), HttpOnly: true, Secure: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds()),
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: s.path("/"), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (s *Server) session(r *http.Request) (access.Identity, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return access.Identity{}, false
	}
	var sess Session
	if err := s.Sealer.Open(sessionCookie, c.Value, &sess); err != nil || s.now().After(sess.Expires) {
		return access.Identity{}, false
	}
	return access.Identity{Subject: sess.Subject, Email: sess.Email, Name: sess.Name}, true
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	st := LoginState{State: randomString(24), Nonce: randomString(24), Verifier: randomString(48), Expires: s.now().Add(loginTTL)}
	if next := r.URL.Query().Get("next"); s.isConnectPath(next) {
		st.Next = next
	}
	v, err := s.Sealer.Seal(loginCookie, st)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.setCookie(w, loginCookie, v, loginTTL)
	http.Redirect(w, r, s.Login.AuthCodeURL(st.State, st.Nonce, st.Verifier), http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(loginCookie)
	var st LoginState
	if err != nil || s.Sealer.Open(loginCookie, c.Value, &st) != nil || s.now().After(st.Expires) {
		http.Error(w, "sign-in expired; start again", http.StatusBadRequest)
		return
	}
	s.clearCookie(w, loginCookie)
	if r.URL.Query().Get("state") != st.State || st.State == "" {
		http.Error(w, "sign-in state mismatch", http.StatusBadRequest)
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		http.Error(w, "sign-in failed", http.StatusUnauthorized)
		return
	}
	id, err := s.Login.Exchange(r.Context(), r.URL.Query().Get("code"), st.Verifier, st.Nonce)
	if err != nil {
		s.Log.Warn("sign-in failed", "error", err)
		http.Error(w, "sign-in failed", http.StatusUnauthorized)
		return
	}
	v, err := s.Sealer.Seal(sessionCookie, Session{Subject: id.Subject, Email: id.Email, Name: id.Name, Expires: s.now().Add(sessionTTL)})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.setCookie(w, sessionCookie, v, sessionTTL)
	s.Log.Info("signed in", "user", id.Principal())
	if s.isConnectPath(st.Next) {
		http.Redirect(w, r, st.Next, http.StatusFound)
		return
	}
	http.Redirect(w, r, s.path("/"), http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.clearCookie(w, sessionCookie)
	http.Redirect(w, r, s.path("/"), http.StatusSeeOther)
}

// sameOrigin rejects cross-site form posts. Browsers send Origin on POST.
func (s *Server) sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin != "" && strings.EqualFold(origin, s.ExternalURL.Scheme+"://"+s.ExternalURL.Host)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	id, ok := s.session(r)
	if !ok {
		http.Redirect(w, r, s.path("/login"), http.StatusFound)
		return
	}
	conns, err := s.Resolver.Connections(r.Context(), id)
	if err != nil {
		s.Log.Error("listing connections failed", "user", id.Principal(), "error", err)
		http.Error(w, "could not list your connections", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = homePage.Execute(w, map[string]any{"User": id, "Connections": conns, "Base": s.basePath()})
}

func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	id, ok := s.session(r)
	if !ok {
		http.Redirect(w, r, s.path("/login"), http.StatusSeeOther)
		return
	}
	s.open(w, r, id)
}

// connectLink serves the connection links AzureTRE's UI opens in a new tab ("Connect").
// Fetch Metadata allows only navigations from this site or typed by the user, so another
// site cannot open sessions in a signed-in user's browser.
func (s *Server) connectLink(w http.ResponseWriter, r *http.Request) {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
	default:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	id, ok := s.session(r)
	if !ok {
		http.Redirect(w, r, s.path("/login")+"?next="+url.QueryEscape(s.path(r.URL.Path)), http.StatusFound)
		return
	}
	s.open(w, r, id)
}

var connectPath = regexp.MustCompile(`^/connect/[a-z0-9]([a-z0-9-]*[a-z0-9])?/[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

func (s *Server) isConnectPath(p string) bool {
	rest, ok := strings.CutPrefix(p, s.basePath())
	return ok && p != "" && connectPath.MatchString(rest)
}

// open hands the user's connection to Guacamole. {name} is the connection Secret or, from
// AzureTRE's UI, the service (user resource) that owns it.
func (s *Server) open(w http.ResponseWriter, r *http.Request, id access.Identity) {
	// Authorization is re-evaluated on every connect, from current workspace membership
	// and service ownership.
	conns, err := s.Resolver.Connections(r.Context(), id)
	if err != nil {
		http.Error(w, "could not list your connections", http.StatusInternalServerError)
		return
	}
	ws, name := r.PathValue("workspace"), r.PathValue("name")
	var match *Connection
	for i := range conns {
		c := &conns[i]
		if c.Workspace != ws || (c.Name != name && c.Service != name) {
			continue
		}
		if match == nil || (c.Ready && !match.Ready) {
			match = c
		}
	}
	if match == nil {
		http.Error(w, "connection not found", http.StatusNotFound)
		return
	}
	c := *match
	if !c.Ready {
		http.Error(w, "not ready: "+c.Reason, http.StatusConflict)
		return
	}
	payload := NewGuacPayload(id.Principal(), map[string]GuacConnection{
		c.ID(): {Protocol: c.Protocol, Parameters: HardenedParameters(c)},
	}, s.now(), HandoffTTL)
	data, err := s.GuacKey.Seal(payload)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.Log.Info("session opened", "user", id.Principal(), "workspace", c.Workspace, "service", c.Service,
		"connection", c.Name, "protocol", c.Protocol, "target", c.Hostname)
	// The payload travels in the URL fragment, which browsers never send to a server,
	// so it stays out of proxy and access logs. The hand-off page first ends any Guacamole
	// session the browser still holds: Guacamole would otherwise re-use that session and
	// ignore the new payload, because the JSON extension does not update existing sessions.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = handoffPage.Execute(w, map[string]string{
		"Target":  s.GuacamolePath + "#/?data=" + url.QueryEscape(data),
		"Session": s.GuacamolePath + "api/session",
		"Script":  s.path("/handoff.js"),
	})
}

var handoffPage = template.Must(template.New("handoff").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Connecting</title><script src="{{.Script}}" defer></script></head>
<body><p id="handoff" data-target="{{.Target}}" data-session="{{.Session}}">Connecting&hellip;</p>
<noscript><a href="{{.Target}}">Continue to the session</a></noscript></body></html>`))

// handoffScript clears the browser's stored Guacamole token, revokes that session, then
// opens Guacamole with the new payload. It is a separate file so the CSP needs no inline script.
const handoffScript = `(function () {
  var el = document.getElementById("handoff");
  var target = el.getAttribute("data-target");
  var go = function () { window.location.replace(target); };
  var token = null;
  try {
    var raw = window.localStorage.getItem("GUAC_AUTH_TOKEN");
    if (raw) { try { token = JSON.parse(raw); } catch (e) { token = raw; } }
    window.localStorage.removeItem("GUAC_AUTH_TOKEN");
  } catch (e) {}
  if (!token) { go(); return; }
  fetch(el.getAttribute("data-session"), {
    method: "DELETE", headers: {"Guacamole-Token": token}, credentials: "same-origin", keepalive: true
  }).then(go, go);
})();
`

var homePage = template.Must(template.New("home").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>KubeTRE access gateway</title>
<style>
body{font-family:system-ui,sans-serif;margin:0;background:#f6f7f9;color:#1d2433}
main{max-width:760px;margin:0 auto;padding:24px 16px}
h1{font-size:1.4rem}.card{background:#fff;border:1px solid #dde1e8;border-radius:8px;padding:14px 16px;margin:10px 0;display:flex;justify-content:space-between;align-items:center;gap:12px}
.meta{color:#5b6475;font-size:.9rem}button{font:inherit;padding:8px 14px;border-radius:6px;border:1px solid #2456c9;background:#2f63d8;color:#fff;cursor:pointer}
button[disabled]{background:#c8ced9;border-color:#c8ced9;cursor:not-allowed}.top{display:flex;justify-content:space-between;align-items:center}
.link{background:none;border:none;color:#2f63d8;padding:0}
</style></head><body><main>
<div class="top"><h1>Your workspace sessions</h1>
<form method="post" action="{{.Base}}/logout"><button class="link" type="submit">Sign out {{.User.Principal}}</button></form></div>
<p class="meta">Clipboard, file transfer and drive mapping are disabled. Use the airlock to move data.</p>
{{range .Connections}}
<div class="card"><div><strong>{{.DisplayName}}</strong>
<div class="meta">{{.Workspace}} · {{.Protocol}}{{if not .Ready}} · {{.Reason}}{{end}}</div></div>
<form method="post" action="{{$.Base}}/connect/{{.Workspace}}/{{.Name}}"><button type="submit"{{if not .Ready}} disabled{{end}}>Connect</button></form></div>
{{else}}<p>You have no desktops or virtual machines yet. Create one from a workspace in the KubeTRE portal.</p>{{end}}
</main></body></html>`))
