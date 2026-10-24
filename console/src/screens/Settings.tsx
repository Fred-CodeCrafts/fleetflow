import { useEffect, useState } from 'react'
import { ErrorMessage, Page, Panel } from './Shared'

export function GeneralSettingsPage() {
  const [health, setHealth] = useState<{ live?: string; ready?: string }>({})
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([
      fetch('/api/healthz').then((response) => response.ok
        ? response.text()
        : Promise.reject(new Error(`Health check returned ${response.status}`))),
      fetch('/api/readyz').then(async (response) => {
        const body = await response.json() as { status?: string; error?: string }
        if (!response.ok) throw new Error(body.error ?? `Readiness check returned ${response.status}`)
        return body.status ?? 'ready'
      }),
    ]).then(([live, ready]) => setHealth({ live, ready })).catch((cause: unknown) =>
      setError(cause instanceof Error ? cause.message : 'Could not check server health.'))
  }, [])

  return <Page eyebrow="Settings" title="General" description="Service health, routing provider, and map data attribution.">
    {error && <ErrorMessage>{error}</ErrorMessage>}
    <Panel title="Service status"><dl className="detail-list">
      <div><dt>Liveness</dt><dd>{health.live ?? 'Checking…'}</dd></div>
      <div><dt>Readiness</dt><dd>{health.ready ?? 'Checking…'}</dd></div>
      <div><dt>Routing provider</dt><dd>OSRM driving profile</dd></div>
      <div><dt>Map</dt><dd>MapLibre GL with OpenFreeMap tiles</dd></div>
      <div><dt>Road data</dt><dd>OpenStreetMap contributors · ODbL</dd></div>
    </dl></Panel>
    <Panel title="Routing service">
      <p className="muted">The server delegates driving route calculations to OSRM. Configure the service with <code>OSRM_URL</code>; the default is <code>http://127.0.0.1:5000</code>.</p>
      <p className="muted">Routes are unavailable while the OSRM service or its Jakarta extract is offline. FleetFlow does not calculate a substitute route.</p>
    </Panel>
  </Page>
}
