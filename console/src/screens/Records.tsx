import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api } from '../api/client'
import { ErrorMessage, Loading, Page, Panel, Status, Table, useLoad } from './Shared'

const emptyVehicle = () => ({
  name: '', type: 'van', status: 'available', registration: '', make: '', model: '',
  year: '', color: '', body_type: '', fuel_type: '', capacity: '', maintenance_history: '',
})

const emptyDriver = () => ({
  name: '', contact_phone: '', license_class: '', license_expiry: '', shift_start: '', shift_end: '', availability: '',
})

const unknown = (value: string | number | null | undefined) => value ?? 'Unknown'

export function VehiclesPage() {
  const vehicles = useLoad(api.vehicles)
  const [form, setForm] = useState(emptyVehicle)
  const [error, setError] = useState('')
  const create = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.createVehicle({
        ...form,
        year: form.year ? Number(form.year) : undefined,
        capacity: form.capacity ? Number(form.capacity) : undefined,
      })
      setForm(emptyVehicle())
      await vehicles.reload()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not create vehicle.') }
  }
  return <Page eyebrow="Resources" title="Vehicles" description="Register fleet vehicles and review their specifications, maintenance, and operating status.">
    <Panel title="Add vehicle"><form className="form-row" onSubmit={create}>
      <label className="field">Name<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
      <label className="field">Registration<input value={form.registration} onChange={(e) => setForm({ ...form, registration: e.target.value })} /></label>
      <label className="field">Make<input value={form.make} onChange={(e) => setForm({ ...form, make: e.target.value })} /></label>
      <label className="field">Model<input value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} /></label>
      <label className="field">Year<input type="number" min="1886" step="1" value={form.year} onChange={(e) => setForm({ ...form, year: e.target.value })} /></label>
      <label className="field">Color<input value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} /></label>
      <label className="field">Body type<input value={form.body_type} onChange={(e) => setForm({ ...form, body_type: e.target.value })} /></label>
      <label className="field">Fuel type<input value={form.fuel_type} onChange={(e) => setForm({ ...form, fuel_type: e.target.value })} /></label>
      <label className="field">Capacity<input type="number" min="0" step="any" value={form.capacity} onChange={(e) => setForm({ ...form, capacity: e.target.value })} /></label>
      <label className="field">Type<select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
        <option value="van">Van</option><option value="truck">Truck</option><option value="car">Car</option><option value="motorcycle">Motorcycle</option>
      </select></label>
      <label className="field">Status<select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
        <option value="available">Available</option><option value="offline">Offline</option>
      </select></label>
      <label className="field">Maintenance history<textarea value={form.maintenance_history} onChange={(e) => setForm({ ...form, maintenance_history: e.target.value })} /></label>
      <button className="button primary">Add vehicle</button>
    </form>{error && <ErrorMessage>{error}</ErrorMessage>}</Panel>
    <Panel title="Vehicle register" action={<button className="button" onClick={() => void vehicles.reload()}>Refresh</button>}>
      {vehicles.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
        <Table headers={['Name', 'Registration', 'Make / model', 'Year', 'Color', 'Body / fuel', 'Capacity', 'Maintenance history', 'Status', 'Record ID']}>{(vehicles.data ?? []).map((v) =>
          <tr key={v.id}><td><Link to={`/resources/vehicles/${encodeURIComponent(v.id)}`}>{v.name}</Link></td><td>{unknown(v.registration)}</td><td>{v.make || v.model ? `${v.make ?? ''} ${v.model ?? ''}`.trim() : 'Unknown'}</td>
            <td>{unknown(v.year)}</td><td>{unknown(v.color)}</td><td>{v.body_type || v.fuel_type ? `${v.body_type ?? 'Unknown'} / ${v.fuel_type ?? 'Unknown'}` : 'Unknown'}</td>
            <td>{unknown(v.capacity)}</td><td>{unknown(v.maintenance_history)}</td><td><Status value={v.status} /></td><td>{v.id}</td></tr>)}</Table>}
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
        <Table headers={['Name', 'Contact phone', 'Existing contact', 'License class', 'License expiry', 'Shift start', 'Shift end', 'Availability', 'Assigned vehicle', 'Record ID']}>{(drivers.data ?? []).map((d) =>
          <tr key={d.id}><td><Link to={`/resources/drivers/${encodeURIComponent(d.id)}`}>{d.name}</Link></td><td>{unknown(d.contact_phone)}</td><td>{unknown(d.contact)}</td><td>{unknown(d.license_class)}</td>
            <td>{unknown(d.license_expiry)}</td><td>{unknown(d.shift_start)}</td><td>{unknown(d.shift_end)}</td><td>{unknown(d.availability)}</td>
            <td>{vehicles.data?.find((v) => v.id === d.vehicle_id)?.name ?? 'Unassigned'}</td><td>{d.id}</td></tr>)}</Table>}
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
        <tr><th scope="row">Make</th><td>{unknown(vehicle.make)}</td></tr>
        <tr><th scope="row">Model</th><td>{unknown(vehicle.model)}</td></tr>
        <tr><th scope="row">Year</th><td>{unknown(vehicle.year)}</td></tr>
        <tr><th scope="row">Color</th><td>{unknown(vehicle.color)}</td></tr>
        <tr><th scope="row">Body type</th><td>{unknown(vehicle.body_type)}</td></tr>
        <tr><th scope="row">Fuel type</th><td>{unknown(vehicle.fuel_type)}</td></tr>
        <tr><th scope="row">Capacity</th><td>{unknown(vehicle.capacity)}</td></tr>
        <tr><th scope="row">Status</th><td><Status value={vehicle.status} /></td></tr>
      </Table>
    </Panel>
    <Panel title="Maintenance history">
      {vehicle.maintenance_history
        ? <p>{vehicle.maintenance_history}</p>
        : <p className="empty-state">No maintenance history recorded.</p>}
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
