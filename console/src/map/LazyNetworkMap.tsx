import { lazy, Suspense, type ComponentProps } from 'react'

const MapView = lazy(() => import('./NetworkMap').then((module) => ({ default: module.NetworkMap })))

export function NetworkMap(props: ComponentProps<typeof MapView>) {
  return <Suspense fallback={<div className={`network-map ${props.compact ? 'compact' : ''}`} aria-label="Loading map" />}>
    <MapView {...props} />
  </Suspense>
}
