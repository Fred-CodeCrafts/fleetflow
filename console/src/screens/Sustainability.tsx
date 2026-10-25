import { Link } from 'react-router-dom'
import { api, type EmissionsSummary, type Vehicle } from '../api/client'
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

function provenanceNote(summary: EmissionsSummary | null) {
  const method = summary?.method
  if (!method) {
    return 'No completed scenario has been recorded yet, so there is no emission model to name.'
  }
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

      <Panel title="Vehicle attribution">
        {!data?.vehicles.length
          ? <p className="empty-state">No completed scenario has attributed emissions to a vehicle yet.</p>
          : <Table headers={['Vehicle', 'Trips', 'Distance', 'CO₂', 'Fuel', 'CO₂ intensity']}>
            {data.vehicles.map((vehicle) => <tr key={vehicle.vehicle_id}>
              <td>{vehicleCell(vehicle.vehicle_id, vehicles.data ?? [])}</td>
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
        <p className="help-text">{provenanceNote(data ?? null)}</p>
        <p className="help-text">
          These are model estimates for simulated scenarios, not measured fleet emissions and not a verified carbon saving.
        </p>
      </Panel>
    </>}
  </Page>
}
