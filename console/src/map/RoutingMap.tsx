import { useEffect, useRef, useState } from 'react'
import * as maplibregl from 'maplibre-gl'
import maplibreWorkerURL from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import 'maplibre-gl/dist/maplibre-gl.css'
import { api, type Coordinate, type OSRMRoute, type VehicleLocation } from '../api/client'
import { ErrorMessage, Panel } from '../screens/Shared'

maplibregl.setWorkerUrl(maplibreWorkerURL)

const jakarta: [number, number] = [106.8456, -6.2088]
const openFreeMapStyle = 'https://tiles.openfreemap.org/styles/liberty'

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

function pointFeature(point: Coordinate, label: string, kind: string): MapFeature {
  return {
    type: 'Feature',
    properties: { label, kind },
    geometry: { type: 'Point', coordinates: [point.lon, point.lat] },
  }
}

function positionFeatures(positions: VehicleLocation[]): MapFeature[] {
  return positions.map((position) => pointFeature(position, position.vehicle_id, 'vehicle'))
}

export function RoutingMap({ route, selectedPoints, onPickPoint, compact = false }: {
  route?: OSRMRoute | null
  selectedPoints?: { from?: Coordinate; to?: Coordinate }
  onPickPoint?: (point: Coordinate) => void
  compact?: boolean
}) {
  const container = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const pickHandler = useRef(onPickPoint)
  const [loaded, setLoaded] = useState(false)
  const [positions, setPositions] = useState<VehicleLocation[]>([])
  const [locationError, setLocationError] = useState('')
  const [mapError, setMapError] = useState('')
  pickHandler.current = onPickPoint

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
  }, [])

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
    vehicleSource.setData(featureCollection(positionFeatures(positions)))
    if (coordinates.length >= 2) {
      const bounds = coordinates.reduce(
        (box, point) => box.extend(point),
        new maplibregl.LngLatBounds(coordinates[0], coordinates[0]),
      )
      instance.fitBounds(bounds, { padding: 72, maxZoom: 15, duration: 500 })
    } else if (waypoints.length) {
      const coordinates: [number, number][] = []
      const bounds = new maplibregl.LngLatBounds()
      for (const feature of waypoints) {
        if (feature.geometry.type !== 'Point') continue
        const point = feature.geometry.coordinates
        coordinates.push(point)
        bounds.extend(point)
      }
      if (waypoints.length > 1) instance.fitBounds(bounds, { padding: 72, maxZoom: 15, duration: 500 })
      else instance.easeTo({ center: coordinates[0], zoom: 14 })
    }
  }, [loaded, positions, route, selectedPoints])

  return <Panel title={compact ? 'Jakarta map' : 'Live fleet map'} action={
    <button className="button" onClick={() => map.current?.easeTo({ center: jakarta, zoom: 11, duration: 500 })}>Jakarta</button>
  }>
    {onPickPoint && <p className="help-text">Click the map to choose a location. OpenStreetMap roads are rendered by OpenFreeMap.</p>}
    <div ref={container} className={`network-map ${compact ? 'compact' : ''}`} aria-label="Map of Jakarta" />
    <div className="map-legend">
      <span><i className="legend-road route" /> OSRM driving route</span>
      <span><i className="legend-road vehicle" /> Reported vehicle GPS</span>
      {positions.length === 0 && <span>No recent GPS positions</span>}
    </div>
    <p className="map-disclaimer">Routes are calculated by OSRM. Vehicle markers appear only after a device sends a GPS position. Map data © OpenStreetMap contributors.</p>
    {mapError && <ErrorMessage>{mapError}</ErrorMessage>}
    {locationError && <ErrorMessage>{locationError}</ErrorMessage>}
  </Panel>
}
