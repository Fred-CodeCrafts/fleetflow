import { describe, expect, it } from 'vitest'
import { runnerLabel } from './Shared'

describe('runner labels', () => {
  it('shows only the SUMO version from the full command output', () => {
    expect(runnerLabel({
      runner: 'sumo',
      sumo_version: 'Eclipse SUMO sumo Version 1.15.0 Build features: Linux x86_64 GNU 12.2.0',
    })).toBe('SUMO 1.15.0')
  })

  it('accepts a version string without the command banner', () => {
    expect(runnerLabel({ runner: 'sumo', sumo_version: '1.15.0' })).toBe('SUMO 1.15.0')
  })

  it('does not expose unrecognized command output', () => {
    expect(runnerLabel({ runner: 'sumo', sumo_version: 'unexpected command output' })).toBe('SUMO')
  })

  it('keeps synthetic and unrecorded labels clear', () => {
    expect(runnerLabel({ runner: 'synthetic' })).toBe('FleetFlow synthetic generator')
    expect(runnerLabel({})).toBe('Not recorded')
  })
})
