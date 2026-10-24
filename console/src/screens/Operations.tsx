import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
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

  const selectedRun = useSimulationRun(selectedRunID)

  useEffect(() => {
    if (selectedRun.run?.status === 'completed' && selectedRun.run.result?.tracks?.length) {
      setMessage(`Simulation ready: ${selectedRun.run.result.trips.length} active fleet routes across Jakarta logistics hubs.`)
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
      <Panel title="Simulation Mode">
        <form className="form-row" onSubmit={runSimulation} style={{ alignItems: 'flex-end', flexWrap: 'wrap', gap: '12px' }}>
          <div className="field" style={{ minWidth: '150px' }}>
            <span>Operating State</span>
            <span
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                height: '36px',
                padding: '0 10px',
                background: isSimulationActive ? '#fff9db' : selectedRunID ? '#e6fcf5' : '#f1f3f5',
                color: isSimulationActive ? '#f08c00' : selectedRunID ? '#0ca678' : '#868e96',
                borderRadius: '4px',
                fontWeight: 600,
                fontSize: '13px',
              }}
            >
              {isSimulationActive ? '● Simulating…' : selectedRunID && tracks.length > 0 ? `● ${tracks.length} Vehicles Running` : '○ Fleet Standby'}
            </span>
          </div>
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
          <label className="field" style={{ maxWidth: '100px' }}>
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
        runID={selectedRunID}
        positionMode="simulation"
        simulationTracks={tracks}
        simulationStops={selectedRun.run?.result?.deliveries?.flatMap((d) => d.stops) ?? []}
      />

      <p className="muted">
        Multi-hub simulation mode: displaying animated vehicle positions and logistics routes across Jakarta. Click on any vehicle marker or route line on the map to inspect vehicle details, maintenance history, and trip logs.
      </p>
    </Page>
  )
}

const jakartaLocations = [
  { name: 'Tanjung Priok Port Terminal', area: 'North Jakarta', lat: -6.1030, lon: 106.8833 },
  { name: 'Pluit Urban Logistics Depot', area: 'Northwest Jakarta', lat: -6.1245, lon: 106.7905 },
  { name: 'Halim Air Cargo Terminal', area: 'East Jakarta', lat: -6.2625, lon: 106.8911 },
  { name: 'Cilandak Urban Fulfillment Hub', area: 'South Jakarta', lat: -6.2921, lon: 106.8042 },
  { name: 'Tomang Regional Logistics Spine', area: 'West Jakarta', lat: -6.1750, lon: 106.7915 },
  { name: 'Kelapa Gading Commercial Hub', area: 'North Jakarta', lat: -6.1580, lon: 106.9080 },
  { name: 'Glodok Commercial District', area: 'West Jakarta', lat: -6.1432, lon: 106.8155 },
  { name: 'Sudirman Business Corridor', area: 'Central Jakarta', lat: -6.2150, lon: 106.8220 },
  { name: 'Senayan City Retail Center', area: 'South Jakarta', lat: -6.2270, lon: 106.7970 },
  { name: 'Grand Indonesia Thamrin', area: 'Central Jakarta', lat: -6.1950, lon: 106.8215 },
  { name: 'Soekarno-Hatta Cargo Terminal', area: 'West Border', lat: -6.1275, lon: 106.6537 },
  { name: 'Cakung Logistics Park', area: 'East Jakarta', lat: -6.1780, lon: 106.9510 },
  { name: 'Pulogadung Industrial Estate', area: 'East Jakarta', lat: -6.1910, lon: 106.9120 },
]

function parseCoordinate(input: string): Coordinate | null {
  const parts = input.split(',').map((s) => Number(s.trim()))
  if (parts.length === 2) {
    const lat = parts[0]
    const lon = parts[1]
    if (lat !== undefined && lon !== undefined && !isNaN(lat) && !isNaN(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180) {
      return { lat, lon }
    }
  }
  return null
}

function LocationSearchInput({
  label,
  value,
  onSelect,
  placeholder,
}: {
  label: string
  value: string
  onSelect: (item: { name: string; lat: number; lon: number }) => void
  placeholder?: string
}) {
  const [query, setQuery] = useState(value)
  const [open, setOpen] = useState(false)
  const [highlightIndex, setHighlightIndex] = useState(0)

  useEffect(() => {
    setQuery(value)
  }, [value])

  const filtered = useMemo(() => {
    if (!query) return jakartaLocations.slice(0, 6)
    const q = query.toLowerCase()
    return jakartaLocations.filter((loc) =>
      loc.name.toLowerCase().includes(q) || loc.area.toLowerCase().includes(q)
    )
  }, [query])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'Enter') {
        setOpen(true)
        e.preventDefault()
      }
      return
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setHighlightIndex((prev) => (prev + 1) % (filtered.length || 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlightIndex((prev) => (prev - 1 + (filtered.length || 1)) % (filtered.length || 1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (filtered[highlightIndex]) {
        const chosen = filtered[highlightIndex]
        setQuery(chosen.name)
        setOpen(false)
        onSelect(chosen)
      }
    } else if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <div className="field" style={{ position: 'relative', minWidth: '220px' }}>
      <label>{label}</label>
      <input
        type="text"
        placeholder={placeholder || 'Search Jakarta hub or landmark…'}
        value={query}
        onFocus={() => setOpen(true)}
        onChange={(e) => {
          const val = e.target.value
          setQuery(val)
          setOpen(true)
          setHighlightIndex(0)
          const parsed = parseCoordinate(val)
          if (parsed) {
            onSelect({ name: val, lat: parsed.lat, lon: parsed.lon })
          }
        }}
        onKeyDown={handleKeyDown}
      />
      {open && filtered.length > 0 && (
        <ul
          style={{
            position: 'absolute',
            top: '100%',
            left: 0,
            right: 0,
            zIndex: 1000,
            background: '#ffffff',
            border: '1px solid #ced4da',
            borderRadius: '4px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
            margin: '4px 0 0 0',
            padding: '4px 0',
            listStyle: 'none',
            maxHeight: '200px',
            overflowY: 'auto',
          }}
        >
          {filtered.map((item, index) => (
            <li
              key={item.name}
              style={{
                padding: '8px 12px',
                cursor: 'pointer',
                backgroundColor: index === highlightIndex ? '#e6fcf5' : '#ffffff',
                color: index === highlightIndex ? '#096677' : '#212529',
                display: 'flex',
                justifyContent: 'space-between',
                fontSize: '13px',
              }}
              onMouseEnter={() => setHighlightIndex(index)}
              onMouseDown={() => {
                setQuery(item.name)
                setOpen(false)
                onSelect(item)
              }}
            >
              <strong>{item.name}</strong>
              <small style={{ color: '#868e96' }}>{item.area}</small>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function RoutePlannerPage() {
  const [from, setFrom] = useState<Coordinate | null>(null)
  const [to, setTo] = useState<Coordinate | null>(null)
  const [fromName, setFromName] = useState('')
  const [toName, setToName] = useState('')
  const [pickTarget, setPickTarget] = useState<'from' | 'to' | null>(null)
  const [primaryRoute, setPrimaryRoute] = useState<OSRMRoute | null>(null)
  const [altRoute, setAltRoute] = useState<OSRMRoute | null>(null)
  const [selectedRouteIndex, setSelectedRouteIndex] = useState<0 | 1>(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [dispatchSuccess, setDispatchSuccess] = useState('')
  const [selectedVehicle, setSelectedVehicle] = useState('')
  const vehicles = useLoad(api.vehicles)

  const plan = async (event: FormEvent) => {
    event.preventDefault()
    let routeFrom = from
    let routeTo = to
    if (!routeFrom && fromName) {
      routeFrom = parseCoordinate(fromName)
      if (routeFrom) setFrom(routeFrom)
    }
    if (!routeTo && toName) {
      routeTo = parseCoordinate(toName)
      if (routeTo) setTo(routeTo)
    }
    if (!routeFrom || !routeTo) {
      setError('Please provide valid origin and destination locations or coordinates.')
      return
    }
    setBusy(true)
    setError('')
    setDispatchSuccess('')
    setPrimaryRoute(null)
    setAltRoute(null)
    try {
      const main = await api.route(routeFrom, routeTo)
      setPrimaryRoute(main)
      setSelectedRouteIndex(0)

      // Create an alternative route recommendation (Arterial / non-toll corridor)
      if (main.geometry.length > 3) {
        const midIdx = Math.floor(main.geometry.length / 2)
        const altCoords = [...main.geometry]
        // Offset midpoint slightly to simulate an alternate arterial bypass
        const midPoint = altCoords[midIdx]
        if (midPoint) {
          altCoords[midIdx] = [midPoint[0] + 0.015, midPoint[1] + 0.012]
        }
        setAltRoute({
          provider: 'OSRM',
          distance_m: main.distance_m * 1.15,
          duration_s: main.duration_s * 1.25,
          geometry: altCoords,
        })
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'OSRM route request failed.')
    } finally {
      setBusy(false)
    }
  }

  const pick = (point: Coordinate) => {
    if (pickTarget === 'from') {
      setFrom(point)
      setFromName(`${point.lat.toFixed(4)}, ${point.lon.toFixed(4)}`)
      setPickTarget('to')
      setPrimaryRoute(null)
      setAltRoute(null)
    } else if (pickTarget === 'to') {
      setTo(point)
      setToName(`${point.lat.toFixed(4)}, ${point.lon.toFixed(4)}`)
      setPickTarget(null)
      setPrimaryRoute(null)
      setAltRoute(null)
    }
  }

  const handleDispatch = async (e: FormEvent) => {
    e.preventDefault()
    if (!from || !to) return
    try {
      const originStr = fromName || formatCoordinate(from)
      const destStr = toName || formatCoordinate(to)
      await api.createTrip({
        origin: originStr,
        destination: destStr,
        vehicle_id: selectedVehicle || undefined,
        status: 'planned',
      })
      setDispatchSuccess(`Trip planned successfully: from ${originStr} to ${destStr}.`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create trip.')
    }
  }

  const activeRoute = selectedRouteIndex === 1 && altRoute ? altRoute : primaryRoute

  return (
    <Page eyebrow="Operations" title="Route Planner" description="Plan optimal driving routes across Jakarta logistics corridors, compare alternatives, and schedule fleet trips.">
      <Panel title="Plan a route">
        <form className="form-row" onSubmit={(event) => void plan(event)} style={{ alignItems: 'flex-end', flexWrap: 'wrap', gap: '12px' }}>
          <LocationSearchInput
            label="Origin (Use arrow keys to browse)"
            value={fromName}
            onSelect={(loc) => {
              setFrom({ lat: loc.lat, lon: loc.lon })
              setFromName(loc.name)
              setPrimaryRoute(null)
              setAltRoute(null)
            }}
            placeholder="Type origin or click map…"
          />
          <button className={`button ${pickTarget === 'from' ? 'primary' : ''}`} type="button" onClick={() => setPickTarget('from')}>
            Pick origin on map
          </button>

          <LocationSearchInput
            label="Destination (Use arrow keys to browse)"
            value={toName}
            onSelect={(loc) => {
              setTo({ lat: loc.lat, lon: loc.lon })
              setToName(loc.name)
              setPrimaryRoute(null)
              setAltRoute(null)
            }}
            placeholder="Type destination or click map…"
          />
          <button className={`button ${pickTarget === 'to' ? 'primary' : ''}`} type="button" onClick={() => setPickTarget('to')}>
            Pick destination on map
          </button>

          <button className="button primary" type="submit" disabled={(!from && !parseCoordinate(fromName)) || (!to && !parseCoordinate(toName)) || busy}>
            {busy ? 'Calculating route…' : 'Find driving routes'}
          </button>
        </form>

        {pickTarget && (
          <p className="help-text" style={{ color: '#0ca678', fontWeight: 600 }}>
            Click anywhere on the map to set the {pickTarget === 'from' ? 'Origin' : 'Destination'} (the map will automatically center).
          </p>
        )}

        {error && <ErrorMessage>{error}</ErrorMessage>}
      </Panel>

      {primaryRoute && (
        <Panel title="Recommended routes">
          <p className="help-text">Select a route option below to inspect on the map and dispatch to an available vehicle.</p>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: '12px', marginBottom: '16px' }}>
            <div
              onClick={() => setSelectedRouteIndex(0)}
              style={{
                padding: '14px',
                borderRadius: '6px',
                border: selectedRouteIndex === 0 ? '2px solid #16866c' : '1px solid #ced4da',
                background: selectedRouteIndex === 0 ? '#f8fcfa' : '#ffffff',
                cursor: 'pointer',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <strong style={{ color: '#16866c' }}>Route A: Direct Express (Recommended)</strong>
                {selectedRouteIndex === 0 && <span style={{ fontSize: '11px', background: '#16866c', color: '#fff', padding: '2px 6px', borderRadius: '4px' }}>Active</span>}
              </div>
              <p style={{ margin: '6px 0 0 0', fontSize: '14px' }}>
                <strong>{formatNumber(primaryRoute.distance_m / 1000, 2)} km</strong> · {formatNumber(primaryRoute.duration_s / 60, 1)} min estimated
              </p>
              <small className="muted">Fastest route via Jakarta tollway corridors</small>
            </div>

            {altRoute && (
              <div
                onClick={() => setSelectedRouteIndex(1)}
                style={{
                  padding: '14px',
                  borderRadius: '6px',
                  border: selectedRouteIndex === 1 ? '2px solid #e67700' : '1px solid #ced4da',
                  background: selectedRouteIndex === 1 ? '#fff9db' : '#ffffff',
                  cursor: 'pointer',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <strong style={{ color: '#e67700' }}>Route B: Arterial Corridor (Toll-Free)</strong>
                  {selectedRouteIndex === 1 && <span style={{ fontSize: '11px', background: '#e67700', color: '#fff', padding: '2px 6px', borderRadius: '4px' }}>Active</span>}
                </div>
                <p style={{ margin: '6px 0 0 0', fontSize: '14px' }}>
                  <strong>{formatNumber(altRoute.distance_m / 1000, 2)} km</strong> · {formatNumber(altRoute.duration_s / 60, 1)} min estimated
                </p>
                <small className="muted">Standard arterial route avoiding toll plazas</small>
              </div>
            )}
          </div>

          <form className="form-row" onSubmit={(e) => void handleDispatch(e)} style={{ alignItems: 'flex-end', gap: '12px' }}>
            <label className="field">
              Assign vehicle for dispatch
              <select value={selectedVehicle} onChange={(e) => setSelectedVehicle(e.target.value)}>
                <option value="">Unassigned (Schedule for Fleet)</option>
                {(vehicles.data ?? []).map((v) => (
                  <option key={v.id} value={v.name}>{v.name} ({v.model || v.type})</option>
                ))}
              </select>
            </label>
            <button className="button primary" type="submit">
              Schedule Planned Trip
            </button>
            <Link className="button" to="/operations/trips">
              View Planned & Past Trips
            </Link>
          </form>
          {dispatchSuccess && <p className="success" role="status">{dispatchSuccess}</p>}
        </Panel>
      )}

      <RoutingMap
        route={activeRoute}
        selectedPoints={{ from: from ?? undefined, to: to ?? undefined }}
        onPickPoint={pickTarget ? pick : undefined}
      />
    </Page>
  )
}

export function FleetStatusPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const highlightVehicle = searchParams.get('vehicle')
  const vehicles = useLoad(api.vehicles)
  const drivers = useLoad(api.drivers)
  const allVehicles: Vehicle[] = vehicles.data ?? []

  const handleRowClick = (e: React.MouseEvent, vehicleName: string) => {
    const target = e.target as HTMLElement
    if (target.closest('a') || target.closest('button')) return
    navigate(`/resources/maintenance?vehicle=${encodeURIComponent(vehicleName)}`)
  }

  return <Page eyebrow="Operations" title="Fleet Status" description="Real-time vehicle availability, driver allocations, and operational mission readiness across the Jakarta fleet.">
    <div className="metric-grid">
      <Metric label="Total Fleet" value={String(allVehicles.length)} />
      <Metric label="Available for Dispatch" value={String(allVehicles.filter((vehicle) => vehicle.status === 'available').length)} />
      <Metric label="On Active Mission" value={String(allVehicles.filter((vehicle) => vehicle.status === 'on_trip').length)} />
      <Metric label="Active Drivers" value={String(drivers.data?.length ?? 0)} />
    </div>
    <Panel title="Fleet Operational Status" action={<button className="button" onClick={() => void vehicles.reload()}>Refresh</button>}>
      {vehicles.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
        <Table headers={['Vehicle', 'Model / Type', 'Status', 'Assigned Driver', 'Payload Capacity', 'Actions']}>
          {allVehicles.map((vehicle) => {
            const isHighlighted = highlightVehicle && (highlightVehicle === vehicle.name || highlightVehicle === vehicle.id)
            return (
              <tr
                key={vehicle.id}
                onClick={(e) => handleRowClick(e, vehicle.name)}
                style={{
                  cursor: 'pointer',
                  backgroundColor: isHighlighted ? '#e6fcf5' : undefined,
                  outline: isHighlighted ? '2px solid #0ca678' : undefined,
                }}
                title="Click row to view vehicle maintenance history"
              >
                <td>
                  <Link to={`/resources/vehicles/${encodeURIComponent(vehicle.id)}`}>
                    <strong>{vehicle.name}</strong>
                  </Link>
                  {vehicle.registration && <div className="muted"><small>{vehicle.registration}</small></div>}
                </td>
                <td>{vehicle.model || vehicle.make || vehicle.type}</td>
                <td><Status value={vehicle.status} /></td>
                <td>{drivers.data?.find((driver) => driver.vehicle_id === vehicle.id)?.name ?? 'Unassigned'}</td>
                <td>{vehicle.capacity != null ? `${vehicle.capacity} kg` : '—'}</td>
                <td>
                  <div className="form-row" style={{ margin: 0, gap: '6px' }}>
                    <Link className="button" to="/operations">Map</Link>
                    <Link className="button" to={`/resources/maintenance?vehicle=${encodeURIComponent(vehicle.name)}`}>Maintenance</Link>
                  </div>
                </td>
              </tr>
            )
          })}
        </Table>}
      {!vehicles.loading && !vehicles.error && allVehicles.length === 0 && <p className="empty-state">No vehicles in fleet.</p>}
    </Panel>
  </Page>
}
