// Package driver holds FleetFlow's driver record: the domain type, the rules a
// driver must satisfy, and the service that applies them (DESIGN 3.7).
//
// The layer order is handler (HTTP) → Service → Repository. The Repository
// interface is declared in this package so the service owns the seam: a memory
// store backs the unit tests and a database-free run, and a Postgres store can
// replace it later without the service noticing. Nothing here imports net/http
// or a database package.
package driver

import (
	"regexp"
	"strings"
	"time"

	"fleetflow/internal/domain"
)

// Error codes. They travel in API error bodies, so they are part of the public
// contract and must stay stable (see domain.Error and DESIGN 3.8).
const (
	codeNameRequired         = "driver_name_required"
	codePhoneInvalid         = "driver_phone_invalid"
	codeLicenseExpiryInvalid = "driver_license_expiry_invalid"
	codeAvailabilityUnknown  = "driver_availability_unknown"
	codeShiftInvalid         = "driver_shift_invalid"
)

var phonePattern = regexp.MustCompile(`^\+?[0-9(). -]+$`)

// Driver is one person who can be given a vehicle. Unlike a vehicle it carries
// no status: a driver is either on file or not, and any availability is derived
// from the trips that reference them. Contact is free-form operator metadata
// (for example a phone number or an email), not an enum, so the service does
// not constrain it.
type Driver struct {
	ID            string
	Name          string
	Contact       string
	ContactPhone  string
	LicenseClass  string
	LicenseExpiry string
	ShiftStart    string
	ShiftEnd      string
	Availability  string
	ContractType  string
	// VehicleID is the vehicle this driver currently holds, or empty when the
	// driver is unassigned. The field is the assignment itself: a driver has at
	// most one vehicle, and a store additionally rejects a vehicle that another
	// driver already holds (T39).
	VehicleID string
}

// New validates the client-supplied fields and returns a Driver. The id is left
// empty: Service.Create assigns one, and a repository fills it in when it loads
// a row.
//
// The rule is the one a client can actually get wrong (PRD US-1): the name must
// not be blank. Human fields (name, contact) are trimmed.
func New(name, contact string) (Driver, error) {
	return NewProfile(name, contact, "", "", "", "", "", "")
}

// NewProfile validates the driver's optional profile values. Missing values
// remain empty, representing unknown rather than an inferred default.
func NewProfile(name, contact, contactPhone, licenseClass, licenseExpiry, shiftStart, shiftEnd, availability string) (Driver, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Driver{}, domain.Invalid(codeNameRequired, "driver name is required")
	}
	contactPhone = strings.TrimSpace(contactPhone)
	if contactPhone != "" {
		digits := 0
		parentheses := 0
		validPunctuation := true
		for _, char := range contactPhone {
			if char >= '0' && char <= '9' {
				digits++
			} else if char == '(' {
				if parentheses != 0 {
					validPunctuation = false
				}
				parentheses++
			} else if char == ')' {
				if parentheses != 1 {
					validPunctuation = false
				}
				parentheses--
			}
		}
		if !phonePattern.MatchString(contactPhone) || digits < 7 || digits > 15 || parentheses != 0 || !validPunctuation {
			return Driver{}, domain.Invalid(codePhoneInvalid, "contact phone must contain 7 to 15 digits and use standard phone punctuation")
		}
	}
	licenseExpiry = strings.TrimSpace(licenseExpiry)
	if licenseExpiry != "" {
		parsed, err := time.Parse("2006-01-02", licenseExpiry)
		if err != nil || parsed.Format("2006-01-02") != licenseExpiry {
			return Driver{}, domain.Invalid(codeLicenseExpiryInvalid, "license expiry must be a valid date in YYYY-MM-DD format")
		}
	}
	shiftStart = strings.TrimSpace(shiftStart)
	shiftEnd = strings.TrimSpace(shiftEnd)
	if (shiftStart == "") != (shiftEnd == "") {
		return Driver{}, domain.Invalid(codeShiftInvalid, "shift start and end must both be supplied or both be unknown")
	}
	if shiftStart != "" {
		for _, value := range []string{shiftStart, shiftEnd} {
			parsed, err := time.Parse("15:04", value)
			if err != nil || parsed.Format("15:04") != value {
				return Driver{}, domain.Invalid(codeShiftInvalid, "shift times must be valid HH:MM values in 24-hour format")
			}
		}
	}
	availability = strings.TrimSpace(availability)
	if availability != "" && availability != "available" && availability != "unavailable" {
		return Driver{}, domain.Invalid(codeAvailabilityUnknown, "availability must be available or unavailable")
	}
	return Driver{
		Name: name, Contact: strings.TrimSpace(contact), ContactPhone: contactPhone,
		LicenseClass: strings.TrimSpace(licenseClass), LicenseExpiry: licenseExpiry,
		ShiftStart: shiftStart, ShiftEnd: shiftEnd, Availability: availability,
	}, nil
}
