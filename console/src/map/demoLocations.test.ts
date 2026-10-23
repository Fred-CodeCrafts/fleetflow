import { describe, expect, it } from 'vitest'
import { demoMapLocations } from './demoLocations'

describe('demo map locations', () => {
  it('uses labeled, static sample points within Jakarta', () => {
    expect(demoMapLocations).toHaveLength(3)
    for (const location of demoMapLocations) {
      expect(location.label).toMatch(/^DEMO vehicle \d{2} · /)
      expect(location.point.lat).toBeGreaterThan(-6.3)
      expect(location.point.lat).toBeLessThan(-6.0)
      expect(location.point.lon).toBeGreaterThan(106.6)
      expect(location.point.lon).toBeLessThan(107.0)
    }
  })
})
