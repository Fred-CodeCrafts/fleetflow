package weights

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Params holds the tunable values of the dynamic cost model. The defaults are
// the starting points from DESIGN.md; they are meant to be tuned by experiment.
type Params struct {
	Alpha  float64       // penalty strength in the cost function
	Beta   float64       // how sharply the penalty rises with congestion
	Gamma  float64       // smoothing rate per report, in (0, 1]
	Lambda float64       // decay rate per quiet cycle, in [0, 1)
	Delta  time.Duration // customization interval
	Hold   time.Duration // grace period after a report before decay starts
}

// DefaultParams returns the starting parameters from DESIGN.md.
func DefaultParams() Params {
	return Params{
		Alpha:  1.5,
		Beta:   2,
		Gamma:  0.3,
		Lambda: 0.1,
		Delta:  5 * time.Second,
		Hold:   10 * time.Second,
	}
}

var (
	ErrInvalidAlpha  = errors.New("alpha must be finite and >= 0")
	ErrInvalidBeta   = errors.New("beta must be finite and > 0")
	ErrInvalidGamma  = errors.New("gamma must be finite and in (0, 1]")
	ErrInvalidLambda = errors.New("lambda must be finite and in [0, 1)")
	ErrInvalidDelta  = errors.New("delta must be > 0")
	ErrInvalidHold   = errors.New("hold must be >= 0")
)

// Validate reports every parameter that is out of range. It returns nil when
// all of them are usable; otherwise the result matches all failing sentinels
// with errors.Is.
func (p Params) Validate() error {
	var errs []error

	if isBadFloat(p.Alpha) || p.Alpha < 0 {
		errs = append(errs, fmt.Errorf("%w: got %g", ErrInvalidAlpha, p.Alpha))
	}
	if isBadFloat(p.Beta) || p.Beta <= 0 {
		errs = append(errs, fmt.Errorf("%w: got %g", ErrInvalidBeta, p.Beta))
	}
	if isBadFloat(p.Gamma) || p.Gamma <= 0 || p.Gamma > 1 {
		errs = append(errs, fmt.Errorf("%w: got %g", ErrInvalidGamma, p.Gamma))
	}
	if isBadFloat(p.Lambda) || p.Lambda < 0 || p.Lambda >= 1 {
		errs = append(errs, fmt.Errorf("%w: got %g", ErrInvalidLambda, p.Lambda))
	}
	if p.Delta <= 0 {
		errs = append(errs, fmt.Errorf("%w: got %s", ErrInvalidDelta, p.Delta))
	}
	if p.Hold < 0 {
		errs = append(errs, fmt.Errorf("%w: got %s", ErrInvalidHold, p.Hold))
	}

	return errors.Join(errs...)
}

// isBadFloat reports whether v is NaN or infinite.
func isBadFloat(v float64) bool {
	return math.IsNaN(v) || math.IsInf(v, 0)
}
