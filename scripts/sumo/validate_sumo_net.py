import argparse
import xml.etree.ElementTree as ET
from pathlib import Path


def validate_network(path):
    root_seen = False
    location = None
    edges = 0
    junctions = 0

    for event, element in ET.iterparse(path, events=("start", "end")):
        if event == "start" and not root_seen:
            root_seen = True
            if element.tag != "net":
                raise ValueError("SUMO network XML root must be <net>")
        elif event == "end":
            if element.tag == "location" and location is None:
                location = element.attrib
            elif element.tag == "edge" and element.attrib.get("function") != "internal":
                edges += 1
            elif element.tag == "junction":
                junctions += 1
            element.clear()

    if location is None or not location.get("origBoundary"):
        raise ValueError("SUMO network XML is missing geographic location metadata")
    if edges == 0 or junctions < 2:
        raise ValueError("SUMO network XML must contain roads and connected junctions")
    print(f"Validated {edges} roads and {junctions} junctions: {location['origBoundary']}")


def main():
    parser = argparse.ArgumentParser(description="Validate a SUMO network XML file.")
    parser.add_argument("network", type=Path)
    args = parser.parse_args()
    if not args.network.is_file():
        parser.error(f"network file does not exist: {args.network}")
    try:
        validate_network(args.network)
    except (ET.ParseError, OSError, ValueError) as error:
        parser.error(str(error))


if __name__ == "__main__":
    main()
