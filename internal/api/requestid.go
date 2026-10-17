package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// requestIDHeader is the header a caller may send a correlation ID in, and the
// header the API returns the effective ID in. Echoing a caller-supplied ID lets
// one ID follow a request across hops.
const requestIDHeader = "X-Request-Id"

// maxRequestIDLen bounds a caller-supplied ID so it cannot bloat the logs or the
// response headers.
const maxRequestIDLen = 64

// requestIDKey is the context key for the current request's ID. The private type
// keeps it from colliding with keys other packages store in the same context.
type requestIDKey struct{}

// contextWithRequestID returns a copy of ctx carrying id. It is separate from the
// middleware of the same purpose so a handler can be tested with an ID without
// standing up the whole chain.
func contextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// requestIDFrom returns the request ID carried by ctx, or "" when the context did
// not come through the request ID middleware.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// newRequestID returns a fresh 128-bit ID in hex. It is random rather than
// sequential so the IDs of a running server cannot be counted or guessed from one
// sample.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice. A clock-derived fallback keeps
		// the request served, which matters more than perfect uniqueness here.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// validRequestID reports whether id is safe to echo back and write to a log:
// printable ASCII, non-empty and bounded. Anything else is rejected so a caller
// cannot inject control characters or newlines into the log stream.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}
