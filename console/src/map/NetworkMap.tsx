import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { IconLayer, PathLayer, PolygonLayer, ScatterplotLayer } from '@deck.gl/layers'
import { MapboxOverlay } from '@deck.gl/mapbox'
import * as maplibregl from 'maplibre-gl'
import maplibreWorkerURL from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import 'maplibre-gl/dist/maplibre-gl.css'
import { api, type GraphEdge, type GraphNode, type MetricsResponse, type NetworkSummary, type RoadGraph, type SimVehicle, type StreamEdge } from '../api/client'
import { ErrorMessage, Panel } from '../screens/Shared'

type VehicleFilter = 'all' | 'fleet' | 'background'
type MapPreferences = { peak: boolean; junctions: boolean; vehicles: VehicleFilter }

/** A geographic box, in degrees. Matches the server's build request. */
export type MapArea = { min_lat: number; min_lon: number; max_lat: number; max_lon: number }
type DrawnVehicle = SimVehicle & { fromLon: number; fromLat: number; toLon: number; toLat: number; movedAt: number }
type Track = { vehicle: SimVehicle; from: [number, number]; to: [number, number]; movedAt: number; lastSeen: number; missingAt?: number }

const prefKey = 'fleetflow.map'
const fallbackPreferences: MapPreferences = { peak: true, junctions: false, vehicles: 'all' }
maplibregl.setWorkerUrl(maplibreWorkerURL)
const iconAtlas = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40" viewBox="0 0 80 40"><circle cx="20" cy="20" r="15" fill="#16866c" stroke="white" stroke-width="3"/><path d="M13 23v-7l2-4h10l3 4v7h-2v3h-2v-3h-6v3h-2v-3z" fill="white"/><circle cx="60" cy="20" r="15" fill="#657780" stroke="white" stroke-width="3"/><path d="M53 23v-7l2-4h10l3 4v7h-2v3h-2v-3h-6v3h-2v-3z" fill="white"/></svg>')}`
const iconMapping = {
  fleet: { x: 0, y: 0, width: 40, height: 40, mask: false },
  background: { x: 40, y: 0, width: 40, height: 40, mask: false },
}

function readPreferences(): MapPreferences {
  try {
    const raw = localStorage.getItem(prefKey)
    return raw ? { ...fallbackPreferences, ...JSON.parse(raw) } as MapPreferences : fallbackPreferences
  } catch (error) {
    console.warn('Could not load saved map preferences; using defaults.', error)
    return fallbackPreferences
  }
}

function edgePath(edge: GraphEdge, nodes: Map<string, GraphNode>): [number, number][] {
  if (edge.shape && edge.shape.length > 1) return edge.shape.map((point) => [point.lon, point.lat])
  const from = nodes.get(edge.from)
  const to = nodes.get(edge.to)
  return from && to ? [[from.lon, from.lat], [to.lon, to.lat]] : []
}

function vehicleKey(vehicle: SimVehicle) {
  return `${vehicle.kind}:${vehicle.arm ?? 'live'}:${vehicle.id}`
}

function colorForRatio(ratio: number, peak: boolean): [number, number, number, number] {
  if (ratio >= 1) return peak ? [193, 72, 57, 215] : [201, 87, 64, 200]
  if (ratio >= .85) return peak ? [213, 156, 49, 200] : [221, 173, 71, 190]
  if (ratio > .5) return peak ? [213, 156, 49, 180] : [221, 173, 71, 170]
  return peak ? [43, 143, 108, 190] : [62, 158, 125, 180]
}

/** Turn two dragged corners into a box with min/max in the right order. */
function normalizeArea(a: [number, number], b: [number, number]): MapArea {
  return {
    min_lon: Math.min(a[0], b[0]),
    max_lon: Math.max(a[0], b[0]),
    min_lat: Math.min(a[1], b[1]),
    max_lat: Math.max(a[1], b[1]),
  }
}

export function NetworkMap({ mode = 'live', onPickNode, selectedNodes, routeEdges = [], compact = false, selectArea = false, area = null, onAreaChange }: {
  mode?: 'live' | 'picker'
  onPickNode?: (id: string) => void
  selectedNodes?: { from: string; to: string }
  routeEdges?: string[]
  compact?: boolean
  /** Let the user drag a box on the map. */
  selectArea?: boolean
  /** The box to draw, in degrees. */
  area?: MapArea | null
  onAreaChange?: (area: MapArea) => void
}) {
  const container = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const overlay = useRef<MapboxOverlay | null>(null)
  const drawLayersRef = useRef<() => void>(() => {})
  const tracks = useRef(new Map<string, Track>())
  const lastRunTime = useRef<number | null>(null)
  const peakLoads = useRef(new Map<string, number>())
  const [graph, setGraph] = useState<RoadGraph | null>(null)
  const [network, setNetwork] = useState<NetworkSummary | null>(null)
  const [vehicles, setVehicles] = useState<DrawnVehicle[]>([])
  const [edges, setEdges] = useState<StreamEdge[]>([])
  const [runMetrics, setRunMetrics] = useState<MetricsResponse['run']>(null)
  const [preferences, setPreferences] = useState(readPreferences)
  const [error, setError] = useState('')
  // The corner where an area drag started, while the drag is in progress.
  const [dragCorner, setDragCorner] = useState<[number, number] | null>(null)
  const [dragCurrent, setDragCurrent] = useState<[number, number] | null>(null)
  const areaChangeRef = useRef(onAreaChange)
  areaChangeRef.current = onAreaChange
  const nodes = useMemo(() => new Map((graph?.nodes ?? []).map((node) => [node.id, node])), [graph])
  const visibleEdges = useMemo(() => {
    const costByID = new Map(edges.map((edge) => [edge.edge_id, edge]))
    return (graph?.edges ?? []).map((edge) => {
      const weight = costByID.get(edge.id)
      const ratio = weight?.load_ratio ?? 0
      return { edge, path: edgePath(edge, nodes), ratio }
    }).filter((item) => item.path.length > 1)
  }, [edges, graph, nodes])
  const pathData = useMemo(() => visibleEdges.map(({ edge, path, ratio }) => ({
    path,
    color: colorForRatio(preferences.peak ? Math.max(ratio, peakLoads.current.get(edge.id) ?? ratio) : ratio, preferences.peak),
    width: routeEdges.includes(edge.id) ? 4 : edge.lanes >= 3 ? 2.5 : 1.5,
  })), [preferences.peak, routeEdges, visibleEdges])
  const filteredVehicles = useMemo(() => vehicles.filter((vehicle) => {
    if (preferences.vehicles === 'fleet') return vehicle.kind === 'live' || vehicle.fleet
    if (preferences.vehicles === 'background') return vehicle.kind === 'simulated' && !vehicle.fleet
    return true
  }), [preferences.vehicles, vehicles])

  const updatePreferences = (next: MapPreferences) => {
    setPreferences(next)
    localStorage.setItem(prefKey, JSON.stringify(next))
  }

  // The box being dragged, or the one already chosen.
  const areaPolygon = useMemo(() => {
    const fromDrag = dragCorner && dragCurrent ? normalizeArea(dragCorner, dragCurrent) : null
    const box = fromDrag ?? area
    if (!box) return []
    return [[
      [box.min_lon, box.min_lat],
      [box.max_lon, box.min_lat],
      [box.max_lon, box.max_lat],
      [box.min_lon, box.max_lat],
    ] as [number, number][]]
  }, [area, dragCorner, dragCurrent])

  const drawLayers = useCallback(() => {
    if (!overlay.current || !graph) return
    const showNodes = mode === 'picker' || (preferences.junctions && (map.current?.getZoom() ?? 0) >= 13)
    const junctions = showNodes ? graph.nodes : []
    const selectedIDs = new Set([selectedNodes?.from, selectedNodes?.to].filter(Boolean))
    const layers = [
      new PathLayer({ id: 'roads', data: pathData, getPath: (item) => item.path, getColor: (item) => item.color, getWidth: (item) => item.width, widthUnits: 'pixels', pickable: false }),
      new ScatterplotLayer({ id: 'junctions', data: junctions, getPosition: (node: GraphNode) => [node.lon, node.lat], getRadius: (node: GraphNode) => selectedIDs.has(node.id) ? 7 : mode === 'picker' ? 4.5 : 2.5, radiusUnits: 'pixels', getFillColor: (node: GraphNode) => node.id === selectedNodes?.from ? [28, 125, 94, 240] : node.id === selectedNodes?.to ? [205, 91, 65, 240] : [68, 116, 130, mode === 'picker' ? 200 : 95], stroked: true, getLineColor: [255, 255, 255, 220], lineWidthMinPixels: 1, pickable: mode === 'picker', onClick: (info) => { const node = info.object as GraphNode | undefined; if (node) onPickNode?.(node.id) } }),
      new IconLayer({ id: 'vehicles', data: filteredVehicles, iconAtlas, iconMapping, getIcon: (vehicle: SimVehicle) => vehicle.fleet ? 'fleet' : 'background', getPosition: (vehicle: DrawnVehicle) => [vehicle.lon, vehicle.lat], getSize: 24, sizeUnits: 'pixels', billboard: true, pickable: true }),
      new PolygonLayer({ id: 'selected-area', data: areaPolygon, getPolygon: (polygon: [number, number][]) => polygon, filled: true, getFillColor: [24, 134, 108, 40], stroked: true, getLineColor: [24, 134, 108, 230], getLineWidth: 2, lineWidthUnits: 'pixels', pickable: false }),
    ]
    overlay.current.setProps({ layers })
  }, [areaPolygon, filteredVehicles, graph, mode, onPickNode, pathData, preferences.junctions, selectedNodes])
  drawLayersRef.current = drawLayers

  useEffect(() => {
    let alive = true
    Promise.all([api.graph(), api.network()]).then(([loaded, summary]) => {
      if (!alive) return
      setGraph(loaded)
      setNetwork(summary)
      setError('')
    }).catch((cause: unknown) => {
      if (alive) setError(cause instanceof Error ? cause.message : 'Could not load the road network.')
    })
    return () => { alive = false }
  }, [])

  useEffect(() => {
    if (!container.current || map.current) return
    const instance = new maplibregl.Map({
      container: container.current,
      style: 'https://tiles.openfreemap.org/styles/positron',
      center: [106.82, -6.2],
      zoom: 11,
    })
    instance.addControl(new maplibregl.NavigationControl({ showCompass: true }), 'top-right')
    const deck = new MapboxOverlay({ interleaved: false, layers: [] })
    instance.addControl(deck as unknown as maplibregl.IControl)
    const onZoom = () => drawLayersRef.current()
    instance.on('zoomend', onZoom)
    map.current = instance
    overlay.current = deck
    return () => {
      instance.off('zoomend', onZoom)
      instance.remove()
      map.current = null
      overlay.current = null
    }
  }, [])

  // Area selection: press, drag, release. The map's own drag-to-pan has to be
  // disabled for the gesture, so it is switched off on mousedown and restored on
  // mouseup — including when the release happens outside the canvas.
  useEffect(() => {
    const instance = map.current
    if (!selectArea || !instance) return

    const cornerOf = (event: maplibregl.MapMouseEvent): [number, number] => [event.lngLat.lng, event.lngLat.lat]

    const onDown = (event: maplibregl.MapMouseEvent) => {
      if (event.originalEvent.button !== 0) return
      instance.dragPan.disable()
      setDragCorner(cornerOf(event))
      setDragCurrent(cornerOf(event))
    }
    const onMove = (event: maplibregl.MapMouseEvent) => {
      setDragCurrent((current) => (current === null ? current : cornerOf(event)))
    }
    const onUp = (event: maplibregl.MapMouseEvent) => {
      instance.dragPan.enable()
      setDragCorner((corner) => {
        if (corner === null) return null
        const [lng, lat] = cornerOf(event)
        const next = normalizeArea(corner, [lng, lat])
        // A click is not a box. Below this the drag was probably a misfire, and
        // building a network for it would fail on an empty area.
        if (next.max_lat - next.min_lat > 1e-4 && next.max_lon - next.min_lon > 1e-4) {
          areaChangeRef.current?.(next)
        }
        return null
      })
      setDragCurrent(null)
    }

    instance.on('mousedown', onDown)
    instance.on('mousemove', onMove)
    instance.on('mouseup', onUp)
    return () => {
      instance.off('mousedown', onDown)
      instance.off('mousemove', onMove)
      instance.off('mouseup', onUp)
      instance.dragPan.enable()
    }
  }, [selectArea])

  useEffect(() => {
    drawLayers()
  }, [drawLayers])

  useEffect(() => {
    if (!network || !map.current) return
    peakLoads.current.clear()
    const b = network.bounds
    if (b.min_lat === b.max_lat || b.min_lon === b.max_lon) {
      map.current.setCenter([(b.min_lon + b.max_lon) / 2, (b.min_lat + b.max_lat) / 2])
      map.current.setZoom(14)
    } else {
      map.current.fitBounds([[b.min_lon, b.min_lat], [b.max_lon, b.max_lat]], { padding: 48, maxZoom: 15, duration: 500 })
    }
  }, [network])

  useEffect(() => {
    const source = new EventSource('/api/stream')
    source.addEventListener('vehicles', (event) => {
      const frame = JSON.parse((event as MessageEvent<string>).data) as { vehicles: SimVehicle[]; now_s: number; running: boolean }
      const now = performance.now()
      if (frame.running && lastRunTime.current !== null && frame.now_s < lastRunTime.current) {
        tracks.current.clear()
        peakLoads.current.clear()
      }
      lastRunTime.current = frame.running ? frame.now_s : null
      const current = new Set(frame.vehicles.map(vehicleKey))
      for (const vehicle of frame.vehicles) {
        const key = vehicleKey(vehicle)
        const to: [number, number] = [vehicle.lon, vehicle.lat]
        const existing = tracks.current.get(key)
        const progress = existing ? Math.min(1, Math.max(0, (now - existing.movedAt) / 250)) : 1
        const from: [number, number] = existing
          ? [existing.from[0] + (existing.to[0] - existing.from[0]) * progress, existing.from[1] + (existing.to[1] - existing.from[1]) * progress]
          : to
        tracks.current.set(key, {
          vehicle, from, to,
          movedAt: now, lastSeen: now,
        })
      }
      for (const [id, track] of tracks.current) {
        if (!current.has(id) && track.missingAt === undefined) track.missingAt = now
        if (track.missingAt !== undefined && now - track.missingAt > 2500) tracks.current.delete(id)
      }
    })
    source.addEventListener('edges', (event) => {
      const frame = JSON.parse((event as MessageEvent<string>).data) as { edges: StreamEdge[] }
      for (const edge of frame.edges) {
        peakLoads.current.set(edge.edge_id, Math.max(peakLoads.current.get(edge.edge_id) ?? edge.load_ratio, edge.load_ratio))
      }
      setEdges(frame.edges)
    })
    source.addEventListener('metrics', (event) => {
      const frame = JSON.parse((event as MessageEvent<string>).data) as MetricsResponse
      setRunMetrics(frame.run)
    })
    source.onerror = () => setError('Live stream disconnected; the browser will reconnect automatically.')
    source.onopen = () => setError('')
    return () => source.close()
  }, [])

  useEffect(() => {
    const timer = window.setInterval(() => {
      const now = performance.now()
      const display: DrawnVehicle[] = []
      for (const track of tracks.current.values()) {
        const alpha = Math.min(1, Math.max(0, (now - track.movedAt) / 250))
        if (track.missingAt !== undefined && now - track.missingAt > 500 && now - track.lastSeen > 2500) continue
        const lon = track.from[0] + (track.to[0] - track.from[0]) * alpha
        const lat = track.from[1] + (track.to[1] - track.from[1]) * alpha
        display.push({ ...track.vehicle, lon, lat, fromLon: track.from[0], fromLat: track.from[1], toLon: track.to[0], toLat: track.to[1], movedAt: track.movedAt })
      }
      if (tracks.current.size > 0) setVehicles(display)
      else setVehicles((current) => current.length ? [] : current)
    }, 50)
    return () => window.clearInterval(timer)
  }, [])

  const recenter = () => {
    if (!network || !map.current) return
    const b = network.bounds
    map.current.fitBounds([[b.min_lon, b.min_lat], [b.max_lon, b.max_lat]], { padding: 48, maxZoom: 15, duration: 500 })
  }

  return <Panel title={compact ? 'Network preview' : 'Live network map'} action={<div className="map-toolbar">
    {network && <span className="map-network-name">{network.name}</span>}
    {mode === 'live' && <>
      <label className="map-toggle"><input type="checkbox" checked={preferences.peak} onChange={(e) => updatePreferences({ ...preferences, peak: e.target.checked })} /> Peak</label>
      <label className="map-toggle"><input type="checkbox" checked={preferences.junctions} onChange={(e) => updatePreferences({ ...preferences, junctions: e.target.checked })} /> Junctions</label>
      <select aria-label="Vehicle visibility" value={preferences.vehicles} onChange={(e) => updatePreferences({ ...preferences, vehicles: e.target.value as VehicleFilter })}>
        <option value="all">All vehicles</option><option value="fleet">Fleet only</option><option value="background">Background only</option>
      </select>
    </>}
    <button className="button" onClick={recenter}>Re-center</button>
  </div>}>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {mode === 'picker' && <p className="help-text">Click a junction to select a trip origin or destination. Selected: {selectedNodes?.from || '—'} → {selectedNodes?.to || '—'}</p>}
    <div ref={container} className={`network-map ${compact ? 'compact' : ''}`} />
    <div className="map-legend">
      <span><i className="legend-road free" /> Load ≤ 0.5× capacity</span><span><i className="legend-road busy" /> Load ≥ 0.85× capacity</span>
      <span><i className="legend-road saturated" /> Saturated ≥ 1× capacity</span>
      {runMetrics && <span className="map-run-status">Seed {runMetrics.seed} · {runMetrics.vehicles} simulated vehicles</span>}
    </div>
    <p className="map-disclaimer">Road colours show simulated edge load divided by estimated capacity. Live reports are not a measured vehicle count; values are zero when no comparison run is active.</p>
  </Panel>
}
