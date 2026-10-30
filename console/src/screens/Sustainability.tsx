import { Link } from 'react-router-dom'
import { api, type EmissionProvenance, type Vehicle } from '../api/client'
import { ErrorMessage, Loading, Metric, Page, Panel, runLabel, Table, formatNumber, useLoad } from './Shared'

/**
 * One sustainability page.
 *
 * The green dashboard and the emissions ledger read the same summary and answer
 * the same question at two resolutions: the fleet total, then the run and the
 * vehicle behind it. Split into two pages they repeated the per-vehicle table and
 * made a reader navigate to see the rows behind the number they were looking at.
 * This page goes from the total down to the attribution in one scroll, so the
 * claims and the evidence are never on different screens.
 */

function provenanceNote(method: EmissionProvenance) {
  return `${method.model} · ${method.class} · ${method.source}. Values are ${method.output_unit} converted from ${method.input_unit}. ${method.assumptions}`
}

/**
 * vehicleCell attributes an emission total to a vehicle.
 *
 * A scenario names its vehicles by plate, and the plate is the registration, so
 * the fleet is matched on that and the row links to the vehicle it describes
 * rather than to the register in general. A plate the fleet does not know is
 * still shown, because hiding the attribution would be worse than an unknown one.
 */
function vehicleCell(plate: string, vehicles: Vehicle[]) {
  const vehicle = vehicles.find((candidate) => candidate.registration === plate)
  if (!vehicle) {
    return plate
  }
  return <Link to={`/resources/vehicles/${encodeURIComponent(vehicle.id)}`}>
    {vehicle.name} · {plate}
  </Link>
}

export function SustainabilityPage() {
  const summary = useLoad(api.emissionsSummary)
  const vehicles = useLoad(api.vehicles)
  const data = summary.data
  const methods = data?.methods ?? (data?.method ? [data.method] : [])

  // The van trips that could move to the electric cargo motorcycle today: same
  // distance, same stops, no tailpipe CO2. This is arithmetic on trips that were
  // already recorded, using the factors printed on this page, not a projection.
  const vans = (data?.vehicles ?? []).filter((vehicle) =>
    vehicle.class_key === 'diesel-van' || vehicle.class_key === 'petrol-van')
  const vanCO2 = vans.reduce((total, vehicle) => total + vehicle.co2_kg, 0)
  const vanShare = (data?.totals.co2_kg ?? 0) > 0 ? (vanCO2 / data!.totals.co2_kg) * 100 : 0

  return <Page
    eyebrow="Sustainability"
    title="Green Dashboard"
    description="Fleet-wide fuel and CO₂ from completed Jakarta scenarios, the vehicles that emit the most, and the runs the numbers come from."
  >
    {summary.loading ? <Loading /> : summary.error ? <ErrorMessage>{summary.error}</ErrorMessage> : <>
      <div className="metric-grid">
        <Metric label="CO₂ emitted" value={`${formatNumber(data?.totals.co2_kg, 1)} kg`} note="Model output" />
        <Metric label="Fuel burned" value={`${formatNumber(data?.totals.fuel_kg, 1)} kg`} note="Model output" />
        <Metric label="Distance driven" value={`${formatNumber(data?.totals.distance_km, 1)} km`} />
        <Metric label="CO₂ intensity" value={`${formatNumber(data?.totals.g_co2_per_km, 1)} g/km`} note="Per kilometre driven" />
        <Metric label="Simulated trips" value={formatNumber(data?.totals.trips, 0)} />
        <Metric label="Scenarios counted" value={formatNumber(data?.totals.runs, 0)} note="Completed runs only" />
      </div>

      <Panel title="Electrification headroom">
        {!vans.length
          ? <p className="empty-state">No van trips are recorded yet, so there is nothing to compare.</p>
          : <>
            <div className="metric-grid">
              <Metric label="Van trips recorded" value={formatNumber(vans.reduce((total, vehicle) => total + vehicle.trips, 0), 0)} note="Diesel and petrol vans" />
              <Metric label="Their CO₂ today" value={`${formatNumber(vanCO2, 1)} kg`} note="Model output" />
              <Metric label="Avoidable at the tailpipe" value={`${formatNumber(vanCO2, 1)} kg`} note={`${formatNumber(vanShare, 1)}% of fleet CO₂`} />
            </div>
            <p className="help-text">
              If every recorded van kilometre had been ridden by the electric cargo motorcycle instead,
              tailpipe CO₂ would fall by the figure above: the electric class burns nothing at the pipe.
              Treat it as an upper bound, not a plan. A van carries 0.8 to 1.2 tonnes and the motorcycle
              about 60 kg, so replacing a full van load takes many motorcycle runs, and those extra
              kilometres are not in this arithmetic; grid energy is not counted either. What is counted is
              the distance already in the ledger, charged at the class factors printed above.
            </p>
          </>}
      </Panel>

      <Panel title="Vehicle attribution">
        {!data?.vehicles.length
          ? <p className="empty-state">No completed scenario has attributed emissions to a vehicle yet.</p>
          : <Table headers={['Vehicle', 'Class', 'Trips', 'Distance', 'CO₂', 'Fuel', 'CO₂ intensity']}>
            {data.vehicles.map((vehicle) => <tr key={vehicle.vehicle_id}>
              <td>{vehicleCell(vehicle.vehicle_id, vehicles.data ?? [])}</td>
              <td>{vehicle.class ?? '—'}</td>
              <td>{vehicle.trips}</td>
              <td>{formatNumber(vehicle.distance_km, 1)} km</td>
              <td>{formatNumber(vehicle.co2_kg, 2)} kg</td>
              <td>{formatNumber(vehicle.fuel_kg, 2)} kg</td>
              <td>{formatNumber(vehicle.g_co2_per_km, 1)} g/km</td>
            </tr>)}
          </Table>}
      </Panel>

      <Panel
        title="Run ledger"
        action={<Link className="button" to="/operations/simulations">Start or inspect a scenario</Link>}
      >
        {!data?.runs.length
          ? <p className="empty-state">No completed scenario has been recorded yet.</p>
          : <Table headers={['Run', 'Recorded', 'Vehicles', 'Trips', 'Distance', 'CO₂', 'Fuel', 'CO₂ intensity', 'Open']}>
            {data.runs.map((run) => <tr key={run.run_id}>
              <td><strong>{runLabel(run.run_id)}</strong></td>
              <td>{new Date(run.created_at).toLocaleString()}</td>
              <td>{run.vehicles}</td>
              <td>{run.trips}</td>
              <td>{formatNumber(run.distance_km, 1)} km</td>
              <td>{formatNumber(run.co2_kg, 2)} kg</td>
              <td>{formatNumber(run.fuel_kg, 2)} kg</td>
              <td>{formatNumber(run.g_co2_per_km, 1)} g/km</td>
              <td><Link to={`/operations/trips?run=${encodeURIComponent(run.run_id)}`}>Simulation trips</Link></td>
            </tr>)}
          </Table>}
      </Panel>

      <Panel title="What these numbers are">
        {methods.length === 0
          ? <p className="help-text">{data?.totals.trips
            ? 'Emission totals are available, but their model provenance was not recorded.'
            : 'No completed scenario has recorded emissions yet, so there is no emission model to name.'}</p>
          : <>
            {methods.length > 1 && <p className="help-text">These totals combine estimates from multiple methods:</p>}
            {methods.map((method) => <p className="help-text" key={`${method.model}:${method.class}:${method.source}`}>
              {provenanceNote(method)}
            </p>)}
          </>}
        <p className="help-text">
          These are model estimates for simulated scenarios, not measured fleet emissions and not a verified carbon saving.
        </p>
      </Panel>
    </>}
  </Page>
}
