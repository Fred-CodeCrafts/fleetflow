package sumo

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestWithEmissionClassPinsGeneratedPassengerType(t *testing.T) {
	source := []byte(`<?xml version="1.0"?><routes><vType id="fleetflow_passenger" vClass="passenger"/><trip id="fleetflow0" type="fleetflow_passenger"/></routes>`)
	pinned, err := withEmissionClass(source)
	if err != nil {
		t.Fatal(err)
	}

	decoder := xml.NewDecoder(strings.NewReader(string(pinned)))
	found := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != "vType" {
			continue
		}
		for _, attribute := range element.Attr {
			if attribute.Name.Local == "emissionClass" && attribute.Value == emissionClassName {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("generated demand did not pin emission class %q: %s", emissionClassName, pinned)
	}
}

func TestWithEmissionClassRejectsMissingOrConflictingType(t *testing.T) {
	for _, test := range []struct {
		name string
		xml  string
	}{
		{name: "missing", xml: `<routes><vType id="other"/></routes>`},
		{name: "conflicting", xml: `<routes><vType id="fleetflow_passenger" emissionClass="Zero"/></routes>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := withEmissionClass([]byte(test.xml)); err == nil {
				t.Fatal("withEmissionClass() accepted invalid generated demand")
			}
		})
	}
}
