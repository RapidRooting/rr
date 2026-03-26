package rapidroot

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sync"
)

// Request wraps http.Request and http.ResponseWriter with convenience methods.
type Request struct {
	Writer http.ResponseWriter
	Req    *http.Request

	pathParams     map[string]string
	queryValues    url.Values
	cookieDefaults *CookieDefaults
	aborted        bool
	wroteHeader    bool
}

var requestPool = sync.Pool{
	New: func() any {
		return &Request{
			pathParams: make(map[string]string, 4),
		}
	},
}

// acquireRequest gets a Request from the pool and initialises it.
func acquireRequest(w http.ResponseWriter, r *http.Request, cd *CookieDefaults) *Request {
	req := requestPool.Get().(*Request)
	req.Writer = w
	req.Req = r
	req.queryValues = r.URL.Query()
	req.cookieDefaults = cd
	req.aborted = false
	req.wroteHeader = false
	return req
}

// releaseRequest clears and returns a Request to the pool.
func releaseRequest(req *Request) {
	req.Writer = nil
	req.Req = nil
	req.queryValues = nil
	req.cookieDefaults = nil
	req.aborted = false
	req.wroteHeader = false
	for k := range req.pathParams {
		delete(req.pathParams, k)
	}
	requestPool.Put(req)
}

// setParam implements paramSetter for tree lookups.
func (r *Request) setParam(key, value string) {
	r.pathParams[key] = value
}

// ---------- Context ---------------------------------------------------------

// Context returns the request context.
func (r *Request) Context() context.Context {
	return r.Req.Context()
}

// WithContext replaces the underlying request's context.
func (r *Request) WithContext(ctx context.Context) {
	r.Req = r.Req.WithContext(ctx)
}

// ---------- Path Parameters -------------------------------------------------

// Param returns the value of a named path parameter (e.g. :id → "42").
func (r *Request) Param(key string) string {
	return r.pathParams[key]
}

// Params returns a copy of all path parameters.
func (r *Request) Params() map[string]string {
	cp := make(map[string]string, len(r.pathParams))
	for k, v := range r.pathParams {
		cp[k] = v
	}
	return cp
}

// ---------- Query -----------------------------------------------------------

// Query returns a single query parameter value.
func (r *Request) Query(key string) string {
	return r.queryValues.Get(key)
}

// QueryValues returns all query parameters.
func (r *Request) QueryValues() url.Values {
	return r.queryValues
}

// ---------- Form ------------------------------------------------------------

// FormValue returns a single form value.
func (r *Request) FormValue(key string) string {
	return r.Req.FormValue(key)
}

// ---------- Abort -----------------------------------------------------------

// Abort marks the request as aborted. Middleware should check IsAborted.
func (r *Request) Abort() {
	r.aborted = true
}

// IsAborted reports whether the request was aborted.
func (r *Request) IsAborted() bool {
	return r.aborted
}

// ---------- Response Writers ------------------------------------------------

// writeHeader writes status code once, preventing double writes.
func (r *Request) writeHeader(code int) {
	if !r.wroteHeader {
		r.Writer.WriteHeader(code)
		r.wroteHeader = true
	}
}

// JSON encodes data as JSON and writes it with the given status code.
func (r *Request) JSON(code int, data any) {
	r.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	r.writeHeader(code)
	if err := json.NewEncoder(r.Writer).Encode(data); err != nil {
		fmt.Fprintf(r.Writer, `{"error":%q}`, err.Error())
	}
}

// XML encodes data as XML and writes it with the given status code.
func (r *Request) XML(code int, data any) {
	r.Writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	r.writeHeader(code)
	if err := xml.NewEncoder(r.Writer).Encode(data); err != nil {
		fmt.Fprintf(r.Writer, "<error>%s</error>", err.Error())
	}
}

// HTML parses the named template file and executes it with data.
func (r *Request) HTML(code int, filename string, data any) {
	tmpl, err := template.ParseFiles(filename)
	if err != nil {
		http.Error(r.Writer, "internal server error", http.StatusInternalServerError)
		return
	}
	r.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	r.writeHeader(code)
	if execErr := tmpl.Execute(r.Writer, data); execErr != nil {
		fmt.Fprintf(r.Writer, "template error: %v", execErr)
	}
}

// Text writes a plain text response.
func (r *Request) Text(code int, text string) {
	r.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	r.writeHeader(code)
	fmt.Fprint(r.Writer, text)
}

// Bytes writes raw bytes with the given status code.
func (r *Request) Bytes(code int, contentType string, data []byte) {
	r.Writer.Header().Set("Content-Type", contentType)
	r.writeHeader(code)
	_, _ = r.Writer.Write(data)
}

// Redirect sends an HTTP redirect.
func (r *Request) Redirect(code int, url string) {
	http.Redirect(r.Writer, r.Req, url, code)
}

// Error writes an HTTP error response.
func (r *Request) Error(code int, msg string) {
	http.Error(r.Writer, msg, code)
}

// Status writes only the status code (for responses with no body).
func (r *Request) Status(code int) {
	r.writeHeader(code)
}
