import { describe, expect, it } from 'vitest'
import { allNavItems, locate, navigation, searchNav, visibleBranches, type NavBranch } from './navigation'
import { screenExists } from '../screens/Screen'

describe('navigation registry', () => {
  it('registers a real screen for every navigation item', () => {
    for (const item of allNavItems()) expect(screenExists(item.id), item.id).toBe(true)
  })

  it('keeps settings small, with the routing lab as its research page', () => {
    const paths = navigation.find((branch) => branch.id === 'settings')?.children.map((item) => item.path)
    expect(paths).toEqual(['/settings/general', '/settings/lab'])
    expect(new Set(paths).size).toBe(paths?.length)
    expect(searchNav('congestion')[0]?.id).toBe('settings.lab')
  })

  it('routes fleet operations and excludes retired experiment pages', () => {
    expect(locate('/operations/trips')?.item.id).toBe('operations.orders')
    expect(locate('/operations/simulations')?.item.id).toBe('operations.simulations')
    expect(searchNav('seed')[0]?.id).toBe('operations.simulations')
    expect(searchNav('OSRM')[0]?.id).toBe('operations.routes')
    expect(locate('/operations/runs')).toBeNull()
    expect(searchNav('comparison')).toEqual([])

    const visibleOps = visibleBranches(() => true).find((b) => b.id === 'operations')?.children.map((c) => c.id)
    expect(visibleOps).toEqual(['operations.overview', 'operations.fleet', 'operations.orders', 'operations.simulations', 'operations.routes'])
    const visibleRes = visibleBranches(() => true).find((b) => b.id === 'resources')?.children.map((c) => c.id)
    expect(visibleRes).toContain('resources.models')
    expect(visibleRes).toContain('resources.maintenance')
  })

  it('hides branches with no permitted children', () => {
    const restricted: NavBranch[] = [{
      id: 'private', label: 'Private', icon: 'settings', path: '/private', requiresVisibleChildren: true,
      children: [{ id: 'private.page', label: 'Private', icon: 'settings', path: '/private/page', requiresPermission: 'private.read' }],
    }]
    expect(visibleBranches(() => false, restricted)).toEqual([])
  })
})
