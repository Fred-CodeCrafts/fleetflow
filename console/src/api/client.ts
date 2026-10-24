export interface Vehicle {
  id: string
  name: string
  type: string
  status: string
  registration: string | null
  make: string | null
  model: string | null
  year: number | null
  color: string | null
  body_type: string | null
  fuel_type: string | null
  capacity: number | null
  maintenance_history: string | null
}

export interface Driver {
  id: string
  name: string
  contact: string
  contact_phone: string | null
  license_class: string | null
  license_expiry: string | null
  shift_start: string | null
  shift_end: string | null
  availability: string | null
  vehicle_id?: string
}

export interface SimulationScenario {
  seed: number
  vehicles: number
  departure_window_s: number
}

export interface SimulatedTrip {
  id: string
  status: 'running' | 'completed'
  depart_s: number
  arrival_s?: number
  duration_s: number
  distance_m: number
  co2_kg: number
  fuel_kg: number
}

export interface SimulatedPosition {
  time_s: number
  longitude: number
  latitude: number
}

export interface SimulatedTrack {
  vehicle_id: string
  positions: SimulatedPosition[]
}

export interface SimulatedDeliveryStop {
  id: string
  order: number
  package_id: string
  latitude: number
  longitude: number
  time_s: number
  status: 'pending' | 'reached' | 'delivered'
}

export interface SimulatedPackage {
  id: string
  stop_id: string
  status: 'in_transit' | 'delivered'
}

export interface SimulatedDeliveryEvent {
  sequence: number
  time_s: number
  type: string
  package_id?: string
  stop_id?: string
}

export interface SimulatedDelivery {
  id: string
  trip_id: string
  vehicle_id: string
  status: 'in_progress' | 'completed'
  packages: SimulatedPackage[]
  stops: SimulatedDeliveryStop[]
  events: SimulatedDeliveryEvent[]
}

export interface EmissionProvenance {
  model: string
  class: string
  source: string
  input_unit: string
  output_unit: string
  assumptions: string
}

export interface SimulationResult {
  sumo_version: string
  seed: number
  started_at: string
  finished_at: string
  emissions?: EmissionProvenance
  trips: SimulatedTrip[]
  tracks: SimulatedTrack[]
  deliveries?: SimulatedDelivery[]
}

export interface SimulationRun {
  id: string
  status: 'queued' | 'running' | 'completed' | 'failed'
  scenario: SimulationScenario
  created_at: string
  started_at?: string
  finished_at?: string
  error?: string
  result?: SimulationResult
}

export interface Coordinate {
  lat: number
  lon: number
}

export interface OSRMRoute {
  geometry: [number, number][]
  distance_m: number
  duration_s: number
  provider: 'OSRM'
}

export interface VehicleLocation extends Coordinate {
  vehicle_id: string
  speed_mps: number
  updated_at: string
  age_s: number
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
  })
  if (!response.ok) {
    let detail = `Request failed (${response.status})`
    let code: string | undefined
    try {
      const body = await response.json() as { error?: string; message?: string; code?: string }
      detail = body.message ?? body.error ?? detail
      code = body.code
    } catch {
      // Error responses may be plain text; the HTTP status remains actionable.
    }
    throw new ApiError(detail, response.status, code)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export const api = {
  vehicles: async () => (await request<{ vehicles: Vehicle[] }>('/api/vehicles')).vehicles,
  drivers: async () => (await request<{ drivers: Driver[] }>('/api/drivers')).drivers,
  simulationRuns: async () => (await request<{ runs: SimulationRun[] }>('/api/simulation/runs')).runs,
  simulationRun: (id: string) =>
    request<SimulationRun>(`/api/simulation/runs/${encodeURIComponent(id)}`),
  startSimulation: (scenario: SimulationScenario) =>
    request<SimulationRun>('/api/simulation/runs', {
      method: 'POST',
      body: JSON.stringify(scenario),
    }),
  route: (from: Coordinate, to: Coordinate) =>
    request<OSRMRoute>('/api/route', {
      method: 'POST',
      body: JSON.stringify({ from, to }),
    }),
  locations: async () => (await request<{ positions: VehicleLocation[] }>('/api/locations')).positions,
  reportLocation: (location: Pick<VehicleLocation, 'vehicle_id' | 'lat' | 'lon' | 'speed_mps'>) =>
    request<void>('/api/locations', { method: 'POST', body: JSON.stringify(location) }),
  createVehicle: (vehicle: Pick<Vehicle, 'name' | 'type' | 'status'> & Partial<Pick<Vehicle,
    'registration' | 'make' | 'model' | 'year' | 'color' | 'body_type' | 'fuel_type' |
    'capacity' | 'maintenance_history'>>) =>
    request<Vehicle>('/api/vehicles', { method: 'POST', body: JSON.stringify(vehicle) }),
  createDriver: (driver: Pick<Driver, 'name'> & Partial<Pick<Driver,
    'contact' | 'contact_phone' | 'license_class' | 'license_expiry' | 'shift_start' | 'shift_end' | 'availability'>>) =>
    request<Driver>('/api/drivers', { method: 'POST', body: JSON.stringify(driver) }),
}
