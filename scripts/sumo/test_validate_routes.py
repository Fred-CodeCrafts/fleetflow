import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path
from unittest.mock import patch, MagicMock

from validate_routes import sample_edges, build_trips_xml, validate_routes


NETWORK_FIXTURE = """<?xml version="1.0" encoding="UTF-8"?>
<net version="1.20">
    <edge id="edge_1" function="normal"/>
    <edge id=":internal_1" function="internal"/>
    <edge id="edge_2" function="normal"/>
    <edge id="edge_3" function="normal"/>
</net>
"""


class ValidateRoutesTests(unittest.TestCase):
    def test_sample_edges_excludes_internal_edges(self):
        with tempfile.NamedTemporaryFile("w+", suffix=".xml", delete=False) as f:
            f.write(NETWORK_FIXTURE)
            f.flush()
            path = Path(f.name)
        try:
            edges = sample_edges(path, max_edges=10)
            self.assertEqual(edges, ["edge_1", "edge_2", "edge_3"])
        finally:
            path.unlink(missing_ok=True)

    def test_build_trips_xml_generates_valid_trips(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            out_file = Path(tmpdir) / "trips.xml"
            count = build_trips_xml(["e1", "e2", "e3"], out_file)
            self.assertEqual(count, 2)
            tree = ET.parse(out_file)
            trips = tree.getroot().findall("trip")
            self.assertEqual(len(trips), 2)
            self.assertEqual(trips[0].attrib["from"], "e1")
            self.assertEqual(trips[0].attrib["to"], "e2")

    @patch("subprocess.run")
    def test_validate_routes_runs_duarouter_and_parses_output(self, mock_run):
        mock_run.return_value = MagicMock(returncode=0, stderr="")
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            routes_file = tmp / "routes.xml"
            routes_file.write_text(
                """<routes><vehicle id="t0"><route edges="e1 e2"/></vehicle></routes>""",
                encoding="utf-8",
            )
            count = validate_routes(
                Path("duarouter"), tmp / "net.xml", tmp / "trips.xml", routes_file
            )
            self.assertEqual(count, 2) # vehicle and route tags


if __name__ == "__main__":
    unittest.main()
