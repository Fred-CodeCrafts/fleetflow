# SUMO road networks

`jakarta.net.xml` is a legacy central-Jakarta SUMO network retained for
reference; FleetFlow does not use it as the configured scenario network.

From the repository root, install the network-build helper and create the
current local scenario network:

```powershell
python -m pip install -r scripts/sumo/requirements.txt
.\scripts\sumo\build-jakarta.ps1
```

The script downloads the Geofabrik Java OpenStreetMap PBF if needed, fetches
OSM boundary relation 6362934, extracts intersecting highways with pyosmium,
and converts them with SUMO `netconvert`. It structurally validates the output.
`-ValidateWithSumo` additionally loads the network into SUMO; this optional
check requires substantial memory for the full-city dataset. The extraction
follows the DKI administrative geometry, buffered by about 2 km for nearby road
context and crossings. The relation's documented bounds are
`106.3146732,-6.3744575,106.9739750,-4.9993635`, including Kepulauan Seribu.

The generated `.net.xml`, boundary GeoJSON, extracted OSM XML, and network
provenance JSON are written under `data/networks/generated/` and excluded from
Git. The metadata includes checksums for the OSM input, boundary, and generated
network. The source is
Geofabrik's Java extract at
<https://download.geofabrik.de/asia/indonesia/java-latest.osm.pbf>. OpenStreetMap
data is © OpenStreetMap contributors and licensed under ODbL 1.0. Preserve
attribution and ODbL obligations when distributing derived network data.
