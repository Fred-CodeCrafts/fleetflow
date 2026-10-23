import { useState, type FormEvent } from 'react'
import { api, type Coordinate, type OSRMRoute, type Trip, type Vehicle } from '../api/client'
import { RoutingMap } from '../map/RoutingMap'
import { ErrorMessage, formatNumber, Loading, Metric, Page, Panel, Status, Table, useLoad } from './Shared'

function formatCoordinate(point: Coordinate | null) {
  return point ? `${point.lat.toFixed(5)}, ${point.lon.toFixed(5)}` : ''
}

export function LiveMapPage() {
  const [demoMode, setDemoMode] = useState(false)
  return <Page eyebrow="Operations" title="Live Fleet Map" description="See GPS positions reported by connected vehicles on the Jakarta street map.">
    <div className="map-mode-controls" role="group" aria-label="Map position source">
      <button className={`button ${!demoMode ? 'primary' : ''}`} type="button"
        aria-pressed={!demoMode} onClick={() => setDemoMode(false)}>Live GPS</button>
      <button className={`button ${demoMode ? 'primary' : ''}`} type="button"
        aria-pressed={demoMode} onClick={() => setDemoMode(true)}>Demo preview</button>
    </div>
    {demoMode && <div className="notice" role="status">
      <strong>Demo preview:</strong> these static sample points are illustrative only, not real vehicles or GPS reports.
    </div>}
    <RoutingMap positionMode={demoMode ? 'demo' : 'live'} />
    <p className="muted">
      {demoMode
        ? 'Switch back to Live GPS to see only positions actually reported by connected devices.'
        : 'No positions appear until a connected device reports GPS data.'}
    </p>
  </Page>
}

export function TripsPage() {
  const trips = useLoad(api.trips)
  const vehicles = useLoad(api.vehicles)
  const [origin, setOrigin] = useState<Coordinate | null>(null)
  const [destination, setDestination] = useState<Coordinate | null>(null)
  const [vehicleID, setVehicleID] = useState('')
  const [pickTarget, setPickTarget] = useState<'origin' | 'destination' | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const create = async (event: FormEvent) => {
    event.preventDefault()
    if (!origin || !destination) {
      setError('Choose both trip locations on the map.')
      return
    }
    setError('')
    setMessage('')
    try {
      await api.createTrip({
        origin: formatCoordinate(origin),
        destination: formatCoordinate(destination),
        vehicle_id: vehicleID,
      })
      setOrigin(null)
      setDestination(null)
      setVehicleID('')
      setMessage('Trip created.')
      await trips.reload()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create trip.')
    }
  }

  const advance = async (trip: Trip) => {
    const next = trip.status === 'planned' ? 'in_progress' : 'completed'
    try {
      await api.tripStatus(trip.id, next)
      await trips.reload()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update trip.')
    }
  }

  const pick = (point: Coordinate) => {
    if (pickTarget === 'origin') {
      setOrigin(point)
      setPickTarget('destination')
    } else if (pickTarget === 'destination') {
      setDestination(point)
      setPickTarget(null)
    }
  }

  return <Page eyebrow="Operations" title="Trips" description="Create a trip using real map locations and assign it to an available vehicle.">
    <Panel title="Create trip" action={<button className="button" type="button" onClick={() => setPickTarget((target) => target ? null : 'origin')}>
      {pickTarget ? 'Stop selecting locations' : 'Choose locations on map'}
    </button>}>
      <form className="form-row" onSubmit={(event) => void create(event)}>
        <label className="field">Origin (latitude, longitude)
          <input required readOnly value={formatCoordinate(origin)} placeholder="Choose on map" />
        </label>
        <label className="field">Destination (latitude, longitude)
          <input required readOnly value={formatCoordinate(destination)} placeholder="Choose on map" />
        </label>
        <label className="field">Available vehicle<select value={vehicleID} onChange={(event) => setVehicleID(event.target.value)}>
          <option value="">Unassigned</option>{(vehicles.data ?? []).filter((vehicle) => vehicle.status === 'available').map((vehicle) =>
            <option key={vehicle.id} value={vehicle.id}>{vehicle.name}</option>)}
        </select></label>
        <button className="button primary" type="submit" disabled={!origin || !destination}>Create trip</button>
      </form>
      {pickTarget && <div className="picker-wrap">
        <p className="help-text">Click to choose {pickTarget}. {origin && `Origin: ${formatCoordinate(origin)}.`} {destination && `Destination: ${formatCoordinate(destination)}.`}</p>
        <RoutingMap
          compact
          selectedPoints={{ from: origin ?? undefined, to: destination ?? undefined }}
          onPickPoint={pick}
        />
      </div>}
      {message && <p className="success" role="status">{message}</p>}
      {error && <ErrorMessage>{error}</ErrorMessage>}
    </Panel>
    <Panel title="Trip register" action={<button className="button" onClick={() => void trips.reload()}>Refresh</button>}>
      {trips.loading ? <Loading /> : trips.error ? <ErrorMessage>{trips.error}</ErrorMessage> :
        <Table headers={['Trip', 'Route', 'Vehicle', 'Status', 'Started', 'Finished', 'Duration', 'Action']}>
          {(trips.data ?? []).map((trip) => <tr key={trip.id}>
            <td>{trip.id.slice(0, 8)}</td><td>{trip.origin} → {trip.destination}</td><td>{trip.vehicle_id ?? 'Unassigned'}</td>
            <td><Status value={trip.status} /></td><td>{trip.started_at ? new Date(trip.started_at).toLocaleString() : 'not recorded'}</td>
            <td>{trip.finished_at ? new Date(trip.finished_at).toLocaleString() : 'not recorded'}</td>
            <td>{trip.duration_s == null ? 'not recorded' : `${formatNumber(trip.duration_s)} s`}</td>
            <td>{trip.status !== 'completed' && <button className="button" onClick={() => void advance(trip)}>{trip.status === 'planned' ? 'Start' : 'Complete'}</button>}</td>
          </tr>)}
        </Table>}
      {!trips.loading && !trips.error && trips.data?.length === 0 && <p className="empty-state">No trips recorded yet.</p>}
    </Panel>
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
