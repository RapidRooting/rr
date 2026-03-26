package rapidroot

import (
	"net/http"
	"path"
)

// cleanPath normalises a URL path: resolves "..", removes double slashes,
// ensures leading '/', strips trailing '/'.
func cleanPath(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if len(cleaned) > 1 && cleaned[len(cleaned)-1] == '/' {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return cleaned
}

func methodNotAllowedHandler(w http.ResponseWriter, _ *http.Request, allowed string) {
	w.Header().Set("Allow", allowed)
	http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
}
