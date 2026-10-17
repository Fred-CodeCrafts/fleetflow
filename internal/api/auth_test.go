package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fleetflow/internal/auth"
)

// postJSONAs sends body to path with a chosen Authorization header, or none when
// header is empty, so a test can present the key it means to.
func postJSONAs(t *testing.T, s *Server, path, body, header string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if header != "" {
		req.Header.Set("Authorization", header)
	}

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// TestIngest_RefusesEveryKindOfBadKey is the T40 done-when: a missing or wrong key
// gets 401. Every way of getting it wrong has to look the same, or the response
// would tell an attacker which part of a key was right.
func TestIngest_RefusesEveryKindOfBadKey(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"another scheme", "Basic " + testAPIKey},
		{"scheme without a credential", "Bearer"},
		{"empty credential", "Bearer "},
		{"unknown key", "Bearer flf_not_a_key_at_all"},
		{"truncated key", "Bearer " + testAPIKey[:len(testAPIKey)-1]},
		{"key without its prefix", "Bearer " + strings.TrimPrefix(testAPIKey, auth.KeyPrefix)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, path := range []string{"/api/reports", "/api/route"} {
				rec := postJSONAs(t, s, path, `{}`, tt.header)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("%s status = %d, want 401: %s", path, rec.Code, rec.Body)
				}
				if code := field(t, decodeBody(t, rec), "code"); code != "unauthenticated" {
					t.Errorf("%s code = %q, want unauthenticated", path, code)
				}
			}
		})
	}
}

// TestIngest_AcceptsTheStoredKey is the other half of the requirement: the key the
// server holds still gets in, and the endpoint then does its ordinary work.
func TestIngest_AcceptsTheStoredKey(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	rec := postJSONAs(t, s, "/api/reports",
		`{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":13.9}`,
		"Bearer "+testAPIKey)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want 202: %s", rec.Code, rec.Body)
	}

	// The scheme is matched case-insensitively, as it is everywhere else.
	rec = postJSONAs(t, s, "/api/route", `{"from_node":"A","to_node":"D"}`, "bearer "+testAPIKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("route status = %d, want 200: %s", rec.Code, rec.Body)
	}
}

// TestReport_UnauthenticatedWritesNothing is the "and does nothing" half of the
// requirement. The report names an edge and crawls on it, so if the refused
// request had reached the weight store that edge would cost more afterwards.
func TestReport_UnauthenticatedWritesNothing(t *testing.T) {
	g := testGraph(t)
	e := newEngine(t, g)
	s := testServer(t, WithEngine(e))

	_, index, ok := g.EdgeByID("e_AB")
	if !ok {
		t.Fatal("the fixture graph has no e_AB edge")
	}
	before := e.ActiveMetric().Cost(index)

	rec := postJSONAs(t, s, "/api/reports",
		`{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":1}`,
		"Bearer flf_wrong_key")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body)
	}

	e.Customize()
	if after := e.ActiveMetric().Cost(index); after != before {
		t.Errorf("edge e_AB costs %v after a refused report, want it unchanged at %v", after, before)
	}
}

// TestIngest_ReportsAMissingKeyStore keeps "I cannot check keys" separate from
// "your key is wrong": a server with no key service answers 503, because a client
// cannot fix that by presenting a different key.
func TestIngest_ReportsAMissingKeyStore(t *testing.T) {
	g := testGraph(t)
	s := NewServer(g,
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		WithEngine(newEngine(t, g)),
	)

	rec := postJSONAs(t, s, "/api/reports", `{}`, "Bearer "+testAPIKey)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	if code := field(t, decodeBody(t, rec), "code"); code != "auth_unavailable" {
		t.Errorf("code = %q, want auth_unavailable", code)
	}
}
