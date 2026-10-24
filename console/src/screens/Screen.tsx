import { FleetStatusPage, LiveMapPage, RoutePlannerPage } from './Operations'
import { DriversPage, VehiclesPage } from './Records'
import { ModelsPage } from './Models'
import { MaintenancePage } from './Maintenance'
import { SimulationPage, TripsPage } from './Simulation'
import { GeneralSettingsPage } from './Settings'
import { EmissionsPage, GreenDashboardPage } from './Sustainability'

const pages = {
  'operations.overview': LiveMapPage,
  'operations.fleet': FleetStatusPage,
  'operations.orders': TripsPage,
  'operations.simulations': SimulationPage,
  'operations.routes': RoutePlannerPage,
  'resources.vehicles': VehiclesPage,
  'resources.models': ModelsPage,
  'resources.drivers': DriversPage,
  'resources.maintenance': MaintenancePage,
  'sustainability.dashboard': GreenDashboardPage,
  'sustainability.emissions': EmissionsPage,
  'settings.general': GeneralSettingsPage,
}

export function screenExists(id: string): boolean {
  return Object.hasOwn(pages, id)
}

export function Screen({ id }: { id: string }) {
  const Component = pages[id as keyof typeof pages]
  return Component ? <Component /> : <div className="error" role="alert">This page is not registered: {id}</div>
}
