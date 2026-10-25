import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, type Coordinate, type OSRMRoute, type SimulatedTrack, type SimulationRun } from '../api/client'
import { RoutingMap } from '../map/RoutingMap'
import { ErrorMessage, formatNumber, Loading, Metric, Page, Panel, Status, Table, useLoad } from './Shared'

export const presets = {
  light: { label: 'Light shift', vehicles: 20, departure_window_s: 300 },
  peak: { label: 'Peak shift', vehicles: 50, departure_window_s: 900 },
  heavy: { label: 'High demand', vehicles: 100, departure_window_s: 1800 },
} as const

export function useSimulationRun(runID: string) {
  const [run, setRun] = useState<SimulationRun | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(Boolean(runID))

  useEffect(() => {
    let disposed = false
    let timeout = 0
    setRun(null)
    setError('')
    setLoading(Boolean(runID))
    if (!runID) return () => { disposed = true }

    const refresh = async () => {
      try {
        const latest = await api.simulationRun(runID)
        if (disposed) return
        setRun(latest)
        setError('')
        setLoading(false)
        if (latest.status === 'queued' || latest.status === 'running') {
          timeout = window.setTimeout(() => void refresh(), 1000)
        }
      } catch (cause) {
        if (!disposed) {
          setError(cause instanceof Error ? cause.message : 'Simulation run is unavailable.')
          setLoading(false)
        }
      }
    }
    void refresh()
    return () => {
      disposed = true
      window.clearTimeout(timeout)
    }
  }, [runID])

  return { run, error, loading }
}

export function TripsPage() {
  const runs = useLoad(api.simulationRuns)
  const tripsData = useLoad(api.trips)
  const vehicles = useLoad(api.vehicles)
  const [searchParams, setSearchParams] = useSearchParams()
  const selectedID = searchParams.get('run') ?? ''
  const vehicleFilter = searchParams.get('vehicle') ?? ''
  const selected = useSimulationRun(selectedID)

  const [activeTab, setActiveTab] = useState<'all' | 'planned' | 'in_progress' | 'completed'>('all')
  const [origin, setOrigin] = useState('')
  const [destination, setDestination] = useState('')
  const [assignedVehicle, setAssignedVehicle] = useState('')
  const [dispatchError, setDispatchError] = useState('')
  const [dispatchBusy, setDispatchBusy] = useState(false)

  const allOperationalTrips = tripsData.data ?? []
  const filteredTrips = allOperationalTrips.filter((t) => {
    if (vehicleFilter && t.vehicle_id !== vehicleFilter) return false
    if (activeTab === 'all') return true
    return t.status === activeTab
  })

  const plannedCount = allOperationalTrips.filter((t) => t.status === 'planned').length
  const inProgressCount = allOperationalTrips.filter((t) => t.status === 'in_progress').length
  const completedCount = allOperationalTrips.filter((t) => t.status === 'completed').length

  const handleCreateTrip = async (e: FormEvent) => {
    e.preventDefault()
    setDispatchError('')
    setDispatchBusy(true)
    try {
      await api.createTrip({
        origin,
        destination,
        vehicle_id: assignedVehicle || undefined,
        status: 'planned',
      })
      setOrigin('')
      setDestination('')
      setAssignedVehicle('')
      await tripsData.reload()
    } catch (err) {
      setDispatchError(err instanceof Error ? err.message : 'Could not create trip.')
    } finally {
      setDispatchBusy(false)
    }
  }

  const handleAdvanceStatus = async (tripId: string, nextStatus: 'in_progress' | 'completed') => {
    try {
      await api.updateTripStatus(tripId, nextStatus)
      await tripsData.reload()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to advance trip status.')
    }
  }

  return (
    <Page
      eyebrow="Operations"
      title="Fleet Trips"
      description="Manage planned dispatches, active vehicle missions, past completed trips, and scenario simulation logs."
      action={<Link className="button primary" to="/operations">Live map & simulation</Link>}
    >
      <div className="metric-grid">
        <Metric label="Planned Dispatches" value={String(plannedCount)} />
        <Metric label="Active In-Progress" value={String(inProgressCount)} />
        <Metric label="Past Completed Trips" value={String(completedCount)} />
        <Metric label="Simulation Runs" value={String(runs.data?.length ?? 0)} />
      </div>

      <Panel
        title="Operational Fleet Trips"
        action={
          <div className="form-row" style={{ margin: 0, gap: '6px' }}>
            <button
              className={`button ${activeTab === 'all' ? 'primary' : ''}`}
              type="button"
              onClick={() => setActiveTab('all')}
            >
              All ({allOperationalTrips.length})
            </button>
            <button
              className={`button ${activeTab === 'planned' ? 'primary' : ''}`}
              type="button"
              onClick={() => setActiveTab('planned')}
            >
              Planned ({plannedCount})
            </button>
            <button
              className={`button ${activeTab === 'in_progress' ? 'primary' : ''}`}
              type="button"
              onClick={() => setActiveTab('in_progress')}
            >
              Active ({inProgressCount})
            </button>
            <button
              className={`button ${activeTab === 'completed' ? 'primary' : ''}`}
              type="button"
              onClick={() => setActiveTab('completed')}
            >
              Past / Completed ({completedCount})
            </button>
            <button className="button" type="button" onClick={() => void tripsData.reload()}>
              Refresh
            </button>
          </div>
        }
      >
        {vehicleFilter && (
          <p className="help-text">
            Filtering trips for vehicle: <strong>{vehicleFilter}</strong>.{' '}
            <button className="button" style={{ padding: '2px 8px', fontSize: '11px' }} onClick={() => setSearchParams({})}>
              Clear filter
            </button>
          </p>
        )}

        {tripsData.loading ? (
          <Loading />
        ) : tripsData.error ? (
          <ErrorMessage>{tripsData.error}</ErrorMessage>
        ) : (
          <Table headers={['Trip ID', 'Origin', 'Destination', 'Assigned Vehicle', 'Status', 'Started', 'Finished / Duration', 'Actions']}>
            {filteredTrips.map((t) => (
              <tr key={t.id}>
                <td>
                  <Link to={`/operations/trips/operational/${encodeURIComponent(t.id)}`} title="Open trip details and route map">
                    <strong>{(t.id || '').slice(0, 12) || 'Trip'}</strong>
                  </Link>
                </td>
                <td>{t.origin}</td>
                <td>{t.destination}</td>
                <td>
                  {t.vehicle_id ? (
                    <Link to={`/operations/fleet?vehicle=${encodeURIComponent(t.vehicle_id)}`}>
                      {(vehicles.data ?? []).find((v) => v.id === t.vehicle_id)?.name ?? t.vehicle_id}
                    </Link>
                  ) : (
                    <span className="muted">Unassigned</span>
                  )}
                </td>
                <td><Status value={t.status} /></td>
                <td>{t.started_at ? new Date(t.started_at).toLocaleTimeString() : '—'}</td>
                <td>
                  {t.duration_s != null
                    ? `${formatNumber(t.duration_s / 60, 1)} min`
                    : t.finished_at
                    ? new Date(t.finished_at).toLocaleTimeString()
                    : '—'}
                </td>
                <td>
                  <Link
                    className="button"
                    style={{ padding: '4px 8px', fontSize: '12px', marginRight: '6px' }}
                    to={`/operations/trips/operational/${encodeURIComponent(t.id)}`}
                  >
                    Open trip
                  </Link>
                  {t.status === 'planned' && (
                    <button
                      className="button primary"
                      style={{ padding: '4px 8px', fontSize: '12px' }}
                      type="button"
                      onClick={() => handleAdvanceStatus(t.id, 'in_progress')}
                    >
                      Start Trip
                    </button>
                  )}
                  {t.status === 'in_progress' && (
                    <button
                      className="button"
                      style={{ padding: '4px 8px', fontSize: '12px', borderColor: '#0ca678', color: '#0ca678' }}
                      type="button"
                      onClick={() => handleAdvanceStatus(t.id, 'completed')}
                    >
                      Complete Trip
                    </button>
                  )}
                  {t.status === 'completed' && <span className="muted">Archived</span>}
                </td>
              </tr>
            ))}
          </Table>
        )}
        {!tripsData.loading && !tripsData.error && filteredTrips.length === 0 && (
          <p className="empty-state">No trips found in this view. Use the form below or the Route Planner to create trips.</p>
        )}
      </Panel>

      <Panel title="Quick Trip Dispatch">
        <form className="form-row" onSubmit={(e) => void handleCreateTrip(e)} style={{ alignItems: 'flex-end', flexWrap: 'wrap', gap: '12px' }}>
          <label className="field">
            Origin
            <input
              required
              placeholder="e.g. Tanjung Priok Port"
              value={origin}
              onChange={(e) => setOrigin(e.target.value)}
            />
          </label>
          <label className="field">
            Destination
            <input
              required
              placeholder="e.g. Kelapa Gading DC"
              value={destination}
              onChange={(e) => setDestination(e.target.value)}
            />
          </label>
          <label className="field">
            Assign Vehicle
            <select value={assignedVehicle} onChange={(e) => setAssignedVehicle(e.target.value)}>
              <option value="">Unassigned</option>
              {(vehicles.data ?? []).map((v) => (
                <option key={v.id} value={v.id}>{v.name} ({v.model || v.type})</option>
              ))}
            </select>
          </label>
          <button className="button primary" type="submit" disabled={dispatchBusy}>
            {dispatchBusy ? 'Dispatching…' : 'Schedule Planned Trip'}
          </button>
        </form>
        {dispatchError && <ErrorMessage>{dispatchError}</ErrorMessage>}
      </Panel>

      <Panel title="Scenario Simulation Runs" action={<button className="button" type="button" onClick={() => void runs.reload()}>Refresh</button>}>
        {runs.loading ? <Loading /> : runs.error ? <ErrorMessage>{runs.error}</ErrorMessage> :
          <Table headers={['Run', 'Status', 'Seed', 'Requested vehicles', 'Created', 'Open']}>
            {(runs.data ?? []).map((run) => <tr key={run.id}>
              <td>{(run.id || '').slice(0, 12)}</td><td><Status value={run.status} /></td><td>{run.scenario.seed}</td>
              <td>{run.scenario.vehicles}</td><td>{new Date(run.created_at).toLocaleString()}</td>
              <td><button className="button" type="button" onClick={() => setSearchParams({ run: run.id })}>View simulation trips</button></td>
            </tr>)}
          </Table>}
        {!runs.loading && !runs.error && runs.data?.length === 0 &&
          <p className="empty-state">No scenario runs yet. Run a simulation from the Live Map to generate scenario logs.</p>}
      </Panel>

      {selectedID && <Panel title="Simulated Scenario Trips">
        {selected.loading ? <Loading /> : selected.error ? <ErrorMessage>{selected.error}</ErrorMessage> :
          selected.run?.status === 'failed' ? <ErrorMessage>{selected.run.error || 'Simulation run failed.'}</ErrorMessage> :
            selected.run?.result ? <Table headers={['Trip', 'Vehicle', 'Origin', 'Destination', 'Status', 'Distance', 'Duration', 'Fuel estimate', 'CO₂ estimate', 'Details']}>
              {(selected.run.result.trips ?? []).map((trip) => <tr key={trip.id}>
                <td>{trip.id}</td>
                <td>
                  {trip.vehicle_id
                    ? <Link to={`/operations/fleet?vehicle=${encodeURIComponent(trip.vehicle_id)}`}>{trip.vehicle_id}</Link>
                    : <span className="muted">Not recorded</span>}
                </td>
                <td>{trip.origin || <span className="muted">Not recorded</span>}</td>
                <td>{trip.destination || <span className="muted">Not recorded</span>}</td>
                <td><Status value={trip.status} /></td>
                <td>{formatNumber(trip.distance_m / 1000, 2)} km</td>
                <td>{formatNumber(trip.duration_s / 60, 1)} min</td>
                <td>{formatNumber(trip.fuel_kg, 3)} kg</td><td>{formatNumber(trip.co2_kg, 3)} kg</td>
                <td><Link to={`/operations/trips/${selectedID}/${encodeURIComponent(trip.id)}`}>Open trip</Link></td>
              </tr>)}
            </Table> : <p className="empty-state">Trip details are available when this run completes.</p>}
        {selected.run?.status === 'completed' && <p className="help-text">
          {selected.run.result?.emissions
            ? `Fuel and CO₂ are ${selected.run.result.emissions.model} ${selected.run.result.emissions.class} estimates (${selected.run.result.emissions.output_unit}); they are not measured fleet data.`
            : 'Emission model provenance is unavailable for this stored run.'}
        </p>}
      </Panel>}
    </Page>
  )
}

export function SimulationPage() {
  const runs = useLoad(api.simulationRuns)
  const [searchParams, setSearchParams] = useSearchParams()
  const selectedID = searchParams.get('run') ?? ''
  const selected = useSimulationRun(selectedID)
  const [preset, setPreset] = useState<keyof typeof presets>('light')
  const [seed, setSeed] = useState(42)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const active = (runs.data ?? []).some((run) => run.status === 'queued' || run.status === 'running')

  const start = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const { vehicles, departure_window_s } = presets[preset]
      const run = await api.startSimulation({ seed, vehicles, departure_window_s })
      setSearchParams({ run: run.id })
      setMessage(`Simulation run ${run.id.slice(0, 12)} queued.`)
      await runs.reload()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not start simulation.')
    } finally {
      setBusy(false)
    }
  }

  return <Page eyebrow="Operations" title="Simulations" description="Generate repeatable vehicle trips and review their simulated routes and history.">
    <Panel title="Start a seeded scenario">
      <form className="form-row" onSubmit={(event) => void start(event)}>
        <label className="field">Demand profile
          <select value={preset} onChange={(event) => setPreset(event.target.value as keyof typeof presets)}>
            {Object.entries(presets).map(([id, value]) =>
              <option key={id} value={id}>{value.label} · {value.vehicles} trips over {value.departure_window_s} s</option>)}
          </select>
        </label>
        <label className="field">Random seed
          <input type="number" min="0" max="2147483647" value={seed}
            onChange={(event) => setSeed(Number(event.target.value))} required />
        </label>
        <button className="button primary" type="submit" disabled={busy || active}>
          {busy ? 'Starting…' : active ? 'A scenario is running' : 'Run simulation'}
        </button>
      </form>
      <p className="help-text">The same network, profile, and seed make demand generation repeatable. This output is simulated, not live GPS.</p>
      {message && <p className="success" role="status">{message}</p>}
      {error && <ErrorMessage>{error}</ErrorMessage>}
    </Panel>
    <Panel title="Run history" action={<button className="button" type="button" onClick={() => void runs.reload()}>Refresh</button>}>
      {runs.loading ? <Loading /> : runs.error ? <ErrorMessage>{runs.error}</ErrorMessage> :
        <Table headers={['Run', 'Status', 'Seed', 'Vehicles', 'Created', 'Open']}>
          {(runs.data ?? []).map((run) => <tr key={run.id}>
            <td>{run.id.slice(0, 12)}</td><td><Status value={run.status} /></td><td>{run.scenario.seed}</td>
            <td>{run.scenario.vehicles}</td><td>{new Date(run.created_at).toLocaleString()}</td>
            <td><button className="button" type="button" onClick={() => setSearchParams({ run: run.id })}>Inspect</button></td>
          </tr>)}
        </Table>}
      {!runs.loading && !runs.error && runs.data?.length === 0 && <p className="empty-state">No simulation runs yet.</p>}
    </Panel>
    {selectedID && <Panel title="Selected run">
      {selected.loading ? <Loading /> : selected.error ? <ErrorMessage>{selected.error}</ErrorMessage> :
        selected.run?.status === 'failed' ? <ErrorMessage>{selected.run.error || 'Simulation run failed.'}</ErrorMessage> :
          selected.run?.result ? <>
            <div className="metric-grid">
              <Metric label="Trips generated" value={String(selected.run.result.trips.length)} />
              <Metric label="Completed" value={String(selected.run.result.trips.filter((trip) => trip.status === 'completed').length)} />
              <Metric label="Version" value={selected.run.result.sumo_version} />
              <Metric label="Seed" value={String(selected.run.scenario.seed)} />
            </div>
            <RoutingMap positionMode="simulation" simulationTracks={selected.run.result.tracks} />
          </> : <p className="empty-state">Simulation run is {selected.run?.status ?? 'loading'}; results will appear when it completes.</p>}
    </Panel>}
  </Page>
}

export function SimulationTripPage() {
  const { runID = '', tripID = '' } = useParams()
  const selected = useSimulationRun(runID)
  const trip = selected.run?.result?.trips.find((item) => item.id === tripID)
  // A trip names its vehicle; SUMO output uses the trip id as the vehicle id,
  // so fall back to the id for older stored runs.
  const trackKey = trip?.vehicle_id || tripID
  const track = selected.run?.result?.tracks.find((item) => item.vehicle_id === trackKey)
  const delivery = selected.run?.result?.deliveries?.find((item) => item.trip_id === tripID)
  const positions = track?.positions ?? []
  const route: SimulatedTrack[] = track ? [track] : []

  if (selected.loading) return <Page eyebrow="Trips" title="Trip details" description="Loading trip history."><Loading /></Page>
  if (selected.error) return <Page eyebrow="Trips" title="Trip details" description="The trip could not be loaded."><ErrorMessage>{selected.error}</ErrorMessage></Page>
  if (!selected.run?.result || !trip) {
    return <Page eyebrow="Trips" title="Trip details" description="This trip is unavailable in the selected simulation run.">
      <ErrorMessage>{selected.run?.error || 'Trip result was not recorded.'}</ErrorMessage>
    </Page>
  }

  const origin = positions[0]
  const destination = positions.at(-1)
  return <Page eyebrow="Trips" title={trip.id} description={`Simulated vehicle route from run ${runID.slice(0, 12)}.`}
    action={<Link className="button" to={`/operations/trips?run=${encodeURIComponent(runID)}`}>Back to trips</Link>}>
    <div className="metric-grid">
      <Metric label="Vehicle" value={trip.vehicle_id || 'Not recorded'} />
      <Metric label="Status" value={trip.status} />
      <Metric label="Distance" value={`${formatNumber(trip.distance_m / 1000, 2)} km`} />
      <Metric label="Duration" value={`${formatNumber(trip.duration_s / 60, 1)} min`} />
      <Metric label="CO₂ estimate" value={`${formatNumber(trip.co2_kg, 3)} kg`} note="Model output" />
    </div>
    <RoutingMap positionMode="simulation" simulationTracks={route} simulationStops={delivery?.stops ?? []} />
    <Panel title="Emission estimate provenance">
      {selected.run.result.emissions
        ? <p className="help-text">
          {selected.run.result.emissions.model} · {selected.run.result.emissions.class} ·
          {' '}{selected.run.result.emissions.source} · values converted from {selected.run.result.emissions.input_unit} to {selected.run.result.emissions.output_unit}.
          {' '}{selected.run.result.emissions.assumptions}
        </p>
        : <p className="empty-state">Emission model provenance is unavailable for this stored run.</p>}
    </Panel>
    <Panel title="Simulated route history">
      <p className="help-text">
        Origin: {origin ? `${origin.latitude.toFixed(5)}, ${origin.longitude.toFixed(5)}` : 'not recorded'} ·
        Destination: {destination ? `${destination.latitude.toFixed(5)}, ${destination.longitude.toFixed(5)}` : 'not recorded'}
      </p>
      {positions.length > 0 ?
        <Table headers={['Simulation time', 'Latitude', 'Longitude']}>
          {positions.map((position, index) => <tr key={`${position.time_s}-${index}`}>
            <td>{formatNumber(position.time_s, 0)} s</td><td>{position.latitude.toFixed(5)}</td><td>{position.longitude.toFixed(5)}</td>
          </tr>)}
        </Table> : <p className="empty-state">No geographic position trace was recorded for this trip.</p>}
    </Panel>
    <Panel title="Generated delivery stops">
      {delivery?.stops.length
        ? <>
          <p className="help-text">Seeded simulation stops inferred from vehicle movement; these are not scanned or live package events.</p>
          <Table headers={['Stop', 'Package', 'Simulation time', 'Latitude', 'Longitude', 'Status']}>
            {delivery.stops.map((stop) => <tr key={stop.id}>
              <td>{stop.order}</td><td>{stop.package_id}</td><td>{formatNumber(stop.time_s, 0)} s</td>
              <td>{stop.latitude.toFixed(5)}</td><td>{stop.longitude.toFixed(5)}</td><td><Status value={stop.status} /></td>
            </tr>)}
          </Table>
        </>
        : <p className="empty-state">Delivery stop history is unavailable for this stored run.</p>}
    </Panel>
    <Panel title="Package event timeline">
      {delivery?.events.length
        ? <>
          <p className="help-text">Events are ordered by simulation time and represent generated scenario history, not real scans.</p>
          <Table headers={['#', 'Simulation time', 'Event', 'Package', 'Stop']}>
            {delivery.events.map((event) => <tr key={`${event.sequence}-${event.type}`}>
              <td>{event.sequence}</td><td>{formatNumber(event.time_s, 0)} s</td>
              <td>{event.type.replaceAll('_', ' ')}</td><td>{event.package_id ?? '—'}</td><td>{event.stop_id ?? '—'}</td>
            </tr>)}
          </Table>
        </>
        : <p className="empty-state">Package event history is unavailable for this stored run.</p>}
    </Panel>
    <p className="help-text">Position samples, distance, duration, fuel, and emissions are simulation output, not real telemetry or measured activity.</p>
  </Page>
}

const jakartaHubLocations: Record<string, Coordinate> = {
  'tanjung priok': { lat: -6.1030, lon: 106.8833 },
  'pluit': { lat: -6.1245, lon: 106.7905 },
  'halim': { lat: -6.2625, lon: 106.8911 },
  'cilandak': { lat: -6.2921, lon: 106.8042 },
  'tomang': { lat: -6.1750, lon: 106.7915 },
  'kelapa gading': { lat: -6.1580, lon: 106.9080 },
  'glodok': { lat: -6.1432, lon: 106.8155 },
  'sudirman': { lat: -6.2150, lon: 106.8220 },
  'senayan': { lat: -6.2270, lon: 106.7970 },
  'grand indonesia': { lat: -6.1950, lon: 106.8215 },
  'soekarno-hatta': { lat: -6.1275, lon: 106.6537 },
  'cakung': { lat: -6.1950, lon: 106.9450 },
  'pasar senen': { lat: -6.1740, lon: 106.8420 },
  'marunda': { lat: -6.1050, lon: 106.9600 },
  'sunter': { lat: -6.1380, lon: 106.8680 },
  'kuningan': { lat: -6.2280, lon: 106.8300 },
}

function resolveLocation(name: string, fallback: Coordinate): Coordinate {
  const lower = name.toLowerCase()
  for (const [key, coord] of Object.entries(jakartaHubLocations)) {
    if (lower.includes(key)) return coord
  }
  const parts = name.split(',').map((p) => Number(p.trim()))
  if (parts.length === 2 && !isNaN(parts[0]!) && !isNaN(parts[1]!)) {
    return { lat: parts[0]!, lon: parts[1]! }
  }
  return fallback
}

export function OperationalTripPage() {
  const { tripID = '' } = useParams()
  const trips = useLoad(api.trips)
  const vehicles = useLoad(api.vehicles)
  const drivers = useLoad(api.drivers)

  const trip = (trips.data ?? []).find((t) => t.id === tripID)
  const vehicle = (vehicles.data ?? []).find((v) => v.name === trip?.vehicle_id || v.id === trip?.vehicle_id)
  const driver = (drivers.data ?? []).find((d) => d.vehicle_id === vehicle?.id)

  const [route, setRoute] = useState<OSRMRoute | null>(null)
  const [routeLoading, setRouteLoading] = useState(false)

  const from = resolveLocation(trip?.origin ?? '', { lat: -6.1750, lon: 106.8215 })
  const to = resolveLocation(trip?.destination ?? '', { lat: -6.2150, lon: 106.8220 })

  useEffect(() => {
    if (!trip) return
    let disposed = false
    setRouteLoading(true)
    api.route(from, to)
      .then((r) => {
        if (!disposed) {
          setRoute(r)
          setRouteLoading(false)
        }
      })
      .catch(() => {
        if (!disposed) setRouteLoading(false)
      })
    return () => {
      disposed = true
    }
  }, [trip?.origin, trip?.destination])

  const handleAdvance = async (nextStatus: 'in_progress' | 'completed') => {
    if (!trip) return
    try {
      await api.updateTripStatus(trip.id, nextStatus)
      await trips.reload()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to update trip.')
    }
  }

  if (trips.loading) {
    return <Page eyebrow="Operations" title="Fleet trip details" description="Loading fleet trip."><Loading /></Page>
  }
  if (trips.error) {
    return <Page eyebrow="Operations" title="Fleet trip details" description="Trip error."><ErrorMessage>{trips.error}</ErrorMessage></Page>
  }
  if (!trip) {
    return (
      <Page eyebrow="Operations" title="Trip details" description="Trip not found." action={<Link className="button" to="/operations/trips">Back to trips</Link>}>
        <ErrorMessage>Trip with ID "{tripID}" could not be found.</ErrorMessage>
      </Page>
    )
  }

  return (
    <Page
      eyebrow="Operations"
      title={`Trip ${trip.id.slice(0, 12)}`}
      description={`Operational fleet mission from ${trip.origin} to ${trip.destination}.`}
      action={<Link className="button" to="/operations/trips">Back to trips</Link>}
    >
      <div className="metric-grid">
        <Metric label="Status" value={trip.status.replace('_', ' ').toUpperCase()} />
        <Metric label="Origin" value={trip.origin} />
        <Metric label="Destination" value={trip.destination} />
        <Metric label="Assigned Vehicle" value={trip.vehicle_id || 'Unassigned'} />
        {route && <Metric label="Estimated Distance" value={`${formatNumber(route.distance_m / 1000, 2)} km`} />}
        {route && <Metric label="Estimated Duration" value={`${formatNumber(route.duration_s / 60, 1)} min`} />}
      </div>

      <Panel
        title="Trip Route & Map"
        action={
          <div style={{ display: 'flex', gap: '8px' }}>
            {trip.status === 'planned' && (
              <button className="button primary" type="button" onClick={() => void handleAdvance('in_progress')}>
                Start Trip
              </button>
            )}
            {trip.status === 'in_progress' && (
              <button
                className="button"
                style={{ borderColor: '#0ca678', color: '#0ca678' }}
                type="button"
                onClick={() => void handleAdvance('completed')}
              >
                Complete Trip
              </button>
            )}
          </div>
        }
      >
        {routeLoading && <Loading />}
        <RoutingMap
          selectedPoints={{ from, to }}
          route={route}
        />
      </Panel>

      <Panel title="Trip Mission Specifications">
        <Table headers={['Field', 'Value']}>
          <tr><th scope="row">Trip ID</th><td>{trip.id}</td></tr>
          <tr><th scope="row">Origin Facility</th><td>{trip.origin} ({from.lat.toFixed(4)}, {from.lon.toFixed(4)})</td></tr>
          <tr><th scope="row">Destination Facility</th><td>{trip.destination} ({to.lat.toFixed(4)}, {to.lon.toFixed(4)})</td></tr>
          <tr><th scope="row">Status</th><td><Status value={trip.status} /></td></tr>
          <tr>
            <th scope="row">Assigned Vehicle</th>
            <td>
              {vehicle ? (
                <Link to={`/resources/vehicles/${encodeURIComponent(vehicle.id)}`}>
                  {vehicle.name} ({vehicle.model || vehicle.type})
                </Link>
              ) : (
                trip.vehicle_id || 'Unassigned'
              )}
            </td>
          </tr>
          <tr>
            <th scope="row">Assigned Driver</th>
            <td>
              {driver ? (
                <Link to={`/resources/drivers/${encodeURIComponent(driver.id)}`}>
                  {driver.name} ({driver.contact_phone || 'Available'})
                </Link>
              ) : (
                'Unassigned'
              )}
            </td>
          </tr>
          <tr><th scope="row">Started At</th><td>{trip.started_at ? new Date(trip.started_at).toLocaleString() : 'Not started yet'}</td></tr>
          <tr><th scope="row">Finished At</th><td>{trip.finished_at ? new Date(trip.finished_at).toLocaleString() : 'In progress or scheduled'}</td></tr>
          <tr><th scope="row">Recorded Duration</th><td>{trip.duration_s != null ? `${formatNumber(trip.duration_s / 60, 1)} minutes` : '—'}</td></tr>
        </Table>
      </Panel>
    </Page>
  )
}
