package rapidroot

// paramSetter is satisfied by any type that can store path parameters.
// *Request implements this interface.
type paramSetter interface {
	setParam(key, value string)
}

// node represents a single segment in the routing tree.
type node struct {
	segment  string
	handler  HandlerFunc
	children map[string]*node // static children keyed by segment
	wildcard *node            // single-segment dynamic child (:param)
	catchAll *node            // greedy catch-all child (*param)

	groupMW []Middleware
	nodeMW  []Middleware
}

func newNode(segment string) *node {
	return &node{
		segment:  segment,
		children: make(map[string]*node),
	}
}

// addStaticChild returns existing or creates new static child.
func (n *node) addStaticChild(seg string) *node {
	if ch, ok := n.children[seg]; ok {
		return ch
	}
	ch := newNode(seg)
	n.children[seg] = ch
	return ch
}

// setWildcard creates or returns the single-segment dynamic child.
func (n *node) setWildcard(name string) *node {
	if n.wildcard != nil {
		n.wildcard.segment = name
		return n.wildcard
	}
	ch := newNode(name)
	n.wildcard = ch
	return ch
}

// setCatchAll creates or returns the catch-all child.
func (n *node) setCatchAll(name string) *node {
	if n.catchAll != nil {
		n.catchAll.segment = name
		return n.catchAll
	}
	ch := newNode(name)
	n.catchAll = ch
	return ch
}

// isDynamic checks if a segment starts with ':'.
func isDynamic(seg string) bool {
	return len(seg) > 1 && seg[0] == ':'
}

// isCatchAll checks if a segment starts with '*'.
func isCatchAll(seg string) bool {
	return len(seg) > 1 && seg[0] == '*'
}

// trimPrefix strips the leading ':' or '*' from a segment.
func trimPrefix(seg string) string {
	return seg[1:]
}

// addRoute inserts a path into the tree and associates it with handler.
//
// Supported segment types:
//   - static:    /users/posts
//   - dynamic:   /users/:id         (matches one segment)
//   - catch-all: /static/*filepath  (matches everything remaining, must be last)
func addRoute(path string, root *node, handler HandlerFunc) {
	cur := root
	segments := splitSegments(path)

	for i, seg := range segments {
		if isCatchAll(seg) {
			if i != len(segments)-1 {
				panic("rapidroot: catch-all *" + trimPrefix(seg) + " must be the last segment in path: " + path)
			}
			cur = cur.setCatchAll(trimPrefix(seg))
		} else if isDynamic(seg) {
			cur = cur.setWildcard(trimPrefix(seg))
		} else {
			cur = cur.addStaticChild(seg)
		}
	}

	cur.handler = handler
}

// lookup traverses the tree to find a matching node.
// Path parameters (including catch-all) are written to ps if non-nil.
func lookup(path string, root *node, ps paramSetter) *node {
	segments := splitSegments(path)
	cur := root

	for i := 0; i < len(segments); i++ {
		seg := segments[i]

		// 1) static match — O(1) map lookup
		if ch, ok := cur.children[seg]; ok {
			cur = ch
			continue
		}

		// 2) single-segment dynamic match
		if cur.wildcard != nil {
			if ps != nil {
				ps.setParam(cur.wildcard.segment, seg)
			}
			cur = cur.wildcard
			continue
		}

		// 3) catch-all: consume this segment + everything remaining
		if cur.catchAll != nil {
			if ps != nil {
				ps.setParam(cur.catchAll.segment, joinSegments(segments[i:]))
			}
			return cur.catchAll
		}

		// no match
		return nil
	}

	// All segments consumed. If no handler here but a catch-all exists,
	// match it with empty value (e.g. GET /static/ on /static/*filepath).
	if cur.handler == nil && cur.catchAll != nil {
		if ps != nil {
			ps.setParam(cur.catchAll.segment, "")
		}
		return cur.catchAll
	}

	return cur
}

// allowedMethods returns the Allow header value for a path, or empty string.
func allowedMethods(path string, trees map[string]*node) string {
	var allowed string
	for method, root := range trees {
		n := lookup(path, root, nil)
		if n != nil && n.handler != nil {
			if allowed != "" {
				allowed += ", "
			}
			allowed += method
		}
	}
	return allowed
}

// splitSegments splits a cleaned path into non-empty segments.
// Uses a stack-allocated array for paths with ≤8 segments (covers ~99% of routes).
func splitSegments(path string) []string {
	if len(path) <= 1 {
		return nil
	}

	// count segments
	n := 0
	for i := 1; i < len(path); i++ {
		if path[i] == '/' {
			n++
		}
	}
	n++ // last segment

	var buf [8]string
	var result []string
	if n <= len(buf) {
		result = buf[:0]
	} else {
		result = make([]string, 0, n)
	}

	start := 1 // skip leading '/'
	for i := 1; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				result = append(result, path[start:i])
			}
			start = i + 1
		}
	}
	return result
}

// joinSegments joins segments with '/' without a leading slash.
func joinSegments(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	if len(segs) == 1 {
		return segs[0]
	}
	n := len(segs) - 1
	for _, s := range segs {
		n += len(s)
	}
	b := make([]byte, 0, n)
	for i, s := range segs {
		if i > 0 {
			b = append(b, '/')
		}
		b = append(b, s...)
	}
	return string(b)
}

// forEachSegment calls fn for each non-empty segment without allocating a slice.
// Kept for walkOrCreate where index access is not needed.
func forEachSegment(path string, fn func(string)) {
	if len(path) <= 1 {
		return
	}
	start := 1
	for i := 1; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				fn(path[start:i])
			}
			start = i + 1
		}
	}
}
