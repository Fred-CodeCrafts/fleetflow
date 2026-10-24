import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import * as maplibregl from 'maplibre-gl'
import maplibreWorkerURL from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import 'maplibre-gl/dist/maplibre-gl.css'
import { api, type Coordinate, type OSRMRoute, type SimulatedDeliveryStop, type SimulatedTrack, type VehicleLocation } from '../api/client'
import { ErrorMessage, Panel } from '../screens/Shared'

maplibregl.setWorkerUrl(maplibreWorkerURL)

const jakarta: [number, number] = [106.8456, -6.2088]
const openFreeMapStyle = 'https://tiles.openfreemap.org/styles/liberty'

const logisticsHubs = [
  { name: 'Tanjung Priok Port Terminal', code: 'HUB-TPK', lat: -6.1030, lon: 106.8833, area: 'North Jakarta' },
  { name: 'Pluit Urban Logistics Depot', code: 'HUB-PLT', lat: -6.1245, lon: 106.7905, area: 'Northwest Jakarta' },
  { name: 'Halim Air Cargo Terminal', code: 'HUB-HLM', lat: -6.2625, lon: 106.8911, area: 'East Jakarta' },
  { name: 'Cilandak Urban Fulfillment Hub', code: 'HUB-CLK', lat: -6.2921, lon: 106.8042, area: 'South Jakarta' },
  { name: 'Tomang Regional Logistics Spine', code: 'HUB-TMG', lat: -6.1750, lon: 106.7915, area: 'West Jakarta' },
]

const vehicleDetailsMap: Record<string, { model: string; driver: string; hub: string; dest: string; tripId: string }> = {
  'B-9012-JKT': { model: 'Isuzu Giga FVM (Heavy Truck)', driver: 'Budi Santoso', hub: 'Tanjung Priok Port Terminal', dest: 'Kelapa Gading Commercial District', tripId: 'trip_tpk_kgd' },
  'B-9431-JKT': { model: 'Mitsubishi Fuso Canter (Box Truck)', driver: 'Agus Setiawan', hub: 'Pluit Urban Logistics Depot', dest: 'Glodok Commercial Hub', tripId: 'trip_plt_gld' },
  'B-9204-JKT': { model: 'Toyota HiAce Commuter (Van)', driver: 'Rian Hidayat', hub: 'Halim Air Cargo Terminal', dest: 'Sudirman Business Corridor', tripId: 'trip_hlm_sdr' },
  'B-9550-JKT': { model: 'Daihatsu Gran Max (Van)', driver: 'Eko Prasetyo', hub: 'Cilandak Fulfillment Hub', dest: 'Senayan City Retail Store', tripId: 'trip_clk_sny' },
  'B-9118-JKT': { model: 'Honda PCX Electric (Cargo Bike)', driver: 'Dimas Pratama', hub: 'Tomang Logistics Spine', dest: 'Grand Indonesia Thamrin', tripId: 'trip_tmg_gni' },
}

type PointGeometry = { type: 'Point'; coordinates: [number, number] }
type LineGeometry = { type: 'LineString'; coordinates: [number, number][] }
type MapFeature = {
  type: 'Feature'
  properties: Record<string, string>
  geometry: PointGeometry | LineGeometry
}
type MapFeatureCollection = { type: 'FeatureCollection'; features: MapFeature[] }

function featureCollection(features: MapFeature[]): MapFeatureCollection {
  return { type: 'FeatureCollection', features }
}

function pointFeature(point: Coordinate, label: string, kind: string, extra: Record<string, string> = {}): MapFeature {
  return {
    type: 'Feature',
    properties: { label, kind, ...extra },
    geometry: { type: 'Point', coordinates: [point.lon, point.lat] },
  }
}

function positionFeatures(positions: VehicleLocation[]): MapFeature[] {
  return positions.map((position) => pointFeature(position, position.vehicle_id, 'vehicle', { vehicle_id: position.vehicle_id }))
}

function getInterpolatedPosition(track: SimulatedTrack, currentTime: number): { lat: number; lon: number; progress: number } | null {
  if (!track.positions || track.positions.length === 0) return null
  const positions = track.positions
  const first = positions[0]
  if (!first) return null
  if (positions.length === 1) {
    return { lat: first.latitude, lon: first.longitude, progress: 1 }
  }
  const last = positions[positions.length - 1]
  if (!last) return null
  if (currentTime <= first.time_s) {
    return { lat: first.latitude, lon: first.longitude, progress: 0 }
  }
  if (currentTime >= last.time_s) {
    return { lat: last.latitude, lon: last.longitude, progress: 1 }
  }
  for (let i = 0; i < positions.length - 1; i++) {
    const p1 = positions[i]
    const p2 = positions[i + 1]
    if (p1 && p2 && currentTime >= p1.time_s && currentTime <= p2.time_s) {
      const span = p2.time_s - p1.time_s
      const frac = span > 0 ? (currentTime - p1.time_s) / span : 0
      const lat = p1.latitude + frac * (p2.latitude - p1.latitude)
      const lon = p1.longitude + frac * (p2.longitude - p1.longitude)
      const totalSpan = last.time_s - first.time_s
      const progress = totalSpan > 0 ? (currentTime - first.time_s) / totalSpan : 1
      return { lat, lon, progress }
    }
  }
  return { lat: last.latitude, lon: last.longitude, progress: 1 }
}

function simulatedPositionFeatures(tracks: SimulatedTrack[], currentTime: number): MapFeature[] {
  return tracks.flatMap((track) => {
    const pos = getInterpolatedPosition(track, currentTime)
    return pos ? [pointFeature({ lon: pos.lon, lat: pos.lat }, track.vehicle_id, 'simulation-vehicle', { vehicle_id: track.vehicle_id })] : []
  })
}

function simulatedRouteFeatures(tracks: SimulatedTrack[]): MapFeature[] {
  return tracks.flatMap((track) => {
    const coordinates = track.positions.map((position) => [position.longitude, position.latitude] as [number, number])
    return coordinates.length >= 2 ? [{
      type: 'Feature',
      properties: { label: track.vehicle_id, kind: 'simulation-route', vehicle_id: track.vehicle_id },
      geometry: { type: 'LineString', coordinates },
    }] : []
  })
}

function deliveryStopFeatures(stops: SimulatedDeliveryStop[]): MapFeature[] {
  return stops.map((stop) =>
    pointFeature({ lat: stop.latitude, lon: stop.longitude }, `Stop ${stop.order}`, 'delivery-stop'),
  )
}

function hubFeatures(): MapFeature[] {
  return logisticsHubs.map((hub) =>
    pointFeature({ lat: hub.lat, lon: hub.lon }, hub.code, 'logistics-hub', { name: hub.name, area: hub.area }),
  )
}

export function RoutingMap({ route, selectedPoints, onPickPoint, compact = false, positionMode = 'live', simulationTracks = [], simulationStops = [], runID }: {
  route?: OSRMRoute | null
  selectedPoints?: { from?: Coordinate; to?: Coordinate }
  onPickPoint?: (point: Coordinate) => void
  compact?: boolean
  positionMode?: 'live' | 'simulation'
  simulationTracks?: SimulatedTrack[]
  simulationStops?: SimulatedDeliveryStop[]
  runID?: string
}) {
  const container = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const pickHandler = useRef(onPickPoint)
  const [loaded, setLoaded] = useState(false)
  const [positions, setPositions] = useState<VehicleLocation[]>([])
  const [locationError, setLocationError] = useState('')
  const [mapError, setMapError] = useState('')

  const [playbackTime, setPlaybackTime] = useState(0)
  const [isPlaying, setIsPlaying] = useState(true)
  const [playbackSpeed, setPlaybackSpeed] = useState(1)
  const [selectedVehicleId, setSelectedVehicleId] = useState<string | null>(null)

  const maxTrackTime = Math.max(
    ...simulationTracks.flatMap((t) => t.positions.map((p) => p.time_s)),
    420,
  )

  pickHandler.current = onPickPoint

  // Animation ticker for dynamic vehicle movement
  useEffect(() => {
    if (!isPlaying || positionMode !== 'simulation' || simulationTracks.length === 0) return
    const interval = window.setInterval(() => {
      setPlaybackTime((prev) => {
        const next = prev + playbackSpeed * 2.5
        return next > maxTrackTime ? 0 : next
      })
    }, 100)
    return () => window.clearInterval(interval)
  }, [isPlaying, maxTrackTime, playbackSpeed, positionMode, simulationTracks.length])

  useEffect(() => {
    if (!container.current) return
    const instance = new maplibregl.Map({
      container: container.current,
      style: openFreeMapStyle,
      center: jakarta,
      zoom: 11,
      minZoom: 8,
      maxZoom: 19,
    })
    map.current = instance
    instance.addControl(new maplibregl.NavigationControl(), 'top-right')
    instance.on('load', () => {
      instance.addSource('planned-route', {
        type: 'geojson',
        data: featureCollection([]),
      })
      instance.addLayer({
        id: 'planned-route-casing',
        type: 'line',
        source: 'planned-route',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': '#ffffff', 'line-width': 9, 'line-opacity': 0.9 },
      })
      instance.addLayer({
        id: 'planned-route-line',
        type: 'line',
        source: 'planned-route',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': '#16866c', 'line-width': 5 },
      })

      // Multi-hub Jakarta Logistics Hubs
      instance.addSource('logistics-hubs', { type: 'geojson', data: featureCollection([]) })
      instance.addLayer({
        id: 'logistics-hub-circles',
        type: 'circle',
        source: 'logistics-hubs',
        paint: {
          'circle-color': '#0c8599',
          'circle-radius': 9,
          'circle-stroke-color': '#ffffff',
          'circle-stroke-width': 2.5,
        },
      })
      instance.addLayer({
        id: 'logistics-hub-labels',
        type: 'symbol',
        source: 'logistics-hubs',
        layout: {
          'text-field': ['get', 'label'],
          'text-offset': [0, 1.4],
          'text-size': 11,
          'text-font': ['Noto Sans Regular'],
        },
        paint: { 'text-color': '#096677', 'text-halo-color': '#ffffff', 'text-halo-width': 1.5 },
      })

      instance.addSource('simulation-routes', { type: 'geojson', data: featureCollection([]) })
      instance.addLayer({
        id: 'simulation-route-lines',
        type: 'line',
        source: 'simulation-routes',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': '#e67700', 'line-width': 4, 'line-opacity': 0.8 },
      })
      instance.addSource('simulation-stops', { type: 'geojson', data: featureCollection([]) })
      instance.addLayer({
        id: 'simulation-stop-circles',
        type: 'circle',
        source: 'simulation-stops',
        paint: {
          'circle-color': '#ffffff',
          'circle-radius': 8,
          'circle-stroke-color': '#8a3ffc',
          'circle-stroke-width': 3,
        },
      })
      instance.addLayer({
        id: 'simulation-stop-labels',
        type: 'symbol',
        source: 'simulation-stops',
        layout: {
          'text-field': ['get', 'label'],
          'text-offset': [0, 1.3],
          'text-size': 11,
          'text-font': ['Noto Sans Regular'],
        },
        paint: { 'text-color': '#5825a4', 'text-halo-color': '#ffffff', 'text-halo-width': 1.5 },
      })
      instance.addSource('waypoints', { type: 'geojson', data: featureCollection([]) })
      instance.addLayer({
        id: 'waypoint-circles',
        type: 'circle',
        source: 'waypoints',
        paint: {
          'circle-color': ['match', ['get', 'kind'], 'from', '#16866c', 'to', '#d05c48', '#657780'],
          'circle-radius': 7,
          'circle-stroke-color': '#ffffff',
          'circle-stroke-width': 2,
        },
      })
      instance.addLayer({
        id: 'waypoint-labels',
        type: 'symbol',
        source: 'waypoints',
        layout: {
          'text-field': ['get', 'label'],
          'text-offset': [0, 1.35],
          'text-size': 12,
          'text-font': ['Noto Sans Regular'],
        },
        paint: { 'text-color': '#182127', 'text-halo-color': '#ffffff', 'text-halo-width': 1.5 },
      })
      instance.addSource('vehicle-positions', { type: 'geojson', data: featureCollection([]) })
      instance.addLayer({
        id: 'vehicle-position-circles',
        type: 'circle',
        source: 'vehicle-positions',
        paint: {
          'circle-color': '#4263eb',
          'circle-radius': 6,
          'circle-stroke-color': '#ffffff',
          'circle-stroke-width': 2,
        },
      })
      instance.addSource('simulation-positions', { type: 'geojson', data: featureCollection([]) })
      instance.addLayer({
        id: 'simulation-position-circles',
        type: 'circle',
        source: 'simulation-positions',
        paint: {
          'circle-color': '#e67700',
          'circle-radius': 9,
          'circle-stroke-color': '#ffffff',
          'circle-stroke-width': 2.5,
        },
      })
      instance.addLayer({
        id: 'simulation-position-labels',
        type: 'symbol',
        source: 'simulation-positions',
        layout: {
          'text-field': ['get', 'label'],
          'text-offset': [0, 1.45],
          'text-size': 12,
          'text-font': ['Noto Sans Regular'],
        },
        paint: { 'text-color': '#9a4b00', 'text-halo-color': '#ffffff', 'text-halo-width': 1.5 },
      })

      // Interactive click handlers for vehicle dots and route lines
      const clickVehicle = (e: maplibregl.MapLayerMouseEvent) => {
        const vid = e.features?.[0]?.properties?.vehicle_id
        if (vid) setSelectedVehicleId(vid)
      }
      instance.on('click', 'simulation-position-circles', clickVehicle)
      instance.on('click', 'simulation-route-lines', clickVehicle)
      instance.on('click', 'vehicle-position-circles', clickVehicle)

      const interactive = ['simulation-position-circles', 'simulation-route-lines', 'vehicle-position-circles', 'logistics-hub-circles']
      for (const id of interactive) {
        instance.on('mouseenter', id, () => { instance.getCanvas().style.cursor = 'pointer' })
        instance.on('mouseleave', id, () => { instance.getCanvas().style.cursor = '' })
      }

      setLoaded(true)
    })
    instance.on('click', (event) => {
      const handler = pickHandler.current
      if (handler) handler({ lat: event.lngLat.lat, lon: event.lngLat.lng })
    })
    instance.on('error', (event) => {
      if (event.error) setMapError(`Map tiles could not be loaded: ${event.error.message}`)
    })

    return () => {
      instance.remove()
      map.current = null
      setLoaded(false)
    }
  }, [])

  useEffect(() => {
    let disposed = false
    let timeout = 0
    if (positionMode !== 'live') {
      setLocationError('')
      return () => { disposed = true }
    }

    const refresh = async () => {
      try {
        const latest = await api.locations()
        if (!disposed) {
          setPositions(latest)
          setLocationError('')
        }
      } catch (cause) {
        if (!disposed) setLocationError(cause instanceof Error ? cause.message : 'Vehicle positions are unavailable.')
      }
      if (!disposed) timeout = window.setTimeout(() => void refresh(), 5000)
    }
    void refresh()
    return () => {
      disposed = true
      window.clearTimeout(timeout)
    }
  }, [positionMode])

  useEffect(() => {
    const instance = map.current
    if (!loaded || !instance) return
    const routeSource = instance.getSource('planned-route') as maplibregl.GeoJSONSource
    const coordinates = route?.geometry ?? []
    routeSource.setData(featureCollection(coordinates.length >= 2 ? [{
      type: 'Feature',
      properties: {},
      geometry: { type: 'LineString', coordinates },
    }] : []))
    const waypointSource = instance.getSource('waypoints') as maplibregl.GeoJSONSource
    const waypoints = [
      ...(selectedPoints?.from ? [pointFeature(selectedPoints.from, 'Origin', 'from')] : []),
      ...(selectedPoints?.to ? [pointFeature(selectedPoints.to, 'Destination', 'to')] : []),
    ]
    waypointSource.setData(featureCollection(waypoints))
    const vehicleSource = instance.getSource('vehicle-positions') as maplibregl.GeoJSONSource
    vehicleSource.setData(featureCollection(positionMode === 'live' ? positionFeatures(positions) : []))

    const hubSource = instance.getSource('logistics-hubs') as maplibregl.GeoJSONSource
    hubSource.setData(featureCollection(positionMode === 'simulation' ? hubFeatures() : []))

    const simulationSource = instance.getSource('simulation-routes') as maplibregl.GeoJSONSource
    const simulationFeatures = positionMode === 'simulation' ? simulatedRouteFeatures(simulationTracks) : []
    simulationSource.setData(featureCollection(simulationFeatures))

    const deliveryStopSource = instance.getSource('simulation-stops') as maplibregl.GeoJSONSource
    const deliveryStops = positionMode === 'simulation' ? deliveryStopFeatures(simulationStops) : []
    deliveryStopSource.setData(featureCollection(deliveryStops))

    const simulationPositionSource = instance.getSource('simulation-positions') as maplibregl.GeoJSONSource
    const simulationPositions = positionMode === 'simulation' ? simulatedPositionFeatures(simulationTracks, playbackTime) : []
    simulationPositionSource.setData(featureCollection(simulationPositions))

    if (positionMode !== 'simulation' && coordinates.length >= 2) {
      const bounds = coordinates.reduce(
        (box, point) => box.extend(point),
        new maplibregl.LngLatBounds(coordinates[0], coordinates[0]),
      )
      instance.fitBounds(bounds, { padding: 72, maxZoom: 15, duration: 500 })
    } else if (positionMode === 'simulation' && simulationFeatures.length && playbackTime === 0) {
      const coords = simulationFeatures.flatMap((feature) =>
        feature.geometry.type === 'LineString' ? feature.geometry.coordinates : [],
      )
      const bounds = new maplibregl.LngLatBounds()
      for (const point of coords) bounds.extend(point)
      for (const feature of deliveryStops) {
        if (feature.geometry.type === 'Point') bounds.extend(feature.geometry.coordinates)
      }
      instance.fitBounds(bounds, { padding: 72, maxZoom: 15, duration: 500 })
    } else if (waypoints.length) {
      const coords: [number, number][] = []
      const bounds = new maplibregl.LngLatBounds()
      for (const feature of waypoints) {
        if (feature.geometry.type !== 'Point') continue
        const point = feature.geometry.coordinates
        coords.push(point)
        bounds.extend(point)
      }
      if (waypoints.length > 1) instance.fitBounds(bounds, { padding: 72, maxZoom: 15, duration: 500 })
      else instance.easeTo({ center: coords[0], zoom: 14 })
    }
  }, [loaded, playbackTime, positionMode, positions, route, selectedPoints, simulationStops, simulationTracks])

  const selectedVehicleInfo = selectedVehicleId ? vehicleDetailsMap[selectedVehicleId] : null
  const selectedTrack = selectedVehicleId ? simulationTracks.find((t) => t.vehicle_id === selectedVehicleId) : null
  const vehicleProgress = selectedTrack ? getInterpolatedPosition(selectedTrack, playbackTime)?.progress ?? 0 : 0

  return <Panel title={compact ? 'Jakarta map' : positionMode === 'simulation' ? 'Simulation map' : 'Live fleet map'} action={
    <button className="button" onClick={() => map.current?.easeTo({ center: jakarta, zoom: 11, duration: 500 })}>Jakarta</button>
  }>
    {onPickPoint && <p className="help-text">Click the map to choose a location. OpenStreetMap roads are rendered by OpenFreeMap.</p>}

    {positionMode === 'simulation' && simulationTracks.length > 0 && (
      <div className="form-row" style={{ alignItems: 'center', marginBottom: '10px', gap: '12px' }}>
        <button
          className="button primary"
          type="button"
          onClick={() => setIsPlaying((p) => !p)}
        >
          {isPlaying ? '⏸ Pause' : '▶ Play'}
        </button>
        <button
          className="button"
          type="button"
          onClick={() => setPlaybackTime(0)}
        >
          ↺ Reset
        </button>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flex: 1, minWidth: '180px' }}>
          <small style={{ minWidth: '55px', fontFamily: 'monospace' }}>
            {Math.floor(playbackTime / 60)}:{(Math.floor(playbackTime % 60)).toString().padStart(2, '0')}
          </small>
          <input
            type="range"
            min={0}
            max={maxTrackTime}
            step={1}
            value={Math.floor(playbackTime)}
            onChange={(e) => setPlaybackTime(Number(e.target.value))}
            style={{ flex: 1 }}
          />
          <small style={{ minWidth: '55px', fontFamily: 'monospace', color: '#687579' }}>
            {Math.floor(maxTrackTime / 60)}:{(Math.floor(maxTrackTime % 60)).toString().padStart(2, '0')}
          </small>
        </div>
        <div className="form-row" style={{ margin: 0, gap: '4px' }}>
          {[1, 2, 5].map((s) => (
            <button
              key={s}
              className={`button ${playbackSpeed === s ? 'primary' : ''}`}
              type="button"
              style={{ minHeight: '28px', padding: '0 8px', fontSize: '11px' }}
              onClick={() => setPlaybackSpeed(s)}
            >
              {s}x
            </button>
          ))}
        </div>
      </div>
    )}

    <div ref={container} className={`network-map ${compact ? 'compact' : ''}`} aria-label="Map of Jakarta" />

    {selectedVehicleId && (
      <div className="panel" style={{ marginTop: '12px', borderLeft: '4px solid #16866c', background: '#f8fcfa' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div>
            <span className="eyebrow">Vehicle Inspector</span>
            <h3 style={{ margin: '4px 0 6px 0', fontSize: '18px' }}>{selectedVehicleId}</h3>
            <p className="muted" style={{ margin: 0 }}>
              {selectedVehicleInfo?.model || 'Fleet Logistics Vehicle'} · Assigned Driver: <strong>{selectedVehicleInfo?.driver || 'Fleet Operator'}</strong>
            </p>
          </div>
          <button className="button" type="button" onClick={() => setSelectedVehicleId(null)}>✕ Close</button>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '12px', margin: '14px 0' }}>
          <div className="field">
            <span style={{ color: '#829095', fontSize: '10px', textTransform: 'uppercase' }}>Dispatch Hub</span>
            <strong>{selectedVehicleInfo?.hub || 'Regional Logistics Hub'}</strong>
          </div>
          <div className="field">
            <span style={{ color: '#829095', fontSize: '10px', textTransform: 'uppercase' }}>Delivery Destination</span>
            <strong>{selectedVehicleInfo?.dest || 'Retail / Commercial Zone'}</strong>
          </div>
          <div className="field">
            <span style={{ color: '#829095', fontSize: '10px', textTransform: 'uppercase' }}>Mission Progress</span>
            <strong>{Math.round(vehicleProgress * 100)}% Complete</strong>
          </div>
        </div>

        <div className="form-row" style={{ margin: 0, gap: '8px', flexWrap: 'wrap' }}>
          <Link className="button primary" to={`/operations/fleet?vehicle=${encodeURIComponent(selectedVehicleId)}`}>
            Fleet Status
          </Link>
          {runID && selectedVehicleInfo?.tripId ? (
            <Link className="button" to={`/operations/trips/${encodeURIComponent(runID)}/${encodeURIComponent(selectedVehicleInfo.tripId)}`}>
              Open Trip Details
            </Link>
          ) : (
            <Link className="button" to={`/operations/trips?vehicle=${encodeURIComponent(selectedVehicleId)}`}>
              All Trips
            </Link>
          )}
          <Link className="button" to={`/resources/maintenance?vehicle=${encodeURIComponent(selectedVehicleId)}`}>
            Maintenance History
          </Link>
        </div>
      </div>
    )}

    <div className="map-legend">
      {positionMode === 'simulation' ?
        <>
          <span><i className="legend-road simulation-route" /> Simulated route and fleet positions</span>
          <span><i className="legend-road" style={{ background: '#0c8599', borderRadius: '50%', width: '10px', height: '10px', display: 'inline-block' }} /> Logistics Hubs</span>
          {simulationStops.length > 0 && <span><i className="legend-road delivery-stop" /> Delivery stops</span>}
        </> :
        <span><i className="legend-road vehicle" /> Reported vehicle GPS</span>}
      {route && <span><i className="legend-road route" /> OSRM driving route</span>}
      {positionMode === 'live' && positions.length === 0 && <span>No recent GPS positions</span>}
    </div>
    <p className="map-disclaimer">
      {positionMode === 'simulation'
        ? 'Routes and positions are generated by fleet simulation; they are not live GPS reports. '
        : 'Vehicle markers show positions reported by connected devices. '}
      Map data © OpenStreetMap contributors.
    </p>
    {mapError && <ErrorMessage>{mapError}</ErrorMessage>}
    {locationError && <ErrorMessage>{locationError}</ErrorMessage>}
  </Panel>
}
