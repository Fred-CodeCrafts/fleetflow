import React, { Component, type ErrorInfo, type ReactNode } from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './shell/Layout'
import { allNavItems, navigation } from './shell/navigation'
import { Screen } from './screens/Screen'
import { OperationalTripPage, SimulationTripPage } from './screens/Simulation'
import { DriverDetailPage, VehicleDetailPage } from './screens/Records'
import './styles.css'

interface ErrorBoundaryProps {
  children: ReactNode
}

interface ErrorBoundaryState {
  hasError: boolean
  error: Error | null
}

class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Uncaught error in console UI:', error, errorInfo)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div style={{ padding: '40px', maxWidth: '600px', margin: '40px auto', background: '#fff', border: '1px solid #dee2e6', borderRadius: '8px' }}>
          <h2 style={{ marginTop: 0, color: '#fa5252' }}>Console Error</h2>
          <p>{this.state.error?.message || 'An unexpected error occurred while rendering this page.'}</p>
          <div style={{ display: 'flex', gap: '8px' }}>
            <button className="button primary" onClick={() => window.location.reload()}>
              Reload Console
            </button>
            <a className="button" href="/operations/trips">
              Return to Trips
            </a>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route index element={<Navigate to="/operations" replace />} />
            {navigation.flatMap((branch) => branch.children.map((item) => (
              <Route key={item.id} path={item.path} element={<Screen id={item.id} />} />
            )))}
            <Route path="/operations/trips/operational/:tripID" element={<OperationalTripPage />} />
            <Route path="/operations/trips/:runID/:tripID" element={<SimulationTripPage />} />
            <Route path="/operations/trips/:tripID" element={<OperationalTripPage />} />
            <Route path="/resources/vehicles/:vehicleID" element={<VehicleDetailPage />} />
            <Route path="/resources/drivers/:driverID" element={<DriverDetailPage />} />
            <Route path="/resources/fleet" element={<Navigate to="/operations/fleet" replace />} />
            <Route path="/sustainability/emissions" element={<Navigate to="/sustainability" replace />} />
            <Route path="/trips" element={<Navigate to="/operations/trips" replace />} />
            <Route path="/trips/*" element={<Navigate to="/operations/trips" replace />} />
            {navigation.filter((branch) => !allNavItems().some((item) => item.path === branch.path)).map((branch) => (
              <Route key={branch.id} path={branch.path} element={<Navigate to={branch.children[0]?.path ?? '/operations'} replace />} />
            ))}
            <Route path="*" element={<Navigate to="/operations" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ErrorBoundary>
  </React.StrictMode>,
)
