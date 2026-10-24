import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
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
    fuel_type: first?.fuel_type ?? '',
    capacity: first ? String(first.capacity_kg) : '',
    maintenance_history: '',
  }
}

const emptyDriver = () => ({
  name: '', contact_phone: '', license_class: '', license_expiry: '', shift_start: '', shift_end: '', availability: '',
})

const unknown = (value: string | number | null | undefined) => value ?? 'Unknown'

export function VehiclesPage() {
  const vehicles = useLoad(api.vehicles)
  const availableModels = getVehicleModels()
  const [form, setForm] = useState(() => initialVehicle(availableModels))
  const [error, setError] = useState('')

  const onSelectModel = (modelName: string) => {
    const selected = availableModels.find((m) => m.name === modelName)
    if (selected) {
      setForm((prev) => ({
        ...prev,
        model: selected.name,
        type: selected.type,
        body_type: selected.body_type,
        fuel_type: selected.fuel_type,
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
        <output>{form.body_type || '—'} · {form.fuel_type || '—'} · {form.type.toUpperCase()}</output>
      </div>
      <label className="field">Status<select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value as 'available' | 'offline' })}>
        <option value="available">Available</option><option value="offline">Offline</option>
      </select></label>
      <label className="field">Maintenance history<textarea placeholder="Service log or notes" value={form.maintenance_history} onChange={(e) => setForm({ ...form, maintenance_history: e.target.value })} /></label>
      <button className="button primary">Add vehicle</button>
    </form>
    <p className="help-text">Capacity indicates maximum payload weight in kilograms (kg). Body type and fuel type are automatically configured from the selected Vehicle Model.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}</Panel>
    <Panel title="Vehicle register" action={<button className="button" onClick={() => void vehicles.reload()}>Refresh</button>}>
      {vehicles.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
        <Table headers={['Name', 'Registration', 'Model', 'Year', 'Color', 'Body / fuel', 'Cargo capacity', 'Maintenance history', 'Status']}>{(vehicles.data ?? []).map((v) =>
          <tr key={v.id}><td><Link to={`/resources/vehicles/${encodeURIComponent(v.id)}`}>{v.name}</Link></td><td>{unknown(v.registration)}</td><td>{v.model || v.make || 'Unknown'}</td>
            <td>{unknown(v.year)}</td><td>{unknown(v.color)}</td><td>{v.body_type || v.fuel_type ? `${v.body_type ?? 'Unknown'} / ${v.fuel_type ?? 'Unknown'}` : 'Unknown'}</td>
            <td>{v.capacity != null ? `${v.capacity} kg` : 'Unknown'}</td>
            <td>
              <Link to={`/resources/maintenance?vehicle=${encodeURIComponent(v.name)}`} title="View full maintenance log for this vehicle">
                {v.maintenance_history ? `${v.maintenance_history} →` : 'View log →'}
              </Link>
            </td>
            <td><Status value={v.status} /></td></tr>)}</Table>}
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
  return <Page eyebrow="Resources" title="Drivers" description="Manage driver contact details, qualifications, shifts, availability, and vehicle assignments.">
    <Panel title="Add driver"><form className="form-row" onSubmit={create}>
      <label className="field">Full name<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
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
    <Panel title="Current vehicle allocation">
      {vehicles.loading || drivers.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
        drivers.error ? <ErrorMessage>{drivers.error}</ErrorMessage> :
        <Table headers={['Vehicle', 'Current driver']}>{(vehicles.data ?? []).map((vehicle) => {
          const current = drivers.data?.find((driver) => driver.vehicle_id === vehicle.id)
          return <tr key={vehicle.id}><td>{vehicle.name}</td><td>{current?.name ?? 'Unassigned'}</td>
          </tr>
        })}</Table>}
    </Panel>
    <Panel title="Driver register">
      {drivers.loading ? <Loading /> : drivers.error ? <ErrorMessage>{drivers.error}</ErrorMessage> :
        <Table headers={['Name', 'Contact phone', 'Existing contact', 'License class', 'License expiry', 'Shift start', 'Shift end', 'Availability', 'Assigned vehicle']}>{(drivers.data ?? []).map((d) =>
          <tr key={d.id}><td><Link to={`/resources/drivers/${encodeURIComponent(d.id)}`}>{d.name}</Link></td><td>{unknown(d.contact_phone)}</td><td>{unknown(d.contact)}</td><td>{unknown(d.license_class)}</td>
            <td>{unknown(d.license_expiry)}</td><td>{unknown(d.shift_start)}</td><td>{unknown(d.shift_end)}</td><td>{unknown(d.availability)}</td>
            <td>{vehicles.data?.find((v) => v.id === d.vehicle_id)?.name ?? 'Unassigned'}</td></tr>)}</Table>}
      {!drivers.loading && !drivers.error && drivers.data?.length === 0 && <p className="empty-state">No drivers registered.</p>}
    </Panel>
  </Page>
}

export function VehicleDetailPage() {
  const { vehicleID = '' } = useParams()
  const vehicles = useLoad(api.vehicles)
  const vehicle = vehicles.data?.find((item) => item.id === vehicleID)

  if (vehicles.loading) return <Page eyebrow="Resources" title="Vehicle profile" description="Loading vehicle record."><Loading /></Page>
  if (vehicles.error) return <Page eyebrow="Resources" title="Vehicle profile" description="The vehicle could not be loaded."><ErrorMessage>{vehicles.error}</ErrorMessage></Page>
  if (!vehicle) return <Page eyebrow="Resources" title="Vehicle profile" description="This vehicle record is unavailable."><ErrorMessage>Vehicle not found.</ErrorMessage></Page>

  return <Page eyebrow="Resources" title={vehicle.name} description="Vehicle specifications and maintenance record."
    action={<Link className="button" to="/resources/vehicles">Back to vehicles</Link>}>
    <Panel title="Vehicle profile">
      <Table headers={['Field', 'Value']}>
        <tr><th scope="row">Registration</th><td>{unknown(vehicle.registration)}</td></tr>
        <tr><th scope="row">Model</th><td>{unknown(vehicle.model || vehicle.make)}</td></tr>
        <tr><th scope="row">Year</th><td>{unknown(vehicle.year)}</td></tr>
        <tr><th scope="row">Color</th><td>{unknown(vehicle.color)}</td></tr>
        <tr><th scope="row">Body type</th><td>{unknown(vehicle.body_type)}</td></tr>
        <tr><th scope="row">Fuel type</th><td>{unknown(vehicle.fuel_type)}</td></tr>
        <tr><th scope="row">Cargo payload capacity</th><td>{vehicle.capacity != null ? `${vehicle.capacity} kg` : 'Unknown'}</td></tr>
        <tr><th scope="row">Status</th><td><Status value={vehicle.status} /></td></tr>
      </Table>
    </Panel>
    <Panel
      title="Maintenance history"
      action={
        <Link className="button" to={`/resources/maintenance?vehicle=${encodeURIComponent(vehicle.name)}`}>
          Open maintenance log
        </Link>
      }
    >
      {vehicle.maintenance_history
        ? <p>{vehicle.maintenance_history}</p>
        : <p className="empty-state">No maintenance history recorded in vehicle profile.</p>}
    </Panel>
    <Panel title="Scenario and trip history">
      <p className="empty-state">Vehicle-to-trip history will appear after automatic dispatch links generated trips to fleet records.</p>
    </Panel>
  </Page>
}

export function DriverDetailPage() {
  const { driverID = '' } = useParams()
  const drivers = useLoad(api.drivers)
  const vehicles = useLoad(api.vehicles)
  const driver = drivers.data?.find((item) => item.id === driverID)

  if (drivers.loading || vehicles.loading) return <Page eyebrow="Resources" title="Driver profile" description="Loading driver record."><Loading /></Page>
  if (drivers.error || vehicles.error) {
    return <Page eyebrow="Resources" title="Driver profile" description="The driver could not be loaded.">
      <ErrorMessage>{drivers.error || vehicles.error}</ErrorMessage>
    </Page>
  }
  if (!driver) return <Page eyebrow="Resources" title="Driver profile" description="This driver record is unavailable."><ErrorMessage>Driver not found.</ErrorMessage></Page>

  const assignedVehicle = vehicles.data?.find((vehicle) => vehicle.id === driver.vehicle_id)
  return <Page eyebrow="Resources" title={driver.name} description="Driver qualifications, availability, and current vehicle."
    action={<Link className="button" to="/resources/drivers">Back to drivers</Link>}>
    <Panel title="Driver profile">
      <Table headers={['Field', 'Value']}>
        <tr><th scope="row">Contact phone</th><td>{unknown(driver.contact_phone)}</td></tr>
        <tr><th scope="row">License class</th><td>{unknown(driver.license_class)}</td></tr>
        <tr><th scope="row">License expiry</th><td>{unknown(driver.license_expiry)}</td></tr>
        <tr><th scope="row">Shift start</th><td>{unknown(driver.shift_start)}</td></tr>
        <tr><th scope="row">Shift end</th><td>{unknown(driver.shift_end)}</td></tr>
        <tr><th scope="row">Availability</th><td>{unknown(driver.availability)}</td></tr>
        <tr><th scope="row">Current vehicle</th><td>{assignedVehicle?.name ?? 'Unassigned'}</td></tr>
      </Table>
    </Panel>
    <Panel title="Scenario and trip history">
      <p className="empty-state">Driver-to-trip history will appear after automatic dispatch links generated trips to fleet records.</p>
    </Panel>
  </Page>
}
