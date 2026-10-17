package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestKindString(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindInternal, "internal"},
		{KindInvalid, "invalid"},
		{KindNotFound, "not_found"},
		{KindConflict, "conflict"},
		{KindUnavailable, "unavailable"},
		{Kind(99), "internal"}, // unknown kinds must not leak a raw number
	}

	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestConstructorsSetKindCodeAndMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		wantKind Kind
		wantCode string
		wantMsg  string
	}{
		{"invalid", Invalid("bad_speed", "speed_mps %v is out of range", -1), KindInvalid, "bad_speed", "speed_mps -1 is out of range"},
		{"not found", NotFound("no_edge", "edge %q not found", "e42"), KindNotFound, "no_edge", `edge "e42" not found`},
		{"conflict", Conflict("driver_busy", "driver %s already has a vehicle", "d1"), KindConflict, "driver_busy", "driver d1 already has a vehicle"},
		{"unavailable", Unavailable("no_metric", "no metric published"), KindUnavailable, "no_metric", "no metric published"},
		{"internal", Internal("db_error", "query failed"), KindInternal, "db_error", "query failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", tt.err.Kind, tt.wantKind)
			}
			if tt.err.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", tt.err.Code, tt.wantCode)
			}
			if tt.err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", tt.err.Message, tt.wantMsg)
			}
			if !strings.Contains(tt.err.Error(), tt.wantCode) {
				t.Errorf("Error() = %q, want it to contain %q", tt.err.Error(), tt.wantCode)
			}
		})
	}
}

func TestWithCause(t *testing.T) {
	cause := errors.New("connection refused")
	base := Unavailable("db_down", "database is not reachable")

	wrapped := base.WithCause(cause)

	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is(wrapped, cause) = false, want true")
	}
	if !strings.Contains(wrapped.Error(), "connection refused") {
		t.Errorf("Error() = %q, want it to include the cause for logs", wrapped.Error())
	}

	// WithCause must not mutate the original, so a shared error stays reusable.
	if base.cause != nil {
		t.Error("WithCause mutated the receiver; it must return a copy")
	}
	if strings.Contains(base.Error(), "connection refused") {
		t.Errorf("base.Error() = %q, want no cause", base.Error())
	}

	// The cause is internal detail: Public must not expose it.
	if _, msg, _ := Public(wrapped); strings.Contains(msg, "connection refused") {
		t.Errorf("Public message = %q, want it not to leak the cause", msg)
	}
}

func TestKindOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Kind
	}{
		{"invalid", Invalid("x", "x"), KindInvalid},
		{"not found", NotFound("x", "x"), KindNotFound},
		{"conflict", Conflict("x", "x"), KindConflict},
		{"unavailable", Unavailable("x", "x"), KindUnavailable},
		{"plain error", errors.New("boom"), KindInternal},
		{"nil", nil, KindInternal},
		// KindOf reports what it finds rather than guessing, so an unrecognised
		// kind passes through and the transport's default case decides.
		{"unknown kind", &Error{Kind: Kind(42)}, Kind(42)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindOf(tt.err); got != tt.want {
				t.Errorf("KindOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindOfFindsAnErrorBehindAWrap(t *testing.T) {
	err := fmt.Errorf("loading vehicle: %w", NotFound("no_vehicle", "vehicle %s does not exist", "v7"))

	if got := KindOf(err); got != KindNotFound {
		t.Errorf("KindOf() = %v, want %v", got, KindNotFound)
	}
	if _, msg, ok := Public(err); !ok || msg != "vehicle v7 does not exist" {
		t.Errorf("Public() = %q, %v, want the domain message and true", msg, ok)
	}
}

func TestPublic(t *testing.T) {
	t.Run("domain error", func(t *testing.T) {
		code, msg, ok := Public(Invalid("bad_ts", "ts is in the future"))
		if !ok || code != "bad_ts" || msg != "ts is in the future" {
			t.Errorf("Public() = %q, %q, %v", code, msg, ok)
		}
	})

	t.Run("plain error is not public", func(t *testing.T) {
		// The whole point: an unclassified error must not be formatted for a
		// client, so the caller substitutes a generic message.
		if code, msg, ok := Public(errors.New("pq: duplicate key value")); ok || code != "" || msg != "" {
			t.Errorf("Public() = %q, %q, %v, want empty and false", code, msg, ok)
		}
	})

	t.Run("empty code falls back to the kind", func(t *testing.T) {
		if code, _, ok := Public(Conflict("", "already assigned")); !ok || code != "conflict" {
			t.Errorf("Public() code = %q, %v, want \"conflict\" and true", code, ok)
		}
	})

	t.Run("nil", func(t *testing.T) {
		if _, _, ok := Public(nil); ok {
			t.Error("Public(nil) ok = true, want false")
		}
	})
}

func TestWithCauseOnNilReceiver(t *testing.T) {
	var e *Error
	if got := e.WithCause(errors.New("x")); got != nil {
		t.Errorf("nil.WithCause() = %v, want nil", got)
	}
}
