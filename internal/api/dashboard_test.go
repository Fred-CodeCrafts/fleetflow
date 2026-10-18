package api

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// dataMetric matches one data-metric attribute value in the dashboard page: the
// path a metrics-strip cell names into GET /api/metrics.
var dataMetric = regexp.MustCompile(`data-metric="([^"]+)"`)

// TestDashboardMetricPathsResolve is the contract between the page and the
// metrics endpoint: every cell in the strip names a path that GET /api/metrics
// actually answers with a number. Without it, a metric renamed on the server
// would leave the cell showing an em dash with nothing failing, which is exactly
// the kind of quiet break this dashboard cannot afford.
func TestDashboardMetricPathsResolve(t *testing.T) {
	s := simServer(t)

	// The arm paths exist only while a run does, so the run is started the way
	// the controls start one before the metrics are read.
	postJSON(t, s, "/api/sim/start", `{"seed":7,"vehicles":4,"fleet_share":0.5,"window_s":0,"step_s":1}`)

	page := getJSON(t, s, "/")
	if page.Code != http.StatusOK {
		t.Fatalf("GET / answered %d, want 200", page.Code)
	}

	matches := dataMetric.FindAllStringSubmatch(page.Body.String(), -1)
	if len(matches) == 0 {
		t.Fatal("the page names no data-metric path: the scan is reading the wrong body")
	}

	metrics := decodeBody(t, getJSON(t, s, "/api/metrics"))

	for _, match := range matches {
		path := match[1]
		value := metricValue(metrics, path)
		if _, ok := value.(float64); !ok {
			t.Errorf("data-metric %q resolves to %T (%v), want a number: a renamed metric would blank this cell", path, value, value)
		}
	}
}

// metricValue resolves one data-metric path against a decoded metrics body. The
// two comparison arms live under the run, so a path that starts with an arm is
// resolved there; everything else is resolved from the body's root. This is the
// rule web/app.js's metricAt applies to the same paths.
func metricValue(body map[string]any, path string) any {
	parts := strings.Split(path, ".")

	var node any = body
	if parts[0] == "baseline" || parts[0] == "fleet_flow" {
		node = body["run"]
	}

	for _, part := range parts {
		object, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node = object[part]
	}
	return node
}
