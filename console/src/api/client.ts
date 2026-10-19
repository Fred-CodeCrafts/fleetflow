export interface GraphNode {
  id: string
  lat: number
  lon: number
}

export interface GraphPoint {
  lat: number
  lon: number
}

export interface GraphEdge {
  id: string
  from: string
  to: string
  length_m: number
  lanes: number
  speed_limit_mps: number
  free_flow_s: number
  capacity: number
  shape?: GraphPoint[]
}

export interface RoadGraph {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface NetworkSummary {
  name: string
  source: string
  nodes: number
  edges: number
  shaped_edges: number
  capacity: number
  length_km: number
  bounds: { min_lat: number; min_lon: number; max_lat: number; max_lon: number }
}

export interface SimVehicle {
  id: string
  kind: 'simulated' | 'live'
  arm?: 'baseline' | 'fleetflow'
  fleet?: boolean
  state: string
  edge_id?: string
  lat: number
  lon: number
  speed_mps?: number
  age_s?: number
}

export interface StreamEdge {
  edge_id: string
  cost_s: number
  free_flow_s: number
  load_ratio: number
}

export interface MetricsArm {
  average_trip_s: number
  p95_trip_s: number
  completed_trips: number
  saturated_edges_pct: number
  route_changes: number
  distance_km: number
  emissions: {
    co2_g: number
    nox_g: number
    co_g: number
    hc_g: number
    pm_g: number
    fuel_litres: number
    method: string
  }
}

export interface MetricsResponse {
  engine: Record<string, unknown>
  run?: {
    seed: number
    vehicles: number
    fleet_share: number
    running: boolean
    baseline: MetricsArm
    fleet_flow: MetricsArm
  } | null
}

export interface Vehicle {
  id: string
  name: string
  type: string
  status: string
}

export interface Driver {
  id: string
  name: string
  contact: string
  vehicle_id?: string
}

export interface Trip {
  id: string
  origin: string
  destination: string
  vehicle_id?: string
  status: string
  started_at: string | null
  finished_at: string | null
  duration_s: number | null
}

export interface SavedRun {
  id: string
  seed: number
  demo: boolean
  config: Record<string, unknown>
  metrics: { baseline?: MetricsArm; fleet_flow?: MetricsArm }
  created_at: string
}

export interface EmissionsResponse {
  run?: {
    seed: number
    vehicles: number
    fleet_share: number
    baseline: EmissionArm
    fleet_flow: EmissionArm
    savings: EmissionArm
  } | null
  trip?: EmissionTrip | null
  classes: string[]
  method: string
}

export interface EmissionArm {
  distance_km: number
  co2_g: number
  nox_g: number
  co_g: number
  hc_g: number
  pm_g: number
  fuel_litres: number
  co2_per_km: number
}

export interface EmissionTrip extends EmissionArm {
  class: string
  fuel: string
  mean_speed_kmh: number
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
  graph: () => request<RoadGraph>('/api/graph'),
  network: () => request<NetworkSummary>('/api/network'),
  metrics: () => request<MetricsResponse>('/api/metrics'),
  emissions: (query = '') => request<EmissionsResponse>(`/api/emissions${query}`),
  vehicles: async () => (await request<{ vehicles: Vehicle[] }>('/api/vehicles')).vehicles,
  drivers: async () => (await request<{ drivers: Driver[] }>('/api/drivers')).drivers,
  trips: async () => (await request<{ trips: Trip[] }>('/api/trips')).trips,
  runs: async () => (await request<{ runs: SavedRun[] }>('/api/runs')).runs,
  route: (from_node: string, to_node: string) =>
    request<{ edges: string[]; cost_s: number; metric_version: number }>('/api/route', {
      method: 'POST',
      body: JSON.stringify({ from_node, to_node }),
    }),
  start: (config: { seed: number; vehicles: number; fleet_share: number }) =>
    request('/api/sim/start', { method: 'POST', body: JSON.stringify(config) }),
  reset: () => request<void>('/api/sim/reset', { method: 'POST' }),
  saveRun: () => request('/api/runs', { method: 'POST' }),
  createVehicle: (vehicle: Pick<Vehicle, 'name' | 'type' | 'status'>) =>
    request<Vehicle>('/api/vehicles', { method: 'POST', body: JSON.stringify(vehicle) }),
  createDriver: (driver: Pick<Driver, 'name' | 'contact'>) =>
    request<Driver>('/api/drivers', { method: 'POST', body: JSON.stringify(driver) }),
  assignDriver: (vehicleID: string, driverID: string) =>
    request(`/api/vehicles/${encodeURIComponent(vehicleID)}/driver`, {
      method: 'POST',
      body: JSON.stringify({ driver_id: driverID }),
    }),
  createTrip: (trip: Pick<Trip, 'origin' | 'destination' | 'vehicle_id'>) =>
    request<Trip>('/api/trips', { method: 'POST', body: JSON.stringify(trip) }),
  tripStatus: (id: string, status: string) =>
    request<Trip>(`/api/trips/${encodeURIComponent(id)}/status`, {
      method: 'POST',
      body: JSON.stringify({ status }),
    }),
  importNetwork: (file: File) =>
    request<NetworkSummary>('/api/network', {
      method: 'POST',
      headers: { 'Content-Type': 'application/xml', 'X-Network-Name': file.name },
      body: file,
    }),
}
