package rapidroot

import "net/http"

// HandlerFunc is a function that handles HTTP requests.
type HandlerFunc func(*Request)

// Middleware wraps a HandlerFunc, returning a new HandlerFunc.
type Middleware func(HandlerFunc) HandlerFunc

// notFoundHandler writes a 404 response.
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// GroupMiddleware attaches middleware to all routes sharing the path prefix.
//
//	router.GroupMiddleware("GET", "/api", authMW)
//	router.GET("/api/users", listUsers)   // authMW applied
//	router.GET("/api/orders", listOrders) // authMW applied
func (r *Router) GroupMiddleware(method, path string, mw ...Middleware) {
	if len(mw) == 0 {
		return
	}
	path = cleanPath(path)
	root := r.getOrCreateRoot(method)
	n := walkOrCreate(path, root)
	n.groupMW = append(n.groupMW, mw...)
}

// Middleware attaches middleware to a single route.
//
//	router.Middleware("GET", "/admin", adminOnly)
func (r *Router) Middleware(method, path string, mw ...Middleware) {
	if len(mw) == 0 {
		return
	}
	path = cleanPath(path)
	root := r.getOrCreateRoot(method)
	n := walkOrCreate(path, root)
	n.nodeMW = append(n.nodeMW, mw...)
}

// walkOrCreate walks or creates tree nodes for path. Used by middleware registration.
func walkOrCreate(path string, root *node) *node {
	cur := root
	forEachSegment(path, func(seg string) {
		if isCatchAll(seg) {
			cur = cur.setCatchAll(trimPrefix(seg))
		} else if isDynamic(seg) {
			cur = cur.setWildcard(trimPrefix(seg))
		} else {
			cur = cur.addStaticChild(seg)
		}
	})
	return cur
}

// applyMiddleware bakes all middleware into handlers at startup.
// Called once before the server starts listening.
func (r *Router) applyMiddleware() {
	for _, root := range r.trees {
		r.traverse(root, nil)
	}
}

// traverse recursively applies middleware to all nodes in the tree.
// parentMW is defensively copied to prevent slice mutation across siblings.
func (r *Router) traverse(n *node, parentMW []Middleware) {
	if n == nil {
		return
	}

	// Build inherited chain: parent group + this node's group middleware.
	// CRITICAL: allocate a new slice so append never mutates the caller's backing array.
	inherited := make([]Middleware, 0, len(parentMW)+len(n.groupMW))
	inherited = append(inherited, parentMW...)
	inherited = append(inherited, n.groupMW...)

	// Full chain for this node: inherited + node-specific middleware.
	if n.handler != nil {
		full := make([]Middleware, 0, len(inherited)+len(n.nodeMW))
		full = append(full, inherited...)
		full = append(full, n.nodeMW...)

		h := n.handler
		for i := len(full) - 1; i >= 0; i-- {
			h = full[i](h)
		}
		n.handler = h
	}

	// Free — middleware is baked into the handler now.
	n.groupMW = nil
	n.nodeMW = nil

	// Recurse with a copy of inherited for each subtree.
	for _, ch := range n.children {
		r.traverse(ch, inherited)
	}
	if n.wildcard != nil {
		r.traverse(n.wildcard, inherited)
	}
	if n.catchAll != nil {
		r.traverse(n.catchAll, inherited)
	}
}
