// Package vehicle holds FleetFlow's vehicle record: the domain type, the rules
// a vehicle must satisfy, and the service that applies them (DESIGN 3.7).
//
// The layer order is handler (HTTP) → Service → Repository. The Repository
// interface is declared in this package so the service owns the seam: a memory
// store backs the unit tests and a database-free run, and a Postgres store can
// replace it later without the service noticing. Nothing here imports net/http
// or a database package.
package vehicle

import (
	"strings"

	"fleetflow/internal/domain"
)

// Error codes. They travel in API error bodies, so they are part of the public
// contract and must stay stable (see domain.Error and DESIGN 3.8).
const (
	codeNameRequired  = "vehicle_name_required"
	codeStatusUnknown = "vehicle_status_unknown"
)

// Status is a vehicle's availability: "available", "on_trip" or "offline". It
// is what the dispatcher filters on and what a trip assignment checks before it
// starts.
type Status uint8

const (
	// StatusAvailable can be given a trip.
	StatusAvailable Status = iota
	// StatusOnTrip is already on a trip, so it cannot be given another.
	StatusOnTrip
	// StatusOffline is out of service.
	StatusOffline
)

// statusNames maps each status to its wire form. It is the single source of
// truth for String and ParseStatus, so the two cannot drift apart.
var statusNames = [...]string{
	StatusAvailable: "available",
	StatusOnTrip:    "on_trip",
	StatusOffline:   "offline",
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

// ParseStatus turns the wire name of a status ("available", "on_trip",
// "offline") into a Status. The name is matched exactly: the empty string and
// anything else are rejected, because a status is a machine token and a blank
// one must not be silently read as available (PRD US-1).
func ParseStatus(name string) (Status, error) {
	for i, n := range statusNames {
		if name == n {
			return Status(i), nil
		}
	}
	return 0, domain.Invalid(codeStatusUnknown,
		"unknown vehicle status %q: want one of %s", name, strings.Join(statusNames[:], ", "))
}

// Vehicle is one record in the fleet. Name holds a name or a plate, since the
// PRD accepts either. Type is free-form operator metadata (for example "van"),
// not an enum, so the service does not constrain it.
type Vehicle struct {
	ID     string
	Name   string
	Type   string
	Status Status
}

// New validates the client-supplied fields and returns a Vehicle. The id is left
// empty: Service.Create assigns one, and a repository fills it in when it loads
// a row.
//
// The rules are the ones a client can actually get wrong (PRD US-1): the name
// must not be blank and the status must be one of the three known names. Human
// fields (name, type) are trimmed; the status is matched exactly, so a
// mistyped token is reported instead of guessed at.
func New(name, typ, status string) (Vehicle, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Vehicle{}, domain.Invalid(codeNameRequired, "vehicle name is required")
	}
	st, err := ParseStatus(status)
	if err != nil {
		return Vehicle{}, err
	}
	return Vehicle{Name: name, Type: strings.TrimSpace(typ), Status: st}, nil
}
