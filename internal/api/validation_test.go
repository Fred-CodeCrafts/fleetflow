package api

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
)

// T45: bad input, judged at two levels. decodeJSON is tested as a unit beside
// itself; these tests judge the endpoints by what a bad client actually
// receives. A helper that refuses everything correctly is worth nothing if a
// handler answers 500, or accepts the request anyway.
func TestIngest_BadBodiesAreRefusedWithACode(t *testing.T) {
	// Every endpoint that takes a JSON body and can be served without a
	// database, each with a body it does accept. The accepted body is sent
	// first: if it did not work, the broken ones below would prove nothing.
	endpoints := []struct {
		path string
		ok   string
	}{
		{"/api/reports", `{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10}`},
		{"/api/route", `{"from_node":"A","to_node":"D"}`},
		{"/api/sim/start", `{}`},
	}

	// Bodies broken in ways that need no knowledge of the fields, so the same
	// set means the same thing on every endpoint.
	broken := []struct {
		name string
		body string
		code string
	}{
		{"no body at all", ``, codeEmptyBody},
		{"not JSON", `nonsense`, codeMalformedJSON},
		{"an array where an object belongs", `[]`, codeBadFieldType},
		{"a string where an object belongs", `"hello"`, codeBadFieldType},
		{"a field the payload does not define", `{"no_such_field":1}`, codeUnknownField},
		{"two objects", `{} {}`, codeTrailingData},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.path, func(t *testing.T) {
			s := simServer(t)

			if rec := postJSON(t, s, endpoint.path, endpoint.ok); rec.Code < 200 || rec.Code > 299 {
				t.Fatalf("the body this table means to contrast with was refused: %d %s",
					rec.Code, rec.Body)
			}

			for _, tt := range broken {
				t.Run(tt.name, func(t *testing.T) {
					rec := postJSON(t, s, endpoint.path, tt.body)

					if rec.Code != http.StatusBadRequest {
						t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
					}
					if got := decodeBody(t, rec)["code"]; got != tt.code {
						t.Errorf("code = %v, want %s", got, tt.code)
					}
				})
			}
		})
	}
}

// TestIngest_RefusalNamesNoGoType pins what a refusal is allowed to say. A
// client that sends the wrong shape should learn that, not the name of the Go
// type that was waiting for it: decodeError's whole point is to keep decoder
// internals out of a public error body.
func TestIngest_RefusalNamesNoGoType(t *testing.T) {
	s := simServer(t)

	rec := postJSON(t, s, "/api/route", `[]`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}

	message, ok := decodeBody(t, rec)["message"].(string)
	if !ok {
		t.Fatalf("message is missing from the refusal: %s", rec.Body)
	}
	if strings.Contains(message, "api.") || strings.Contains(message, "fleetflow/") {
		t.Errorf("message = %q, want no Go type or package name in a public error", message)
	}
}

// TestReport_SpeedBoundIsInclusive pins the bound the refusal itself promises:
// "greater than 0 and at most 90". An off-by-one here is invisible until a
// client sending exactly the limit is refused for no reason.
func TestReport_SpeedBoundIsInclusive(t *testing.T) {
	tests := []struct {
		name   string
		speed  float64
		status int
	}{
		{"a plausible speed", 13.9, http.StatusAccepted},
		{"just under the limit", maxReportSpeedMPS - 0.001, http.StatusAccepted},
		{"exactly the limit", maxReportSpeedMPS, http.StatusAccepted},
		{"just past the limit", maxReportSpeedMPS + 0.001, http.StatusBadRequest},
	}

	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":%v}`, tt.speed)
			rec := postJSON(t, s, "/api/reports", body)

			if rec.Code != tt.status {
				t.Fatalf("speed %v: status = %d, want %d: %s", tt.speed, rec.Code, tt.status, rec.Body)
			}
			if tt.status == http.StatusBadRequest {
				if got := decodeBody(t, rec)["code"]; got != codeSpeedOutOfRange {
					t.Errorf("code = %v, want %s", got, codeSpeedOutOfRange)
				}
			}
		})
	}
}

// TestValidReportSpeed_RefusesWhatJSONCannotCarry covers the two values the
// bounds alone would wave through. NaN is the reason the guard is written as a
// named function: every comparison against it is false, so a bare
// `speed > 0 && speed <= max` would accept it, and an infinity is only caught
// by the upper bound happening to be finite.
func TestValidReportSpeed_RefusesWhatJSONCannotCarry(t *testing.T) {
	for _, speed := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, -1, maxReportSpeedMPS + 1} {
		if validReportSpeed(speed) {
			t.Errorf("validReportSpeed(%v) = true, want false", speed)
		}
	}

	if !validReportSpeed(maxReportSpeedMPS) {
		t.Errorf("validReportSpeed(%v) = false, want true: the bound is inclusive", maxReportSpeedMPS)
	}
}
