package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fleetflow/internal/domain"
)

// testPayload is a stand-in for a real request body.
type testPayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestDecodeJSON_AcceptsOneWellFormedObject(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/vehicles", strings.NewReader(`{"name":"van-1","count":2}`))

	var got testPayload
	if err := decodeJSON(rec, req, &got); err != nil {
		t.Fatalf("decodeJSON() error = %v, want nil", err)
	}
	if got.Name != "van-1" || got.Count != 2 {
		t.Errorf("decoded %+v, want name van-1 and count 2", got)
	}
}

func TestDecodeJSON_RejectsBadInput(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "malformed json",
			body:     `{"name": }`,
			wantCode: codeMalformedJSON,
		},
		{
			name:     "empty body",
			body:     ``,
			wantCode: codeEmptyBody,
		},
		{
			name:     "unknown field",
			body:     `{"name":"van-1","colour":"red"}`,
			wantCode: codeUnknownField,
			wantMsg:  "colour",
		},
		{
			name:     "wrong field type",
			body:     `{"count":"two"}`,
			wantCode: codeBadFieldType,
			wantMsg:  "count",
		},
		{
			name:     "not an object",
			body:     `["van-1"]`,
			wantCode: codeBadFieldType,
		},
		{
			name:     "trailing data",
			body:     `{"name":"van-1"}{"name":"van-2"}`,
			wantCode: codeTrailingData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/vehicles", strings.NewReader(tt.body))

			err := decodeJSON(rec, req, &testPayload{})

			if err == nil {
				t.Fatal("decodeJSON() error = nil, want a rejection")
			}
			code, msg, ok := domain.Public(err)
			if !ok {
				t.Fatalf("decode error %v is not a typed domain error", err)
			}
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("message = %q, want it to name %q", msg, tt.wantMsg)
			}
			if status := statusFor(err); status != http.StatusBadRequest {
				t.Errorf("statusFor() = %d, want 400", status)
			}
		})
	}
}

func TestDecodeJSON_RejectsAnOversizedBody(t *testing.T) {
	rec := httptest.NewRecorder()
	// A well-formed object that is simply too big: the limit must stop it before
	// the decoder finishes building the string.
	body := `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/vehicles", strings.NewReader(body))

	err := decodeJSON(rec, req, &testPayload{})

	if !errors.Is(err, errBodyTooLarge) {
		t.Fatalf("decodeJSON() error = %v, want errBodyTooLarge", err)
	}
	if status := statusFor(err); status != http.StatusRequestEntityTooLarge {
		t.Errorf("statusFor() = %d, want 413", status)
	}
}

func TestDecodeJSON_ReportsRatherThanLeaksDecoderInternals(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/vehicles", strings.NewReader(`{"name": }`))

	_, msg, ok := domain.Public(decodeJSON(rec, req, &testPayload{}))
	if !ok {
		t.Fatal("decode error is not a typed domain error")
	}
	if strings.Contains(msg, "json:") {
		t.Errorf("message = %q, want it not to echo decoder internals", msg)
	}
}
