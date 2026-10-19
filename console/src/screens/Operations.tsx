import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, type MetricsResponse, type SavedRun, type Trip, type Vehicle } from '../api/client'
import { NetworkMap } from '../map/LazyNetworkMap'
import { ErrorMessage, formatNumber, Loading, Metric, Page, Panel, Status, Table, useLoad } from './Shared'

function savedSimulationDefaults() {
  const defaults = { seed: 42, vehicles: 100, fleet_share: 0.5 }
  try {
    return { ...defaults, ...JSON.parse(window.localStorage.getItem('fleetflow.simulation') ?? '{}') as Partial<typeof defaults> }
  } catch (error) {
    console.warn('Could not load saved simulation defaults; using defaults.', error)
    return defaults
  }
}

export function LiveMapPage() {
  return <Page eyebrow="Operations" title="Live Map" description="Track simulated fleet traffic and road congestion as the comparison runs.">
    <NetworkMap />
  </Page>
}

export function TripsPage() {
  const trips = useLoad(api.trips)
  const vehicles = useLoad(api.vehicles)
  const [origin, setOrigin] = useState('')
  const [destination, setDestination] = useState('')
  const [vehicleID, setVehicleID] = useState('')
  const [pickOpen, setPickOpen] = useState(false)
  const [pickTarget, setPickTarget] = useState<'origin' | 'destination'>('origin')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    setMessage('')
    try {
      await api.createTrip({ origin, destination, vehicle_id: vehicleID })
      setOrigin('')
      setDestination('')
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
  const pick = (id: string) => {
    if (pickTarget === 'origin') {
      setOrigin(id)
      setPickTarget('destination')
    } else {
      setDestination(id)
      setPickTarget('origin')
      setPickOpen(false)
    }
  }

  return <Page eyebrow="Operations" title="Trips" description="Create and assign trips, then follow their lifecycle.">
    <Panel title="Create trip" action={<button className="button" onClick={() => setPickOpen((open) => !open)}>{pickOpen ? 'Hide map picker' : 'Pick locations on map'}</button>}>
      <form className="form-row" onSubmit={create}>
        <label className="field">Origin node<input required value={origin} onChange={(e) => setOrigin(e.target.value)} placeholder="e.g. node-1" /></label>
        <label className="field">Destination node<input required value={destination} onChange={(e) => setDestination(e.target.value)} placeholder="e.g. node-8" /></label>
        <label className="field">Available vehicle<select value={vehicleID} onChange={(e) => setVehicleID(e.target.value)}>
          <option value="">Unassigned</option>{(vehicles.data ?? []).filter((v) => v.status === 'available').map((v) =>
            <option key={v.id} value={v.id}>{v.name}</option>)}
        </select></label>
        <button className="button primary" type="submit">Create trip</button>
      </form>
      {pickOpen && <div className="picker-wrap">
        <p className="help-text">Select {pickTarget} by clicking a junction. Current: {origin || '—'} → {destination || '—'}</p>
        <NetworkMap mode="picker" onPickNode={pick} selectedNodes={{ from: origin, to: destination }} compact />
      </div>}
      {message && <p className="success">{message}</p>}
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
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [pickTarget, setPickTarget] = useState<'from' | 'to' | null>(null)
  const [route, setRoute] = useState<{ edges: string[]; cost_s: number } | null>(null)
  const [error, setError] = useState('')
  const plan = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    setRoute(null)
    try {
      setRoute(await api.route(from, to))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Route request failed.')
    }
  }
  const pick = (id: string) => {
    if (pickTarget === 'from') {
      setFrom(id)
      setPickTarget('to')
    } else {
      setTo(id)
      setPickTarget(null)
    }
  }
  return <Page eyebrow="Operations" title="Route Planner" description="Compare a live-weight route between selected road-network junctions.">
    <Panel title="Plan a route">
      <form className="form-row" onSubmit={plan}>
        <label className="field">Origin node<input required value={from} onChange={(e) => setFrom(e.target.value)} /></label>
        <label className="field">Destination node<input required value={to} onChange={(e) => setTo(e.target.value)} /></label>
        <button className="button" type="button" onClick={() => setPickTarget((v) => v ? null : 'from')}>{pickTarget ? 'Cancel map pick' : 'Pick on map'}</button>
        <button className="button primary" type="submit">Find route</button>
      </form>
      {route && <p className="success">Estimated {formatNumber(route.cost_s)} seconds · {route.edges.length} road segments</p>}
      {error && <ErrorMessage>{error}</ErrorMessage>}
    </Panel>
    <NetworkMap mode={pickTarget ? 'picker' : 'live'} onPickNode={pick} selectedNodes={{ from, to }} routeEdges={route?.edges} />
  </Page>
}

export function ComparisonRunsPage() {
  const runs = useLoad(api.runs)
  const [saved] = useState(savedSimulationDefaults)
  const [seed, setSeed] = useState(saved.seed)
  const [vehicles, setVehicles] = useState(saved.vehicles)
  const [fleetShare, setFleetShare] = useState(saved.fleet_share)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const start = async () => {
    setBusy(true)
    setError('')
    try {
      await api.start({ seed, vehicles, fleet_share: fleetShare })
      window.localStorage.setItem('fleetflow.simulation', JSON.stringify({ seed, vehicles, fleet_share: fleetShare }))
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not start the run.') }
    finally { setBusy(false) }
  }
  const reset = async () => {
    setBusy(true)
    try { await api.reset() } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not reset the run.') }
    finally { setBusy(false) }
  }
  const save = async () => {
    setBusy(true)
    try { await api.saveRun(); await runs.reload() } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not save the run.') }
    finally { setBusy(false) }
  }
  return <Page eyebrow="Operations" title="Comparison Runs" description="Run the same seeded demand through baseline and congestion-aware routing.">
    <Panel title="Run controls">
      <div className="form-row">
        <label className="field">Demand seed<input type="number" value={seed} onChange={(e) => setSeed(Number(e.target.value))} /></label>
        <label className="field">Vehicles<input type="number" min="1" value={vehicles} onChange={(e) => setVehicles(Number(e.target.value))} /></label>
        <label className="field">Fleet share<input type="number" min="0" max="1" step="0.05" value={fleetShare} onChange={(e) => setFleetShare(Number(e.target.value))} /></label>
        <button className="button primary" onClick={() => void start()} disabled={busy}>Start comparison</button>
        <button className="button danger" onClick={() => void reset()} disabled={busy}>Reset</button>
        <button className="button" onClick={() => void save()} disabled={busy}>Save current run</button>
      </div>
      {error && <ErrorMessage>{error}</ErrorMessage>}
    </Panel>
    <Panel title="Saved comparisons" action={<button className="button" onClick={() => void runs.reload()}>Refresh</button>}>
      {runs.loading ? <Loading /> : runs.error ? <ErrorMessage>{runs.error}</ErrorMessage> :
        <Table headers={['Run', 'Seed', 'Saved', 'Baseline trips', 'FleetFlow trips', 'Mode']}>
          {(runs.data ?? []).map((run: SavedRun) => <tr key={run.id}><td>{run.id.slice(0, 8)}</td><td>{run.seed}</td>
            <td>{new Date(run.created_at).toLocaleString()}</td><td>{run.metrics.baseline?.completed_trips ?? '—'}</td>
            <td>{run.metrics.fleet_flow?.completed_trips ?? '—'}</td><td><Status value={run.demo ? 'demo' : 'live'} /></td></tr>)}
        </Table>}
      {!runs.loading && !runs.error && runs.data?.length === 0 && <p className="empty-state">No saved comparison runs.</p>}
    </Panel>
  </Page>
}

export function PerformancePage() {
  const metrics = useLoad<MetricsResponse>(api.metrics, [])
  const run = metrics.data?.run
  return <Page eyebrow="Analytics" title="Performance" description="Simulation metrics for the current paired comparison. Results are not field measurements.">
    {metrics.error && <ErrorMessage>{metrics.error}</ErrorMessage>}
    {metrics.loading ? <Loading /> : run ? <>
      <div className="metric-grid">
        <Metric label="Demand seed" value={String(run.seed)} note={`${run.vehicles} vehicles`} />
        <Metric label="Fleet share" value={`${formatNumber(run.fleet_share * 100)}%`} />
        <Metric label="Baseline trips" value={String(run.baseline.completed_trips)} />
        <Metric label="FleetFlow trips" value={String(run.fleet_flow.completed_trips)} />
      </div>
      <Panel title="Side-by-side results"><Table headers={['Measure', 'Baseline', 'FleetFlow']}>
        <tr><td>Average trip time</td><td>{formatNumber(run.baseline.average_trip_s)} s</td><td>{formatNumber(run.fleet_flow.average_trip_s)} s</td></tr>
        <tr><td>95th percentile trip</td><td>{formatNumber(run.baseline.p95_trip_s)} s</td><td>{formatNumber(run.fleet_flow.p95_trip_s)} s</td></tr>
        <tr><td>Saturated edges</td><td>{formatNumber(run.baseline.saturated_edges_pct)}%</td><td>{formatNumber(run.fleet_flow.saturated_edges_pct)}%</td></tr>
        <tr><td>Route changes</td><td>{formatNumber(run.baseline.route_changes, 0)}</td><td>{formatNumber(run.fleet_flow.route_changes, 0)}</td></tr>
      </Table></Panel>
    </> : <Panel title="No active run"><p className="muted">Start a comparison run to see live performance metrics.</p><Link className="button primary" to="/operations/runs">Open comparison runs</Link></Panel>}
  </Page>
}

export function FleetStatusPage() {
  const vehicles = useLoad(api.vehicles)
  const drivers = useLoad(api.drivers)
  const allVehicles: Vehicle[] = vehicles.data ?? []
  return <Page eyebrow="Resources" title="Fleet Status" description="Current vehicle availability and driver assignment overview.">
    <div className="metric-grid">
      <Metric label="Vehicles" value={String(allVehicles.length)} />
      <Metric label="Available" value={String(allVehicles.filter((v) => v.status === 'available').length)} />
      <Metric label="On trip" value={String(allVehicles.filter((v) => v.status === 'on_trip').length)} />
      <Metric label="Drivers" value={String(drivers.data?.length ?? 0)} />
    </div>
    <Panel title="Fleet register">{vehicles.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
      <Table headers={['Vehicle', 'Type', 'Status', 'Assigned driver']}>
        {allVehicles.map((v) => <tr key={v.id}><td>{v.name}</td><td>{v.type}</td><td><Status value={v.status} /></td>
          <td>{drivers.data?.find((d) => d.vehicle_id === v.id)?.name ?? 'Unassigned'}</td></tr>)}
      </Table>}</Panel>
  </Page>
}
