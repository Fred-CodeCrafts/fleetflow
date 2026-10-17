// Package domain holds types shared across FleetFlow's feature layers, including
// the typed errors a transport maps to status codes.
//
// Nothing here imports net/http or a database package: the routing core, the
// services and the API all depend on this package, so it must stay
// transport-agnostic. Deciding that "conflict" means 409 is the API's job, not
// the domain's (DESIGN 3.8).
package domain

import (
	"errors"
	"fmt"
)

// Kind classifies a failure so a transport can choose a status code without
// knowing which operation failed. The zero value is KindInternal, so an error
// that nobody classified never looks like the client's fault.
type Kind uint8

const (
	// KindInternal is an unexpected failure: the server's problem, not the
	// caller's, and its details must not reach a response body.
	KindInternal Kind = iota
	// KindInvalid is input the domain refuses.
	KindInvalid
	// KindNotFound is a reference to something that does not exist.
	KindNotFound
	// KindConflict is a request that contradicts current state, such as
	// assigning a driver who already has a vehicle.
	KindConflict
	// KindUnavailable is a dependency or capability that is not ready.
	KindUnavailable
)

// String returns the stable, machine-readable name of the kind. It appears in
// API error bodies, so treat it as part of the public contract.
func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindUnavailable:
		return "unavailable"
	default:
		return "internal"
	}
}

// Error is a domain failure: a stable code for clients, a message for humans,
// and an optional cause that is logged but never sent to a client.
type Error struct {
	Kind    Kind
	Code    string
	Message string

	// cause is kept private so callers must go through Error()/Unwrap(), which
	// keeps the internal detail out of any message a transport might echo.
	cause error
}

// Error implements the error interface. The result is for logs: it may include
// the wrapped cause, so never put it in a response body. Use Public for that.
func (e *Error) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.Message + ": " + e.cause.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// WithCause returns a copy of e carrying cause. It copies rather than mutates so
// a package-level error can be reused safely. The cause is available to
// errors.Is/As and to logs, but never to a response body.
func (e *Error) WithCause(cause error) *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.cause = cause
	return &clone
}

// Invalid reports input the domain refuses.
func Invalid(code, format string, args ...any) *Error {
	return newError(KindInvalid, code, format, args...)
}

// NotFound reports a reference to something that does not exist.
func NotFound(code, format string, args ...any) *Error {
	return newError(KindNotFound, code, format, args...)
}

// Conflict reports a request that contradicts current state.
func Conflict(code, format string, args ...any) *Error {
	return newError(KindConflict, code, format, args...)
}

// Unavailable reports a dependency or capability that is not ready.
func Unavailable(code, format string, args ...any) *Error {
	return newError(KindUnavailable, code, format, args...)
}

// Internal reports an unexpected failure. The reason is for the logs; callers
// should attach the underlying error with WithCause.
func Internal(code, format string, args ...any) *Error {
	return newError(KindInternal, code, format, args...)
}

func newError(kind Kind, code, format string, args ...any) *Error {
	return &Error{Kind: kind, Code: code, Message: fmt.Sprintf(format, args...)}
}

// KindOf reports the kind of err, or KindInternal when err is not a domain error
// or is nil.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}

// Public returns the code and message that are safe to send to a client for err.
// The last result is false when err is not a domain error, in which case the
// caller must substitute its own generic message rather than leak internals.
func Public(err error) (code, message string, ok bool) {
	var e *Error
	if !errors.As(err, &e) {
		return "", "", false
	}
	code = e.Code
	if code == "" {
		code = e.Kind.String()
	}
	return code, e.Message, true
}
