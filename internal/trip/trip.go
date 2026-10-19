// Package trip holds FleetFlow's trip record: the domain type, the rules a trip
// must satisfy, and the service that applies them (DESIGN 3.7).
//
// The layer order is handler (HTTP) → Service → Repository. The Repository
// interface is declared in this package so the service owns the seam: a memory
// store backs the unit tests and a database-free run, and a Postgres store can
// replace it later without the service noticing. Nothing here imports net/http
// or a database package.
package trip

import (
	"strings"
	"time"

	"fleetflow/internal/domain"
)

// Error codes. They travel in API error bodies, so they are part of the public
// contract and must stay stable (see domain.Error and DESIGN 3.8).
const (
	codeOriginRequired      = "trip_origin_required"
	codeDestinationRequired = "trip_destination_required"
	codeStatusUnknown       = "trip_status_unknown"
)

// Status is a trip's progress: "planned", "in_progress" or "completed". It is
// what a dispatcher tracks while a vehicle carries out the trip.
type Status uint8

const (
	// StatusPlanned is scheduled but not yet started.
	StatusPlanned Status = iota
	// StatusInProgress is underway.
	StatusInProgress
	// StatusCompleted has finished.
	StatusCompleted
)

// statusNames maps each status to its wire form. It is the single source of
// truth for String and ParseStatus, so the two cannot drift apart.
var statusNames = [...]string{
	StatusPlanned:    "planned",
	StatusInProgress: "in_progress",
	StatusCompleted:  "completed",
}

// String returns the wire name of the status, which is also the string a
// dashboard sends back on create. An out-of-range value returns "unknown"
// rather than pretending to be a real status.
func (s Status) String() string {
	if s.Valid() {
		return statusNames[s]
	}
	return "unknown"
}

// Valid reports whether s is one of the three known statuses.
func (s Status) Valid() bool { return uint8(s) < uint8(len(statusNames)) }

// Advances reports whether moving from s to next is a move forwards in the trip's
// life, which is the only direction allowed: planned, then in progress, then
// completed. A finished trip is a record of what happened, so it must not be
// un-finished. Setting the status a trip already has is allowed and changes
// nothing, so a retried request is not an error.
func (s Status) Advances(next Status) bool { return next >= s }

// ParseStatus turns the wire name of a status ("planned", "in_progress",
// "completed") into a Status. The name is matched exactly: the empty string and
// anything else are rejected, because a status is a machine token and a blank
// one must not be silently read as planned.
func ParseStatus(name string) (Status, error) {
	for i, n := range statusNames {
		if name == n {
			return Status(i), nil
		}
	}
	return 0, domain.Invalid(codeStatusUnknown,
		"unknown trip status %q: want one of %s", name, strings.Join(statusNames[:], ", "))
}

// Trip is one journey in the fleet: it moves from an origin to a destination
// using a vehicle. Origin and Destination are free-form human text, so the
// service only checks that they are not blank.
//
// StartedAt and FinishedAt are set by the transitions that cause them, not by
// the client: a trip is started when its status first reaches in_progress and
// finished when it first reaches completed. A zero value means "has not
// happened yet", which is what makes the duration computable rather than
// guessed. PRD US-9 asks a completed trip to be listed with start, end and
// duration, and a created_at timestamp alone cannot answer that.
type Trip struct {
	ID          string
	Origin      string
	Destination string
	VehicleID   string
	Status      Status
	StartedAt   time.Time
	FinishedAt  time.Time
}

// Duration returns how long the trip took, and false when it has not finished.
//
// A trip that finished without a start recorded (a row written before this
// field existed, or one moved straight from planned to completed) reports
// false rather than zero: "we do not know" and "it took no time" are different
// answers and the console shows them differently.
func (t Trip) Duration() (time.Duration, bool) {
	if t.StartedAt.IsZero() || t.FinishedAt.IsZero() {
		return 0, false
	}
	d := t.FinishedAt.Sub(t.StartedAt)
	if d < 0 {
		// Clock skew between a client and the database can produce this. A
		// negative duration is not a measurement, so it is not reported.
		return 0, false
	}
	return d, true
}

// New validates the client-supplied fields and returns a Trip. The id is left
// empty: Service.Create assigns one, and a repository fills it in when it loads
// a row.
//
// The rules are the ones a client can actually get wrong: the origin and
// destination must not be blank and the status must be one of the three known
// names. Human fields are trimmed; the status is matched exactly, so a mistyped
// token is reported instead of guessed at.
func New(origin, destination, vehicleID, status string) (Trip, error) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return Trip{}, domain.Invalid(codeOriginRequired, "trip origin is required")
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return Trip{}, domain.Invalid(codeDestinationRequired, "trip destination is required")
	}
	st, err := ParseStatus(status)
	if err != nil {
		return Trip{}, err
	}
	return Trip{
		Origin:      origin,
		Destination: destination,
		VehicleID:   strings.TrimSpace(vehicleID),
		Status:      st,
	}, nil
}
