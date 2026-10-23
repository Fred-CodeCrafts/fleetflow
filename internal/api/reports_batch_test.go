package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReportBatch_AcceptsAStepSizedBatch is the regression guard for a silent
// data loss.
//
// The endpoint inherited the 64 KiB body limit every other endpoint uses, which is
// right for a small JSON object and wrong for a batch. A 60,000-vehicle step on
// the Jakarta network sends around 900 observations, and the run logged
//
//	413 body_too_large: request body is too large
//
// once per step, losing every report in it: the weights never learned the speed
// and the arm silently degraded to free flow. The limit is now sized from the
// observation count instead.
func TestReportBatch_AcceptsAStepSizedBatch(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	const observations = 921
	var body strings.Builder
	body.WriteString(`{"observations":[`)
	for i := 0; i < observations; i++ {
		if i > 0 {
			body.WriteString(",")
		}
		fmt.Fprintf(&body, `{"vehicle_id":"v%04d","edge_id":"e_AB","speed_mps":10}`, i)
	}
	body.WriteString(`]}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/reports/batch", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), fmt.Sprintf(`"accepted":%d`, observations)) {
		t.Errorf("body = %s, want %d accepted", rec.Body.String(), observations)
	}
}

// TestReportBatch_RefusesAMalformedBatchWhole checks the batch is all-or-nothing:
// one bad observation must not leave the weights half updated.
func TestReportBatch_RefusesAMalformedBatchWhole(t *testing.T) {
	s := testServer(t, WithEngine(newEngine(t, testGraph(t))))

	// The second observation has no edge, so the whole request is invalid.
	body := `{"observations":[
		{"vehicle_id":"v1","edge_id":"e_AB","speed_mps":10},
		{"vehicle_id":"v2","speed_mps":10}
	]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/reports/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), codeEdgeIDRequired) {
		t.Errorf("body = %s, want code %s", rec.Body.String(), codeEdgeIDRequired)
	}
}
