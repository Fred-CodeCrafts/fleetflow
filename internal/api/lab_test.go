package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func getLab(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	s := testServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestLabDynamicRunsTheSeededExperiment(t *testing.T) {
	rec := getLab(t, "/api/lab/dynamic?seeds=3&share=0.5")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var got LabDynamicResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Summary.Runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(got.Summary.Runs))
	}
	if got.Config.Seeds != 3 || got.Config.Share != 0.5 {
		t.Fatalf("config not echoed: %+v", got.Config)
	}
	if got.Summary.ImprovementPct <= 0 {
		t.Fatalf("improvement = %.2f%%, want the dynamic arm to do better", got.Summary.ImprovementPct)
	}
	// The provenance rule: a number has to name what produced it.
	if got.Method.Model == "" || got.Method.NotIncluded == "" {
		t.Fatalf("method is missing: %+v", got.Method)
	}
	if got.DurationMS < 0 {
		t.Fatalf("duration = %d ms", got.DurationMS)
	}
	for i, run := range got.Summary.Runs {
		if run.Seed != 1+int64(i) {
			t.Fatalf("run %d seed = %d, want %d", i, run.Seed, 1+i)
		}
		if len(run.Series) != got.Config.Waves {
			t.Fatalf("run %d has %d waves, want %d", i, len(run.Series), got.Config.Waves)
		}
	}
}

func TestLabDynamicIsReproducible(t *testing.T) {
	path := "/api/lab/dynamic?seeds=3&share=0.5&waves=6"
	first := getLab(t, path)
	second := getLab(t, path)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("status = %d and %d", first.Code, second.Code)
	}

	var a, b LabDynamicResponse
	if err := json.Unmarshal(first.Body.Bytes(), &a); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if a.Summary.ImprovementPct != b.Summary.ImprovementPct || a.Summary.StaticMeanS != b.Summary.StaticMeanS {
		t.Fatalf("two identical requests disagreed: %+v versus %+v", a.Summary, b.Summary)
	}
}

func TestLabDynamicRejectsBadParameters(t *testing.T) {
	for _, query := range []string{
		"share=2",
		"share=-1",
		"share=half",
		"seeds=0",
		"seeds=1000",
		"waves=0",
		"vehicles=0",
		"grid=1",
		"grid=99",
	} {
		rec := getLab(t, "/api/lab/dynamic?"+query)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", query, rec.Code)
		}
	}
}

// Zero adoption is the control: both arms must be identical, so a reader can see
// that the reported improvement comes from the routing policy and not from noise.
func TestLabDynamicAtZeroShareTiesTheBaseline(t *testing.T) {
	rec := getLab(t, "/api/lab/dynamic?seeds=2&share=0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got LabDynamicResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Summary.ImprovementPct != 0 {
		t.Fatalf("improvement at zero adoption = %g, want 0", got.Summary.ImprovementPct)
	}
	for _, run := range got.Summary.Runs {
		if run.StaticMeanS != run.DynamicMeanS {
			t.Fatalf("seed %d: static %.4f s, dynamic %.4f s", run.Seed, run.StaticMeanS, run.DynamicMeanS)
		}
	}
}
