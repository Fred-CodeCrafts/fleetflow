package sumo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
)

func pinEmissionClass(path string) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated demand: %w", err)
	}
	pinned, err := withEmissionClass(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, pinned, 0600); err != nil {
		return fmt.Errorf("write generated demand: %w", err)
	}
	return nil
}

func withEmissionClass(source []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(source))
	matched := 0
	insertion := -1

	for {
		tokenStart := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode generated demand XML: %w", err)
		}

		if element, ok := token.(xml.StartElement); ok && element.Name.Local == "vType" {
			typeID := ""
			class := ""
			for _, attribute := range element.Attr {
				switch attribute.Name.Local {
				case "id":
					typeID = attribute.Value
				case "emissionClass":
					class = attribute.Value
				}
			}
			if typeID == defaultVehicleTypeID {
				matched++
				if class != "" && class != emissionClassName {
					return nil, fmt.Errorf("generated vehicle type %q has unexpected emission class %q", typeID, class)
				}
				if class == "" {
					tokenEnd := int(decoder.InputOffset())
					elementStart := bytes.LastIndex(source[:tokenEnd], []byte("<"))
					if elementStart < tokenStart || !bytes.HasPrefix(source[elementStart:tokenEnd], []byte("<vType")) {
						return nil, fmt.Errorf("could not locate generated vehicle type %q in XML", typeID)
					}
					insertion = tokenEnd - 1
					if source[insertion-1] == '/' {
						insertion--
					}
				}
			}
		}
	}
	if matched != 1 {
		return nil, fmt.Errorf("generated demand must contain one vehicle type %q, found %d", defaultVehicleTypeID, matched)
	}
	if insertion < 0 {
		return source, nil
	}
	pinned := make([]byte, 0, len(source)+len(` emissionClass="HBEFA3/PC_G_EU4"`))
	pinned = append(pinned, source[:insertion]...)
	pinned = append(pinned, ` emissionClass="`...)
	pinned = append(pinned, emissionClassName...)
	pinned = append(pinned, '"')
	pinned = append(pinned, source[insertion:]...)
	return pinned, nil
}
