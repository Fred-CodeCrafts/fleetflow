package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fleetflow/internal/domain"
)

func TestWithRequestID_GeneratesAnID(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if id := rec.Header().Get(requestIDHeader); !validRequestID(id) {
		t.Fatalf("X-Request-Id = %q, want a generated valid ID", id)
	}
}

func TestWithRequestID_ReusesACallerSuppliedID(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(requestIDHeader, "trace-abc-123")

	s.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != "trace-abc-123" {
		t.Errorf("X-Request-Id = %q, want the caller's ID echoed", got)
	}
}

func TestWithRequestID_ReplacesUnsafeIDs(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"newline could forge a log line", "abc\ndef"},
		{"control character", "abc\x1bdef"},
		{"over the length bound", strings.Repeat("a", maxRequestIDLen+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testServer(t)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			req.Header.Set(requestIDHeader, tt.id)

			s.ServeHTTP(rec, req)

			got := rec.Header().Get(requestIDHeader)
			if got == tt.id {
				t.Fatalf("unsafe ID %q was echoed back unchanged", tt.id)
			}
			if !validRequestID(got) {
				t.Errorf("replacement ID %q is not itself valid", got)
			}
		})
	}
}

func TestGeneratedRequestIDsDiffer(t *testing.T) {
	s := testServer(t)
	seen := make(map[string]bool, 50)

	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		id := rec.Header().Get(requestIDHeader)
		if seen[id] {
			t.Fatalf("duplicate request ID %q after %d requests", id, i)
		}
		seen[id] = true
	}
}

func TestWriteErrorCarriesTheRequestID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/vehicles", nil).
		WithContext(contextWithRequestID(context.Background(), "trace-7"))
	rec := httptest.NewRecorder()

	writeError(rec, req, domain.Invalid("empty_name", "name must not be empty"))

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.RequestID != "trace-7" {
		t.Errorf("request_id = %q, want trace-7", body.RequestID)
	}
}

func TestWithLogging_RecordsOneStructuredLine(t *testing.T) {
	buf := &bytes.Buffer{}
	s := testServer(t, WithLogger(slog.New(slog.NewJSONHandler(buf, nil))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(requestIDHeader, "trace-99")
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.1.1.1")
	req.RemoteAddr = "10.0.0.1:54321"

	s.ServeHTTP(rec, req)

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, buf.String())
	}

	if got := line["request_id"]; got != "trace-99" {
		t.Errorf("log request_id = %v, want trace-99", got)
	}
	if got := line["method"]; got != http.MethodGet {
		t.Errorf("log method = %v", got)
	}
	if got := line["path"]; got != "/healthz" {
		t.Errorf("log path = %v", got)
	}
	if got := line["status"]; got != float64(http.StatusOK) {
		t.Errorf("log status = %v, want 200", got)
	}
	if got := line["remote"]; got != "198.51.100.9" {
		t.Errorf("log remote = %v, want the first forwarded hop", got)
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("log line has no duration_ms")
	}
}

func TestWithLogging_RecordsTheHandlerStatus(t *testing.T) {
	buf := &bytes.Buffer{}
	s := testServer(t, WithLogger(slog.New(slog.NewJSONHandler(buf, nil))))

	rec := httptest.NewRecorder()
	// No such route: the mux answers 404, and the log must say so rather than
	// reporting the 200 default.
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("log line is not JSON: %v", err)
	}
	if got := line["status"]; got != float64(http.StatusNotFound) {
		t.Errorf("log status = %v, want 404", got)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name      string
		remote    string
		forwarded string
		want      string
	}{
		{"first forwarded hop wins", "10.0.0.1:9999", "198.51.100.9, 10.1.1.1", "198.51.100.9"},
		{"single forwarded hop", "10.0.0.1:9999", "198.51.100.9", "198.51.100.9"},
		{"no forwarded header", "192.0.2.5:1234", "", "192.0.2.5"},
		{"blank forwarded entry", "192.0.2.5:1234", "   ", "192.0.2.5"},
		{"remote without a port", "192.0.2.5", "", "192.0.2.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			req.RemoteAddr = tt.remote
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			if got := clientIP(req); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidRequestID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"trace-1", true},
		{"", false},
		{strings.Repeat("a", maxRequestIDLen), true},
		{strings.Repeat("a", maxRequestIDLen+1), false},
		{"has space", false},
		{"tab\there", false},
	}

	for _, tt := range tests {
		if got := validRequestID(tt.id); got != tt.want {
			t.Errorf("validRequestID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}
