import { useState } from 'react'
import { api, type EmissionsResponse } from '../api/client'
import { ErrorMessage, formatNumber, Loading, Metric, Page, Panel, useLoad } from './Shared'

export function EmissionsPage() {
  const data = useLoad<EmissionsResponse>(() => api.emissions())
  const [distance, setDistance] = useState(12)
  const [speed, setSpeed] = useState(28)
  const [vehicleClass, setVehicleClass] = useState('passenger_car')
  const [estimate, setEstimate] = useState<EmissionsResponse['trip']>()
  const [error, setError] = useState('')
  const calculate = async (event: React.FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      const result = await api.emissions(`?class=${encodeURIComponent(vehicleClass)}&distance=${distance}&speed=${speed}`)
      setEstimate(result.trip)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not calculate estimate.')
    }
  }
  const run = data.data?.run
  const saving = Number((run?.savings.co2_g ?? 0).toFixed(1))
  const intensityDifference = Number((run?.savings.co2_per_km ?? 0).toFixed(2))
  const savingsDescription = saving === 0
    ? 'No estimated CO₂ difference at displayed precision'
    : saving > 0
    ? `${formatNumber(saving)} g less estimated CO₂ in the FleetFlow arm`
    : `${formatNumber(Math.abs(saving))} g more estimated CO₂ in the FleetFlow arm`

  return <Page eyebrow="Analytics" title="Emissions" description="Estimated exhaust emissions for the paired simulation and a single trip.">
    {data.loading && <Loading />}
    {data.error && <ErrorMessage>{data.error}</ErrorMessage>}
    {run && <div className="emission-comparison">
      <Panel title="Baseline · static shortest path">
        <div className="emission-primary">{formatNumber(run.baseline.co2_g)}<small>g CO₂</small></div>
        <div className="emission-sub">{formatNumber(run.baseline.co2_per_km, 2)} g/km · {formatNumber(run.baseline.distance_km)} km</div>
        <div className="emission-sub">{formatNumber(run.baseline.fuel_litres, 2)} L estimated fuel</div>
      </Panel>
      <Panel title="FleetFlow · dynamic weights">
        <div className="emission-primary">{formatNumber(run.fleet_flow.co2_g)}<small>g CO₂</small></div>
        <div className="emission-sub">{formatNumber(run.fleet_flow.co2_per_km, 2)} g/km · {formatNumber(run.fleet_flow.distance_km)} km</div>
        <div className="emission-sub">{formatNumber(run.fleet_flow.fuel_litres, 2)} L estimated fuel</div>
      </Panel>
      <section className={`savings-card ${saving >= 0 ? 'positive' : 'negative'}`}>
        <span>CO₂ savings · baseline minus FleetFlow</span><strong>{saving === 0 ? '' : saving > 0 ? '+' : '−'}{formatNumber(Math.abs(saving))} g</strong>
        <small>{savingsDescription} · {formatNumber(intensityDifference, 2)} g/km intensity difference</small>
      </section>
    </div>}
    {!data.loading && !data.error && !run && <div className="notice">Start a comparison run to see the two arms side by side. The calculator below is available now.</div>}
    <Panel title="Estimate a trip">
      <form className="form-row" onSubmit={calculate}>
        <label className="field">Vehicle class<select value={vehicleClass} onChange={(e) => setVehicleClass(e.target.value)}>
          {(data.data?.classes ?? ['passenger_car', 'light_commercial']).map((name) => <option key={name} value={name}>{name.replace('_', ' ')}</option>)}
        </select></label>
        <label className="field">Distance (km)<input type="number" min="0" step="0.1" required value={distance} onChange={(e) => setDistance(Number(e.target.value))} /></label>
        <label className="field">Mean speed (km/h)<input type="number" min="0" step="1" required value={speed} onChange={(e) => setSpeed(Number(e.target.value))} /></label>
        <button className="button primary" type="submit">Calculate</button>
      </form>
      {error && <ErrorMessage>{error}</ErrorMessage>}
      {estimate && <div className="metric-grid estimate-results">
        <Metric label="CO₂" value={`${formatNumber(estimate.co2_g)} g`} note={`${formatNumber(estimate.co2_per_km, 2)} g/km`} />
        <Metric label="NOx" value={`${formatNumber(estimate.nox_g, 2)} g`} />
        <Metric label="CO" value={`${formatNumber(estimate.co_g, 2)} g`} />
        <Metric label="Fuel" value={`${formatNumber(estimate.fuel_litres, 3)} L`} />
      </div>}
    </Panel>
    <Panel title="How the estimate works">
      <p className="method-copy">For each completed simulated trip, FleetFlow estimates mean speed from that trip’s actual route distance and travel time. It evaluates a speed-based COPERT-style emission-factor curve for each trip separately, then sums the results for each comparison arm. The curve is non-linear, so averaging trips first would distort the estimate.</p>
      <p className="method-copy">The model includes a reference vehicle class and fuel; it does not model cold starts, road gradient, payload, ambient temperature, or actual measured tailpipe output. The speed fit is clamped to 10–130 km/h. These values are simulation estimates, not real-world measurements.</p>
      <div className="notice">{data.data?.method ?? 'COPERT-style speed curve; estimated outputs only.'}</div>
    </Panel>
  </Page>
}
