import { useEffect, useMemo, useState } from 'react'
import { NavLink, Outlet, useLocation, useNavigate, useNavigationType } from 'react-router-dom'
import { allNavItems, locate, navigation, searchNav, visibleBranches } from './navigation'

export function Layout() {
  const location = useLocation()
  const navigate = useNavigate()
  const navigationType = useNavigationType()
  const [query, setQuery] = useState('')
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [apiOnline, setApiOnline] = useState<boolean | null>(null)
  const active = locate(location.pathname)
  const results = useMemo(() => searchNav(query), [query])
  const branches = visibleBranches(() => true)

  useEffect(() => {
    const check = () => fetch('/api/healthz').then((r) => setApiOnline(r.ok)).catch(() => setApiOnline(false))
    check()
    const timer = window.setInterval(check, 15000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setPaletteOpen((open) => !open)
      }
      if (event.key === 'Escape') setPaletteOpen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  useEffect(() => {
    setPaletteOpen(false)
  }, [location.pathname])

  // A route change keeps the window's scroll offset, so a page opened after
  // scrolling a long table started halfway down. A newly opened page starts at
  // the top; a back or forward step is left alone, because the browser restores
  // the offset it remembers for that history entry.
  useEffect(() => {
    if (navigationType === 'POP') return
    window.scrollTo(0, 0)
  }, [location.pathname, navigationType])

  const go = (path: string) => {
    navigate(path)
    setPaletteOpen(false)
    setQuery('')
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <NavLink to="/operations" className="brand">
          <span className="brand-mark">F</span>
          <span>FleetFlow<small>FLEET OPERATIONS</small></span>
        </NavLink>
        <div className="sidebar-scroll">
          {branches.map((branch) => (
            <section className="nav-section" key={branch.id}>
              <div className="nav-heading">{branch.label}</div>
              {branch.children.map((item) => (
                <NavLink className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}
                  to={item.path} key={item.id} end={item.path === '/operations'}>
                  <span className="nav-glyph" aria-hidden="true">{item.label.slice(0, 1)}</span>
                  <span>{item.label}</span>
                </NavLink>
              ))}
            </section>
          ))}
        </div>
        <div className="sidebar-footer">
          <span className={`status-dot ${apiOnline === false ? 'offline' : ''}`} />
          <span>{apiOnline === null ? 'Connecting' : apiOnline ? 'API connected' : 'API unavailable'}</span>
          <span className="version-tag">MVP</span>
        </div>
      </aside>

      <main className="main-column">
        <header className="topbar">
          <div className="breadcrumbs">
            <span>{active?.branch.label ?? 'FleetFlow'}</span>
            {active && <><span className="crumb-separator">/</span><strong>{active.item.label}</strong></>}
          </div>
          <div className="top-actions">
            <button className="palette-trigger" onClick={() => setPaletteOpen(true)}>
              <span>Search pages and actions</span><kbd>Ctrl K</kbd>
            </button>
            <button className="avatar" title="FleetFlow operator">FO</button>
          </div>
        </header>
        <div className="page-viewport"><Outlet /></div>
      </main>

      {paletteOpen && (
        <div className="palette-backdrop" role="presentation" onMouseDown={(event) => {
          if (event.target === event.currentTarget) setPaletteOpen(false)
        }}>
          <section className="command-palette" role="dialog" aria-modal="true" aria-label="Navigate">
            <div className="palette-input">
              <span aria-hidden="true">⌕</span>
              <input autoFocus value={query} onChange={(event) => setQuery(event.target.value)}
                placeholder="Search FleetFlow..." />
              <kbd>ESC</kbd>
            </div>
            <div className="palette-results">
              {(query ? results : allNavItems(navigation).slice(0, 7)).map((item) => (
                <button key={item.id} onClick={() => go(item.path)}>
                  <span className="nav-glyph">{item.label.slice(0, 1)}</span>
                  <span>{item.label}</span><small>{item.path}</small>
                </button>
              ))}
              {query && results.length === 0 && <p className="empty-state">No pages match “{query}”.</p>}
            </div>
          </section>
        </div>
      )}
    </div>
  )
}
