import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { ApiError } from '../api/client'

/**
 * The three seeded scenario modes. They live here rather than in one screen
 * because both the pages that start a run and the pages that report one name the
 * same modes, and a mode should have one name.
 */
export const presets = {
  light: { label: 'Light shift', vehicles: 20, departure_window_s: 300 },
  peak: { label: 'Peak shift', vehicles: 50, departure_window_s: 900 },
  heavy: { label: 'High demand', vehicles: 100, departure_window_s: 1800 },
} as const

/**
 * runLabel names a scenario run the way a person chose it.
 *
 * A seeded preset keeps a readable id, and showing the mode name is both clearer
 * and shorter than truncating that id to fit a column; a run someone started is
 * named by its id, because there is nothing else to call it.
 */
export function runLabel(id: string) {
  const mode = id.startsWith('sim_preset_') ? id.slice('sim_preset_'.length) : ''
  return mode in presets ? presets[mode as keyof typeof presets].label : id
}

export function Page({ title, description, eyebrow, action, children }: {
  title: string
  description: string
  eyebrow: string
  action?: ReactNode
  children: ReactNode
}) {
  return <>
    <header className="page-heading">
      <div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{description}</p></div>
      {action}
    </header>
    <div className="page-content">{children}</div>
  </>
}

export function Panel({ title, action, children, className = '' }: {
  title: string
  action?: ReactNode
  children: ReactNode
  className?: string
}) {
  return <section className={`panel ${className}`}>
    <div className="panel-heading"><h2>{title}</h2>{action}</div>
    {children}
  </section>
}

export function Metric({ label, value, note }: { label: string; value: string; note?: string }) {
  return <div className="metric-card">
    <div className="metric-label">{label}</div><div className="metric-value">{value}</div>
    {note && <div className="metric-note">{note}</div>}
  </div>
}

export function useLoad<T>(load: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const reload = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setData(await load())
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setLoading(false)
    }
  // Callers pass stable API methods or intentionally include changing ids.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)
  useEffect(() => { void reload() }, [reload])
  return { data, error, loading, reload, setData }
}

export function errorText(cause: unknown): string {
  if (cause instanceof ApiError) {
    if (cause.status === 401) return 'Authentication is required for this action.'
    if (cause.status === 404) return 'This feature is not available in this server configuration.'
    return `${cause.code ? `${cause.code}: ` : ''}${cause.message}`
  }
  return cause instanceof Error ? cause.message : 'An unexpected error occurred.'
}

export function ErrorMessage({ children }: { children: ReactNode }) {
  return <div className="error" role="alert">{children}</div>
}

export function Loading({ label = 'Loading…' }: { label?: string }) {
  return <div className="empty-state">{label}</div>
}

export function Table({ headers, children }: { headers: string[]; children: ReactNode }) {
  return <div className="table-wrap"><table><thead><tr>{headers.map((header) =>
    <th key={header}>{header}</th>)}</tr></thead><tbody>{children}</tbody></table></div>
}

export function Status({ value }: { value: string }) {
  return <span className={`badge ${value === 'offline' || value === 'completed' ? 'neutral' : ''}`}>{value.replaceAll('_', ' ')}</span>
}

export function formatNumber(value: number | undefined, digits = 1) {
  return value == null || !Number.isFinite(value) ? '—' : value.toLocaleString(undefined, { maximumFractionDigits: digits })
}
