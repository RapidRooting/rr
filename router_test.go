package rapidroot

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------- Tree Unit Tests -------------------------------------------------

func TestForEachSegment(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"/", nil},
		{"", nil},
		{"/users", []string{"users"}},
		{"/users/42/posts", []string{"users", "42", "posts"}},
		{"/a/b/c/d", []string{"a", "b", "c", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			var got []string
			forEachSegment(tt.path, func(s string) { got = append(got, s) })
			if len(got) != len(tt.want) {
				t.Fatalf("forEachSegment(%q) = %v, want %v", tt.path, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("segment[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "/"},
		{"/", "/"},
		{"users", "/users"},
		{"/users/", "/users"},
		{"/users//posts", "/users/posts"},
		{"/a/../b", "/b"},
		{"/a/./b", "/a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := cleanPath(tt.in); got != tt.want {
				t.Errorf("cleanPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTreeAddAndLookup(t *testing.T) {
	root := newNode("")
	noop := func(*Request) {}

	addRoute("/users", root, noop)
	addRoute("/users/:id", root, noop)
	addRoute("/users/:id/posts", root, noop)
	addRoute("/static", root, noop)

	tests := []struct {
		path       string
		wantMatch  bool
		wantParams map[string]string
	}{
		{"/users", true, nil},
		{"/users/42", true, map[string]string{"id": "42"}},
		{"/users/42/posts", true, map[string]string{"id": "42"}},
		{"/static", true, nil},
		{"/missing", false, nil},
		{"/users/42/unknown", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := &Request{pathParams: make(map[string]string)}
			n := lookup(tt.path, root, req)
			gotMatch := n != nil && n.handler != nil
			if gotMatch != tt.wantMatch {
				t.Fatalf("lookup(%q) matched=%v, want %v", tt.path, gotMatch, tt.wantMatch)
			}
			for k, want := range tt.wantParams {
				if got := req.pathParams[k]; got != want {
					t.Errorf("param[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

// ---------- Router Integration Tests ----------------------------------------

func newTestRouter() *Router {
	r := NewRouter()

	r.GET("/", func(req *Request) { req.Text(http.StatusOK, "root") })
	r.GET("/hello", func(req *Request) { req.Text(http.StatusOK, "hello") })
	r.POST("/users", func(req *Request) { req.Text(http.StatusCreated, "created") })
	r.GET("/users/:id", func(req *Request) { req.Text(http.StatusOK, "user:"+req.Param("id")) })
	r.GET("/users/:id/posts/:postID", func(req *Request) {
		req.Text(http.StatusOK, req.Param("id")+":"+req.Param("postID"))
	})

	r.applyMiddleware()
	return r
}

func TestRouterBasicRouting(t *testing.T) {
	router := newTestRouter()

	tests := []struct {
		method, path string
		wantCode     int
		wantBody     string
	}{
		{"GET", "/", 200, "root"},
		{"GET", "/hello", 200, "hello"},
		{"POST", "/users", 201, "created"},
		{"GET", "/users/42", 200, "user:42"},
		{"GET", "/users/42/posts/7", 200, "42:7"},
		{"GET", "/notfound", 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRouterMethodNotAllowed(t *testing.T) {
	r := NewRouter()
	r.GET("/users", func(req *Request) { req.Text(200, "ok") })
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/users", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
	if allow := w.Header().Get("Allow"); allow == "" {
		t.Error("expected Allow header")
	}
}

func TestRouterGroupMiddleware(t *testing.T) {
	r := NewRouter()

	addHeader := func(key, val string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(req *Request) {
				req.Writer.Header().Set(key, val)
				next(req)
			}
		}
	}

	r.GroupMiddleware("GET", "/api", addHeader("X-Group", "yes"))
	r.Middleware("GET", "/api/special", addHeader("X-Node", "yes"))

	r.GET("/api/items", func(req *Request) { req.Text(200, "items") })
	r.GET("/api/special", func(req *Request) { req.Text(200, "special") })
	r.GET("/other", func(req *Request) { req.Text(200, "other") })

	r.applyMiddleware()

	// /api/items should have group MW
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/items", nil))
	if w.Header().Get("X-Group") != "yes" {
		t.Error("group middleware not applied to /api/items")
	}

	// /api/special should have both
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/special", nil))
	if w.Header().Get("X-Group") != "yes" {
		t.Error("group middleware not applied to /api/special")
	}
	if w.Header().Get("X-Node") != "yes" {
		t.Error("node middleware not applied to /api/special")
	}

	// /other should have neither
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/other", nil))
	if w.Header().Get("X-Group") != "" {
		t.Error("group middleware should NOT apply to /other")
	}
}

func TestRouterAbort(t *testing.T) {
	r := NewRouter()

	abortMW := func(next HandlerFunc) HandlerFunc {
		return func(req *Request) {
			req.Abort()
			req.Text(http.StatusForbidden, "forbidden")
			// note: next is NOT called because we aborted
		}
	}

	r.Middleware("GET", "/secret", abortMW)
	r.GET("/secret", func(req *Request) { req.Text(200, "should not reach") })
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/secret", nil))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if w.Body.String() != "forbidden" {
		t.Errorf("body = %q, want 'forbidden'", w.Body.String())
	}
}

func TestRouterGroup(t *testing.T) {
	r := NewRouter()

	api := r.Group("/api/v1")
	api.GET("/users", func(req *Request) { req.Text(200, "users-v1") })
	api.GET("/users/:id", func(req *Request) { req.Text(200, "user:"+req.Param("id")) })

	r.applyMiddleware()

	tests := []struct {
		path     string
		wantCode int
		wantBody string
	}{
		{"/api/v1/users", 200, "users-v1"},
		{"/api/v1/users/99", 200, "user:99"},
		{"/api/v1/missing", 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRouterGroupWithMiddleware(t *testing.T) {
	r := NewRouter()

	callLog := make([]string, 0)

	logMW := func(label string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(req *Request) {
				callLog = append(callLog, label)
				next(req)
			}
		}
	}

	admin := r.Group("/admin", logMW("admin-mw"))
	admin.GET("/dashboard", func(req *Request) {
		callLog = append(callLog, "handler")
		req.Text(200, "dashboard")
	})

	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/admin/dashboard", nil))

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if len(callLog) != 2 || callLog[0] != "admin-mw" || callLog[1] != "handler" {
		t.Errorf("call order = %v, want [admin-mw handler]", callLog)
	}
}

func TestRequestContext(t *testing.T) {
	r := NewRouter()
	r.GET("/ctx", func(req *Request) {
		if req.Context() == nil {
			t.Fatal("context is nil")
		}
		req.Text(200, "ok")
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/ctx", nil))
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRequestPoolNoLeak(t *testing.T) {
	r := NewRouter()
	r.GET("/users/:id", func(req *Request) { req.Text(200, req.Param("id")) })
	r.applyMiddleware()

	for _, id := range []string{"1", "2", "3", "42", "100"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/users/"+id, nil))
		if w.Body.String() != id {
			t.Errorf("got %q, want %q (pool leak?)", w.Body.String(), id)
		}
	}
}

func TestRequestJSON(t *testing.T) {
	r := NewRouter()
	r.GET("/json", func(req *Request) {
		req.JSON(200, map[string]string{"msg": "hello"})
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/json", nil))

	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRootRoute(t *testing.T) {
	r := NewRouter()
	r.GET("/", func(req *Request) { req.Text(200, "root") })
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Body.String() != "root" {
		t.Errorf("body = %q, want 'root'", w.Body.String())
	}
}

func TestMultipleDynamicSegments(t *testing.T) {
	r := NewRouter()
	r.GET("/a/:x/b/:y/c/:z", func(req *Request) {
		req.Text(200, req.Param("x")+"-"+req.Param("y")+"-"+req.Param("z"))
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/a/1/b/2/c/3", nil))
	if w.Body.String() != "1-2-3" {
		t.Errorf("body = %q, want '1-2-3'", w.Body.String())
	}
}

// ---------- Cookie Tests ----------------------------------------------------

func TestCookieSetAndRead(t *testing.T) {
	r := NewRouter()
	r.GET("/set", func(req *Request) {
		req.SetCookie("session", "abc123", time.Now().Add(time.Hour))
		req.Text(200, "ok")
	})
	r.GET("/read", func(req *Request) {
		c, err := req.Cookie("session")
		if err != nil {
			req.Text(400, "no cookie")
			return
		}
		req.Text(200, c.Value)
	})
	r.applyMiddleware()

	// Set cookie
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/set", nil))

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "session" || c.Value != "abc123" {
		t.Errorf("cookie = %s=%s, want session=abc123", c.Name, c.Value)
	}
	if !c.HttpOnly {
		t.Error("expected HttpOnly=true (secure default)")
	}
	if !c.Secure {
		t.Error("expected Secure=true (secure default)")
	}

	// Read cookie back
	w2 := httptest.NewRecorder()
	readReq := httptest.NewRequest("GET", "/read", nil)
	readReq.AddCookie(c)
	r.ServeHTTP(w2, readReq)
	if w2.Body.String() != "abc123" {
		t.Errorf("body = %q, want 'abc123'", w2.Body.String())
	}
}

func TestCookieCustomDefaults(t *testing.T) {
	r := NewRouter()
	r.CookieDefaults.Secure = false
	r.CookieDefaults.Path = "/api"
	r.CookieDefaults.Domain = "example.com"

	r.GET("/set", func(req *Request) {
		req.SetCookie("token", "xyz", time.Now().Add(time.Hour))
		req.Text(200, "ok")
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/set", nil))

	c := w.Result().Cookies()[0]
	if c.Secure {
		t.Error("expected Secure=false")
	}
	if c.Path != "/api" {
		t.Errorf("path = %q, want '/api'", c.Path)
	}
	if c.Domain != "example.com" {
		t.Errorf("domain = %q, want 'example.com'", c.Domain)
	}
}

func TestCookieRemove(t *testing.T) {
	r := NewRouter()
	r.GET("/remove", func(req *Request) {
		req.RemoveCookie("session")
		req.Text(200, "ok")
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/remove", nil))

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1 (expired)", c.MaxAge)
	}
}

func TestCookieSetObj(t *testing.T) {
	r := NewRouter()
	r.GET("/custom", func(req *Request) {
		req.SetCookieObj(&http.Cookie{
			Name:     "custom",
			Value:    "val",
			Path:     "/special",
			HttpOnly: false,
		})
		req.Text(200, "ok")
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/custom", nil))

	c := w.Result().Cookies()[0]
	if c.Name != "custom" || c.Value != "val" {
		t.Errorf("cookie = %s=%s", c.Name, c.Value)
	}
	if c.Path != "/special" {
		t.Errorf("path = %q, want '/special'", c.Path)
	}
}

func TestRemoveAllCookiesExcept(t *testing.T) {
	r := NewRouter()
	r.GET("/clean", func(req *Request) {
		req.RemoveAllCookiesExcept("keep")
		req.Text(200, "ok")
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	httpReq := httptest.NewRequest("GET", "/clean", nil)
	httpReq.AddCookie(&http.Cookie{Name: "keep", Value: "1"})
	httpReq.AddCookie(&http.Cookie{Name: "drop1", Value: "2"})
	httpReq.AddCookie(&http.Cookie{Name: "drop2", Value: "3"})
	r.ServeHTTP(w, httpReq)

	// Should have Set-Cookie headers expiring drop1 and drop2, but not keep
	responseCookies := w.Result().Cookies()
	expired := make(map[string]bool)
	for _, c := range responseCookies {
		if c.MaxAge == -1 {
			expired[c.Name] = true
		}
	}
	if expired["keep"] {
		t.Error("'keep' should not be expired")
	}
	if !expired["drop1"] {
		t.Error("'drop1' should be expired")
	}
	if !expired["drop2"] {
		t.Error("'drop2' should be expired")
	}
}

// ---------- Catch-All Tests -------------------------------------------------

func TestCatchAllBasic(t *testing.T) {
	r := NewRouter()
	r.GET("/static/*filepath", func(req *Request) {
		req.Text(200, req.Param("filepath"))
	})
	r.applyMiddleware()

	tests := []struct {
		path     string
		wantCode int
		wantBody string
	}{
		{"/static/css/main.css", 200, "css/main.css"},
		{"/static/js/app.js", 200, "js/app.js"},
		{"/static/img/logo.png", 200, "img/logo.png"},
		{"/static/a/b/c/d/e", 200, "a/b/c/d/e"},
		{"/static/single", 200, "single"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestCatchAllWithStaticRoutes(t *testing.T) {
	r := NewRouter()

	// static route takes priority over catch-all
	r.GET("/files/special", func(req *Request) {
		req.Text(200, "special")
	})
	r.GET("/files/*path", func(req *Request) {
		req.Text(200, "catch:"+req.Param("path"))
	})
	r.applyMiddleware()

	// exact static match wins
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/files/special", nil))
	if w.Body.String() != "special" {
		t.Errorf("static route: body = %q, want 'special'", w.Body.String())
	}

	// everything else goes to catch-all
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/files/other", nil))
	if w.Body.String() != "catch:other" {
		t.Errorf("catch-all: body = %q, want 'catch:other'", w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/files/deep/nested/path", nil))
	if w.Body.String() != "catch:deep/nested/path" {
		t.Errorf("catch-all deep: body = %q, want 'catch:deep/nested/path'", w.Body.String())
	}
}

func TestCatchAllWithDynamic(t *testing.T) {
	r := NewRouter()

	// :id matches single segment, *rest matches everything
	r.GET("/repo/:owner/:name/blob/*path", func(req *Request) {
		req.Text(200, req.Param("owner")+"/"+req.Param("name")+":"+req.Param("path"))
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/repo/folium1/rapidroot/blob/main/tree.go", nil))
	if w.Body.String() != "folium1/rapidroot:main/tree.go" {
		t.Errorf("body = %q, want 'folium1/rapidroot:main/tree.go'", w.Body.String())
	}
}

func TestCatchAllMiddleware(t *testing.T) {
	r := NewRouter()

	addHeader := func(key, val string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(req *Request) {
				req.Writer.Header().Set(key, val)
				next(req)
			}
		}
	}

	r.GroupMiddleware("GET", "/assets", addHeader("X-Assets", "yes"))
	r.GET("/assets/*filepath", func(req *Request) {
		req.Text(200, req.Param("filepath"))
	})
	r.applyMiddleware()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/assets/css/style.css", nil))

	if w.Header().Get("X-Assets") != "yes" {
		t.Error("group middleware not applied to catch-all route")
	}
	if w.Body.String() != "css/style.css" {
		t.Errorf("body = %q, want 'css/style.css'", w.Body.String())
	}
}

func TestCatchAllPanicOnNonLast(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for catch-all not in last position")
		}
	}()

	root := newNode("")
	// *filepath not last — should panic
	addRoute("/static/*filepath/other", root, func(*Request) {})
}