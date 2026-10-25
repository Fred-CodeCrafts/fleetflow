// Package fleet defines the light-commercial vehicles a simulated Jakarta fleet
// is made of, and what each class of them burns.
//
// Why it is its own package: the scenario generator needs a vehicle's emission
// class to compute what it burned, and the dispatcher needs its make, model and
// capacity to register it. Keeping the catalogue in one place means the vehicle
// the generator charged for 12 km is the same vehicle the register describes, and
// neither package has to import the other to find out.
//
// What this is not: a measured model. The factors below are planning assumptions
// for congested Jakarta delivery work, stated so the arithmetic behind every
// number in the console can be checked by hand. They are not homologation values
// and not a certified inventory, and the API says so in the provenance it
// returns with every emissions figure.
package fleet

import (
	"math"
	"strconv"
	"strings"
)

// Profile is the specification a class of vehicle is registered with. The
// scenario decides where and how fast a vehicle drives; the profile says what
// the vehicle is.
type Profile struct {
	Make       string
	Model      string
	Type       string
	BodyType   string
	FuelType   string
	Color      string
	Year       int
	CapacityKg float64
	// Class names the emission class in Classes.
	Class string
}

// Class is an emission class: the fuel it burns and how much of it per kilometre.
type Class struct {
	Name string
	Fuel string
	// FuelKgPerKm is the tank-to-wheel fuel burn of this class on urban delivery
	// work. A battery-electric class burns no liquid fuel and is zero here.
	FuelKgPerKm float64
	// CO2PerKgFuel is the carbon content of the fuel, in kilograms of CO2 per
	// kilogram burned. Diesel is about 3.16 and petrol about 3.10; these follow
	// from combustion stoichiometry rather than from a drive cycle, so they are
	// the least contentious numbers in this file.
	CO2PerKgFuel float64
	// Note states the basis of the figure, so an unfamiliar reader does not have
	// to guess whether it is measured.
	Note string
}

// Classes are the emission classes a simulated fleet vehicle can belong to.
//
// The fuel figures are planning assumptions for congested urban work: a loaded
// 14 t box truck creeps along at 30-35 litres per 100 km, a 4.5 t box truck at
// about 20, a diesel van at about 13, a small petrol van at about 12, and an
// electric cargo motorcycle at nothing at the tailpipe. They exist so that a
// truck and a motorcycle on the same kilometre produce different numbers, which
// a single gram-per-kilometre constant cannot do.
var Classes = map[string]Class{
	"diesel-heavy": {
		Name:         "Diesel heavy truck",
		Fuel:         "diesel",
		FuelKgPerKm:  0.30,
		CO2PerKgFuel: 3.16,
		Note:         "14 t box truck, congested urban delivery, about 35 l/100 km",
	},
	"diesel-medium": {
		Name:         "Diesel medium truck",
		Fuel:         "diesel",
		FuelKgPerKm:  0.18,
		CO2PerKgFuel: 3.16,
		Note:         "4.5 t box truck, urban delivery, about 21 l/100 km",
	},
	"diesel-van": {
		Name:         "Diesel van",
		Fuel:         "diesel",
		FuelKgPerKm:  0.11,
		CO2PerKgFuel: 3.16,
		Note:         "1.2 t panel van, urban delivery, about 13 l/100 km",
	},
	"petrol-van": {
		Name:         "Petrol van",
		Fuel:         "petrol",
		FuelKgPerKm:  0.09,
		CO2PerKgFuel: 3.10,
		Note:         "0.8 t blind van, urban delivery, about 12 l/100 km",
	},
	"electric-light": {
		Name:         "Electric cargo motorcycle",
		Fuel:         "electric",
		FuelKgPerKm:  0,
		CO2PerKgFuel: 0,
		Note:         "Battery-electric: no tailpipe CO2; grid energy is not counted",
	},
}

// profiles are the seven light-commercial vehicles a simulated plate can be.
// A plate is assigned one by its number, so the same vehicle keeps the same
// specification on every dispatch and every scenario.
var profiles = []Profile{
	{Make: "Isuzu", Model: "Giga FVM Heavy Cargo", Type: "truck", BodyType: "Heavy Cargo Box", FuelType: "diesel", Color: "White", Year: 2022, CapacityKg: 14000, Class: "diesel-heavy"},
	{Make: "Mitsubishi", Model: "Fuso Canter 136 HDL", Type: "truck", BodyType: "Box Truck", FuelType: "diesel", Color: "Yellow", Year: 2023, CapacityKg: 4500, Class: "diesel-medium"},
	{Make: "Toyota", Model: "HiAce Commuter Cargo", Type: "van", BodyType: "Van", FuelType: "diesel", Color: "Silver", Year: 2023, CapacityKg: 1200, Class: "diesel-van"},
	{Make: "Daihatsu", Model: "Gran Max Blind Van", Type: "van", BodyType: "Blind Van", FuelType: "petrol", Color: "White", Year: 2024, CapacityKg: 800, Class: "petrol-van"},
	{Make: "Honda", Model: "PCX Electric Cargo", Type: "motorcycle", BodyType: "Motorcycle Cargo", FuelType: "electric", Color: "Blue", Year: 2024, CapacityKg: 60, Class: "electric-light"},
	{Make: "Hino", Model: "Dutro 130 HD Cargo", Type: "truck", BodyType: "Box Truck", FuelType: "diesel", Color: "Green", Year: 2023, CapacityKg: 4500, Class: "diesel-medium"},
	{Make: "Wuling", Model: "Formo Blind Van", Type: "van", BodyType: "Blind Van", FuelType: "petrol", Color: "White", Year: 2023, CapacityKg: 650, Class: "petrol-van"},
}

// For returns the profile for a plate, chosen from its number so a plate always
// describes the same vehicle.
func For(plate string) Profile {
	number := plateNumber(plate)
	return profiles[(number-9001)%len(profiles)]
}

// plateNumber reads the digits out of a plate, defaulting to the first plate in
// the sequence when it carries none.
func plateNumber(plate string) int {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, plate)
	number, err := strconv.Atoi(digits)
	if err != nil || number < 9001 {
		return 9001
	}
	return number
}

// Name is what the register calls a vehicle: the unit number the fleet works in.
//
// The plate is not a name — it is the registration, and the register shows the
// two side by side — so the name is derived from the plate number and stays the
// same for that vehicle on every dispatch.
func Name(plate string) string {
	return "Unit " + strconv.Itoa(plateNumber(plate)-9000)
}

// Emission returns the class with this name, and whether the catalogue has it.
func Emission(className string) (Class, bool) {
	class, ok := Classes[className]
	return class, ok
}

// Burn returns the fuel and CO2 of travelling distanceM metres in a class.
//
// An unknown class is reported rather than treated as zero: charging nothing for
// a vehicle nobody described would quietly understate the fleet total. Figures
// are rounded to the gram, which is finer than the model justifies and coarse
// enough that the JSON stays readable.
func Burn(className string, distanceM float64) (fuelKg, co2Kg float64, ok bool) {
	class, ok := Emission(className)
	if !ok || distanceM <= 0 {
		return 0, 0, ok
	}
	km := distanceM / 1000
	fuelKg = math.Round(class.FuelKgPerKm*km*1000) / 1000
	co2Kg = math.Round(fuelKg*class.CO2PerKgFuel*1000) / 1000
	return fuelKg, co2Kg, true
}
