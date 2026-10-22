# Bundled road networks

`jakarta.net.xml` is the default network FleetFlow loads at startup. It covers
central Jakarta, not a single corridor: roughly 32 km east–west and 30 km
north–south.

## What it contains

| | |
|---|---|
| Junctions | 8,508 |
| Road segments (directed) | 9,529 |
| Total length | 2,289 km |
| Road classes | `motorway`, `trunk`, `primary`, `secondary`, and their `_link` ramps |
| Geographic bounds | 106.6874–106.9907 E, 6.3647–6.0958 S |
| Projection | UTM zone 48S (`+proj=utm +zone=48 +ellps=WGS84`) |
| File size | 31 MB |

Local streets and residential roads are deliberately excluded. Including them
takes the same area to 321,678 ways, which is neither drawable in the console nor
useful for a network-scale routing demonstration — the routing signal lives on
the arterials.

## How it was generated

The data is OpenStreetMap, © OpenStreetMap contributors, licensed ODbL 1.0. See
[NOTICE](../../NOTICE). Reproduce it with:

```sh
# 1. Fetch the arterial roads for central Jakarta from Overpass.
curl -s --data-urlencode 'data=[out:xml][timeout:300];
  way["highway"~"^(motorway|trunk|primary|secondary)(_link)?$"]
  (-6.35,106.70,-6.10,106.95);(._;>;);out body;' \
  -o jakarta.osm.xml https://overpass-api.de/api/interpreter

# 2. Convert to a SUMO network.
netconvert --osm-files jakarta.osm.xml --output-file jakarta.net.xml \
  --geometry.remove --roundabouts.guess --ramps.guess --junctions.join \
  --tls.guess-signals --tls.discard-simple \
  --keep-edges.by-vclass passenger --remove-edges.isolated
```

The bounding box is the parameter worth changing: widen it for a larger area,
narrow it for a smaller one. Drop `secondary` from the highway filter to keep
only the main arterials and toll roads.

## Using a different network

Point `GRAPH_PATH` at any other `.net.xml` to override the default, or import one
at runtime from **Settings → Road Network** in the console.
