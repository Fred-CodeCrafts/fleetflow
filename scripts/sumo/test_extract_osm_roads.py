import argparse
import json
import tempfile
import unittest
from pathlib import Path

import osmium

from extract_osm_roads import extract, load_boundary


OSM_FIXTURE = """<?xml version="1.0" encoding="UTF-8"?>
<osm version="0.6" generator="FleetFlow test">
  <node id="1" lat="-6.50" lon="106.10"/>
  <node id="2" lat="-6.20" lon="106.50"/>
  <node id="3" lat="-6.10" lon="106.60"/>
  <node id="4" lat="-6.20" lon="106.20"/>
  <node id="5" lat="-6.10" lon="106.30"/>
  <way id="10">
    <nd ref="1"/><nd ref="2"/><nd ref="3"/>
    <tag k="highway" v="residential"/>
  </way>
  <way id="20">
    <nd ref="4"/><nd ref="5"/>
    <tag k="highway" v="residential"/>
  </way>
</osm>
"""

BOUNDARY_FIXTURE = {
    "type": "FeatureCollection",
    "features": [
        {
            "type": "Feature",
            "properties": {"osm_type": "relation", "osm_id": 6362934},
            "geometry": {
                "type": "Polygon",
                "coordinates": [[
                    [106.4, -6.3],
                    [106.7, -6.3],
                    [106.7, -6.0],
                    [106.4, -6.0],
                    [106.4, -6.3],
                ]],
            },
        }
    ],
}


class ExtractOSMRoadsTests(unittest.TestCase):
    def test_extracts_intersecting_highways_with_all_referenced_nodes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "source.osm"
            destination = root / "roads.osm"
            boundary_file = root / "boundary.geojson"
            source.write_text(OSM_FIXTURE, encoding="utf-8")
            boundary_file.write_text(json.dumps(BOUNDARY_FIXTURE), encoding="utf-8")

            extract(source, destination, load_boundary(boundary_file), 0)

            class ObjectIDs(osmium.SimpleHandler):
                def __init__(self):
                    super().__init__()
                    self.nodes = set()
                    self.ways = set()

                def node(self, node):
                    self.nodes.add(node.id)

                def way(self, way):
                    self.ways.add(way.id)

            result = ObjectIDs()
            result.apply_file(str(destination))
            self.assertEqual(result.nodes, {1, 2, 3})
            self.assertEqual(result.ways, {10})

    def test_rejects_a_different_osm_relation(self):
        with tempfile.TemporaryDirectory() as directory:
            boundary_file = Path(directory) / "boundary.geojson"
            data = json.loads(json.dumps(BOUNDARY_FIXTURE))
            data["features"][0]["properties"]["osm_id"] = 1
            boundary_file.write_text(json.dumps(data), encoding="utf-8")

            with self.assertRaises(argparse.ArgumentTypeError):
                load_boundary(boundary_file)


if __name__ == "__main__":
    unittest.main()
