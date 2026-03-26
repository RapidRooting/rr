package rapidroot

import "net/http"

// Router is an HTTP request multiplexer backed by a prefix tree.
// It implements http.Handler.
type Router struct {
	trees map[string]*node

	// CookieDefaults are applied to every cookie set via Request.SetCookie.
	// Modify these before calling Run/Serve.
	CookieDefaults CookieDefaults
}

// NewRouter returns a new Router with secure cookie defaults.
func NewRouter() *Router {
	return &Router{
		trees:          make(map[string]*node),
		CookieDefaults: defaultCookieDefaults(),
	}
}

// getOrCreateRoot returns the root node for method, creating it if needed.
func (r *Router) getOrCreateRoot(method string) *node {
	if root, ok := r.trees[method]; ok {
		return root
	}
	root := newNode("")
	r.trees[method] = root
	return root
}

// handle registers a handler for the given method and path.
func (r *Router) handle(method, path string, handler HandlerFunc) {
	if handler == nil {
		panic("rapidroot: nil handler for " + method + " " + path)
	}
	path = cleanPath(path)
	root := r.getOrCreateRoot(method)
	addRoute(path, root, handler)
}

// ServeHTTP dispatches the request to the matching handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := cleanPath(req.URL.Path)

	root, ok := r.trees[req.Method]
	if ok {
		rr := acquireRequest(w, req, &r.CookieDefaults)
		defer releaseRequest(rr)

		n := lookup(path, root, rr)
		if n != nil && n.handler != nil {
			n.handler(rr)
			return
		}
	}

	// path exists for another method → 405
	if allowed := allowedMethods(path, r.trees); allowed != "" {
		methodNotAllowedHandler(w, req, allowed)
		return
	}

	notFoundHandler(w, req)
}

// ---------- Route Registration ----------------------------------------------

// GET registers a handler for GET requests.
func (r *Router) GET(path string, h HandlerFunc) { r.handle(http.MethodGet, path, h) }

// POST registers a handler for POST requests.
func (r *Router) POST(path string, h HandlerFunc) { r.handle(http.MethodPost, path, h) }

// PUT registers a handler for PUT requests.
func (r *Router) PUT(path string, h HandlerFunc) { r.handle(http.MethodPut, path, h) }

// DELETE registers a handler for DELETE requests.
func (r *Router) DELETE(path string, h HandlerFunc) { r.handle(http.MethodDelete, path, h) }

// PATCH registers a handler for PATCH requests.
func (r *Router) PATCH(path string, h HandlerFunc) { r.handle(http.MethodPatch, path, h) }

// OPTIONS registers a handler for OPTIONS requests.
func (r *Router) OPTIONS(path string, h HandlerFunc) { r.handle(http.MethodOptions, path, h) }

// HEAD registers a handler for HEAD requests.
func (r *Router) HEAD(path string, h HandlerFunc) { r.handle(http.MethodHead, path, h) }

// ---------- Route Groups ----------------------------------------------------

// RouteGroup is a set of routes sharing a common prefix and middleware.
type RouteGroup struct {
	router *Router
	prefix string
	mw     []Middleware
}

// Group creates a route group with a common prefix and optional middleware.
//
//	api := router.Group("/api/v1", authMiddleware)
//	api.GET("/users", listUsers)     // → GET /api/v1/users with authMiddleware
//	api.POST("/users", createUser)   // → POST /api/v1/users with authMiddleware
func (r *Router) Group(prefix string, mw ...Middleware) *RouteGroup {
	return &RouteGroup{
		router: r,
		prefix: cleanPath(prefix),
		mw:     mw,
	}
}

func (g *RouteGroup) fullPath(path string) string {
	if path == "/" || path == "" {
		return g.prefix
	}
	return g.prefix + cleanPath(path)
}

func (g *RouteGroup) register(method, path string, h HandlerFunc) {
	full := g.fullPath(path)
	g.router.handle(method, full, h)
	if len(g.mw) > 0 {
		n := walkOrCreate(cleanPath(full), g.router.getOrCreateRoot(method))
		n.nodeMW = append(n.nodeMW, g.mw...)
	}
}

// GET registers a GET handler under the group prefix.
func (g *RouteGroup) GET(path string, h HandlerFunc) { g.register(http.MethodGet, path, h) }

// POST registers a POST handler under the group prefix.
func (g *RouteGroup) POST(path string, h HandlerFunc) { g.register(http.MethodPost, path, h) }

// PUT registers a PUT handler under the group prefix.
func (g *RouteGroup) PUT(path string, h HandlerFunc) { g.register(http.MethodPut, path, h) }

// DELETE registers a DELETE handler under the group prefix.
func (g *RouteGroup) DELETE(path string, h HandlerFunc) { g.register(http.MethodDelete, path, h) }

// PATCH registers a PATCH handler under the group prefix.
func (g *RouteGroup) PATCH(path string, h HandlerFunc) { g.register(http.MethodPatch, path, h) }
