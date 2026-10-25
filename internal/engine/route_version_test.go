package engine

import (
	"errors"
	"testing"
)

func TestRouteWithVersion_ReportsTheSnapshotThatAnswered(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	path, version, err := e.RouteWithVersion("A", "D")
	if err != nil {
		t.Fatalf("RouteWithVersion() = %v", err)
	}
	if len(path.Edges) == 0 {
		t.Fatal("RouteWithVersion() returned an empty path for a reachable pair")
	}
	if want := e.ActiveMetric().Version(); version != want {
		t.Errorf("version = %d, want the active snapshot %d", version, want)
	}
}

func TestRouteWithVersion_FollowsANewSnapshot(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	_, before, err := e.RouteWithVersion("A", "D")
	if err != nil {
		t.Fatalf("RouteWithVersion() = %v", err)
	}

	e.Customize()

	_, after, err := e.RouteWithVersion("A", "D")
	if err != nil {
		t.Fatalf("RouteWithVersion() = %v", err)
	}
	if after <= before {
		t.Errorf("version = %d after a customization, want more than %d", after, before)
	}
}

func TestRouteWithVersion_ReportsNoVersionWhenTheQueryFails(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	_, version, err := e.RouteWithVersion("A", "nowhere")
	if !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("err = %v, want ErrUnknownNode", err)
	}
	if version != 0 {
		t.Errorf("version = %d, want 0 when no snapshot answered", version)
	}
}

func TestRoute_StillAnswersWithoutAVersion(t *testing.T) {
	g := loadFixture(t)
	e := newTestEngine(t, g)

	path, err := e.Route("A", "D")
	if err != nil {
		t.Fatalf("Route() = %v", err)
	}
	if len(path.Edges) == 0 {
		t.Error("Route() returned an empty path for a reachable pair")
	}
}
