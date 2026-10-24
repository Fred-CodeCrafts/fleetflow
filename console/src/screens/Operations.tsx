import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, type Coordinate, type OSRMRoute, type Vehicle } from '../api/client'
import { RoutingMap } from '../map/RoutingMap'
import { ErrorMessage, formatNumber, Loading, Metric, Page, Panel, Status, Table, useLoad } from './Shared'
import { presets, useSimulationRun } from './Simulation'

function formatCoordinate(point: Coordinate | null) {
  return point ? `${point.lat.toFixed(5)}, ${point.lon.toFixed(5)}` : ''
}

export function LiveMapPage() {
  const runs = useLoad(api.simulationRuns)
  const [selectedRunID, setSelectedRunID] = useState<string>('')
  const [preset, setPreset] = useState<keyof typeof presets>('light')
  const [seed, setSeed] = useState(42)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [viewMode, setViewMode] = useState<'live' | 'simulation'>('live')

  const selectedRun = useSimulationRun(selectedRunID)

  useEffect(() => {
    if (selectedRun.run?.status === 'completed' && selectedRun.run.result?.tracks?.length) {
      setViewMode('simulation')
      setMessage(`Simulation completed: ${selectedRun.run.result.trips.length} trips generated.`)
    }
  }, [selectedRun.run?.status, selectedRun.run?.result?.tracks?.length, selectedRun.run?.result?.trips?.length])

  const runSimulation = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const { vehicles, departure_window_s } = presets[preset]
      const run = await api.startSimulation({ seed, vehicles, departure_window_s })
      setSelectedRunID(run.id)
      setMessage(`Simulation run queued. Generating traffic demand…`)
      await runs.reload()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not start simulation.')
    } finally {
      setBusy(false)
    }
  }

  const tracks = selectedRun.run?.result?.tracks ?? []
  const isSimulationActive = selectedRun.run?.status === 'queued' || selectedRun.run?.status === 'running'

  return (
    <Page
      eyebrow="Operations"
      title="Live Fleet Map"
      description="Monitor live GPS positions reported by fleet vehicles, or run traffic simulations on the Jakarta street network."
    >
      <Panel
        title="Simulation"
        action={
          <div className="form-row" style={{ margin: 0 }}>
            <button
              className={`button ${viewMode === 'live' ? 'primary' : ''}`}
              type="button"
              onClick={() => setViewMode('live')}
            >
              Live GPS mode
            </button>
            <button
              className={`button ${viewMode === 'simulation' ? 'primary' : ''}`}
              type="button"
              onClick={() => setViewMode('simulation')}
              disabled={tracks.length === 0}
            >
              Simulation mode {tracks.length > 0 ? `(${tracks.length} tracks)` : ''}
            </button>
          </div>
        }
      >
        <form className="form-row" onSubmit={runSimulation}>
          <label className="field">
            Demand profile
            <select value={preset} onChange={(e) => setPreset(e.target.value as keyof typeof presets)}>
              {Object.entries(presets).map(([id, value]) => (
                <option key={id} value={id}>
                  {value.label} · {value.vehicles} trips over {value.departure_window_s} s
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            Seed
            <input
              type="number"
              min="0"
              max="2147483647"
              value={seed}
              onChange={(e) => setSeed(Number(e.target.value))}
              required
            />
          </label>
          <button className="button primary" type="submit" disabled={busy || isSimulationActive}>
            {busy ? 'Starting…' : isSimulationActive ? 'Simulation in progress…' : 'Run simulation'}
          </button>
          {selectedRunID && (
            <Link className="button" to={`/operations/trips?run=${encodeURIComponent(selectedRunID)}`}>
              View trips
            </Link>
          )}
        </form>
        {isSimulationActive && <Loading />}
        {message && <p className="success" role="status">{message}</p>}
        {error && <ErrorMessage>{error}</ErrorMessage>}
      </Panel>

      <RoutingMap
        positionMode={viewMode === 'simulation' ? 'simulation' : 'live'}
        simulationTracks={viewMode === 'simulation' ? tracks : undefined}
      />

      <p className="muted">
        {viewMode === 'simulation'
          ? `Displaying simulated tracks on the map. Switch to Live GPS mode to monitor connected vehicle devices.`
          : 'Live GPS mode: waiting for connected fleet vehicles to report positions. Run a simulation to visualize traffic on the network.'}
      </p>
    </Page>
  )
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
