import argparse
import subprocess
import sys
import tempfile
import xml.etree.ElementTree as ET
from pathlib import Path


def sample_edges(network_path, max_edges=20):
    edges = []
    for event, element in ET.iterparse(network_path, events=("end",)):
        if element.tag == "edge" and element.attrib.get("function") != "internal":
            edge_id = element.attrib.get("id")
            if edge_id and not edge_id.startswith(":"):
                edges.append(edge_id)
                if len(edges) >= max_edges * 4:
                    element.clear()
                    break
        element.clear()
    if len(edges) < 2:
        raise ValueError(f"network {network_path} has fewer than 2 valid edges")
    step = max(1, len(edges) // max_edges)
    return edges[::step][:max_edges]


def build_trips_xml(edges, output_path):
    root = ET.Element("routes")
    vtype = ET.SubElement(root, "vType", id="passenger_test", vClass="passenger")
    trip_count = 0
    for i in range(len(edges) - 1):
        from_edge = edges[i]
        to_edge = edges[i + 1]
        if from_edge != to_edge:
            ET.SubElement(
                root,
                "trip",
                attrib={
                    "id": f"test_trip_{trip_count}",
                    "depart": str(float(trip_count * 10)),
                    "from": from_edge,
                    "to": to_edge,
                    "type": "passenger_test",
                },
            )
            trip_count += 1
    tree = ET.ElementTree(root)
    tree.write(output_path, encoding="utf-8", xml_declaration=True)
    return trip_count


def validate_routes(duarouter_bin, network_path, trips_path, routes_path):
    cmd = [
        str(duarouter_bin),
        "-n", str(network_path),
        "--route-files", str(trips_path),
        "--output-file", str(routes_path),
        "--seed", "42",
        "--ignore-errors", "true",
        "--no-warnings", "true",
    ]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"duarouter failed (exit {result.returncode}): {result.stderr}")

    routes_found = 0
    for event, element in ET.iterparse(routes_path, events=("end",)):
        if element.tag in ("vehicle", "route"):
            routes_found += 1
        element.clear()
    return routes_found


def main():
    parser = argparse.ArgumentParser(description="Validate network connectivity and SUMO routing.")
    parser.add_argument("network", type=Path, help="Path to SUMO .net.xml file")
    parser.add_argument("--duarouter", type=Path, default=Path("duarouter"), help="Path to duarouter executable")
    args = parser.parse_args()

    if not args.network.is_file():
        parser.error(f"network file not found: {args.network}")

    with tempfile.TemporaryDirectory() as tmpdir:
        tmp = Path(tmpdir)
        trips_file = tmp / "trips.xml"
        routes_file = tmp / "routes.rou.xml"

        edges = sample_edges(args.network)
        trip_count = build_trips_xml(edges, trips_file)
        print(f"Generated {trip_count} representative route queries across {len(edges)} sampled edges.")

        routed = validate_routes(args.duarouter, args.network, trips_file, routes_file)
        if routed == 0:
            sys.exit(f"Validation failed: 0 routes generated for {trip_count} test trips.")
        print(f"Validation passed: {routed}/{trip_count} representative routes successfully planned on network.")


if __name__ == "__main__":
    main()
