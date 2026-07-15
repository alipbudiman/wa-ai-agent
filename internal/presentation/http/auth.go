package route

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// adminSessionCookieName is the browser cookie that carries the signed
	// session "certificate" once an admin has logged in.
	adminSessionCookieName = "wa_admin_session"
	// adminSessionTTL is how long a successful login stays valid. The user
	// requested a 30-day "certificate" session: log in once, stay signed in.
	adminSessionTTL = 30 * 24 * time.Hour
	// adminCertPurpose binds the signing key to this login certificate so the
	// raw ADMIN_SECRET_KEY is never used directly as an HMAC key or stored in
	// the cookie.
	adminCertPurpose = "wa-ai-agent-admin-cert-v1"
)

// adminSecretKey returns the configured ADMIN_SECRET_KEY (trimmed). An empty
// value means the dashboard login is disabled and the whole system is
// access-restricted: nobody can log in and every protected route is blocked.
func adminSecretKey() string {
	return strings.TrimSpace(os.Getenv("ADMIN_SECRET_KEY"))
}

// adminSigningKey derives a stable HMAC key from ADMIN_SECRET_KEY. Tying the
// session certificate to the admin secret means rotating the secret instantly
// invalidates every outstanding 30-day session, which is the desired behaviour.
func adminSigningKey() []byte {
	mac := hmac.New(sha256.New, []byte(adminCertPurpose))
	mac.Write([]byte(adminSecretKey()))
	return mac.Sum(nil)
}

// signSessionCertificate builds a self-contained signed certificate in the form
// "<expiryBase64>.<signatureBase64>". The expiry is the only payload; the
// signature proves the server minted it. This is stateless: no server-side
// session store is required to validate it.
func signSessionCertificate(expiry int64) string {
	payload := strconv.FormatInt(expiry, 10)

	mac := hmac.New(sha256.New, adminSigningKey())
	mac.Write([]byte(payload))

	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifySessionCertificate validates a certificate and returns true only when
// the signature matches (constant-time) AND the expiry is still in the future.
// Any decode/parse problem is treated as invalid rather than raising.
func verifySessionCertificate(raw string) bool {
	if raw == "" {
		return false
	}

	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 {
		return false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, adminSigningKey())
	mac.Write(payloadBytes)
	if !hmac.Equal(signatureBytes, mac.Sum(nil)) {
		return false
	}

	expiry, err := strconv.ParseInt(string(payloadBytes), 10, 64)
	if err != nil {
		return false
	}

	return expiry >= time.Now().Unix()
}

// requestIsTLS reports whether the effective transport is HTTPS, so the cookie
// can be marked Secure only when it actually would be sent over TLS. It honours
// X-Forwarded-Proto for deployments behind a TLS-terminating reverse proxy.
func requestIsTLS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	return strings.HasPrefix(proto, "https")
}

// setAdminSessionCookie writes the 30-day signed certificate as a HttpOnly,
// SameSite=Lax cookie scoped to the whole site.
func setAdminSessionCookie(c *gin.Context, expiry int64) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		adminSessionCookieName,
		signSessionCertificate(expiry),
		int(adminSessionTTL.Seconds()),
		"/",
		"",
		requestIsTLS(c),
		true,
	)
}

// clearAdminSessionCookie expires the session cookie immediately.
func clearAdminSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminSessionCookieName, "", -1, "/", "", requestIsTLS(c), true)
}

// isAdminAuthenticated reports whether the current request carries a valid,
// unexpired session certificate. When ADMIN_SECRET_KEY is empty it always
// returns false: login is disabled, so no certificate can be considered valid.
func isAdminAuthenticated(c *gin.Context) bool {
	if adminSecretKey() == "" {
		return false
	}

	cookie, err := c.Cookie(adminSessionCookieName)
	if err != nil {
		return false
	}

	return verifySessionCertificate(cookie)
}

// isPublicAuthPath lists routes that must stay reachable without a session.
// /login and /logout are the auth UI. The /v1 bot API is a programmatic
// integration surface: the brain container calls it service-to-service
// (e.g. /v1/contact/info, /v1/message/send) and external integrations use it
// too, so it cannot depend on the admin session cookie.
func isPublicAuthPath(path string) bool {
	if path == "/login" || path == "/logout" {
		return true
	}
	return path == BOT_ROUTE_BASE || strings.HasPrefix(path, BOT_ROUTE_BASE+"/")
}

// wantsHTML reports whether the client expects an HTML response (browser
// navigation). API/XHR callers get JSON errors instead of redirects.
func wantsHTML(c *gin.Context) bool {
	return strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/html")
}

// rejectAccess handles the "access restricted" case used when ADMIN_SECRET_KEY
// is not configured: browsers see the restricted page, API callers get 403 JSON.
func rejectAccess(c *gin.Context) {
	if wantsHTML(c) {
		c.HTML(http.StatusForbidden, "restricted.gohtml", gin.H{})
		c.Abort()
		return
	}

	c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{
		Success: false,
		Message: "admin access is restricted: ADMIN_SECRET_KEY is not configured",
	})
}

// RequireAdminAuth is the global gate for the dashboard. It lets the auth UI
// (/login, /logout) and the programmatic /v1 bot API through unconditionally,
// then either admits authenticated requests or rejects them. When
// ADMIN_SECRET_KEY is empty it restricts the dashboard. Logged-out browser
// requests are redirected to /login; API callers get 401.
func RequireAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicAuthPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		if isAdminAuthenticated(c) {
			c.Next()
			return
		}

		if adminSecretKey() == "" {
			rejectAccess(c)
			return
		}

		if wantsHTML(c) {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
			Success: false,
			Message: "authentication required",
		})
	}
}

// AuthRouter registers the login and logout routes. RequireAdminAuth
// allowlists exactly these two paths, so they must be registered for the gate
// to let a logged-out user reach them.
func AuthRouter(router *gin.Engine) *gin.Engine {
	router.GET("/login", loginPageHandler)
	router.POST("/login", loginSubmitHandler)
	router.GET("/logout", logoutHandler)
	router.POST("/logout", logoutHandler)
	return router
}

// loginPageHandler renders the single-field login form. Already-authenticated
// users bounce to home; if the admin secret is not configured the page is
// replaced by the "access restricted" notice.
func loginPageHandler(c *gin.Context) {
	if isAdminAuthenticated(c) {
		c.Redirect(http.StatusFound, "/")
		return
	}

	if adminSecretKey() == "" {
		rejectAccess(c)
		return
	}

	c.HTML(http.StatusOK, "login.gohtml", gin.H{"Error": ""})
}

// loginSubmitHandler validates the submitted ADMIN_SECRET_KEY against the
// configured value using a constant-time compare. On match it mints a 30-day
// certificate cookie and redirects home; otherwise it re-renders the form with
// an error. When the secret is not configured, no login can ever succeed.
func loginSubmitHandler(c *gin.Context) {
	if adminSecretKey() == "" {
		rejectAccess(c)
		return
	}

	submitted := c.PostForm("admin_secret_key")
	expected := adminSecretKey()

	// Constant-time compare; also reject empty submissions explicitly.
	if submitted == "" ||
		subtle.ConstantTimeCompare([]byte(submitted), []byte(expected)) != 1 {
		c.HTML(http.StatusUnauthorized, "login.gohtml", gin.H{
			"Error": "Invalid secret key. Access denied.",
		})
		return
	}

	expiry := time.Now().Add(adminSessionTTL).Unix()
	setAdminSessionCookie(c, expiry)
	c.Redirect(http.StatusFound, "/")
}

// logoutHandler clears the session cookie and returns the user to the login page.
func logoutHandler(c *gin.Context) {
	clearAdminSessionCookie(c)
	c.Redirect(http.StatusFound, "/login")
}
