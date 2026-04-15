import { useEffect, useState, type ReactNode } from 'react'
import { api } from '../api'

type Page = 'pods' | 'forecasts' | 'scaling-history' | 'dry-run' | 'manual-scale' | 'resource-chart'

interface Props {
  page: Page
  onNavigate: (p: Page) => void
  children: ReactNode
}

const NAV_ITEMS: { id: Page; label: string; icon: string }[] = [
  { id: 'pods',           label: 'Pods',            icon: '⬡' },
  { id: 'forecasts',      label: 'CPU Forecasts',   icon: '◈' },
  { id: 'scaling-history',label: 'Scaling History', icon: '≡' },
  { id: 'dry-run',        label: 'Dry-Run Queue',   icon: '⏸' },
  { id: 'manual-scale',   label: 'Manual Scale',    icon: '↑' },
  { id: 'resource-chart', label: 'Resource Usage',  icon: '◎' },
]

export function Layout({ page, onNavigate, children }: Props) {
  const [connected, setConnected] = useState<boolean | null>(null)

  useEffect(() => {
    const check = () => api.health().then(setConnected).catch(() => setConnected(false))
    check()
    const t = setInterval(check, 15_000)
    return () => clearInterval(t)
  }, [])

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      {/* Sidebar */}
      <aside style={{
        width: 220,
        flexShrink: 0,
        background: '#0d1117',
        borderRight: '1px solid #1e2535',
        display: 'flex',
        flexDirection: 'column',
        padding: '20px 0',
      }}>
        <div style={{ padding: '0 20px 24px', borderBottom: '1px solid #1e2535' }}>
          <div style={{ fontSize: 11, fontWeight: 700, color: '#7c3aed', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
            Intelligent
          </div>
          <div style={{ fontSize: 15, fontWeight: 700, color: '#f1f5f9', marginTop: 2, lineHeight: 1.3 }}>
            Cluster Optimizer
          </div>
        </div>

        <nav style={{ marginTop: 12, flex: 1 }}>
          {NAV_ITEMS.map(item => (
            <button
              key={item.id}
              onClick={() => onNavigate(item.id)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                width: '100%',
                padding: '10px 20px',
                background: page === item.id ? '#1e2535' : 'transparent',
                border: 'none',
                borderLeft: `3px solid ${page === item.id ? '#7c3aed' : 'transparent'}`,
                color: page === item.id ? '#f1f5f9' : '#64748b',
                fontWeight: page === item.id ? 600 : 400,
                textAlign: 'left',
                transition: 'all 0.15s',
              }}
            >
              <span style={{ fontSize: 16 }}>{item.icon}</span>
              {item.label}
            </button>
          ))}
        </nav>
      </aside>

      {/* Main content */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {/* Top bar */}
        <header style={{
          height: 56,
          flexShrink: 0,
          background: '#0d1117',
          borderBottom: '1px solid #1e2535',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 24px',
        }}>
          <div style={{ fontSize: 15, fontWeight: 600, color: '#f1f5f9' }}>
            {NAV_ITEMS.find(n => n.id === page)?.label}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{
              width: 8,
              height: 8,
              borderRadius: '50%',
              background: connected === true ? '#22c55e' : connected === false ? '#ef4444' : '#94a3b8',
            }} />
            <span style={{ fontSize: 13, color: '#94a3b8' }}>
              {connected === true ? 'Connected' : connected === false ? 'Disconnected' : 'Checking…'}
            </span>
          </div>
        </header>

        {/* Page content */}
        <main style={{ flex: 1, overflow: 'auto', padding: 24 }}>
          {children}
        </main>
      </div>
    </div>
  )
}
