import { useEffect, useState } from 'react'
import { getOperatorKey, saveOperatorKey } from '../api/client'
import { ErrorMessage, Page, Panel } from './Shared'

export function GeneralSettingsPage() {
  const [health, setHealth] = useState<{ live?: string; ready?: string; routing?: string; writes?: string }>({})
  const [error, setError] = useState('')
  const [operatorKey, setOperatorKey] = useState(getOperatorKey)
  const [keySaved, setKeySaved] = useState(false)

  useEffect(() => {
    Promise.all([
      fetch('/api/healthz').then((response) => response.ok
        ? response.text()
        : Promise.reject(new Error(`Health check returned ${response.status}`))),
      fetch('/api/readyz').then(async (response) => {
        const body = (await response.json().catch(() => ({}))) as { status?: string; routing?: string; writes?: string; error?: string }
        return {
          ready: body.status ?? (response.ok ? 'ok' : `unavailable (${response.status})`),
          routing: body.routing ?? 'available',
          writes: body.writes ?? 'unknown',
        }
      }),
    ]).then(([live, readyInfo]) => setHealth({ live, ready: readyInfo.ready, routing: readyInfo.routing, writes: readyInfo.writes })).catch((cause: unknown) =>
      setError(cause instanceof Error ? cause.message : 'Could not check server health.'))
  }, [])

  const isHealthy = health.ready === 'ok' || health.ready === 'ready'

  return <Page eyebrow="Settings" title="General" description="Service health, routing provider, and map data attribution.">
    {error && <ErrorMessage>{error}</ErrorMessage>}
    <Panel title="Service status"><dl className="detail-list">
      <div>
        <dt>Service health</dt>
        <dd>
          {isHealthy ? (
            <span style={{ color: '#0ca678', fontWeight: 600 }}>● Healthy</span>
          ) : health.ready ? (
            <span style={{ color: '#f08c00', fontWeight: 600 }}>Degraded ({health.ready})</span>
          ) : error ? (
            <span style={{ color: '#e03131', fontWeight: 600 }}>Offline</span>
          ) : (
            'Checking…'
          )}
        </dd>
      </div>
      <div><dt>Liveness</dt><dd>{health.live ?? 'Checking…'}</dd></div>
      <div><dt>Readiness</dt><dd>{health.ready ?? 'Checking…'}</dd></div>
      <div><dt>Write access</dt><dd>{health.writes ?? 'Checking…'}</dd></div>
      <div><dt>Routing provider</dt><dd>OSRM driving profile (Jakarta Metropolitan Area)</dd></div>
      <div><dt>Map data attribution</dt><dd>MapLibre GL with OpenFreeMap tiles · OpenStreetMap contributors (ODbL)</dd></div>
    </dl></Panel>
    <Panel title="Operator API key">
      <p className="muted">An operator key is required to view driver records and to make changes when write access is protected. The key is kept in this browser tab only.</p>
      <form className="form-row" onSubmit={(event) => {
        event.preventDefault()
        saveOperatorKey(operatorKey)
        setOperatorKey(getOperatorKey())
        setKeySaved(true)
      }}>
        <label className="field">Bearer key
          <input type="password" autoComplete="off" value={operatorKey} onChange={(event) => {
            setOperatorKey(event.target.value)
            setKeySaved(false)
          }} />
        </label>
        <button className="button primary" type="submit">Save key for this tab</button>
        <button className="button" type="button" onClick={() => {
          saveOperatorKey('')
          setOperatorKey('')
          setKeySaved(false)
        }}>Clear key</button>
      </form>
      {keySaved && <p className="success" role="status">Operator key saved in this tab.</p>}
    </Panel>
    <Panel title="Routing service">
      <p className="muted">The server delegates driving route calculations to OSRM with the Jakarta metropolitan dataset. Configure the service with <code>OSRM_URL</code>; the default is <code>http://127.0.0.1:5000</code>.</p>
      <p className="muted">A built-in Jakarta fallback router ensures route planning and fleet simulation remain operational when the standalone OSRM container is starting or offline.</p>
    </Panel>
  </Page>
}
