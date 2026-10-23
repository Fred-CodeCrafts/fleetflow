import type { Coordinate } from '../api/client'

export interface DemoMapLocation {
  label: string
  point: Coordinate
}

export const demoMapLocations: DemoMapLocation[] = [
  { label: 'DEMO vehicle 01 · Monas', point: { lat: -6.1754, lon: 106.8272 } },
  { label: 'DEMO vehicle 02 · GBK', point: { lat: -6.2186, lon: 106.8025 } },
  { label: 'DEMO vehicle 03 · Kota Tua', point: { lat: -6.1352, lon: 106.8133 } },
]
