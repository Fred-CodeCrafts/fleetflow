import { FleetStatusPage, LiveMapPage, RoutePlannerPage, TripsPage } from './Operations'
import { DriversPage, VehiclesPage } from './Records'
import { GeneralSettingsPage } from './Settings'

const pages = {
  'operations.overview': LiveMapPage,
  'operations.orders': TripsPage,
  'operations.routes': RoutePlannerPage,
  'resources.vehicles': VehiclesPage,
  'resources.drivers': DriversPage,
  'resources.fleet': FleetStatusPage,
  'settings.general': GeneralSettingsPage,
}

export function screenExists(id: string): boolean {
  return Object.hasOwn(pages, id)
}

export function Screen({ id }: { id: string }) {
  const Component = pages[id as keyof typeof pages]
  return Component ? <Component /> : <div className="error" role="alert">This page is not registered: {id}</div>
}
