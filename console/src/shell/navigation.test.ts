import { describe, expect, it } from 'vitest'
import { allNavItems, locate, navigation, searchNav, visibleBranches, type NavBranch } from './navigation'
import { screenExists } from '../screens/Screen'

describe('navigation registry', () => {
  it('registers a real screen for every navigation item', () => {
    for (const item of allNavItems()) expect(screenExists(item.id), item.id).toBe(true)
  })

  it('gives each settings group its own route', () => {
    const paths = navigation.find((branch) => branch.id === 'settings')?.children.map((item) => item.path)
    expect(paths).toEqual([
      '/settings/general',
      '/settings/simulation',
      '/settings/weights',
      '/settings/map',
      '/settings/network',
    ])
    expect(new Set(paths).size).toBe(paths?.length)
  })

  it('locates the most specific route and searches by keyword', () => {
    expect(locate('/operations/trips')?.item.id).toBe('operations.orders')
    expect(searchNav('co2')[0]?.id).toBe('analytics.emissions')
  })

  it('hides branches with no permitted children', () => {
    const restricted: NavBranch[] = [{
      id: 'private', label: 'Private', icon: 'settings', path: '/private', requiresVisibleChildren: true,
      children: [{ id: 'private.page', label: 'Private', icon: 'settings', path: '/private/page', requiresPermission: 'private.read' }],
    }]
    expect(visibleBranches(() => false, restricted)).toEqual([])
  })
})
