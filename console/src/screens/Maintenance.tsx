import { useEffect, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Vehicle } from '../api/client'
import { ErrorMessage, Metric, Page, Panel, Status, Table, useLoad } from './Shared'

export interface MaintenanceRecord {
  id: string
  vehicle_id?: string
  vehicle_name: string
  service_date: string
  service_type: string
  odometer_km: number
  cost_idr: number
  status: 'completed' | 'in_progress' | 'scheduled'
  facility: string
  notes: string
  emission_impact?: string
}

const defaultRecords: MaintenanceRecord[] = [
  {
    id: 'maint-01',
    vehicle_name: 'B-9012-JKT',
    service_date: '2026-10-18',
    service_type: 'Scheduled 20,000 km Service',
    odometer_km: 20450,
    cost_idr: 1850000,
    status: 'completed',
    facility: 'Jakarta Pusat Fleet Depot',
    notes: 'Engine oil replaced (5W-30 synthetic), air filter and fuel filter renewed. Catalytic converter inspected clean.',
    emission_impact: 'Optimal Euro 4 compliance; HC and CO levels nominal',
  },
  {
    id: 'maint-02',
    vehicle_name: 'B-9431-JKT',
    service_date: '2026-10-21',
    service_type: 'Brake Pad Replacement & Alignment',
    odometer_km: 35120,
    cost_idr: 2400000,
    status: 'completed',
    facility: 'Cilandak Service Center',
    notes: 'Front and rear ceramic pads installed. Rotor resurfaced. Tire pressure calibrated to 36 psi.',
    emission_impact: 'Reduced rolling friction; estimated +2% fuel efficiency',
  },
  {
    id: 'maint-03',
    vehicle_name: 'B-9204-JKT',
    service_date: '2026-10-24',
    service_type: 'Cooling System & Battery Diagnostic',
    odometer_km: 18900,
    cost_idr: 950000,
    status: 'in_progress',
    facility: 'Halim Logistics Garage',
    notes: 'Radiator flushed and refilled with coolant. Alternator output verified 14.2V.',
    emission_impact: 'Prevents thermal throttling and excess fuel burn',
  },
]

const STORAGE_KEY = 'fleetflow_maintenance_records_v1'

function loadStoredRecords(): MaintenanceRecord[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return defaultRecords
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) && parsed.length > 0 ? parsed : defaultRecords
  } catch {
    return defaultRecords
  }
}

function saveStoredRecords(records: MaintenanceRecord[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(records))
  } catch {
    // Ignore storage errors in restricted contexts
  }
}

export function MaintenancePage() {
  const vehicles = useLoad(api.vehicles)
  const [searchParams, setSearchParams] = useSearchParams()
  const vehicleFilter = searchParams.get('vehicle') ?? ''

  const [records, setRecords] = useState<MaintenanceRecord[]>(loadStoredRecords)
  const [selectedVehicle, setSelectedVehicle] = useState(vehicleFilter)
  const [serviceType, setServiceType] = useState('Routine Inspection')
  const [serviceDate, setServiceDate] = useState(new Date().toISOString().slice(0, 10))
  const [odometerKm, setOdometerKm] = useState(25000)
  const [costIdr, setCostIdr] = useState(1200000)
  const [status, setStatus] = useState<'completed' | 'in_progress' | 'scheduled'>('completed')
  const [facility, setFacility] = useState('Tanjung Priok Main Workshop')
  const [notes, setNotes] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (vehicleFilter) setSelectedVehicle(vehicleFilter)
  }, [vehicleFilter])

  const allVehicles: Vehicle[] = vehicles.data ?? []

  const filteredRecords = selectedVehicle
    ? records.filter((r) => r.vehicle_name.toLowerCase() === selectedVehicle.toLowerCase())
    : records

  const completedCount = records.filter((r) => r.status === 'completed').length
  const inProgressCount = records.filter((r) => r.status === 'in_progress').length
  const scheduledCount = records.filter((r) => r.status === 'scheduled').length

  const addRecord = (event: FormEvent) => {
    event.preventDefault()
    setError('')
    const targetVehicle = selectedVehicle || allVehicles[0]?.name || 'Fleet Vehicle'
    if (!targetVehicle) {
      setError('Please select or specify a vehicle.')
      return
    }

    const newRecord: MaintenanceRecord = {
      id: `maint-${Date.now()}`,
      vehicle_name: targetVehicle,
      service_date: serviceDate,
      service_type: serviceType,
      odometer_km: Number(odometerKm),
      cost_idr: Number(costIdr),
      status,
      facility,
      notes,
      emission_impact: 'Inspected and certified for city emission compliance',
    }

    const updated = [newRecord, ...records]
    setRecords(updated)
    saveStoredRecords(updated)
    setNotes('')
  }

  return (
    <Page
      eyebrow="Resources"
      title="Maintenance History"
      description="Track scheduled vehicle maintenance, workshop service logs, and emission inspection compliance across the Jakarta fleet."
    >
      <div className="metric-grid">
        <Metric label="Total Service Records" value={String(records.length)} />
        <Metric label="Completed" value={String(completedCount)} />
        <Metric label="In Progress" value={String(inProgressCount)} />
        <Metric label="Scheduled" value={String(scheduledCount)} />
      </div>

      <Panel
        title="Service Records"
        action={
          <div className="form-row" style={{ margin: 0 }}>
            <label className="field" style={{ margin: 0, minWidth: '220px' }}>
              <select
                value={selectedVehicle}
                onChange={(e) => {
                  setSelectedVehicle(e.target.value)
                  if (e.target.value) setSearchParams({ vehicle: e.target.value })
                  else setSearchParams({})
                }}
              >
                <option value="">All vehicles ({records.length})</option>
                {allVehicles.map((v) => (
                  <option key={v.id} value={v.name}>
                    {v.name} {v.registration ? `(${v.registration})` : ''}
                  </option>
                ))}
                {records
                  .map((r) => r.vehicle_name)
                  .filter((name, i, arr) => arr.indexOf(name) === i && !allVehicles.some((v) => v.name === name))
                  .map((name) => (
                    <option key={name} value={name}>{name}</option>
                  ))}
              </select>
            </label>
            {selectedVehicle && (
              <button
                className="button"
                type="button"
                onClick={() => {
                  setSelectedVehicle('')
                  setSearchParams({})
                }}
              >
                Clear filter
              </button>
            )}
          </div>
        }
      >
        <Table headers={['Vehicle', 'Service Date', 'Service Type', 'Odometer', 'Workshop / Facility', 'Cost (IDR)', 'Emission Impact & Notes', 'Status']}>
          {filteredRecords.map((r) => (
            <tr key={r.id}>
              <td>
                <strong>{r.vehicle_name}</strong>
              </td>
              <td>{r.service_date}</td>
              <td>{r.service_type}</td>
              <td>{r.odometer_km.toLocaleString()} km</td>
              <td>{r.facility}</td>
              <td>Rp {r.cost_idr.toLocaleString('id-ID')}</td>
              <td>
                <div>{r.notes || 'Routine maintenance'}</div>
                {r.emission_impact && (
                  <small className="muted" style={{ display: 'block', marginTop: '2px', color: '#16866c' }}>
                    🌿 {r.emission_impact}
                  </small>
                )}
              </td>
              <td>
                <Status value={r.status === 'completed' ? 'available' : r.status === 'in_progress' ? 'assigned' : 'idle'} />
              </td>
            </tr>
          ))}
        </Table>
        {filteredRecords.length === 0 && (
          <p className="empty-state">No maintenance records found for the selected filter.</p>
        )}
      </Panel>

      <Panel title="Log Maintenance Event">
        <form className="form-row" onSubmit={addRecord}>
          <label className="field">
            Vehicle
            <select
              value={selectedVehicle || allVehicles[0]?.name || ''}
              onChange={(e) => setSelectedVehicle(e.target.value)}
              required
            >
              {allVehicles.map((v) => (
                <option key={v.id} value={v.name}>
                  {v.name} {v.registration ? `(${v.registration})` : ''}
                </option>
              ))}
              {allVehicles.length === 0 && <option value="B-9012-JKT">B-9012-JKT</option>}
            </select>
          </label>
          <label className="field">
            Service date
            <input
              type="date"
              value={serviceDate}
              onChange={(e) => setServiceDate(e.target.value)}
              required
            />
          </label>
          <label className="field">
            Service type
            <input
              placeholder="e.g. Oil change & inspection"
              value={serviceType}
              onChange={(e) => setServiceType(e.target.value)}
              required
            />
          </label>
          <label className="field">
            Odometer (km)
            <input
              type="number"
              min="0"
              value={odometerKm}
              onChange={(e) => setOdometerKm(Number(e.target.value))}
            />
          </label>
          <label className="field">
            Cost (IDR)
            <input
              type="number"
              min="0"
              step="50000"
              value={costIdr}
              onChange={(e) => setCostIdr(Number(e.target.value))}
            />
          </label>
          <label className="field">
            Facility / Workshop
            <input
              placeholder="Workshop location"
              value={facility}
              onChange={(e) => setFacility(e.target.value)}
            />
          </label>
          <label className="field">
            Status
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value as 'completed' | 'in_progress' | 'scheduled')}
            >
              <option value="completed">Completed</option>
              <option value="in_progress">In Progress</option>
              <option value="scheduled">Scheduled</option>
            </select>
          </label>
          <label className="field" style={{ flex: '1 1 100%' }}>
            Service notes & emission check
            <textarea
              placeholder="Record replacement parts, technician diagnostics, and emissions inspection notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </label>
          <button className="button primary" type="submit">
            Log maintenance record
          </button>
        </form>
        {error && <ErrorMessage>{error}</ErrorMessage>}
      </Panel>
    </Page>
  )
}
