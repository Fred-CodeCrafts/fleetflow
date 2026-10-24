import tempfile
import unittest
from pathlib import Path

from validate_sumo_net import validate_network


class ValidateSumoNetworkTests(unittest.TestCase):
    def test_accepts_a_network_with_roads_and_geographic_bounds(self):
        with tempfile.TemporaryDirectory() as directory:
            network = Path(directory) / "network.net.xml"
            network.write_text(
                """<net>
<location origBoundary="106.3,-6.4,107.0,-5.2"/>
<edge id="road-1"/>
<edge id="internal-1" function="internal"/>
<junction id="junction-1"/>
<junction id="junction-2"/>
</net>""",
                encoding="utf-8",
            )

            validate_network(network)

    def test_rejects_network_without_geographic_bounds(self):
        with tempfile.TemporaryDirectory() as directory:
            network = Path(directory) / "network.net.xml"
            network.write_text(
                "<net><edge id='road-1'/><junction id='a'/><junction id='b'/></net>",
                encoding="utf-8",
            )

            with self.assertRaisesRegex(ValueError, "location metadata"):
                validate_network(network)


if __name__ == "__main__":
    unittest.main()
