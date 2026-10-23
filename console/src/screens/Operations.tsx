import { useState, type FormEvent } from 'react'
import { api, type Coordinate, type OSRMRoute, type Vehicle } from '../api/client'
import { RoutingMap } from '../map/RoutingMap'
import { ErrorMessage, formatNumber, Loading, Metric, Page, Panel, Status, Table, useLoad } from './Shared'

function formatCoordinate(point: Coordinate | null) {
  return point ? `${point.lat.toFixed(5)}, ${point.lon.toFixed(5)}` : ''
}

export function LiveMapPage() {
  return <Page eyebrow="Operations" title="Live Fleet Map" description="See GPS positions reported by connected vehicles on the Jakarta street map.">
    <RoutingMap positionMode="live" />
    <p className="muted">No positions appear until a connected device reports GPS data.</p>
  </Page>
}

export function RoutePlannerPage() {
  const [from, setFrom] = useState<Coordinate | null>(null)
  const [to, setTo] = useState<Coordinate | null>(null)
  const [pickTarget, setPickTarget] = useState<'from' | 'to' | null>(null)
  const [route, setRoute] = useState<OSRMRoute | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const plan = async (event: FormEvent) => {
    event.preventDefault()
    if (!from || !to) return
    setBusy(true)
    setError('')
    setRoute(null)
    try {
      setRoute(await api.route(from, to))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'OSRM route request failed.')
    } finally {
      setBusy(false)
    }
  }

  const pick = (point: Coordinate) => {
    if (pickTarget === 'from') {
      setFrom(point)
      setPickTarget('to')
      setRoute(null)
    } else if (pickTarget === 'to') {
      setTo(point)
      setPickTarget(null)
      setRoute(null)
    }
  }

  return <Page eyebrow="Operations" title="Route Planner" description="Request a driving route from OSRM for two points on the Jakarta map.">
    <Panel title="Plan a route">
      <form className="form-row" onSubmit={(event) => void plan(event)}>
        <div className="field location-value"><span>Origin</span><output>{formatCoordinate(from) || 'Choose a point'}</output></div>
        <button className="button" type="button" onClick={() => setPickTarget('from')}>Choose origin</button>
        <div className="field location-value"><span>Destination</span><output>{formatCoordinate(to) || 'Choose a point'}</output></div>
        <button className="button" type="button" onClick={() => setPickTarget('to')}>Choose destination</button>
        <button className="button primary" type="submit" disabled={!from || !to || busy}>{busy ? 'Routing…' : 'Find driving route'}</button>
      </form>
      {route && <p className="success" role="status">
        {route.provider} · {formatNumber(route.distance_m / 1000, 2)} km · {formatNumber(route.duration_s / 60, 1)} min estimated
      </p>}
      {error && <ErrorMessage>{error}</ErrorMessage>}
    </Panel>
    <RoutingMap
      route={route}
      selectedPoints={{ from: from ?? undefined, to: to ?? undefined }}
      onPickPoint={pickTarget ? pick : undefined}
    />
    {pickTarget && <p className="help-text">Click the map to set the {pickTarget === 'from' ? 'origin' : 'destination'}.</p>}
  </Page>
}

export function FleetStatusPage() {
  const vehicles = useLoad(api.vehicles)
  const drivers = useLoad(api.drivers)
  const allVehicles: Vehicle[] = vehicles.data ?? []
  return <Page eyebrow="Resources" title="Fleet Status" description="Current vehicle availability and driver assignments.">
    <div className="metric-grid">
      <Metric label="Vehicles" value={String(allVehicles.length)} />
      <Metric label="Available" value={String(allVehicles.filter((vehicle) => vehicle.status === 'available').length)} />
      <Metric label="On trip" value={String(allVehicles.filter((vehicle) => vehicle.status === 'on_trip').length)} />
      <Metric label="Drivers" value={String(drivers.data?.length ?? 0)} />
    </div>
    <Panel title="Fleet register">{vehicles.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
      <Table headers={['Vehicle', 'Type', 'Status', 'Assigned driver']}>
        {allVehicles.map((vehicle) => <tr key={vehicle.id}><td>{vehicle.name}</td><td>{vehicle.type}</td><td><Status value={vehicle.status} /></td>
          <td>{drivers.data?.find((driver) => driver.vehicle_id === vehicle.id)?.name ?? 'Unassigned'}</td></tr>)}
      </Table>}</Panel>
  </Page>
}
