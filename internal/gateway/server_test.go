package gateway

import (
	"context"
	"errors"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/isBioku/kubetre/internal/access"
)

// fakeLogin stands in for the OIDC provider: the "code" is the user's email.
type fakeLogin struct{ lastNonce, lastVerifier string }

func (f *fakeLogin) AuthCodeURL(state, nonce, verifier string) string {
	f.lastNonce, f.lastVerifier = nonce, verifier
	return "https://idp.example.com/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeLogin) Exchange(_ context.Context, code, verifier, nonce string) (access.Identity, error) {
	if verifier != f.lastVerifier || nonce != f.lastNonce {
		return access.Identity{}, errors.New("pkce or nonce mismatch")
	}
	for _, id := range []access.Identity{rita, ravi, olive, eve} {
		if strings.EqualFold(id.Email, code) {
			return id, nil
		}
	}
	return access.Identity{}, errors.New("unknown user")
}

type harness struct {
	t      *testing.T
	srv    *httptest.Server
	key    GuacKey
	client *http.Client
	login  *fakeLogin
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	key, _ := ParseGuacKey(testKeyHex)
	sealer, _ := NewSealer([]byte("0123456789abcdef0123456789abcdef"))
	h := &harness{t: t, key: key, login: &fakeLogin{}}
	gw := &Server{
		Resolver: fixture(t), Login: h.login, Sealer: sealer, GuacKey: key, GuacamolePath: "/guacamole/",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	h.srv = httptest.NewTLSServer(gw.Handler())
	t.Cleanup(h.srv.Close)
	gw.ExternalURL, _ = url.Parse(h.srv.URL)
	jar, _ := cookiejar.New(nil)
	h.client = h.srv.Client()
	h.client.Jar = jar
	h.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return h
}

func (h *harness) get(path string) *http.Response {
	resp, err := h.client.Get(h.srv.URL + path)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

func (h *harness) post(path, origin string) *http.Response {
	req, _ := http.NewRequest("POST", h.srv.URL+path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

// signIn runs /login then /callback as the given user.
func (h *harness) signIn(email string) {
	h.t.Helper()
	resp := h.get("/login")
	loc, _ := url.Parse(resp.Header.Get("Location"))
	state := loc.Query().Get("state")
	resp = h.get("/callback?state=" + url.QueryEscape(state) + "&code=" + url.QueryEscape(email))
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		h.t.Fatalf("callback: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestHomeRequiresSignIn(t *testing.T) {
	h := newHarness(t)
	if resp := h.get("/"); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Fatalf("anonymous home: %d", resp.StatusCode)
	}
}

func TestCallbackRejectsWrongState(t *testing.T) {
	h := newHarness(t)
	h.get("/login")
	if resp := h.get("/callback?state=forged&code=rita@example.com"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("forged state: %d", resp.StatusCode)
	}
	if resp := h.get("/"); resp.StatusCode != http.StatusFound {
		t.Fatal("a forged callback must not create a session")
	}
}

func TestHomeListsOnlyTheUsersSessions(t *testing.T) {
	h := newHarness(t)
	h.signIn("rita@example.com")
	resp := h.get("/")
	body, _ := io.ReadAll(resp.Body)
	page := string(body)
	if resp.StatusCode != 200 || !strings.Contains(page, "rita-vm display") || strings.Contains(page, "ravi-vm") {
		t.Fatalf("home page: %d\n%s", resp.StatusCode, page)
	}
	if resp.Header.Get("Content-Security-Policy") == "" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("missing clickjacking protection")
	}
}

func TestConnectHandsGuacamoleOneHardenedSession(t *testing.T) {
	h := newHarness(t)
	h.signIn("rita@example.com")
	resp := h.post("/connect/study/rita-vm-credentials", h.srv.URL)
	loc := handoffTarget(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(loc, "/guacamole/#/?data=") {
		t.Fatalf("connect: %d %s", resp.StatusCode, loc)
	}
	data, _ := url.QueryUnescape(strings.TrimPrefix(loc, "/guacamole/#/?data="))
	p := openLikeGuacamole(t, h.key, data)
	if len(p.Connections) != 1 || !p.SingleUse || p.Username != "rita@example.com" {
		t.Fatalf("payload = %+v", p)
	}
	if ttl := time.Until(time.UnixMilli(p.Expires)); ttl <= 0 || ttl > HandoffTTL {
		t.Fatalf("payload expires in %v", ttl)
	}
	c := p.Connections["study/rita-vm-credentials"]
	if c.Protocol != "rdp" || c.Parameters["hostname"] != "10.240.0.70" || c.Parameters["disable-copy"] != "true" || c.Parameters["enable-drive"] != "false" {
		t.Fatalf("connection = %+v", c)
	}
}

func TestConnectRefusals(t *testing.T) {
	h := newHarness(t)
	h.signIn("rita@example.com")
	for name, tc := range map[string]struct {
		path, origin string
		want         int
	}{
		"someone else's VM":      {"/connect/study/ravi-vm-credentials", h.srv.URL, http.StatusNotFound},
		"other workspace":        {"/connect/other/ravi-other-credentials", h.srv.URL, http.StatusNotFound},
		"target outside":         {"/connect/study/rita-evil-vnc", h.srv.URL, http.StatusConflict},
		"cross-site post":        {"/connect/study/rita-vm-credentials", "https://evil.example.com", http.StatusForbidden},
		"post without an origin": {"/connect/study/rita-vm-credentials", "", http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			if resp := h.post(tc.path, tc.origin); resp.StatusCode != tc.want {
				t.Fatalf("got %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestTamperedSessionCookieIsRejected(t *testing.T) {
	h := newHarness(t)
	h.signIn("rita@example.com")
	u, _ := url.Parse(h.srv.URL)
	for _, c := range h.client.Jar.Cookies(u) {
		if c.Name == sessionCookie {
			c.Value = c.Value[:len(c.Value)-2] + "AA"
			h.client.Jar.SetCookies(u, []*http.Cookie{c})
		}
	}
	if resp := h.get("/"); resp.StatusCode != http.StatusFound {
		t.Fatalf("tampered cookie accepted: %d", resp.StatusCode)
	}
}

// newHarnessAt serves the gateway under a base path, as it runs behind AzureTRE's UI.
func newHarnessAt(t *testing.T, base string) *harness {
	t.Helper()
	key, _ := ParseGuacKey(testKeyHex)
	sealer, _ := NewSealer([]byte("0123456789abcdef0123456789abcdef"))
	h := &harness{t: t, key: key, login: &fakeLogin{}}
	gw := &Server{
		Resolver: fixture(t), Login: h.login, Sealer: sealer, GuacKey: key, GuacamolePath: "/guacamole/",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	var handler http.Handler
	h.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(h.srv.Close)
	gw.ExternalURL, _ = url.Parse(h.srv.URL + base)
	handler = gw.Handler()
	jar, _ := cookiejar.New(nil)
	h.client = h.srv.Client()
	h.client.Jar = jar
	h.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return h
}

func (h *harness) getFrom(path, fetchSite string) *http.Response {
	req, _ := http.NewRequest("GET", h.srv.URL+path, nil)
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

// The UI's Connect button opens /gateway/connect/<workspace>/<user resource> in a new tab.
// A user who is not yet signed in to the gateway goes through sign-in and lands back on
// the connection.
func TestConnectLinkFromTheUISignsInAndReturns(t *testing.T) {
	h := newHarnessAt(t, "/gateway")
	resp := h.getFrom("/gateway/connect/study/rita-vm", "same-origin")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/gateway/login?next=%2Fgateway%2Fconnect%2Fstudy%2Frita-vm" {
		t.Fatalf("anonymous connect: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp = h.get(resp.Header.Get("Location"))
	loc, _ := url.Parse(resp.Header.Get("Location"))
	resp = h.get("/gateway/callback?state=" + url.QueryEscape(loc.Query().Get("state")) + "&code=rita@example.com")
	if resp.Header.Get("Location") != "/gateway/connect/study/rita-vm" {
		t.Fatalf("callback should return to the connection, got %s", resp.Header.Get("Location"))
	}
	for _, c := range resp.Cookies() {
		if c.Path != "/gateway/" {
			t.Fatalf("cookie %s has path %q", c.Name, c.Path)
		}
	}
	resp = h.getFrom("/gateway/connect/study/rita-vm", "same-origin")
	if target := handoffTarget(t, resp); resp.StatusCode != http.StatusOK || !strings.HasPrefix(target, "/guacamole/#/?data=") {
		t.Fatalf("connect by user resource name: %d %s", resp.StatusCode, target)
	}
	if resp := h.getFrom("/gateway/connect/study/rita-vm", "none"); resp.StatusCode != http.StatusOK {
		t.Fatalf("typed URL: %d", resp.StatusCode)
	}
	for name, tc := range map[string]struct {
		path, site string
		want       int
	}{
		"cross-site link":     {"/gateway/connect/study/rita-vm", "cross-site", http.StatusForbidden},
		"same-site link":      {"/gateway/connect/study/rita-vm", "same-site", http.StatusForbidden},
		"no fetch metadata":   {"/gateway/connect/study/rita-vm", "", http.StatusForbidden},
		"someone else's VM":   {"/gateway/connect/study/ravi-vm", "same-origin", http.StatusNotFound},
		"outside the gateway": {"/connect/study/rita-vm", "same-origin", http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			if resp := h.getFrom(tc.path, tc.site); resp.StatusCode != tc.want {
				t.Fatalf("got %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestLoginIgnoresReturnPathsOutsideConnect(t *testing.T) {
	h := newHarnessAt(t, "/gateway")
	for _, next := range []string{"https://evil.example.com/", "//evil.example.com", "/gateway/../admin", "/gateway/connect/a/b/../../x", "/other/connect/a/b"} {
		resp := h.get("/gateway/login?next=" + url.QueryEscape(next))
		loc, _ := url.Parse(resp.Header.Get("Location"))
		resp = h.get("/gateway/callback?state=" + url.QueryEscape(loc.Query().Get("state")) + "&code=rita@example.com")
		if got := resp.Header.Get("Location"); got != "/gateway/" {
			t.Fatalf("next=%q redirected to %q", next, got)
		}
	}
}

var handoffAttr = regexp.MustCompile(`data-target="([^"]+)"`)

// handoffTarget reads the Guacamole URL from the broker's hand-off page.
func handoffTarget(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	m := handoffAttr.FindStringSubmatch(string(body))
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

// A browser that still holds a Guacamole session would make Guacamole ignore the new
// payload, so the hand-off page ends that session first, using only same-origin script.
func TestHandoffEndsTheOldGuacamoleSession(t *testing.T) {
	h := newHarnessAt(t, "/gateway")
	h.signIn2("/gateway", "rita@example.com")
	resp := h.getFrom("/gateway/connect/study/rita-vm", "same-origin")
	body, _ := io.ReadAll(resp.Body)
	page := string(body)
	if !strings.Contains(page, `<script src="/gateway/handoff.js" defer></script>`) ||
		!strings.Contains(page, `data-session="/guacamole/api/session"`) || strings.Contains(page, "<script>") {
		t.Fatalf("hand-off page:\n%s", page)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("CSP = %q", csp)
	}
	js := h.getFrom("/gateway/handoff.js", "same-origin")
	src, _ := io.ReadAll(js.Body)
	for _, want := range []string{`removeItem("GUAC_AUTH_TOKEN")`, `method: "DELETE"`, `"Guacamole-Token": token`, "location.replace(target)"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("handoff.js is missing %s", want)
		}
	}
	if ct := js.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("handoff.js content type = %q", ct)
	}
}

// signIn2 signs in through a gateway served under a base path.
func (h *harness) signIn2(base, email string) {
	h.t.Helper()
	resp := h.get(base + "/login")
	loc, _ := url.Parse(resp.Header.Get("Location"))
	h.get(base + "/callback?state=" + url.QueryEscape(loc.Query().Get("state")) + "&code=" + url.QueryEscape(email))
}
