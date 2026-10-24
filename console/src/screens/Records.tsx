import { useState, type FormEvent } from 'react'
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
          <tr key={v.id}><td>{v.name}</td><td>{unknown(v.registration)}</td><td>{v.make || v.model ? `${v.make ?? ''} ${v.model ?? ''}`.trim() : 'Unknown'}</td>
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
          <tr key={d.id}><td>{d.name}</td><td>{unknown(d.contact_phone)}</td><td>{unknown(d.contact)}</td><td>{unknown(d.license_class)}</td>
            <td>{unknown(d.license_expiry)}</td><td>{unknown(d.shift_start)}</td><td>{unknown(d.shift_end)}</td><td>{unknown(d.availability)}</td>
            <td>{vehicles.data?.find((v) => v.id === d.vehicle_id)?.name ?? 'Unassigned'}</td><td>{d.id}</td></tr>)}</Table>}
      {!drivers.loading && !drivers.error && drivers.data?.length === 0 && <p className="empty-state">No drivers registered.</p>}
    </Panel>
  </Page>
}
