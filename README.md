# RapidRoot

A fast, zero-allocation HTTP router for Go.

Routes are stored in a prefix tree with O(1) static child lookups. Path matching uses byte-level scanning — no `strings.Split`, no intermediate slices, no allocations on the hot path.

```
BenchmarkStaticRoute     ~90 ns/op    0 allocs/op
BenchmarkParamRoute      ~95 ns/op    0 allocs/op
BenchmarkCatchAllRoute   ~98 ns/op    0 allocs/op
```

## Install

```
go get github.com/Folium1/RapidRoot
```

Requires Go 1.22+.

## Quick start

```go
package main

import (
	"net/http"

	rr "github.com/Folium1/RapidRoot"
)

func main() {
	r := rr.NewRouter()

	r.GET("/", func(req *rr.Request) {
		req.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	r.GET("/users/:id", func(req *rr.Request) {
		req.Text(http.StatusOK, "user: "+req.Param("id"))
	})

	r.GET("/static/*filepath", func(req *rr.Request) {
		http.ServeFile(req.Writer, req.Req, "./public/"+req.Param("filepath"))
	})

	r.Run(":8080") // graceful shutdown on SIGINT/SIGTERM
}
```

## Route types

```go
r.GET("/users",              handler)  // static
r.GET("/users/:id",          handler)  // param — matches one segment
r.GET("/static/*filepath",   handler)  // catch-all — matches everything remaining
```

Params are accessed via `req.Param("id")`. Catch-all must always be the last segment.

## Methods

```go
r.GET(path, handler)
r.POST(path, handler)
r.PUT(path, handler)
r.DELETE(path, handler)
r.PATCH(path, handler)
r.HEAD(path, handler)
r.OPTIONS(path, handler)
```

## Route groups

```go
api := r.Group("/api/v1", authMiddleware)
api.GET("/users", listUsers)
api.POST("/users", createUser)

// nested groups inherit parent middleware
admin := api.Group("/admin", adminOnly)
admin.GET("/stats", statsHandler)
```

## Middleware

Middleware wraps handlers. Applied in registration order, innermost last.

```go
func Logger() rr.Middleware {
	return func(next rr.HandlerFunc) rr.HandlerFunc {
		return func(req *rr.Request) {
			start := time.Now()
			next(req)
			log.Printf("%s %s %v", req.Req.Method, req.Req.URL.Path, time.Since(start))
		}
	}
}

// per-route
r.Middleware("GET", "/api/admin", adminOnly)

// group — applies to all routes under the prefix
r.GroupMiddleware("GET", "/api", Logger())
```

Built-in panic recovery:

```go
r.GroupMiddleware("GET", "/", rr.Recovery())
```

## Standard library compatibility

RapidRoot implements `http.Handler`, so it works with any middleware that wraps `http.Handler`:

```go
// wrap with third-party middleware
handler := corsMiddleware(router)
http.ListenAndServe(":8080", handler)
```

Mount existing `http.Handler` / `http.HandlerFunc` directly:

```go
r.Handle("GET", "/debug/pprof/", http.DefaultServeMux)
r.HandleFunc("GET", "/health", func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
})
```

## Responses

```go
req.JSON(200, obj)                            // application/json
req.XML(200, obj)                             // application/xml
req.Text(200, "hello")                        // text/plain
req.HTML(200, "template.html", data)          // text/html (template)
req.Bytes(200, "application/pdf", pdfBytes)   // raw bytes
req.Redirect(301, "/new-location")
req.Error(400, "bad request")
req.Status(204)                               // no body
```

## Request helpers

```go
req.Param("id")          // path parameter
req.Query("page")        // query string (lazy-parsed, zero cost if unused)
req.FormValue("email")   // form value
req.Context()            // request context
req.Cookie("session")    // read cookie
req.Cookies()            // all cookies
```

## Cookies

Secure by default (`HttpOnly`, `Secure`, `SameSiteStrict`). Configure on the router:

```go
r.CookieDefaults.Secure = false          // local dev
r.CookieDefaults.Domain = "example.com"
```

```go
req.SetCookie("session", token, time.Now().Add(24*time.Hour))
req.RemoveCookie("session")
req.RemoveAllCookiesExcept("csrf")
```

## Server

```go
r.Run(":8080")                             // HTTP + graceful shutdown
r.RunTLS(":443", "cert.pem", "key.pem")   // HTTPS + graceful shutdown

// full control over timeouts
srv := &http.Server{
	Addr:         ":8080",
	Handler:      r,
	ReadTimeout:  5 * time.Second,
	WriteTimeout: 10 * time.Second,
}
r.Serve(srv)
```

## Behavior

- **Trailing slash redirect** — `GET /users/` → 301 to `/users`. Disable with `r.RedirectTrailingSlash = false`.
- **405 Method Not Allowed** — returns `Allow` header when the path exists for a different method.
- **Path cleaning** — double slashes, `..`, `.` are resolved before matching.

## Benchmarks

Run locally:

```
go test -bench=. -benchmem ./...
```

Results on Intel Xeon @ 2.80GHz:

```
BenchmarkStaticRoute         13796587     88.99 ns/op    0 B/op   0 allocs/op
BenchmarkParamRoute          12271286     95.25 ns/op    0 B/op   0 allocs/op
BenchmarkCatchAllRoute       11897052    102.10 ns/op    0 B/op   0 allocs/op
BenchmarkLookup_Static       36888102     32.67 ns/op    0 B/op   0 allocs/op
BenchmarkLookup_Param        26869453     45.90 ns/op    0 B/op   0 allocs/op
BenchmarkLookup_CatchAll     49619484     26.63 ns/op    0 B/op   0 allocs/op
BenchmarkParallel            20460794     91.63 ns/op    0 B/op   0 allocs/op
```

Zero allocations on all routing paths. The only allocations come from `net/http` itself (404/405 error formatting).

## License

MIT