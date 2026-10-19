import { useEffect, useState } from 'react'
import { NetworkMap } from '../map/LazyNetworkMap'
import { NetworkPage } from './Network'
import { ErrorMessage, Page, Panel } from './Shared'

type SimulationPreferences = { seed: number; vehicles: number; fleet_share: number }
type MapPreferences = { peak: boolean; junctions: boolean; vehicles: 'all' | 'fleet' | 'background' }
const simulationKey = 'fleetflow.simulation'
const mapKey = 'fleetflow.map'

export function GeneralSettingsPage() {
  const [health, setHealth] = useState<{ live?: string; ready?: string }>({})
  const [error, setError] = useState('')
  useEffect(() => {
    Promise.all([
      fetch('/healthz').then((r) => r.ok ? r.text() : Promise.reject(new Error(`Health check returned ${r.status}`))),
      fetch('/readyz').then(async (r) => {
        const body = await r.json() as { status?: string; error?: string }
        if (!r.ok) throw new Error(body.error ?? `Readiness check returned ${r.status}`)
        return body.status ?? 'ready'
      }),
    ]).then(([live, ready]) => setHealth({ live, ready })).catch((cause: unknown) =>
      setError(cause instanceof Error ? cause.message : 'Could not check server health.'))
  }, [])
  return <Page eyebrow="Settings" title="General" description="Service health and workspace information.">
    {error && <ErrorMessage>{error}</ErrorMessage>}
    <Panel title="Service status"><dl className="detail-list">
      <div><dt>Liveness</dt><dd>{health.live ?? 'Checking…'}</dd></div>
      <div><dt>Readiness</dt><dd>{health.ready ?? 'Checking…'}</dd></div>
      <div><dt>Workspace</dt><dd>FleetFlow simulation console</dd></div>
      <div><dt>Evidence mode</dt><dd>Simulation only — no real-world validation</dd></div>
    </dl></Panel>
  </Page>
}

export function SimulationSettingsPage() {
  const [prefs, setPrefs] = useState<SimulationPreferences>(() => {
    try { return { seed: 42, vehicles: 100, fleet_share: .5, ...JSON.parse(localStorage.getItem(simulationKey) ?? '{}') } }
    catch { return { seed: 42, vehicles: 100, fleet_share: .5 } }
  })
  const [saved, setSaved] = useState(false)
  const update = (key: keyof SimulationPreferences, value: number) => {
    setPrefs((current) => ({ ...current, [key]: value }))
    setSaved(false)
  }
  const save = () => {
    localStorage.setItem(simulationKey, JSON.stringify(prefs))
    setSaved(true)
  }
  return <Page eyebrow="Settings" title="Simulation" description="Defaults used to prefill comparison-run controls in this browser.">
    <Panel title="Demand defaults">
      <div className="form-row">
        <label className="field">Seed<input type="number" value={prefs.seed} onChange={(e) => update('seed', Number(e.target.value))} /></label>
        <label className="field">Vehicles<input type="number" min="1" value={prefs.vehicles} onChange={(e) => update('vehicles', Number(e.target.value))} /></label>
        <label className="field">Fleet share<input type="number" min="0" max="1" step=".05" value={prefs.fleet_share} onChange={(e) => update('fleet_share', Number(e.target.value))} /></label>
        <button className="button primary" onClick={save}>Save defaults</button>
      </div>
      {saved && <p className="success">Simulation defaults saved in this browser.</p>}
      <p className="help-text">These are local UI defaults; they do not change the server’s simulation model or its fixed step and rerouting parameters.</p>
    </Panel>
  </Page>
}

export function WeightsSettingsPage() {
  return <Page eyebrow="Settings" title="Routing Weights" description="The live congestion-cost model and its startup parameters.">
    <Panel title="Model parameters">
      <div className="settings-parameter-grid">
        <div><strong>α · 1.5</strong><span>Penalty strength</span></div>
        <div><strong>β · 2</strong><span>Congestion curve</span></div>
        <div><strong>γ · 0.3</strong><span>Per-report smoothing</span></div>
        <div><strong>λ · 0.1</strong><span>Decay per quiet cycle</span></div>
        <div><strong>Δ · 5 s</strong><span>Customization interval</span></div>
        <div><strong>Hold · 10 s</strong><span>Grace before decay</span></div>
      </div>
      <p className="help-text">These are the service’s validated defaults. Runtime editing is not exposed yet; the cost model is `free-flow × (1 + α × congestion^β)`.</p>
    </Panel>
  </Page>
}

export function MapSettingsPage() {
  const [prefs, setPrefs] = useState<MapPreferences>(() => {
    try { return { peak: true, junctions: false, vehicles: 'all', ...JSON.parse(localStorage.getItem(mapKey) ?? '{}') } }
    catch { return { peak: true, junctions: false, vehicles: 'all' } }
  })
  const update = (next: MapPreferences) => {
    setPrefs(next)
    localStorage.setItem(mapKey, JSON.stringify(next))
  }
  return <Page eyebrow="Settings" title="Map" description="Choose the default traffic layers and congestion view.">
    <Panel title="Map display defaults">
      <div className="settings-controls">
        <label><input type="checkbox" checked={prefs.peak} onChange={(e) => update({ ...prefs, peak: e.target.checked })} /> Show peak congestion</label>
        <label><input type="checkbox" checked={prefs.junctions} onChange={(e) => update({ ...prefs, junctions: e.target.checked })} /> Show junction dots</label>
        <label className="field">Vehicle layer<select value={prefs.vehicles} onChange={(e) => update({ ...prefs, vehicles: e.target.value as MapPreferences['vehicles'] })}>
          <option value="all">All vehicles</option><option value="fleet">Fleet vehicles only</option><option value="background">Background traffic only</option>
        </select></label>
      </div>
    </Panel>
    <NetworkMap compact />
  </Page>
}

export function NetworkSettingsPage() {
  return <NetworkPage />
}
