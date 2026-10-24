import argparse
import json
import sys
from pathlib import Path

import osmium
from shapely.geometry import LineString, shape
from shapely.prepared import prep


class RoadSelector(osmium.SimpleHandler):
    def __init__(self, boundary, buffer_degrees):
        super().__init__()
        self.expanded_boundary = boundary.buffer(buffer_degrees)
        self.boundary = prep(self.expanded_boundary)
        self.bounds = self.expanded_boundary.bounds
        self.way_ids = set()
        self.node_ids = set()
        self.ways_seen = 0

    def way(self, way):
        self.ways_seen += 1
        if self.ways_seen % 500_000 == 0:
            print(f"Scanned {self.ways_seen:,} OSM ways for boundary intersections.", file=sys.stderr, flush=True)
        if "highway" not in way.tags:
            return

        references = []
        coordinates = []
        west, south, east, north = self.bounds
        min_lon = float("inf")
        min_lat = float("inf")
        max_lon = float("-inf")
        max_lat = float("-inf")
        for node in way.nodes:
            references.append(node.ref)
            if not node.location.valid():
                continue
            lon, lat = node.location.lon, node.location.lat
            coordinates.append((lon, lat))
            min_lon = min(min_lon, lon)
            min_lat = min(min_lat, lat)
            max_lon = max(max_lon, lon)
            max_lat = max(max_lat, lat)

        if len(coordinates) < 2:
            return
        bbox_intersects = not (
            max_lon < west or min_lon > east or max_lat < south or min_lat > north
        )
        if bbox_intersects and self.boundary.intersects(LineString(coordinates)):
            self.way_ids.add(way.id)
            self.node_ids.update(references)


class SelectedRoadWriter(osmium.SimpleHandler):
    def __init__(self, output, way_ids, node_ids):
        super().__init__()
        self.output = output
        self.way_ids = way_ids
        self.node_ids = node_ids
        self.node_count = 0
        self.way_count = 0
        self.ways_seen = 0

    def node(self, node):
        if node.id in self.node_ids:
            self.output.add_node(node)
            self.node_count += 1

    def way(self, way):
        self.ways_seen += 1
        if self.ways_seen % 500_000 == 0:
            print(f"Scanned {self.ways_seen:,} OSM ways while writing selected roads.", file=sys.stderr, flush=True)
        if way.id in self.way_ids:
            self.output.add_way(way)
            self.way_count += 1


def load_boundary(path):
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise argparse.ArgumentTypeError(f"cannot read OSM boundary GeoJSON: {error}") from error

    features = data.get("features", [])
    feature = next(
        (
            item
            for item in features
            if item.get("properties", {}).get("osm_type") == "relation"
            and item.get("properties", {}).get("osm_id") == 6362934
        ),
        None,
    )
    if feature is None:
        raise argparse.ArgumentTypeError(
            "GeoJSON must contain OSM administrative relation 6362934"
        )
    boundary = shape(feature["geometry"])
    if boundary.is_empty or not boundary.is_valid:
        raise argparse.ArgumentTypeError("OSM administrative boundary geometry is invalid")
    return boundary


def extract(source, destination, boundary, buffer_degrees):
    """Write roads intersecting the buffered administrative polygon."""
    print("Scanning OSM PBF for highway segments intersecting the DKI boundary.", flush=True)
    selector = RoadSelector(boundary, buffer_degrees)
    selector.apply_file(str(source), locations=True, idx="sparse_file_array")
    if not selector.way_ids:
        raise RuntimeError("no OSM highway ways intersect the requested bbox")

    print(
        f"Selected {len(selector.way_ids):,} road ways; writing their complete node geometry.",
        flush=True,
    )
    destination.parent.mkdir(parents=True, exist_ok=True)
    with osmium.SimpleWriter(str(destination), overwrite=True) as output:
        writer = SelectedRoadWriter(output, selector.way_ids, selector.node_ids)
        writer.apply_file(str(source))

    if writer.node_count == 0 or writer.way_count == 0:
        raise RuntimeError("the extracted OSM network has no complete road geometry")
    print(
        f"Extracted {writer.way_count:,} highway ways and {writer.node_count:,} nodes.",
        flush=True,
    )


def main():
    parser = argparse.ArgumentParser(
        description="Extract OSM highway ways intersecting a buffered boundary."
    )
    parser.add_argument("source", type=Path, help="Input OSM PBF file")
    parser.add_argument("destination", type=Path, help="Output OSM XML file")
    parser.add_argument("--boundary-geojson", required=True, type=Path)
    parser.add_argument("--buffer-degrees", type=float, default=0.02)
    args = parser.parse_args()

    if not args.source.is_file():
        parser.error(f"source PBF does not exist: {args.source}")
    if args.buffer_degrees < 0 or args.buffer_degrees > 0.1:
        parser.error("--buffer-degrees must be between 0 and 0.1")
    try:
        boundary = load_boundary(args.boundary_geojson)
    except argparse.ArgumentTypeError as error:
        parser.error(str(error))
    extract(args.source, args.destination, boundary, args.buffer_degrees)


if __name__ == "__main__":
    main()
