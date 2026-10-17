package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fleetflow/internal/domain"
	"fleetflow/internal/engine"
)

func TestStatusFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"invalid input", domain.Invalid("bad_speed", "speed out of range"), http.StatusBadRequest},
		{"missing record", domain.NotFound("no_vehicle", "no such vehicle"), http.StatusNotFound},
		{"contradicts state", domain.Conflict("driver_busy", "driver already assigned"), http.StatusConflict},
		{"not ready", domain.Unavailable("no_metric", "no metric yet"), http.StatusServiceUnavailable},
		{"untyped error is internal", errors.New("pq: relation does not exist"), http.StatusInternalServerError},
		{"unknown edge", engine.ErrEdgeNotFound, http.StatusNotFound},
		{"no metric published", engine.ErrNoMetric, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusFor(tt.err); got != tt.want {
				t.Errorf("statusFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestStatusForFindsTheCauseInAWrappedError(t *testing.T) {
	// A service that wraps the engine's sentinel must still produce its status.
	err := fmt.Errorf("routing: %w", engine.ErrEdgeNotFound)

	if got := statusFor(err); got != http.StatusNotFound {
		t.Errorf("statusFor(wrapped ErrEdgeNotFound) = %d, want 404", got)
	}
}

func TestWriteError_DomainError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/vehicles", nil)

	writeError(rec, req, domain.Invalid("empty_name", "name must not be empty"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Code != "empty_name" {
		t.Errorf("code = %q, want empty_name", body.Code)
	}
	if body.Message != "name must not be empty" {
		t.Errorf("message = %q", body.Message)
	}
}

func TestWriteError_InternalErrorDoesNotLeakDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/vehicles", nil)

	// A raw driver error: useful in logs, not something to hand a client.
	writeError(rec, req, errors.New(`pq: password authentication failed for user "fleetflow"`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	got := rec.Body.String()
	for _, secret := range []string{"password", "fleetflow", "pq:"} {
		if strings.Contains(got, secret) {
			t.Errorf("body leaked internal detail %q: %s", secret, got)
		}
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Code != "internal" {
		t.Errorf("code = %q, want internal", body.Code)
	}
}

func TestWriteError_DomainErrorDoesNotLeakWrappedCause(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/vehicles/v1", nil)

	err := domain.NotFound("no_vehicle", "vehicle v1 does not exist").
		WithCause(errors.New(`pq: connection to "10.0.0.5:5432" failed`))

	writeError(rec, req, err)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := rec.Body.String(); strings.Contains(got, "10.0.0.5") {
		t.Errorf("body leaked the wrapped cause: %s", got)
	}
}
