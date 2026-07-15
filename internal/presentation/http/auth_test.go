package route

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// templateGlob resolves the templates glob relative to this test file so the
// login/restricted handlers can render regardless of the test binary's CWD
// (the package imports usecase/agent, whose init() writes skills/COMMAND_LISTS.md
// to CWD, so this test must be run from the repo root).
func templateGlob(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file) // .../internal/presentation/http
	return filepath.Join(dir, "..", "templates", "*.gohtml")
}

// withSecret sets ADMIN_SECRET_KEY for the duration of a test and forces the
// signing key to be derived from it. t.Setenv restores the previous value on
// exit and disables parallelism for this test, avoiding env races.
func withSecret(t *testing.T, secret string) {
	t.Helper()
	t.Setenv("ADMIN_SECRET_KEY", secret)
}

func TestAdminSecretKey_TrimmedAndEmpty(t *testing.T) {
	withSecret(t, "  spaced-secret  ")
	if got := adminSecretKey(); got != "spaced-secret" {
		t.Fatalf("adminSecretKey() = %q, want trimmed %q", got, "spaced-secret")
	}

	t.Setenv("ADMIN_SECRET_KEY", "   ")
	if adminSecretKey() != "" {
		t.Fatalf("adminSecretKey() should be empty for whitespace-only value")
	}
}

func TestSignVerifyCertificate_RoundTrip(t *testing.T) {
	withSecret(t, "strong-secret")

	expiry := time.Now().Add(adminSessionTTL).Unix()
	cert := signSessionCertificate(expiry)
	if cert == "" {
		t.Fatal("signSessionCertificate returned empty")
	}
	if !verifySessionCertificate(cert) {
		t.Fatalf("verifySessionCertificate(%q) = false, want true", cert)
	}
}

func TestVerifyCertificate_RejectsTamperAndJunk(t *testing.T) {
	withSecret(t, "strong-secret")

	expiry := time.Now().Add(adminSessionTTL).Unix()
	valid := signSessionCertificate(expiry)

	cases := map[string]string{
		"empty":         "",
		"no separator":  "garbage",
		"one part":      "abc",
		"bad base64":    "!!!.!!!",
		"tampered sig":  valid[:strings.LastIndex(valid, ".")] + ".AAAA",
		"tampered body": "MTIzNDU2Nzg5MA." + valid[strings.Index(valid, ".")+1:],
		"extra parts":   valid + ".extra",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if verifySessionCertificate(token) {
				t.Fatalf("verifySessionCertificate(%q) = true, want false", token)
			}
		})
	}
}

func TestVerifyCertificate_RejectsExpired(t *testing.T) {
	withSecret(t, "strong-secret")

	past := time.Now().Add(-time.Hour).Unix()
	cert := signSessionCertificate(past)
	if verifySessionCertificate(cert) {
		t.Fatalf("verifySessionCertificate accepted an expired certificate")
	}
}

func TestSigningKey_RotatesWithSecret(t *testing.T) {
	withSecret(t, "secret-A")
	keyA := adminSigningKey()
	expiry := time.Now().Add(adminSessionTTL).Unix()
	certA := signSessionCertificate(expiry)

	t.Setenv("ADMIN_SECRET_KEY", "secret-B")
	keyB := adminSigningKey()

	if string(keyA) == string(keyB) {
		t.Fatal("signing key should change when ADMIN_SECRET_KEY changes")
	}
	// Certificate minted under the old secret must no longer verify.
	if verifySessionCertificate(certA) {
		t.Fatal("rotating ADMIN_SECRET_KEY must invalidate existing sessions")
	}
}

func newAuthTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequireAdminAuth())
	// Templates live one directory up from this package; load them so the
	// login/restricted handlers can render during integration tests.
	r.LoadHTMLGlob(templateGlob(t))
	AuthRouter(r)
	r.GET("/api/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func perform(r *gin.Engine, method, target, accept string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func validCookie(t *testing.T) *http.Cookie {
	t.Helper()
	expiry := time.Now().Add(adminSessionTTL).Unix()
	return &http.Cookie{Name: adminSessionCookieName, Value: signSessionCertificate(expiry)}
}

func TestRequireAdminAuth_BrowserRedirectsToLogin(t *testing.T) {
	withSecret(t, "strong-secret")
	r := newAuthTestEngine(t)

	w := perform(r, http.MethodGet, "/", "text/html", nil)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to /login", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

func TestRequireAdminAuth_APIReturnsUnauthorized(t *testing.T) {
	withSecret(t, "strong-secret")
	r := newAuthTestEngine(t)

	w := perform(r, http.MethodGet, "/api/ping", "application/json", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "authentication required") {
		t.Fatalf("body = %q, want authentication required message", w.Body.String())
	}
}

func TestRequireAdminAuth_AllowsWithValidCookie(t *testing.T) {
	withSecret(t, "strong-secret")
	r := newAuthTestEngine(t)

	w := perform(r, http.MethodGet, "/api/ping", "application/json", validCookie(t))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with valid session", w.Code)
	}
}

func TestRequireAdminAuth_PublicLoginPathReachable(t *testing.T) {
	withSecret(t, "strong-secret")
	r := newAuthTestEngine(t)

	w := perform(r, http.MethodGet, "/login", "text/html", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 on /login without session", w.Code)
	}
}

func TestRequireAdminAuth_EmptySecretRestrictsEverything(t *testing.T) {
	withSecret(t, "")
	r := newAuthTestEngine(t)

	// API -> 403 JSON.
	w := perform(r, http.MethodGet, "/api/ping", "application/json", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 when secret is empty", w.Code)
	}

	// Browser -> restricted page.
	w = perform(r, http.MethodGet, "/", "text/html", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 restricted page when secret is empty", w.Code)
	}
}

func TestLoginSubmit_CorrectKeyMintsCookie(t *testing.T) {
	withSecret(t, "the-real-key")
	r := newAuthTestEngine(t)

	form := strings.NewReader("admin_secret_key=the-real-key")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect home", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location = %q, want /", loc)
	}
	setCookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, adminSessionCookieName+"=") {
		t.Fatalf("Set-Cookie = %q, want %s cookie", setCookie, adminSessionCookieName)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("Set-Cookie = %q, want HttpOnly", setCookie)
	}
}

func TestLoginSubmit_WrongKeyRejected(t *testing.T) {
	withSecret(t, "the-real-key")
	r := newAuthTestEngine(t)

	form := strings.NewReader("admin_secret_key=wrong")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 on wrong key", w.Code)
	}
	// A wrong key must not mint a session certificate cookie at all.
	if setCookie := w.Header().Get("Set-Cookie"); strings.Contains(setCookie, adminSessionCookieName+"=") &&
		!strings.Contains(setCookie, "Max-Age=0") {
		t.Fatalf("wrong key must not issue a long-lived session cookie; Set-Cookie=%q", setCookie)
	}
}

func TestLoginSubmit_EmptySecretNeverLogsIn(t *testing.T) {
	withSecret(t, "")
	r := newAuthTestEngine(t)

	form := strings.NewReader("admin_secret_key=anything")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 when secret empty (login disabled)", w.Code)
	}
}

func TestLogout_ClearsCookie(t *testing.T) {
	withSecret(t, "strong-secret")
	r := newAuthTestEngine(t)

	w := perform(r, http.MethodGet, "/logout", "text/html", nil)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 to /login", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
	setCookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, adminSessionCookieName+"=") {
		t.Fatalf("Set-Cookie = %q, want cleared %s cookie", setCookie, adminSessionCookieName)
	}
	// clearing is done with Max-Age <= 0
	if !strings.Contains(setCookie, "Max-Age=0") && !strings.Contains(setCookie, "expires") {
		t.Fatalf("Set-Cookie = %q, want expired/cleared cookie", setCookie)
	}
}
