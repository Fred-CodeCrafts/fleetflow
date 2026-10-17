package api

import (
	"errors"
	"log/slog"
	"net/http"

	"fleetflow/internal/domain"
	"fleetflow/internal/engine"
)

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
	// The routing core defines its own sentinels and must stay free of transport
	// concerns, so translate them here instead of teaching the hot path about
	// status codes.
	switch {
	case errors.Is(err, engine.ErrEdgeNotFound):
		return http.StatusNotFound
	case errors.Is(err, engine.ErrNoMetric):
		return http.StatusServiceUnavailable
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
	code, message, ok := domain.Public(err)
	if !ok {
		code, message = "internal", "internal server error"
		slog.ErrorContext(r.Context(), "request failed",
			slog.String("request_id", requestIDFrom(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)
	}

	writeJSON(w, statusFor(err), errorBody{
		Code:      code,
		Message:   message,
		RequestID: requestIDFrom(r.Context()),
	})
}
