package rapidroot

import (
	"net/http"
	"time"
)

// CookieDefaults holds the default attributes applied to every cookie
// set via Request.SetCookie. Configure them on the Router before starting:
//
//	router := NewRouter()
//	router.CookieDefaults.Secure = false   // e.g. for local dev
//	router.CookieDefaults.Path = "/api"
type CookieDefaults struct {
	HttpOnly bool
	Secure   bool
	SameSite http.SameSite
	Path     string
	Domain   string
	MaxAge   int
}

// defaultCookieDefaults returns secure-by-default cookie settings.
func defaultCookieDefaults() CookieDefaults {
	return CookieDefaults{
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	}
}

// Cookie returns a single cookie by name, or an error if not found.
func (r *Request) Cookie(name string) (*http.Cookie, error) {
	return r.Req.Cookie(name)
}

// Cookies returns all cookies from the request.
func (r *Request) Cookies() []*http.Cookie {
	return r.Req.Cookies()
}

// SetCookie sets a cookie using the router's default attributes.
func (r *Request) SetCookie(name, value string, expires time.Time) {
	d := r.cookieDefaults
	http.SetCookie(r.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Expires:  expires,
		HttpOnly: d.HttpOnly,
		Secure:   d.Secure,
		SameSite: d.SameSite,
		Path:     d.Path,
		Domain:   d.Domain,
		MaxAge:   d.MaxAge,
	})
}

// SetCookieObj sets a fully custom cookie, ignoring defaults.
func (r *Request) SetCookieObj(cookie *http.Cookie) {
	http.SetCookie(r.Writer, cookie)
}

// RemoveCookie expires a cookie by name.
func (r *Request) RemoveCookie(name string) {
	http.SetCookie(r.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: r.cookieDefaults.HttpOnly,
		Secure:   r.cookieDefaults.Secure,
		SameSite: r.cookieDefaults.SameSite,
		Path:     r.cookieDefaults.Path,
		Domain:   r.cookieDefaults.Domain,
	})
}

// RemoveAllCookies expires every cookie present in the request.
func (r *Request) RemoveAllCookies() {
	for _, c := range r.Cookies() {
		r.RemoveCookie(c.Name)
	}
}

// RemoveAllCookiesExcept expires all cookies except the named ones.
func (r *Request) RemoveAllCookiesExcept(keep ...string) {
	set := make(map[string]struct{}, len(keep))
	for _, k := range keep {
		set[k] = struct{}{}
	}
	for _, c := range r.Cookies() {
		if _, ok := set[c.Name]; !ok {
			r.RemoveCookie(c.Name)
		}
	}
}

// SetSecureFlagFromRequest sets the Secure default based on whether
// the current request arrived over TLS.
func (r *Request) SetSecureFlagFromRequest() {
	r.cookieDefaults.Secure = r.Req.TLS != nil
}
