import { useRef, useState } from 'react'
import { api, roadClasses, type BuildResult } from '../api/client'
import { NetworkMap } from '../map/LazyNetworkMap'
import type { MapArea } from '../map/NetworkMap'
import { ErrorMessage, formatNumber, Loading, Metric, Page, Panel, useLoad } from './Shared'

export function NetworkPage() {
  const network = useLoad(api.network)
  const input = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [mapVersion, setMapVersion] = useState(0)

  // The area builder. `area` is what the map drew; `classes` is what to keep.
  const [area, setArea] = useState<MapArea | null>(null)
  const [classes, setClasses] = useState<string[]>(['motorway', 'trunk', 'primary', 'secondary'])
  const [building, setBuilding] = useState(false)
  const [built, setBuilt] = useState<BuildResult | null>(null)
  const [buildError, setBuildError] = useState('')

  const upload = async (file?: File) => {
    if (!file) return
    setBusy(true)
    setMessage('')
    setError('')
    try {
      const loaded = await api.importNetwork(file)
      network.setData(loaded)
      setMapVersion((version) => version + 1)
      setMessage(`Loaded ${loaded.name}: ${loaded.nodes.toLocaleString()} junctions and ${loaded.edges.toLocaleString()} road segments.`)
      if (input.current) input.current.value = ''
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'The network could not be imported.')
    } finally {
      setBusy(false)
    }
  }

  const toggleClass = (name: string) => {
    setClasses((current) => current.includes(name)
      ? current.filter((c) => c !== name)
      : [...current, name])
  }

  const build = async (install: boolean) => {
    if (!area) {
      setBuildError('Drag a box on the map first.')
      return
    }
    setBuilding(true)
    setBuildError('')
    setBuilt(null)
    try {
      const result = await api.buildNetwork({ ...area, classes, install })
      setBuilt(result)
      if (install) {
        network.setData(result.network)
        setMapVersion((version) => version + 1)
      }
    } catch (cause) {
      setBuildError(cause instanceof Error ? cause.message : 'The network could not be built.')
    } finally {
      setBuilding(false)
    }
  }

  return <Page eyebrow="Settings / Network" title="Road Network" description="Inspect the active graph, draw an area to build a network from, or load a SUMO netconvert network.">
    {network.error && <ErrorMessage>{network.error}</ErrorMessage>}
    {network.loading ? <Loading /> : network.data && <>
      <div className="metric-grid">
        <Metric label="Junctions" value={network.data.nodes.toLocaleString()} />
        <Metric label="Road segments" value={network.data.edges.toLocaleString()} note={`${network.data.shaped_edges.toLocaleString()} include road geometry`} />
        <Metric label="Network length" value={`${formatNumber(network.data.length_km)} km`} />
        <Metric label="Capacity" value={formatNumber(network.data.capacity, 0)} note="estimated vehicles across the graph" />
      </div>
      <Panel title="Active network">
        <dl className="detail-list">
          <div><dt>Name</dt><dd>{network.data.name}</dd></div>
          <div><dt>Source</dt><dd>{sourceLabel(network.data.source)}</dd></div>
          <div><dt>Geographic bounds</dt><dd>{formatNumber(network.data.bounds.min_lat, 4)}, {formatNumber(network.data.bounds.min_lon, 4)} â€” {formatNumber(network.data.bounds.max_lat, 4)}, {formatNumber(network.data.bounds.max_lon, 4)}</dd></div>
        </dl>
      </Panel>
    </>}

    <Panel title="Build a network from the map">
      <p className="muted">
        Drag a box on the map below to choose an area. The service fetches the roads inside it
        from OpenStreetMap, converts them to a SUMO network, and can load it as the active graph.
        Import is blocked while a comparison run is active.
      </p>
      <div className="settings-controls">
        <span className="field-label">Road classes</span>
        {roadClasses.map((name) => (
          <label key={name} className="map-toggle">
            <input type="checkbox" checked={classes.includes(name)} onChange={() => toggleClass(name)} />
            {name}
          </label>
        ))}
      </div>
      {area ? (
        <p className="help-text">
          Selected area: {formatNumber(area.min_lat, 4)}, {formatNumber(area.min_lon, 4)} â€”{' '}
          {formatNumber(area.max_lat, 4)}, {formatNumber(area.max_lon, 4)}
        </p>
      ) : (
        <p className="help-text">No area selected yet. Drag on the map to choose one.</p>
      )}
      <div className="form-row">
        <button className="button primary" onClick={() => void build(true)} disabled={building || !area}>
          {building ? 'Buildingâ€¦' : 'Build and load'}
        </button>
        <button className="button" onClick={() => void build(false)} disabled={building || !area}>
          Build only
        </button>
        {area && <button className="button" onClick={() => setArea(null)} disabled={building}>Clear area</button>}
        {built && <a className="button" href={api.downloadBuiltNetwork()}>Download .net.xml</a>}
      </div>
      {building && <p className="muted">Fetching roads and converting them; this can take up to a minute.</p>}
      {built && <p className="success" role="status">
        Built {built.network.nodes.toLocaleString()} junctions and {built.network.edges.toLocaleString()} road segments
        from {built.osm_ways.toLocaleString()} OpenStreetMap ways
        ({formatNumber(built.bytes / 1024, 0)} KiB, {formatNumber(built.duration_ms / 1000, 1)} s).
      </p>}
      {buildError && <ErrorMessage>{buildError}</ErrorMessage>}
    </Panel>

    <NetworkMap
      key={mapVersion}
      compact
      selectArea
      area={area}
      onAreaChange={setArea}
    />

    <Panel title="Import a SUMO network">
      <p className="muted">Choose a <code>.net.xml</code> generated by SUMO netconvert. Import is blocked while a comparison run is active.</p>
      <div className="form-row">
        <input ref={input} type="file" accept=".net.xml,.xml,application/xml,text/xml" onChange={(event) => void upload(event.target.files?.[0])} />
        {busy && <span className="muted">Validating and loading networkâ€¦</span>}
      </div>
      {message && <p className="success" role="status">{message}</p>}
      {error && <ErrorMessage>{error}</ErrorMessage>}
    </Panel>
  </Page>
}

/** sourceLabel says where the active network came from, in words. */
function sourceLabel(source: string): string {
  switch (source) {
    case 'config': return 'Loaded at startup from GRAPH_PATH'
    case 'upload': return 'Imported as a .net.xml file'
    case 'area': return 'Built from a map area'
    case 'generated': return 'Built from a map area (not loaded)'
    default: return source
  }
}

