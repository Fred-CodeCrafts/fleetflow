import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api } from '../api/client'
import { ErrorMessage, Loading, Page, Panel, Status, Table, useLoad } from './Shared'
import { getVehicleModels, type VehicleModel } from './Models'

const initialVehicle = (models: VehicleModel[]) => {
  const first = models[0]
  return {
    name: '',
    type: (first?.type ?? 'van') as 'van' | 'truck' | 'car' | 'motorcycle',
    status: 'available' as 'available' | 'offline',
    registration: '',
    make: '',
    model: first?.name ?? '',
    year: '',
    color: '',
    body_type: first?.body_type ?? '',
    capacity: first ? String(first.capacity_kg) : '',
  }
}

const emptyDriver = () => ({
  name: '', contact_phone: '', license_class: '', license_expiry: '', shift_start: '', shift_end: '', availability: '', contract_type: 'Full-time',
})

const unknown = (value: string | number | null | undefined) => value ?? 'Unknown'

export function VehiclesPage() {
  const navigate = useNavigate()
  const vehicles = useLoad(api.vehicles)
  const drivers = useLoad(api.drivers)
  const availableModels = getVehicleModels()
  const [form, setForm] = useState(() => initialVehicle(availableModels))
  const [error, setError] = useState('')

  const handleRowClick = (e: React.MouseEvent, vehicleName: string) => {
    const target = e.target as HTMLElement
    if (target.closest('a') || target.closest('button') || target.closest('select') || target.closest('input')) return
    navigate(`/resources/maintenance?vehicle=${encodeURIComponent(vehicleName)}`)
  }

  const onSelectModel = (modelName: string) => {
    const selected = availableModels.find((m) => m.name === modelName)
    if (selected) {
      setForm((prev) => ({
        ...prev,
        model: selected.name,
        type: selected.type,
        body_type: selected.body_type,
        capacity: String(selected.capacity_kg),
      }))
    } else {
      setForm((prev) => ({ ...prev, model: modelName }))
    }
  }

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.createVehicle({
        ...form,
        year: form.year ? Number(form.year) : undefined,
        capacity: form.capacity ? Number(form.capacity) : undefined,
      })
      setForm(initialVehicle(availableModels))
      await vehicles.reload()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not create vehicle.') }
  }

  const assignDriver = async (driverId: string, vehicleId: string) => {
    try {
      await api.assignDriverVehicle(driverId, vehicleId)
      await Promise.all([vehicles.reload(), drivers.reload()])
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update the allocation.')
    }
  }

  return <Page eyebrow="Resources" title="Vehicles" description="Register fleet vehicles and review their specifications, maintenance, and operating status.">
    <Panel title="Add vehicle"><form className="form-row" onSubmit={create}>
      <label className="field">Name<input required placeholder="Vehicle identifier" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
      <label className="field">Registration<input placeholder="e.g. B 1234 ABC" value={form.registration} onChange={(e) => setForm({ ...form, registration: e.target.value })} /></label>
      <label className="field">Model
        <select value={form.model} onChange={(e) => onSelectModel(e.target.value)}>
          {availableModels.map((m) => (
            <option key={m.id} value={m.name}>{m.name}</option>
          ))}
        </select>
      </label>
      <label className="field">Year<input type="number" min="1886" step="1" placeholder="Year" value={form.year} onChange={(e) => setForm({ ...form, year: e.target.value })} /></label>
      <label className="field">Color<input placeholder="Color" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} /></label>
      <label className="field">
        Cargo capacity (kg)
        <input type="number" min="0" step="any" placeholder="Payload in kg" value={form.capacity} onChange={(e) => setForm({ ...form, capacity: e.target.value })} />
      </label>
      <div className="field location-value">
        <span>Model specs</span>
        <output>{form.body_type || '—'} · {form.type.toUpperCase()}</output>
      </div>
      <label className="field">Status<select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value as 'available' | 'offline' })}>
        <option value="available">Available</option><option value="offline">Offline</option>
      </select></label>
      <button className="button primary">Add vehicle</button>
    </form>
    <p className="help-text">Capacity indicates maximum payload weight in kilograms (kg). Body type is pre-filled from the selected Vehicle Model.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}</Panel>
    <Panel title="Vehicle register" action={<button className="button" onClick={() => void Promise.all([vehicles.reload(), drivers.reload()])}>Refresh</button>}>
      {vehicles.loading || drivers.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
        <Table headers={['Name', 'Registration', 'Model', 'Year', 'Color', 'Body type', 'Cargo capacity', 'Allocation', 'Status']}>{(vehicles.data ?? []).map((v) => {
          const assignedDriver = (drivers.data ?? []).find((d) => d.vehicle_id === v.id)
          return (
            <tr
              key={v.id}
              onClick={(e) => handleRowClick(e, v.name)}
              style={{ cursor: 'pointer' }}
              title="Click row to view vehicle maintenance history"
            >
              <td><Link to={`/resources/vehicles/${encodeURIComponent(v.id)}`}>{v.name}</Link></td>
              <td>{unknown(v.registration)}</td>
              <td>{v.model || v.make || 'Unknown'}</td>
              <td>{unknown(v.year)}</td>
              <td>{unknown(v.color)}</td>
              <td>{unknown(v.body_type)}</td>
              <td>{v.capacity != null ? `${v.capacity} kg` : 'Unknown'}</td>
              <td>
                <select
                  aria-label={`Allocate a driver to ${v.name}`}
                  value={assignedDriver?.id ?? ''}
                  onChange={(e) => {
                    const driverId = e.target.value
                    if (driverId) void assignDriver(driverId, v.id)
                    else if (assignedDriver) void assignDriver(assignedDriver.id, '')
                  }}
                  style={{ minWidth: '150px', padding: '3px 6px', fontSize: '12px' }}
                >
                  <option value="">Unallocated</option>
                  {(drivers.data ?? []).map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name}{d.vehicle_id && d.vehicle_id !== v.id ? ' (on another vehicle)' : ''}
                    </option>
                  ))}
                </select>
              </td>
              <td><Status value={v.status} /></td>
            </tr>
          )
        })}</Table>}
      {!vehicles.loading && !vehicles.error && vehicles.data?.length === 0 && <p className="empty-state">No vehicles registered.</p>}
    </Panel>
  </Page>
}

export function DriversPage() {
  const drivers = useLoad(api.drivers)
  const vehicles = useLoad(api.vehicles)
  const [form, setForm] = useState(emptyDriver)
  const [error, setError] = useState('')

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.createDriver(form)
      setForm(emptyDriver())
      await drivers.reload()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not create driver.') }
  }
  return <Page eyebrow="Resources" title="Drivers" description="Manage driver contact details, qualifications, shifts, availability, contract types, and vehicle assignments.">
    <Panel title="Add driver"><form className="form-row" onSubmit={create}>
      <label className="field">Full name<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
      <label className="field">Contract type
        <select value={form.contract_type} onChange={(e) => setForm({ ...form, contract_type: e.target.value })}>
          <option value="Full-time">Full-time (PKWT/PKWTT)</option>
          <option value="Contract">Fixed-Term Contract</option>
          <option value="Freelance">Freelance / On-Demand</option>
          <option value="Mitra">Mitra Partner</option>
        </select>
      </label>
      <label className="field">Contact phone<input type="tel" value={form.contact_phone} onChange={(e) => setForm({ ...form, contact_phone: e.target.value })} /></label>
      <label className="field">License class<input value={form.license_class} onChange={(e) => setForm({ ...form, license_class: e.target.value })} /></label>
      <label className="field">License expiry<input type="date" value={form.license_expiry} onChange={(e) => setForm({ ...form, license_expiry: e.target.value })} /></label>
      <label className="field">Shift start<input type="time" value={form.shift_start} onChange={(e) => setForm({ ...form, shift_start: e.target.value })} /></label>
      <label className="field">Shift end<input type="time" value={form.shift_end} onChange={(e) => setForm({ ...form, shift_end: e.target.value })} /></label>
      <label className="field">Availability<select value={form.availability} onChange={(e) => setForm({ ...form, availability: e.target.value })}>
        <option value="">Unknown</option><option value="available">Available</option><option value="unavailable">Unavailable</option>
      </select></label>
      <button className="button primary">Add driver</button>
    </form>{error && <ErrorMessage>{error}</ErrorMessage>}</Panel>
    <Panel title="Driver register">
      {drivers.loading ? <Loading /> : drivers.error ? <ErrorMessage>{drivers.error}</ErrorMessage> :
        <Table headers={['Name', 'Contract', 'Contact phone', 'License class', 'License expiry', 'Shift', 'Availability', 'Assigned car', 'Maintenance']}>{(drivers.data ?? []).map((d) => {
          const heldVehicle = vehicles.data?.find((v) => v.id === d.vehicle_id)
          return (
            <tr key={d.id}><td><Link to={`/resources/drivers/${encodeURIComponent(d.id)}`}>{d.name}</Link></td>
              <td><span style={{ fontSize: '12px', background: '#e7f5ff', color: '#1c7ed6', padding: '2px 6px', borderRadius: '4px' }}>{unknown(d.contract_type)}</span></td>
              <td>{unknown(d.contact_phone)}</td><td>{unknown(d.license_class)}</td>
              <td>{unknown(d.license_expiry)}</td><td>{d.shift_start && d.shift_end ? `${d.shift_start} - ${d.shift_end}` : '—'}</td><td>{unknown(d.availability)}</td>
              <td>
                {heldVehicle
                  ? <Link to={`/resources/vehicles/${encodeURIComponent(heldVehicle.id)}`}>{heldVehicle.name}</Link>
                  : <span className="muted">Unassigned</span>}
              </td>
              <td>
                {heldVehicle ? (
                  <Link
                    className="button"
                    style={{ padding: '2px 8px', fontSize: '11px' }}
                    to={`/resources/maintenance?vehicle=${encodeURIComponent(heldVehicle.name)}`}
                    title={`View maintenance notes for ${heldVehicle.name}`}
                  >
                    Notes →
                  </Link>
                ) : (
                  <span className="muted">—</span>
                )}
              </td>
            </tr>
          )
        })}</Table>}
      {!drivers.loading && !drivers.error && drivers.data?.length === 0 && (
        <p className="empty-state">
          No drivers are registered in this FleetFlow database. Add a driver above to populate the roster; simulation runs do not create driver records.
        </p>
      )}
    </Panel>
  </Page>
}

export function VehicleDetailPage() {
  const { vehicleID = '' } = useParams()
  const vehicles = useLoad(api.vehicles)
  const trips = useLoad(api.trips)
  const vehicle = vehicles.data?.find((item) => item.id === vehicleID)

  if (vehicles.loading) return <Page eyebrow="Resources" title="Vehicle profile" description="Loading vehicle record."><Loading /></Page>
  if (vehicles.error) return <Page eyebrow="Resources" title="Vehicle profile" description="The vehicle could not be loaded."><ErrorMessage>{vehicles.error}</ErrorMessage></Page>
  if (!vehicle) return <Page eyebrow="Resources" title="Vehicle profile" description="This vehicle record is unavailable."><ErrorMessage>Vehicle not found.</ErrorMessage></Page>

  // A trip records the vehicle it used, so this vehicle's history is the trip
  // list filtered by id — newest first, because the last job is what a fleet
  // manager opens the profile to see.
  const vehicleTrips = (trips.data ?? [])
    .filter((trip) => trip.vehicle_id === vehicleID)
    .sort((a, b) => (b.started_at ?? '').localeCompare(a.started_at ?? ''))

  return <Page eyebrow="Resources" title={vehicle.name} description="Vehicle specifications and maintenance record."
    action={<Link className="button" to="/resources/vehicles">Back to vehicles</Link>}>
    <Panel title="Vehicle profile">
      <Table headers={['Field', 'Value']}>
        <tr><th scope="row">Registration</th><td>{unknown(vehicle.registration)}</td></tr>
        <tr><th scope="row">Model</th><td>{unknown(vehicle.model || vehicle.make)}</td></tr>
        <tr><th scope="row">Year</th><td>{unknown(vehicle.year)}</td></tr>
        <tr><th scope="row">Color</th><td>{unknown(vehicle.color)}</td></tr>
        <tr><th scope="row">Body type</th><td>{unknown(vehicle.body_type)}</td></tr>
        <tr><th scope="row">Cargo payload capacity</th><td>{vehicle.capacity != null ? `${vehicle.capacity} kg` : 'Unknown'}</td></tr>
        <tr><th scope="row">Status</th><td><Status value={vehicle.status} /></td></tr>
      </Table>
    </Panel>
    <Panel
      title="Maintenance log"
      action={
        <Link className="button" to={`/resources/maintenance?vehicle=${encodeURIComponent(vehicle.name)}`}>
          Open maintenance page
        </Link>
      }
    >
      <p>Vehicle maintenance logs, workshop repairs, and service history are managed on the <Link to={`/resources/maintenance?vehicle=${encodeURIComponent(vehicle.name)}`}>Maintenance page</Link>.</p>
    </Panel>
    <Panel title="Scenario and trip history">
      {trips.loading ? <Loading /> : trips.error ? <ErrorMessage>{trips.error}</ErrorMessage> :
        vehicleTrips.length === 0
          ? <p className="empty-state">No trips are recorded against this vehicle yet. Trips appear here once a scenario dispatch or a scheduled trip uses it.</p>
          : <Table headers={['Route', 'Status', 'Started', 'Finished', 'Open']}>
            {vehicleTrips.map((trip) => <tr key={trip.id}>
              <td>{trip.origin} → {trip.destination}</td>
              <td><Status value={trip.status} /></td>
              <td>{trip.started_at ? new Date(trip.started_at).toLocaleString() : '—'}</td>
              <td>{trip.finished_at ? new Date(trip.finished_at).toLocaleString() : '—'}</td>
              <td><Link to={`/operations/trips/operational/${encodeURIComponent(trip.id)}`}>Open trip</Link></td>
            </tr>)}
          </Table>}
    </Panel>
  </Page>
}

export function DriverDetailPage() {
  const { driverID = '' } = useParams()
  const drivers = useLoad(api.drivers)
  const vehicles = useLoad(api.vehicles)
  const trips = useLoad(api.trips)
  const driver = drivers.data?.find((item) => item.id === driverID)

  if (drivers.loading || vehicles.loading) return <Page eyebrow="Resources" title="Driver profile" description="Loading driver record."><Loading /></Page>
  if (drivers.error || vehicles.error) {
    return <Page eyebrow="Resources" title="Driver profile" description="The driver could not be loaded.">
      <ErrorMessage>{drivers.error || vehicles.error}</ErrorMessage>
    </Page>
  }
  if (!driver) return <Page eyebrow="Resources" title="Driver profile" description="This driver record is unavailable."><ErrorMessage>Driver not found.</ErrorMessage></Page>

  const assignedVehicle = vehicles.data?.find((vehicle) => vehicle.id === driver.vehicle_id)
  // Trips belong to the vehicle, and a driver drives the vehicle they hold, so the
  // driver's history is that vehicle's history while the assignment stands.
  const driverTrips = (trips.data ?? [])
    .filter((trip) => assignedVehicle && trip.vehicle_id === assignedVehicle.id)
    .sort((a, b) => (b.started_at ?? '').localeCompare(a.started_at ?? ''))
  return <Page eyebrow="Resources" title={driver.name} description="Driver qualifications, availability, and current vehicle."
    action={<Link className="button" to="/resources/drivers">Back to drivers</Link>}>
    <Panel title="Driver profile">
      <Table headers={['Field', 'Value']}>
        <tr><th scope="row">Contract type</th><td>{unknown(driver.contract_type)}</td></tr>
        <tr><th scope="row">Contact phone</th><td>{unknown(driver.contact_phone)}</td></tr>
        <tr><th scope="row">License class</th><td>{unknown(driver.license_class)}</td></tr>
        <tr><th scope="row">License expiry</th><td>{unknown(driver.license_expiry)}</td></tr>
        <tr><th scope="row">Shift start</th><td>{unknown(driver.shift_start)}</td></tr>
        <tr><th scope="row">Shift end</th><td>{unknown(driver.shift_end)}</td></tr>
        <tr><th scope="row">Availability</th><td>{unknown(driver.availability)}</td></tr>
        <tr><th scope="row">Current vehicle</th><td>
          {assignedVehicle
            ? `${assignedVehicle.name}${assignedVehicle.registration ? ` · ${assignedVehicle.registration}` : ''}`
            : 'Unassigned'}
        </td></tr>
      </Table>
    </Panel>
    {assignedVehicle ? (
      <Panel
        title="Assigned Vehicle Maintenance Notes"
        action={
          <Link className="button" to={`/resources/maintenance?vehicle=${encodeURIComponent(assignedVehicle.name)}`}>
            Open maintenance log
          </Link>
        }
      >
        <p>Driver is currently assigned to <strong>{assignedVehicle.name}</strong> ({assignedVehicle.model || assignedVehicle.type}). Track maintenance notes, workshop inspection logs, and scheduled services on the <Link to={`/resources/maintenance?vehicle=${encodeURIComponent(assignedVehicle.name)}`}>Maintenance page</Link>.</p>
      </Panel>
    ) : (
      <Panel title="Assigned Vehicle Maintenance Notes">
        <p className="empty-state">No vehicle currently assigned. Assign a vehicle to track its maintenance notes.</p>
      </Panel>
    )}
    <Panel title="Scenario and trip history">
      {trips.loading ? <Loading /> : trips.error ? <ErrorMessage>{trips.error}</ErrorMessage> :
        driverTrips.length === 0
          ? <p className="empty-state">No trips are recorded for this driver's vehicle yet. Trips appear here once the vehicle is used by a scenario dispatch or a scheduled trip.</p>
          : <Table headers={['Route', 'Status', 'Started', 'Finished', 'Open']}>
            {driverTrips.map((trip) => <tr key={trip.id}>
              <td>{trip.origin} → {trip.destination}</td>
              <td><Status value={trip.status} /></td>
              <td>{trip.started_at ? new Date(trip.started_at).toLocaleString() : '—'}</td>
              <td>{trip.finished_at ? new Date(trip.finished_at).toLocaleString() : '—'}</td>
              <td><Link to={`/operations/trips/operational/${encodeURIComponent(trip.id)}`}>Open trip</Link></td>
            </tr>)}
          </Table>}
    </Panel>
  </Page>
}
