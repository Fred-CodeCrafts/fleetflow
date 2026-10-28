import { lazy, Suspense } from 'react'
import { Loading } from './Shared'

const pages = {
  'operations.overview': lazy(() => import('./Operations').then((module) => ({ default: module.LiveMapPage }))),
  'operations.fleet': lazy(() => import('./Operations').then((module) => ({ default: module.FleetStatusPage }))),
  'operations.orders': lazy(() => import('./Simulation').then((module) => ({ default: module.TripsPage }))),
  'operations.simulations': lazy(() => import('./Simulation').then((module) => ({ default: module.SimulationPage }))),
  'operations.routes': lazy(() => import('./Operations').then((module) => ({ default: module.RoutePlannerPage }))),
  'resources.vehicles': lazy(() => import('./Records').then((module) => ({ default: module.VehiclesPage }))),
  'resources.models': lazy(() => import('./Models').then((module) => ({ default: module.ModelsPage }))),
  'resources.drivers': lazy(() => import('./Records').then((module) => ({ default: module.DriversPage }))),
  'resources.maintenance': lazy(() => import('./Maintenance').then((module) => ({ default: module.MaintenancePage }))),
  'sustainability.dashboard': lazy(() => import('./Sustainability').then((module) => ({ default: module.SustainabilityPage }))),
  'settings.general': lazy(() => import('./Settings').then((module) => ({ default: module.GeneralSettingsPage }))),
  'settings.lab': lazy(() => import('./Lab').then((module) => ({ default: module.LabPage }))),
}

export function screenExists(id: string): boolean {
  return Object.hasOwn(pages, id)
}

export function Screen({ id }: { id: string }) {
  const Component = pages[id as keyof typeof pages]
  return Component
    ? <Suspense fallback={<Loading />}><Component /></Suspense>
    : <div className="error" role="alert">This page is not registered: {id}</div>
}
