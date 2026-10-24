package api

import (
	"errors"
	"log/slog"
	"net/http"

	"fleetflow/internal/auth"
	"fleetflow/internal/domain"
)

// refusedError is a request the transport refused before any domain rule saw it:
// a body over the size limit, a client over its rate limit. It carries the status
// and the body to answer with, because no domain rule describes these failures —
// how large a body may be, and how fast a client may call, are transport and
// deployment concerns, not fleet rules.
type refusedError struct {
	status  int
	code    string
	message string
}

// Error implements the error interface. The text is for logs; clients get the
// code and message through writeError.
func (e *refusedError) Error() string { return e.code + ": " + e.message }

// errorBody is the body of every error response the API writes. One shape means a
// client always parses an error the same way, whatever went wrong.
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// RequestID lets a client quote one exact request in a bug report, and lets
	// the server find that request in the logs.
	RequestID string `json:"request_id,omitempty"`
}

// statusFor is the one place that turns a failure into an HTTP status code. Every
// handler reports failures through writeError, so the same failure cannot be
// reported two different ways by two endpoints (DESIGN 3.8).
func statusFor(err error) int {
	// A refusal already knows the status it wants.
	var refused *refusedError
	if errors.As(err, &refused) {
		return refused.status
	}

	// A rejected API key is a 401 rather than the 404 its domain Kind would
	// otherwise produce. The client did not ask for something that is missing: it
	// failed to authenticate, and a 404 would suggest the key exists somewhere to
	// be found (DESIGN 3.5, PRD US-13).
	if errors.Is(err, auth.ErrUnauthenticated) {
		return http.StatusUnauthorized
	}

	switch domain.KindOf(err) {
	case domain.KindInvalid:
		return http.StatusBadRequest
	case domain.KindNotFound:
		return http.StatusNotFound
	case domain.KindConflict:
		return http.StatusConflict
	case domain.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// writeError reports err to the client: the status comes from statusFor, and the
// body from the error's public code and message.
//
// An error that is not a typed domain error is an internal failure. Its text can
// carry database or driver detail, so the client receives a generic message and
// the detail goes to the log, tagged with the request ID.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := statusFor(err)

	var refused *refusedError
	var code, message string

	if errors.As(err, &refused) {
		// A refusal was written for the client, so its own wording is what should
		// be returned.
		code, message = refused.code, refused.message
	} else if publicCode, publicMessage, ok := domain.Public(err); ok {
		code, message = publicCode, publicMessage
	} else {
		// An unclassified error is an internal failure. Its text can carry
		// database or driver detail, so the detail goes to the log and the client
		// gets a generic message.
		code, message = "internal", "internal server error"
		loggerFromContext(r.Context()).ErrorContext(r.Context(), "request failed",
			slog.String("request_id", requestIDFrom(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)
	}

	writeJSON(w, status, errorBody{
		Code:      code,
		Message:   message,
		RequestID: requestIDFrom(r.Context()),
	})
}
