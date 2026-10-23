# SUMO road networks

`jakarta.net.xml` is a legacy central-Jakarta SUMO network retained for
reference; FleetFlow does not use it as the configured scenario network.

Build the current local scenario network with
[`scripts/sumo/build-jakarta.ps1`](../../scripts/sumo/build-jakarta.ps1). It
downloads the Geofabrik Java OpenStreetMap PBF if needed, extracts a buffered
area around DKI Jakarta, converts it with SUMO `netconvert`, and verifies that
SUMO can load the output. The requested rectangle
`106.25,-6.45,107.05,-4.95` contains the OSM administrative bounds of DKI
Jakarta relation 6362934 (`106.3146732,-6.3744575,106.9739750,-4.9993635`),
including Kepulauan Seribu, with surrounding road context.

The generated `.net.xml`, extracted PBF/XML, and network provenance JSON are
written under `data/networks/generated/` and excluded from Git. The source is
Geofabrik's Java extract at
<https://download.geofabrik.de/asia/indonesia/java-latest.osm.pbf>. OpenStreetMap
data is © OpenStreetMap contributors and licensed under ODbL 1.0. Preserve
attribution and ODbL obligations when distributing derived network data.
