import { Link } from 'react-router-dom'
import { api, type EmissionsSummary, type Vehicle } from '../api/client'
import { ErrorMessage, Loading, Metric, Page, Panel, runLabel, Table, formatNumber, useLoad } from './Shared'

/**
 * The two emissions screens read one endpoint and are meant to be used in a row:
 * the dashboard answers "how are we doing", the ledger answers "which run did
 * that". They therefore share the aggregation, the provenance note and the
 * cross-link, so neither can show a number the other disagrees with.
 */

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

function provenanceNote(summary: EmissionsSummary | null) {
  const method = summary?.method
  if (!method) {
    return 'No completed scenario has been recorded yet, so there is no emission model to name.'
  }
  return `${method.model} · ${method.class} · ${method.source}. Values are ${method.output_unit} converted from ${method.input_unit}. ${method.assumptions}`
}

function EmissionsProvenance({ summary }: { summary: EmissionsSummary | null }) {
  return <Panel title="What these numbers are">
    <p className="help-text">{provenanceNote(summary)}</p>
    <p className="help-text">
      These are model estimates for simulated scenarios, not measured fleet emissions and not a verified carbon saving.
    </p>
  </Panel>
}

export function GreenDashboardPage() {
  const summary = useLoad(api.emissionsSummary)
  const vehicles = useLoad(api.vehicles)
  const data = summary.data

  return <Page
    eyebrow="Sustainability"
    title="Green Dashboard"
    description="Fleet-wide fuel and CO₂ from completed Jakarta scenarios, with the vehicles that emit the most."
    action={<Link className="button" to="/sustainability/emissions">Open the emissions ledger</Link>}
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

      <Panel title="Heaviest emitting vehicles" action={<Link className="button" to="/sustainability/emissions">Per-run ledger</Link>}>
        {!data?.vehicles.length
          ? <p className="empty-state">No completed scenario has attributed emissions to a vehicle yet.</p>
          : <Table headers={['Vehicle', 'Trips', 'Distance', 'CO₂', 'Fuel', 'CO₂ intensity']}>
            {data.vehicles.slice(0, 15).map((vehicle) => <tr key={vehicle.vehicle_id}>
              <td>{vehicleCell(vehicle.vehicle_id, vehicles.data ?? [])}</td>
              <td>{vehicle.trips}</td>
              <td>{formatNumber(vehicle.distance_km, 1)} km</td>
              <td>{formatNumber(vehicle.co2_kg, 1)} kg</td>
              <td>{formatNumber(vehicle.fuel_kg, 1)} kg</td>
              <td>{formatNumber(vehicle.g_co2_per_km, 1)} g/km</td>
            </tr>)}
          </Table>}
      </Panel>

      <EmissionsProvenance summary={data ?? null} />
    </>}
  </Page>
}

export function EmissionsPage() {
  const summary = useLoad(api.emissionsSummary)
  const vehicles = useLoad(api.vehicles)
  const data = summary.data

  return <Page
    eyebrow="Sustainability"
    title="Emissions"
    description="One row per completed scenario: what it drove, what it burned, and what it emitted."
    action={<Link className="button" to="/sustainability">Back to the dashboard</Link>}
  >
    {summary.loading ? <Loading /> : summary.error ? <ErrorMessage>{summary.error}</ErrorMessage> : <>
      <div className="metric-grid">
        <Metric label="Scenarios counted" value={formatNumber(data?.totals.runs, 0)} />
        <Metric label="CO₂ emitted" value={`${formatNumber(data?.totals.co2_kg, 1)} kg`} note="Across every counted run" />
        <Metric label="CO₂ intensity" value={`${formatNumber(data?.totals.g_co2_per_km, 1)} g/km`} />
      </div>

      <Panel title="Run ledger" action={<Link className="button" to="/operations/simulations">Start or inspect a scenario</Link>}>
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

      <Panel title="Per-vehicle attribution">
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

      <EmissionsProvenance summary={data ?? null} />
    </>}
  </Page>
}
