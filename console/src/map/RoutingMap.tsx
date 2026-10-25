import { lazy, Suspense, type ComponentProps } from 'react'
import { Loading } from '../screens/Shared'

// maplibre-gl is about a megabyte of renderer that no page needs until a map is
// on screen. Imported statically it was module-preloaded on every route,
// including the trip and vehicle lists, so the console paid for the map before
// it knew whether the page had one. It is now its own chunk, fetched when a map
// first renders.
const MapCanvas = lazy(() =>
  import('./RoutingMapCanvas').then((module) => ({ default: module.RoutingMap })))

export function RoutingMap(props: ComponentProps<typeof MapCanvas>) {
  return (
    <Suspense fallback={<Loading />}>
      <MapCanvas {...props} />
    </Suspense>
  )
}
