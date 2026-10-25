import { useState, type FormEvent } from 'react'
import { ErrorMessage, Page, Panel, Table } from './Shared'

export interface VehicleModel {
  id: string
  name: string
  type: 'van' | 'truck' | 'car' | 'motorcycle'
  body_type: string
  fuel_type: string
  capacity_kg: number
}

export const defaultModels: VehicleModel[] = [
  { id: 'gran-max', name: 'Daihatsu Gran Max Blind Van', type: 'van', body_type: 'Blind Van', fuel_type: 'Petrol', capacity_kg: 750 },
  { id: 'elf-box', name: 'Isuzu Elf Box 4-Wheel', type: 'truck', body_type: 'Box', fuel_type: 'Diesel', capacity_kg: 2500 },
  { id: 'hilux-pickup', name: 'Toyota Hilux Single Cabin', type: 'truck', body_type: 'Pickup', fuel_type: 'Diesel', capacity_kg: 1000 },
  { id: 'l300-box', name: 'Mitsubishi Colt L300 Box', type: 'van', body_type: 'Box', fuel_type: 'Diesel', capacity_kg: 1200 },
  { id: 'beat-delivery', name: 'Honda Beat Street Delivery', type: 'motorcycle', body_type: 'Motorcycle Cargo', fuel_type: 'Petrol', capacity_kg: 30 },
  { id: 'formo-van', name: 'Wuling Formo Blind Van', type: 'van', body_type: 'Blind Van', fuel_type: 'Petrol', capacity_kg: 650 },
  { id: 'dutro-130', name: 'Hino Dutro 130 HD Cargo', type: 'truck', body_type: 'Box', fuel_type: 'Diesel', capacity_kg: 4500 },
  { id: 'gesits-electric', name: 'Gesits Electric Scooter', type: 'motorcycle', body_type: 'Motorcycle Cargo', fuel_type: 'Electric', capacity_kg: 25 },
]

const STORAGE_KEY = 'fleetflow_vehicle_models'

export function getVehicleModels(): VehicleModel[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as VehicleModel[]
      if (Array.isArray(parsed) && parsed.length > 0) return parsed
    }
  } catch {
    // Fallback to default models if localStorage is unavailable
  }
  return defaultModels
}

export function saveVehicleModels(models: VehicleModel[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(models))
  } catch {
    // Ignore storage failure
  }
}

export function ModelsPage() {
  const [models, setModels] = useState<VehicleModel[]>(getVehicleModels)
  const [name, setName] = useState('')
  const [type, setType] = useState<'van' | 'truck' | 'car' | 'motorcycle'>('van')
  const [bodyType, setBodyType] = useState('Blind Van')
  const [fuelType, setFuelType] = useState('Petrol')
  const [capacityKg, setCapacityKg] = useState('750')
  const [error, setError] = useState('')

  const handleCreate = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    const trimmed = name.trim()
    if (!trimmed) {
      setError('Model name is required.')
      return
    }
    const cap = Number(capacityKg)
    if (isNaN(cap) || cap < 0) {
      setError('Capacity must be a positive number in kg.')
      return
    }
    const newModel: VehicleModel = {
      id: `model-${Date.now()}`,
      name: trimmed,
      type,
      body_type: bodyType.trim() || 'Standard',
      fuel_type: fuelType.trim() || 'Petrol',
      capacity_kg: cap,
    }
    const updated = [...models, newModel]
    setModels(updated)
    saveVehicleModels(updated)
    setName('')
    setCapacityKg('750')
  }

  const handleDelete = (id: string) => {
    const updated = models.filter((m) => m.id !== id)
    setModels(updated)
    saveVehicleModels(updated)
  }

  const handleReset = () => {
    setModels(defaultModels)
    saveVehicleModels(defaultModels)
  }

  return (
    <Page
      eyebrow="Resources"
      title="Vehicle Models"
      description="Manage vehicle model specifications, body configurations, fuel types, and cargo payload capacities."
      action={
        <button className="button" type="button" onClick={handleReset}>
          Reset to default models
        </button>
      }
    >
      <Panel title="Add vehicle model">
        <form className="form-row" onSubmit={handleCreate}>
          <label className="field">
            Model name
            <input
              required
              placeholder="e.g. Daihatsu Gran Max Blind Van"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          <label className="field">
            Vehicle type
            <select value={type} onChange={(e) => setType(e.target.value as 'van' | 'truck' | 'car' | 'motorcycle')}>
              <option value="van">Van</option>
              <option value="truck">Truck</option>
              <option value="car">Car</option>
              <option value="motorcycle">Motorcycle</option>
            </select>
          </label>
          <label className="field">
            Body type
            <input
              required
              placeholder="e.g. Blind Van, Box, Pickup"
              value={bodyType}
              onChange={(e) => setBodyType(e.target.value)}
            />
          </label>
          <label className="field">
            Fuel type
            <select value={fuelType} onChange={(e) => setFuelType(e.target.value)}>
              <option value="Petrol">Petrol</option>
              <option value="Diesel">Diesel</option>
              <option value="Electric">Electric</option>
              <option value="Hybrid">Hybrid</option>
              <option value="CNG">CNG</option>
            </select>
          </label>
          <label className="field">
            Cargo capacity (kg)
            <input
              type="number"
              min="0"
              step="any"
              required
              placeholder="Payload in kg"
              value={capacityKg}
              onChange={(e) => setCapacityKg(e.target.value)}
            />
          </label>
          <button className="button primary" type="submit">
            Add model
          </button>
        </form>
        <p className="help-text">
          Registered models appear in the Model dropdown when adding new vehicles. Specifications defined here automatically populate vehicle body and fuel configurations.
        </p>
        {error && <ErrorMessage>{error}</ErrorMessage>}
      </Panel>

      <Panel title="Registered models">
        <Table headers={['Model name', 'Vehicle type', 'Body type', 'Fuel type', 'Cargo capacity (kg)', 'Action']}>
          {models.map((m) => (
            <tr key={m.id}>
              <td><strong>{m.name}</strong></td>
              <td>{m.type}</td>
              <td>{m.body_type}</td>
              <td>{m.fuel_type}</td>
              <td>{m.capacity_kg} kg</td>
              <td>
                <button
                  className="button"
                  type="button"
                  onClick={() => handleDelete(m.id)}
                  title="Remove this model"
                >
                  Delete
                </button>
              </td>
            </tr>
          ))}
        </Table>
      </Panel>
    </Page>
  )
}
