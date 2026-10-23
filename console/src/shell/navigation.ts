/**
 * The navigation registry.
 *
 * Fleetbase builds its sidebar from a registry rather than from a hardcoded
 * component tree: a branch carries an id, a label, an icon, a route, a default
 * child route and a keyword list, and sections contribute items into named
 * registries. This module is that idea, written for this console.
 *
 * Why it is worth the indirection:
 * - A screen becomes reachable by registering it, so routes, the sidebar, the
 *   command palette and the breadcrumb all read one source instead of three.
 * - Search over `keywords` is free, which is what makes a command palette
 *   useful rather than decorative.
 * - `requiresPermission` keeps an item out of the sidebar without every
 *   component learning about permissions.
 *
 * The registry is a plain data module with no React in it, so the tests can
 * assert the shape of the whole navigation without rendering anything.
 */

/** What an icon name refers to. The icon set is this console's own, so the
 *  names are ours rather than a library's. */
export type IconName =
  | 'dashboard'
  | 'map'
  | 'orders'
  | 'routes'
  | 'vehicles'
  | 'drivers'
  | 'trips'
  | 'runs'
  | 'analytics'
  | 'emissions'
  | 'network'
  | 'simulation'
  | 'settings'
  | 'reports'
  | 'chevron'
  | 'search'
  | 'plus'
  | 'refresh'
  | 'play'
  | 'stop'
  | 'save'
  | 'check'
  | 'warning'
  | 'offline'
  | 'online'

/** One leaf in the sidebar: a screen. */
export interface NavItem {
  id: string
  label: string
  icon: IconName
  /** Route path, absolute, as react-router matches it. */
  path: string
  /** Words that should find this item in the command palette. */
  keywords?: string[]
  /** Hidden from the sidebar when false. Defaults to visible. */
  visible?: boolean
  /** A permission string the session must hold. Absent means everyone. */
  requiresPermission?: string
}

/** A top-level section, which may hold children. */
export interface NavBranch {
  id: string
  label: string
  icon: IconName
  /** Where the branch's own header link goes. */
  path: string
  keywords?: string[]
  children: NavItem[]
  /**
   * A branch with no visible children is hidden, which is how a section whose
   * screens are all permission-gated disappears cleanly for a restricted user.
   */
  requiresVisibleChildren?: boolean
}

/** Everything the console can navigate to, in sidebar order. */
export const navigation: NavBranch[] = [
  {
    id: 'operations',
    label: 'Operations',
    icon: 'map',
    path: '/operations',
    keywords: ['dispatch', 'live map', 'track vehicles', 'control room'],
    children: [
      {
        id: 'operations.overview',
        label: 'Live Map',
        icon: 'map',
        path: '/operations',
        keywords: ['dashboard', 'live', 'map', 'track', 'vehicles', 'congestion'],
      },
      {
        id: 'operations.orders',
        label: 'Trips',
        icon: 'orders',
        path: '/operations/trips',
        keywords: ['orders', 'trips', 'dispatch', 'assign', 'planned', 'in progress', 'completed'],
      },
      {
        id: 'operations.routes',
        label: 'Route Planner',
        icon: 'routes',
        path: '/operations/routes',
        keywords: ['route', 'planner', 'origin', 'destination', 'shortest path', 'cost'],
      },
      {
        id: 'operations.runs',
        label: 'Comparison Runs',
        icon: 'runs',
        path: '/operations/runs',
        keywords: ['runs', 'comparison', 'saved', 'baseline', 'results', 'seed'],
        // Hidden from the sidebar at the operator's request. The screen and its
        // route still work; only the entry point is gone, so the comparison
        // machinery stays reachable by URL and is not deleted.
        visible: false,
      },
    ],
  },
  {
    id: 'resources',
    label: 'Resources',
    icon: 'vehicles',
    path: '/resources',
    keywords: ['drivers', 'vehicles', 'fleet'],
    requiresVisibleChildren: true,
    children: [
      {
        id: 'resources.vehicles',
        label: 'Vehicles',
        icon: 'vehicles',
        path: '/resources/vehicles',
        keywords: ['vehicle', 'truck', 'van', 'status', 'capacity', 'emission standard'],
      },
      {
        id: 'resources.drivers',
        label: 'Drivers',
        icon: 'drivers',
        path: '/resources/drivers',
        keywords: ['driver', 'contact', 'assignment', 'shift'],
      },
      {
        id: 'resources.fleet',
        label: 'Fleet Status',
        icon: 'trips',
        path: '/resources/fleet',
        keywords: ['fleet', 'availability', 'assigned', 'idle', 'maintenance'],
      },
    ],
  },
  {
    id: 'analytics',
    label: 'Analytics',
    icon: 'analytics',
    path: '/analytics',
    keywords: ['metrics', 'reports', 'performance'],
    requiresVisibleChildren: true,
    children: [
      {
        id: 'analytics.overview',
        label: 'Performance',
        icon: 'analytics',
        path: '/analytics',
        keywords: ['performance', 'average trip', 'p95', 'saturation', 'route changes'],
      },
      {
        id: 'analytics.emissions',
        label: 'Emissions',
        icon: 'emissions',
        path: '/analytics/emissions',
        keywords: ['emissions', 'co2', 'carbon', 'fuel', 'nox', 'green', 'sustainability'],
      },
      {
        id: 'analytics.results',
        label: 'Run Results',
        icon: 'reports',
        path: '/analytics/results',
        keywords: ['results', 'ten seeds', 'evidence', 'comparison', 'table'],
        // Hidden from the sidebar at the operator's request, like Comparison
        // Runs. The route and screen remain.
        visible: false,
      },
    ],
  },
  {
    id: 'settings',
    label: 'Settings',
    icon: 'settings',
    path: '/settings',
    keywords: ['configuration', 'setup'],
    requiresVisibleChildren: true,
    children: [
      {
        id: 'settings.general',
        label: 'General',
        icon: 'settings',
        path: '/settings/general',
        keywords: ['general', 'environment', 'version', 'about'],
      },
      {
        id: 'settings.simulation',
        label: 'Simulation',
        icon: 'simulation',
        path: '/settings/simulation',
        keywords: ['simulation', 'seed', 'vehicles', 'fleet share', 'demo', 'step'],
      },
      {
        id: 'settings.weights',
        label: 'Routing Weights',
        icon: 'routes',
        path: '/settings/weights',
        keywords: ['weights', 'alpha', 'beta', 'gamma', 'lambda', 'decay', 'smoothing', 'hysteresis'],
      },
      {
        id: 'settings.map',
        label: 'Map',
        icon: 'map',
        path: '/settings/map',
        keywords: ['map', 'basemap', 'layers', 'junction dots', 'colour scale'],
      },
      {
        id: 'settings.network',
        label: 'Road Network',
        icon: 'network',
        path: '/settings/network',
        keywords: ['network', 'graph', 'import', 'sumo', 'net.xml', 'osm', 'jakarta'],
      },
    ],
  },
]

/** Flatten the registry to every navigable item, for search and for routes. */
export function allNavItems(branches: NavBranch[] = navigation): NavItem[] {
  return branches.flatMap((branch) => branch.children)
}

/** The branches a session should see: no empty sections, no hidden items. */
export function visibleBranches(
  can: (permission: string) => boolean,
  branches: NavBranch[] = navigation,
): NavBranch[] {
  return branches
    .map((branch) => ({
      ...branch,
      children: branch.children.filter(
        (item) => item.visible !== false && (!item.requiresPermission || can(item.requiresPermission)),
      ),
    }))
    .filter((branch) => !branch.requiresVisibleChildren || branch.children.length > 0)
}

/** Which branch and item a path belongs to, for the breadcrumb and the sidebar
 *  highlight. Longest matching path wins, so `/operations/trips` does not
 *  resolve to `/operations`. */
export function locate(
  pathname: string,
  branches: NavBranch[] = navigation,
): { branch: NavBranch; item: NavItem } | null {
  let best: { branch: NavBranch; item: NavItem } | null = null
  for (const branch of branches) {
    for (const item of branch.children) {
      const matches = pathname === item.path || pathname.startsWith(item.path + '/')
      if (!matches) continue
      if (!best || item.path.length > best.item.path.length) best = { branch, item }
    }
  }
  return best
}

/** Search the registry by label and keyword. Used by the command palette. */
export function searchNav(query: string, branches: NavBranch[] = navigation): NavItem[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  const scored: { item: NavItem; score: number }[] = []
  for (const item of allNavItems(branches)) {
    const label = item.label.toLowerCase()
    const keywords = item.keywords ?? []
    let score = 0
    if (label === q) score = 100
    else if (label.startsWith(q)) score = 80
    else if (label.includes(q)) score = 60
    else if (keywords.some((k) => k.toLowerCase() === q)) score = 50
    else if (keywords.some((k) => k.toLowerCase().includes(q))) score = 30
    if (score > 0) scored.push({ item, score })
  }
  return scored.sort((a, b) => b.score - a.score || a.item.label.localeCompare(b.item.label)).map((s) => s.item)
}
