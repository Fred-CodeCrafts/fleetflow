// Package run holds FleetFlow's saved comparison run: the demand a run was
// started with and the metrics it ended with, kept as the API serialized them
// (DESIGN 3.5, 3.7; PRD US-9).
//
// The layer order is handler (HTTP) → Service → Repository. The Repository
// interface is declared here, in the service package, so the service owns the
// seam: a memory store backs the unit tests and a database-free run, and a
// Postgres store replaces it without the service noticing. Nothing here imports
// net/http or a database package.
package run

import "time"

// Run is one saved comparison run: the demand it was started with and the metrics
// it ended with, kept as the API serialized them.
//
// Config and Metrics are the raw JSON documents the transport already produced,
// not decoded structs. The API owns their shape, so this package never parses
// them: it stores the bytes it was handed and returns them unchanged, which is
// what keeps the database and the API from disagreeing about a field neither
// added.
type Run struct {
	// ID identifies the record, minted by the service so a run is never stored
	// without one.
	ID string
	// Seed is the demand seed the run used. Any int64 is valid, zero and
	// negatives included: only the simulator may say what a seed means.
	Seed int64
	// Config is the run's configuration document, as the API serialized it.
	Config []byte
	// Metrics is the run's result metrics document, as the API serialized it.
	Metrics []byte
	// CreatedAt is when the run was saved, set by the store.
	CreatedAt time.Time
}
