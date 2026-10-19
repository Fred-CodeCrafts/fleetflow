import { useState, type FormEvent } from 'react'
import { api } from '../api/client'
import { ErrorMessage, Loading, Page, Panel, Status, Table, useLoad } from './Shared'

export function VehiclesPage() {
  const vehicles = useLoad(api.vehicles)
  const [form, setForm] = useState({ name: '', type: 'van', status: 'available' })
  const [error, setError] = useState('')
  const create = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.createVehicle(form)
      setForm({ name: '', type: 'van', status: 'available' })
      await vehicles.reload()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not create vehicle.') }
  }
  return <Page eyebrow="Resources" title="Vehicles" description="Register fleet vehicles and review their current operating status.">
    <Panel title="Add vehicle"><form className="form-row" onSubmit={create}>
      <label className="field">Name or plate<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
      <label className="field">Type<select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
        <option value="van">Van</option><option value="truck">Truck</option><option value="car">Car</option><option value="motorcycle">Motorcycle</option>
      </select></label>
      <label className="field">Status<select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
        <option value="available">Available</option><option value="offline">Offline</option>
      </select></label><button className="button primary">Add vehicle</button>
    </form>{error && <ErrorMessage>{error}</ErrorMessage>}</Panel>
    <Panel title="Vehicle register" action={<button className="button" onClick={() => void vehicles.reload()}>Refresh</button>}>
      {vehicles.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
        <Table headers={['Name / plate', 'Type', 'Status', 'Record ID']}>{(vehicles.data ?? []).map((v) =>
          <tr key={v.id}><td>{v.name}</td><td>{v.type}</td><td><Status value={v.status} /></td><td>{v.id}</td></tr>)}</Table>}
      {!vehicles.loading && !vehicles.error && vehicles.data?.length === 0 && <p className="empty-state">No vehicles registered.</p>}
    </Panel>
  </Page>
}

export function DriversPage() {
  const drivers = useLoad(api.drivers)
  const vehicles = useLoad(api.vehicles)
  const [form, setForm] = useState({ name: '', contact: '' })
  const [error, setError] = useState('')
  const [assigning, setAssigning] = useState(false)
  const create = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.createDriver(form)
      setForm({ name: '', contact: '' })
      await drivers.reload()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not create driver.') }
  }
  const assign = async (vehicleID: string, driverID: string) => {
    setError('')
    setAssigning(true)
    try {
      await api.assignDriver(vehicleID, driverID)
      await Promise.all([drivers.reload(), vehicles.reload()])
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update the driver assignment.')
    } finally {
      setAssigning(false)
    }
  }
  return <Page eyebrow="Resources" title="Drivers" description="Manage driver records and see their vehicle assignments.">
    <Panel title="Add driver"><form className="form-row" onSubmit={create}>
      <label className="field">Full name<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
      <label className="field">Contact<input required value={form.contact} onChange={(e) => setForm({ ...form, contact: e.target.value })} /></label>
      <button className="button primary">Add driver</button>
    </form>{error && <ErrorMessage>{error}</ErrorMessage>}</Panel>
    <Panel title="Vehicle assignments">
      {vehicles.loading || drivers.loading ? <Loading /> : vehicles.error ? <ErrorMessage>{vehicles.error}</ErrorMessage> :
        <Table headers={['Vehicle', 'Driver', 'Assignment']}>{(vehicles.data ?? []).map((vehicle) => {
          const current = drivers.data?.find((driver) => driver.vehicle_id === vehicle.id)
          return <tr key={vehicle.id}><td>{vehicle.name}</td><td>{current?.name ?? 'Unassigned'}</td>
            <td><select aria-label={`Assign driver to ${vehicle.name}`} disabled={assigning} value={current?.id ?? ''}
              onChange={(event) => void assign(vehicle.id, event.target.value)}>
              <option value="">Unassign</option>{(drivers.data ?? []).filter((driver) => !driver.vehicle_id || driver.vehicle_id === vehicle.id).map((driver) =>
                <option key={driver.id} value={driver.id}>{driver.name}</option>)}
            </select></td></tr>
        })}</Table>}
    </Panel>
    <Panel title="Driver register">
      {drivers.loading ? <Loading /> : drivers.error ? <ErrorMessage>{drivers.error}</ErrorMessage> :
        <Table headers={['Name', 'Contact', 'Assigned vehicle', 'Record ID']}>{(drivers.data ?? []).map((d) =>
          <tr key={d.id}><td>{d.name}</td><td>{d.contact}</td><td>{vehicles.data?.find((v) => v.id === d.vehicle_id)?.name ?? 'Unassigned'}</td><td>{d.id}</td></tr>)}</Table>}
      {!drivers.loading && !drivers.error && drivers.data?.length === 0 && <p className="empty-state">No drivers registered.</p>}
    </Panel>
  </Page>
}

export function RunResultsPage() {
  const runs = useLoad(api.runs)
  return <Page eyebrow="Analytics" title="Run Results" description="Saved simulation comparisons and their captured metrics.">
    <Panel title="Saved run results" action={<button className="button" onClick={() => void runs.reload()}>Refresh</button>}>
      {runs.loading ? <Loading /> : runs.error ? <ErrorMessage>{runs.error}</ErrorMessage> :
        <Table headers={['Run', 'Seed', 'Created', 'Baseline average', 'FleetFlow average', 'CO₂ baseline / FleetFlow']}>
          {(runs.data ?? []).map((run) => <tr key={run.id}><td>{run.id.slice(0, 8)}</td><td>{run.seed}</td>
            <td>{new Date(run.created_at).toLocaleString()}</td>
            <td>{run.metrics.baseline?.average_trip_s?.toFixed(1) ?? '—'} s</td>
            <td>{run.metrics.fleet_flow?.average_trip_s?.toFixed(1) ?? '—'} s</td>
            <td>{run.metrics.baseline?.emissions?.co2_g?.toFixed(0) ?? '—'} / {run.metrics.fleet_flow?.emissions?.co2_g?.toFixed(0) ?? '—'} g</td>
          </tr>)}
        </Table>}
      {!runs.loading && !runs.error && runs.data?.length === 0 && <p className="empty-state">No saved runs yet. Save a comparison from Operations → Comparison Runs.</p>}
    </Panel>
  </Page>
}
