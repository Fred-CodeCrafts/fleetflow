import { ComparisonRunsPage, FleetStatusPage, LiveMapPage, PerformancePage, RoutePlannerPage, TripsPage } from './Operations'
import { DriversPage, RunResultsPage, VehiclesPage } from './Records'
import { EmissionsPage } from './Emissions'
import { NetworkPage } from './Network'
import { GeneralSettingsPage, MapSettingsPage, SimulationSettingsPage, WeightsSettingsPage } from './Settings'

const pages = {
  'operations.overview': LiveMapPage,
  'operations.orders': TripsPage,
  'operations.routes': RoutePlannerPage,
  'operations.runs': ComparisonRunsPage,
  'resources.vehicles': VehiclesPage,
  'resources.drivers': DriversPage,
  'resources.fleet': FleetStatusPage,
  'analytics.overview': PerformancePage,
  'analytics.emissions': EmissionsPage,
  'analytics.results': RunResultsPage,
  'settings.general': GeneralSettingsPage,
  'settings.simulation': SimulationSettingsPage,
  'settings.weights': WeightsSettingsPage,
  'settings.map': MapSettingsPage,
  'settings.network': NetworkPage,
}

export function screenExists(id: string): boolean {
  return Object.hasOwn(pages, id)
}

export function Screen({ id }: { id: string }) {
  const Component = pages[id as keyof typeof pages]
  return Component ? <Component /> : <div className="error" role="alert">This page is not registered: {id}</div>
}
