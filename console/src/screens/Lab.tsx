import { useEffect, useState } from 'react'
import { api, type LabDynamicResponse, type LabRun } from '../api/client'
import { ErrorMessage, errorText, formatNumber, Metric, Page, Panel, Table } from './Shared'

// The adoption levels the screen offers. 0 is kept on purpose: it is the control,
// where both arms must produce identical numbers.
const shares = [0, 0.25, 0.5, 0.75, 1]

function shareLabel(value: number) {
  return value === 0 ? '0% (control)' : `${Math.round(value * 100)}%`
}

export function LabPage() {
  const [share, setShare] = useState(0.5)
  const [seeds, setSeeds] = useState(5)
  const [data, setData] = useState<LabDynamicResponse | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const run = async (nextShare = share, nextSeeds = seeds) => {
    setBusy(true)
    setError('')
    try {
      setData(await api.labDynamic({ share: nextShare, seeds: nextSeeds }))
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  // The screen opens on the default experiment rather than an empty form: the
  // first thing a reader should see is a result, not a prompt.
  useEffect(() => {
    void run(0.5, 5)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const summary = data?.summary
  const firstRun: LabRun | undefined = summary?.runs[0]

  return <Page
    eyebrow="Operations · Routing"
    title="Routing Lab"
    description="The same seeded demand routed twice: once believing the speed limit, once on congestion learned from the fleet's own speed reports."
  >
    <Panel
      title="Experiment"
      action={<button className="button primary" type="button" disabled={busy} onClick={() => void run()}>
        {busy ? 'Running…' : 'Run experiment'}
      </button>}
    >
      <form className="form-grid" onSubmit={(event) => { event.preventDefault(); void run() }}>
        <label className="field">
          Fleet on dynamic routing (adoption)
          <select value={share} onChange={(event) => setShare(Number(event.target.value))}>
            {shares.map((value) => <option key={value} value={value}>{shareLabel(value)}</option>)}
          </select>
        </label>
        <label className="field" style={{ maxWidth: '120px' }}>
          Seeds
          <input
            type="number"
            min={1}
            max={25}
            value={seeds}
            onChange={(event) => setSeeds(Number(event.target.value))}
            required
          />
        </label>
      </form>
      {data && <p className="help-text">
        {data.config.grid}×{data.config.grid} lattice · {data.config.waves} waves · {data.config.vehicles} trips per wave · seeds {data.config.base_seed}–{data.config.base_seed + data.config.seeds - 1} · {formatNumber(data.duration_ms / 1000, 2)} s to run
      </p>}
      {error && <ErrorMessage>{error}</ErrorMessage>}
    </Panel>

    {summary && <>
      <div className="metric-grid">
        <Metric label="Static routing" value={`${formatNumber(summary.static_mean_s)} s`} note="free-flow belief, same demand" />
        <Metric label="Dynamic routing" value={`${formatNumber(summary.dynamic_mean_s)} s`} note={`${shareLabel(summary.share)} adoption`} />
        <Metric
          label="Mean improvement"
          value={`${summary.improvement_pct >= 0 ? '+' : ''}${formatNumber(summary.improvement_pct, 2)}%`}
          note={`worst seed ${formatNumber(summary.improvement_min_pct, 2)}%, best ${formatNumber(summary.improvement_max_pct, 2)}%`}
        />
      </div>

      <Panel title="Per seed">
        <Table headers={['Seed', 'Static (s)', 'Dynamic (s)', 'Improvement', 'Route changes']}>
          {summary.runs.map((run) => <tr key={run.seed}>
            <td>{run.seed}</td>
            <td>{formatNumber(run.static_mean_s)}</td>
            <td>{formatNumber(run.dynamic_mean_s)}</td>
            <td>{run.improvement_pct >= 0 ? '+' : ''}{formatNumber(run.improvement_pct, 2)}%</td>
            <td>{run.dynamic_route_changes}</td>
          </tr>)}
        </Table>
        <p className="help-text">
          A trip time is the sum of its edges at free-flow time multiplied by the congestion
          the wave itself creates. Route changes count trips whose dynamic path differs from
          the free-flow one.
        </p>
      </Panel>

      {firstRun && <Panel title="Wave by wave, seed 1">
        <WaveChart run={firstRun} />
      </Panel>}

      <Panel title="What produced these numbers">
        <dl className="detail-list">
          <dt>Model</dt><dd>{data.method.model}</dd>
          <dt>Network</dt><dd>{data.method.network}</dd>
          <dt>Routing</dt><dd>{data.method.routing}</dd>
          <dt>Congestion</dt><dd>{data.method.congestion}</dd>
          <dt>Assumptions</dt><dd>{data.method.assumptions}</dd>
          <dt>Not included</dt><dd>{data.method.not_included}</dd>
        </dl>
        <p className="help-text">
          This is the idea from the author's thesis proposal, run at grid scale. It is not
          the thesis: the proposal's network, its contraction hierarchies and its SUMO loop
          are not here. A percentage from this page describes this model, not Jakarta.
        </p>
      </Panel>
    </>}
  </Page>
}

/** Two lines over the waves of one seed: what a static and a dynamic fleet paid. */
function WaveChart({ run }: { run: LabRun }) {
  const series = run.series
  if (series.length < 2) return <p className="help-text">Not enough waves to draw.</p>

  const values = series.flatMap((wave) => [wave.static_mean_s, wave.dynamic_mean_s])
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = Math.max(max - min, 1e-6)
  const x = (i: number) => 34 + (i * 276) / (series.length - 1)
  const y = (value: number) => 118 - ((value - min) / span) * 96
  const points = (pick: (wave: typeof series[number]) => number) =>
    series.map((wave, i) => `${x(i).toFixed(1)},${y(pick(wave)).toFixed(1)}`).join(' ')

  return <div>
    <svg viewBox="0 0 328 140" width="100%" height="170" role="img"
      aria-label={`Mean trip time per wave, seed ${run.seed}: static versus dynamic`}>
      <line x1="34" y1="118" x2="310" y2="118" stroke="#d0d5dd" />
      <line x1="34" y1="22" x2="34" y2="118" stroke="#d0d5dd" />
      <text x="4" y="26" fontSize="9" fill="#667085">{formatNumber(max)} s</text>
      <text x="4" y="120" fontSize="9" fill="#667085">{formatNumber(min)} s</text>
      <text x="34" y="134" fontSize="9" fill="#667085">wave 1</text>
      <text x="278" y="134" fontSize="9" fill="#667085">wave {series.length}</text>
      <polyline fill="none" stroke="#b42318" strokeWidth="2" points={points((wave) => wave.static_mean_s)} />
      <polyline fill="none" stroke="#067647" strokeWidth="2" points={points((wave) => wave.dynamic_mean_s)} />
    </svg>
    <p className="help-text">
      <span style={{ color: '#b42318' }}>— static</span>{' '}
      <span style={{ color: '#067647' }}>— dynamic</span>{' '}
      · mean seconds per trip, wave by wave. The first wave is the same for both arms:
      the dynamic engine has nothing to report yet.
    </p>
  </div>
}
