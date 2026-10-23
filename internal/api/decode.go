package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"fleetflow/internal/domain"
)

// maxBodyBytes bounds a request body. Every endpoint here takes a small JSON
// object, so anything larger is a mistake or an attack; refusing it at the door
// stops a careless client from making the service allocate for it.
const maxBodyBytes = 64 << 10 // 64 KiB

// maxBatchBodyBytes bounds the one endpoint whose body is not a small object.
//
// POST /api/reports/batch carries a step's observations, one per occupied edge. A
// city network has thousands of them: the Jakarta graph at 60,000 vehicles sent
// 921 observations in one batch, and the 64 KiB limit refused it as
// body_too_large, which silently cost the run those reports. The bound is sized
// from maxBatchReports at roughly 64 bytes an observation, which is more than one
// carries, so the count limit is what a client actually meets.
const maxBatchBodyBytes = maxBatchReports * 64

// Error codes for input the transport refuses before the domain sees it. They
// travel in API error bodies, so they are part of the public contract.
const (
	codeMalformedBody = "malformed_request"
	codeMalformedJSON = "malformed_json"
	codeEmptyBody     = "empty_body"
	codeUnknownField  = "unknown_field"
	codeBadFieldType  = "bad_field_type"
	codeTrailingData  = "trailing_data"
	codeBodyTooLarge  = "body_too_large"
)

// refusedBodyTooLarge refuses a body that exceeds maxBodyBytes.
var refusedBodyTooLarge = &refusedError{
	status:  http.StatusRequestEntityTooLarge,
	code:    codeBodyTooLarge,
	message: "request body is too large",
}

// decodeJSON reads exactly one JSON object from the request body into v.
//
// It refuses everything a handler should not have to think about: a body over
// maxBodyBytes, malformed JSON, a field the payload does not define, or a second
// value after the object. Unknown fields are rejected on purpose, because a
// misspelled key would otherwise be ignored silently and the request would look
// accepted.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return decodeJSONLimit(w, r, v, maxBodyBytes)
}

// decodeJSONLimit is decodeJSON with an explicit body bound, for the endpoints
// whose payload is legitimately larger than a small object.
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		return decodeError(err)
	}

	// A well-formed request carries one object. Anything after it means the client
	// sent something other than what it thinks it sent, so report it instead of
	// silently ignoring the rest.
	if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return domain.Invalid(codeTrailingData, "request body must contain a single JSON object")
	}

	return nil
}

// decodeError turns a json decoder failure into a typed error a client can act
// on. It never returns the raw error, whose text can carry decoder internals.
func decodeError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return refusedBodyTooLarge
	}
	if errors.Is(err, io.EOF) {
		return domain.Invalid(codeEmptyBody, "request body is required")
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field != "" {
			return domain.Invalid(codeBadFieldType, "field %q must be of type %s", typeErr.Field, typeErr.Type)
		}
		// A mismatch on the whole value names the target type, and every target
		// here is a struct in this package, so the raw text would tell a client
		// it sent an array "where api.routeRequest belongs". Name the shape
		// instead: it is what the client has to change, and it leaks nothing.
		return domain.Invalid(codeBadFieldType, "request body must be a JSON object")
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return domain.Invalid(codeMalformedJSON, "malformed JSON at byte offset %d", syntaxErr.Offset)
	}

	// DisallowUnknownFields reports a plain error with no type to match on, so
	// fall back to its text. It names the offending field, which is the useful
	// part for whoever sent it.
	if strings.Contains(err.Error(), "unknown field") {
		return domain.Invalid(codeUnknownField, "%s", err.Error())
	}

	return domain.Invalid(codeMalformedBody, "request body could not be read")
}
