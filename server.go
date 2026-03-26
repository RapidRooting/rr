package rapidroot

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run starts the HTTP server with graceful shutdown on SIGINT/SIGTERM.
func (r *Router) Run(addr string) error {
	return r.serve(&http.Server{Addr: addr, Handler: r})
}

// RunTLS starts the HTTPS server with graceful shutdown.
func (r *Router) RunTLS(addr, certFile, keyFile string) error {
	srv := &http.Server{Addr: addr, Handler: r}
	return r.serveTLS(srv, certFile, keyFile)
}

// Serve lets you pass a fully configured *http.Server.
//
//	srv := &http.Server{
//	    Addr:         ":8080",
//	    Handler:      router,
//	    ReadTimeout:  5 * time.Second,
//	    WriteTimeout: 10 * time.Second,
//	}
//	router.Serve(srv)
func (r *Router) Serve(srv *http.Server) error {
	if srv.Handler == nil {
		srv.Handler = r
	}
	return r.serve(srv)
}

func (r *Router) serve(srv *http.Server) error {
	r.applyMiddleware()

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stdout, "rapidroot: listening on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	return r.awaitShutdown(srv, errCh)
}

func (r *Router) serveTLS(srv *http.Server, certFile, keyFile string) error {
	r.applyMiddleware()

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stdout, "rapidroot: listening on %s (TLS)\n", srv.Addr)
		if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	return r.awaitShutdown(srv, errCh)
}

func (r *Router) awaitShutdown(srv *http.Server, errCh <-chan error) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("rapidroot: server error: %w", err)
	case sig := <-quit:
		fmt.Fprintf(os.Stdout, "\nrapidroot: received %v, shutting down...\n", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("rapidroot: shutdown error: %w", err)
	}

	fmt.Fprintln(os.Stdout, "rapidroot: server stopped")
	return nil
}
