import { describe, expect, it } from 'vitest'
import type { Vehicle } from '../api/client'
import { describeRecord, recordMatches, type MaintenanceRecord } from './Maintenance'

const fleet = [
  { id: 'v12', name: 'Unit 12', registration: 'B-9012-JKT' },
  { id: 'v48', name: 'Unit 48', registration: 'B-9048-JKT' },
] as Vehicle[]

function record(overrides: Partial<MaintenanceRecord>): MaintenanceRecord {
  return {
    id: 'maint-01',
    vehicle_name: 'Unit 12',
    service_date: '2026-10-18',
    service_type: 'Scheduled service',
    odometer_km: 20450,
    cost_idr: 1850000,
    status: 'completed',
    facility: 'Jakarta Pusat Fleet Depot',
    notes: '',
    ...overrides,
  }
}

describe('maintenance records', () => {
  it('reads the unit and plate from a record that stored only the plate', () => {
    const legacy = record({ vehicle_name: 'B-9012-JKT' })
    expect(describeRecord(legacy, fleet)).toEqual({ name: 'Unit 12', registration: 'B-9012-JKT' })
  })

  it('reads both from a record that stored both', () => {
    const current = record({ vehicle_name: 'Unit 12', vehicle_registration: 'B-9012-JKT' })
    expect(describeRecord(current, fleet)).toEqual({ name: 'Unit 12', registration: 'B-9012-JKT' })
  })

  it('keeps what it was written with when the unit has left the fleet', () => {
    const gone = record({ vehicle_name: 'B-9999-JKT' })
    expect(describeRecord(gone, fleet)).toEqual({ name: 'B-9999-JKT', registration: '' })
  })

  it('finds a legacy record when the filter names the unit', () => {
    const legacy = record({ vehicle_name: 'B-9012-JKT' })
    expect(recordMatches(legacy, fleet, 'Unit 12')).toBe(true)
  })

  it('finds a record when the filter names the plate', () => {
    const current = record({ vehicle_name: 'Unit 12', vehicle_registration: 'B-9012-JKT' })
    expect(recordMatches(current, fleet, 'B-9012-JKT')).toBe(true)
  })

  it('does not match another unit with a shared prefix', () => {
    const legacy = record({ vehicle_name: 'B-9012-JKT' })
    expect(recordMatches(legacy, fleet, 'Unit 1')).toBe(false)
    expect(recordMatches(legacy, fleet, 'B-9048-JKT')).toBe(false)
  })
})
