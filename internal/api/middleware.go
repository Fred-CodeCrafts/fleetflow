package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// withRequestID ensures every request carries an ID: it reuses the caller's when
// it is safe, otherwise generates one, returns it in the response header, and puts
// it in the context so handlers and log lines can quote it.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
		}

		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(contextWithRequestID(r.Context(), id)))
	})
}

// withLogging writes one structured line per request. The request ID ties the line
// to the response the client saw, so a reported ID can be looked up.
func withLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		// Log against a background context: the request's own context is already
		// cancelled by the time a client disconnects, and that is exactly the
		// case worth recording.
		logger.LogAttrs(context.Background(), slog.LevelInfo, "http request",
			slog.String("request_id", requestIDFrom(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.Status()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("remote", clientIP(r)),
		)
	})
}

// withTimeout gives a request a deadline, cancelling its context when the
// deadline passes. Repositories observe the context, so the deadline is what
// stops a slow dependency from holding the connection open: the service maps the
// resulting error to domain.Unavailable, which the API answers as 503.
//
// It is not applied to the event stream, which is meant to stay open.
func withTimeout(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder remembers the status code so the logging middleware can report
// it. A handler that writes a body without calling WriteHeader has answered 200,
// so an unset status reads as 200.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the status before passing it on.
func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

// Status returns the status the handler answered with.
func (r *statusRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// Unwrap returns the writer this one wraps. It is what lets
// http.ResponseController reach capabilities the wrapper does not expose itself —
// flushing, above all, which the event stream needs and which a wrapper that
// embedded http.ResponseWriter would otherwise hide from it.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// clientIP returns the address a request is attributed to. Behind Cloud Run or Fly
// the socket peer is the platform's proxy, so the first X-Forwarded-For entry is
// the real client. That header is trivially spoofable, which is acceptable
// because the service is only reachable through those proxies: this is a fair-use
// limit, not a security boundary (DESIGN 3.5).
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		if ip := strings.TrimSpace(first); ip != "" {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
