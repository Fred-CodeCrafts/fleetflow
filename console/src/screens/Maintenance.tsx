import { useEffect, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Vehicle } from '../api/client'
import { ErrorMessage, Metric, Page, Panel, Status, Table, useLoad } from './Shared'

export interface MaintenanceRecord {
  id: string
  vehicle_id?: string
  vehicle_name: string
  vehicle_registration?: string
  service_date: string
  service_type: string
  odometer_km: number
  cost_idr: number
  status: 'completed' | 'in_progress' | 'scheduled'
  facility: string
  notes: string
}

const defaultRecords: MaintenanceRecord[] = [
  {
    id: 'maint-01',
    vehicle_name: 'Unit 12',
    vehicle_registration: 'B-9012-JKT',
    service_date: '2026-10-18',
    service_type: 'Scheduled 20,000 km Service',
    odometer_km: 20450,
    cost_idr: 1850000,
    status: 'completed',
    facility: 'Jakarta Pusat Fleet Depot',
    notes: 'Engine oil replaced (15W-40 heavy-duty), oil filter and fuel filter renewed. Brake lines checked.',
  },
  {
    id: 'maint-02',
    vehicle_name: 'Unit 48',
    vehicle_registration: 'B-9048-JKT',
    service_date: '2026-10-21',
    service_type: 'Brake Pad Replacement & Alignment',
    odometer_km: 35120,
    cost_idr: 2400000,
    status: 'completed',
    facility: 'Cilandak Service Center',
    notes: 'Front and rear ceramic pads installed. Rotor resurfaced. Wheel alignment and tire pressure calibrated to 36 psi.',
  },
  {
    id: 'maint-03',
    vehicle_name: 'Unit 63',
    vehicle_registration: 'B-9063-JKT',
    service_date: '2026-10-24',
    service_type: 'Cooling System & Battery Diagnostic',
    odometer_km: 18900,
    cost_idr: 950000,
    status: 'in_progress',
    facility: 'Halim Logistics Garage',
    notes: 'Radiator flushed and refilled with coolant. Alternator output verified 14.2V and battery terminals cleaned.',
  },
]

const STORAGE_KEY = 'fleetflow_maintenance_records_v3'
const LEGACY_STORAGE_KEY = 'fleetflow_maintenance_records_v2'

function loadStoredRecords(): MaintenanceRecord[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed) && parsed.length > 0) return parsed
    }

    // The v2 seeded rows named a unit by its registration alone, so filtering by
    // unit name could not find them. Rows the operator logged under v2 are kept;
    // only the three seeded rows are replaced by ones naming a real unit.
    const legacy = localStorage.getItem(LEGACY_STORAGE_KEY)
    if (legacy) {
      const parsed = JSON.parse(legacy) as MaintenanceRecord[]
      if (Array.isArray(parsed)) {
        const seeded = new Set(defaultRecords.map((record) => record.id))
        return [...parsed.filter((record) => !seeded.has(record.id)), ...defaultRecords]
      }
    }
  } catch {
    // Ignore storage errors in restricted contexts
  }
  return defaultRecords
}

function saveStoredRecords(records: MaintenanceRecord[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(records))
  } catch {
    // Ignore storage errors in restricted contexts
  }
}

/** A record names a unit and a plate, so a row reads the same whether it was
 *  written with a name, a registration, or both. The fleet is the source of
 *  truth for which pair belongs together; a row whose unit has left the fleet
 *  keeps whatever it was written with. */
export function describeRecord(record: MaintenanceRecord, fleet: Vehicle[]) {
  const wantedPlate = (record.vehicle_registration || record.vehicle_name).toLowerCase()
  const wantedName = record.vehicle_name.toLowerCase()
  const byPlate = fleet.find((vehicle) => vehicle.registration?.toLowerCase() === wantedPlate)
  const unit = byPlate ?? fleet.find((vehicle) => vehicle.name.toLowerCase() === wantedName)
  return {
    name: unit?.name ?? record.vehicle_name,
    registration: unit?.registration ?? record.vehicle_registration ?? '',
  }
}

/** True when the chosen filter value names this record's unit, by either half of
 *  its identity. A legacy row stores the plate where a name would go, and that is
 *  exactly the case an equality check on the name alone used to miss. */
export function recordMatches(record: MaintenanceRecord, fleet: Vehicle[], selected: string) {
  const unit = describeRecord(record, fleet)
  const wanted = selected.toLowerCase()
  return (
    unit.name.toLowerCase() === wanted ||
    unit.registration.toLowerCase() === wanted ||
    record.vehicle_name.toLowerCase() === wanted
  )
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

  // The filter takes a unit, but a record may have been written with the plate,
  // so both sides are matched by name and by registration.
  const filteredRecords = selectedVehicle
    ? records.filter((record) => recordMatches(record, allVehicles, selectedVehicle))
    : records

  // Units that appear in the log but not in the fleet still need a way to be
  // filtered for, so they are offered after the fleet.
  const logOnlyUnits = records
    .map((record) => describeRecord(record, allVehicles))
    .filter((unit, index, all) => all.findIndex((other) => other.name === unit.name) === index)
    .filter((unit) => !allVehicles.some((vehicle) => vehicle.name === unit.name))

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
      vehicle_registration: allVehicles.find((vehicle) => vehicle.name === targetVehicle)?.registration ?? undefined,
      service_date: serviceDate,
      service_type: serviceType,
      odometer_km: Number(odometerKm),
      cost_idr: Number(costIdr),
      status,
      facility,
      notes,
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
      description="Track scheduled vehicle maintenance, workshop service logs, and repairs across the Jakarta fleet."
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
                {logOnlyUnits.map((unit) => (
                  <option key={unit.name} value={unit.name}>
                    {unit.name} {unit.registration ? `(${unit.registration})` : ''}
                  </option>
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
        <Table headers={['Unit / Registration', 'Service Date', 'Service Type', 'Odometer', 'Workshop / Facility', 'Cost (IDR)', 'Service Notes', 'Status']}>
          {filteredRecords.map((r) => {
            const unit = describeRecord(r, allVehicles)
            return (
            <tr key={r.id}>
              <td>
                <strong>{unit.name}</strong>
                {unit.registration && <div className="muted"><small>{unit.registration}</small></div>}
              </td>
              <td>{r.service_date}</td>
              <td>{r.service_type}</td>
              <td>{r.odometer_km.toLocaleString()} km</td>
              <td>{r.facility}</td>
              <td>Rp {r.cost_idr.toLocaleString('id-ID')}</td>
              <td>
                <div>{r.notes || 'Routine maintenance'}</div>
              </td>
              <td>
                <Status value={r.status === 'completed' ? 'available' : r.status === 'in_progress' ? 'assigned' : 'idle'} />
              </td>
            </tr>
            )
          })}
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
            Service notes
            <textarea
              placeholder="Record replacement parts, technician diagnostics, and workshop repair notes"
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
