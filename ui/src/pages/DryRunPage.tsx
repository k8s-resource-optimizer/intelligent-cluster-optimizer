import { useEffect, useState } from 'react'
import { api, type DryRunDecision, type PodInfo } from '../api'
import { toast } from '../components/Toast'

function ActionButton({
  label,
  variant,
  loading,
  onClick,
}: {
  label: string
  variant: 'approve' | 'reject'
  loading: boolean
  onClick: () => void
}) {
  const bg = variant === 'approve' ? '#166534' : '#7f1d1d'
  const hover = variant === 'approve' ? '#15803d' : '#991b1b'
  const color = variant === 'approve' ? '#bbf7d0' : '#fecaca'

  return (
    <button
      onClick={onClick}
      disabled={loading}
      style={{
        padding: '6px 14px',
        borderRadius: 5,
        border: 'none',
        background: loading ? '#1e2535' : bg,
        color: loading ? '#64748b' : color,
        fontWeight: 600,
        transition: 'background 0.15s',
      }}
      onMouseEnter={e => { if (!loading) (e.target as HTMLButtonElement).style.background = hover }}
      onMouseLeave={e => { if (!loading) (e.target as HTMLButtonElement).style.background = bg }}
    >
      {loading ? '…' : label}
    </button>
  )
}

function bytesToMi(bytes: number): string {
  return (bytes / (1024 * 1024)).toFixed(0) + 'Mi'
}

function UsageSummary({ decisions, pods, filter }: { decisions: DryRunDecision[], pods: PodInfo[], filter: string }) {
  const deployments = filter
    ? [filter]
    : [...new Set(decisions.map(d => d.deployment_name))]
  if (deployments.length === 0 || pods.length === 0) return null

  const rows = deployments.map(name => {
    const matching = pods.filter(p => p.name.startsWith(name))
    const cpuTotal = matching.reduce((s, p) => s + p.cpu_usage_millicores, 0)
    const memTotal = matching.reduce((s, p) => s + p.memory_usage_bytes, 0)
    return { name, cpu: cpuTotal, mem: memTotal, found: matching.length > 0 }
  }).filter(r => r.found)

  if (rows.length === 0) return null

  return (
    <div style={{
      background: '#0d1117',
      border: '1px solid #1e2535',
      borderRadius: 8,
      padding: '12px 16px',
      marginBottom: 16,
      display: 'flex',
      gap: 24,
      flexWrap: 'wrap',
      alignItems: 'center',
    }}>
      <span style={{ fontSize: 12, color: '#64748b', fontWeight: 600, textTransform: 'uppercase', letterSpacing: 1 }}>
        Live Usage
      </span>
      {rows.map(r => (
        <div key={r.name} style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <span style={{ fontFamily: 'monospace', fontSize: 13, color: '#f1f5f9', fontWeight: 600 }}>{r.name}</span>
          <span style={{ fontSize: 13, color: '#94a3b8' }}>
            CPU <strong style={{ color: '#60a5fa' }}>{r.cpu}m</strong>
          </span>
          <span style={{ fontSize: 13, color: '#94a3b8' }}>
            Mem <strong style={{ color: '#a78bfa' }}>{bytesToMi(r.mem)}</strong>
          </span>
        </div>
      ))}
    </div>
  )
}

type ResourceSel = { cpu: boolean; memory: boolean }

export function DryRunPage() {
  const [decisions, setDecisions] = useState<DryRunDecision[]>([])
  const [pods, setPods] = useState<PodInfo[]>([])
  const [filter, setFilter] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState<Record<string, 'approve' | 'reject' | null>>({})
  const [sel, setSel] = useState<Record<string, ResourceSel>>({})

  const load = () => {
    api.dryRunPending()
      .then(data => { setDecisions(data ?? []); setError(null) })
      .catch(e => setError(String(e)))
    api.pods().then(setPods).catch(() => {})
  }

  useEffect(() => {
    load()
    const t = setInterval(load, 30_000)
    return () => clearInterval(t)
  }, [])

  const getSel = (id: string): ResourceSel => sel[id] ?? { cpu: true, memory: true }

  const toggleSel = (id: string, field: 'cpu' | 'memory') => {
    setSel(prev => {
      const cur = prev[id] ?? { cpu: true, memory: true }
      return { ...prev, [id]: { ...cur, [field]: !cur[field] } }
    })
  }

  const handle = async (id: string, action: 'approve' | 'reject') => {
    setLoading(prev => ({ ...prev, [id]: action }))
    try {
      if (action === 'approve') {
        const { cpu, memory } = getSel(id)
        await api.approve(id, cpu, memory)
        toast(`Approved and applied scaling decision`, 'success')
      } else {
        await api.reject(id)
        toast(`Scaling decision rejected`, 'info')
      }
    } catch (e) {
      toast(`Failed to ${action}: ${e}`, 'error')
    } finally {
      setLoading(prev => ({ ...prev, [id]: null }))
      load()
    }
  }

  const sorted = [...decisions].sort(
    (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  )
  const filtered = filter
    ? sorted.filter(d => d.deployment_name === filter)
    : sorted

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <h2 style={{ margin: 0 }}>Dry-Run Queue</h2>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <select
            value={filter}
            onChange={e => setFilter(e.target.value)}
            style={{
              padding: '5px 10px',
              borderRadius: 5,
              border: '1px solid #1e2535',
              background: '#0d1117',
              color: filter ? '#f1f5f9' : '#64748b',
              fontSize: 13,
              outline: 'none',
              width: 180,
              cursor: 'pointer',
            }}
          >
            <option value="">All apps</option>
            {[...new Set(decisions.map(d => d.deployment_name))].sort().map(name => (
              <option key={name} value={name}>{name}</option>
            ))}
          </select>
          <button
            onClick={load}
            style={{
              padding: '6px 14px',
              borderRadius: 5,
              border: '1px solid #1e2535',
              background: 'transparent',
              color: '#94a3b8',
              fontSize: 12,
            }}
          >
            Refresh
          </button>
        </div>
      </div>

      <UsageSummary decisions={decisions} pods={pods} filter={filter} />

      {error && (
        <div style={{ color: '#fca5a5', background: '#450a0a', padding: '10px 14px', borderRadius: 6, marginBottom: 16, fontSize: 13 }}>
          {error}
        </div>
      )}

      {filtered.length === 0 && !error ? (
        <div style={{ color: '#64748b', textAlign: 'center', padding: 48 }}>
          {decisions.length > 0 ? 'No matches for filter' : 'No pending dry-run decisions'}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {filtered.map(d => (
            <div key={d.id} style={{
              background: '#0d1117',
              border: `1px solid ${d.scaling_type === 'vertical' ? '#1e3a2f' : '#1e2535'}`,
              borderRadius: 8,
              padding: 20,
            }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
                <div style={{ flex: 1 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
                    <span style={{ fontFamily: 'monospace', fontSize: 14, fontWeight: 600, color: '#f1f5f9' }}>
                      {d.deployment_name}
                      {d.container_name && <span style={{ color: '#64748b', fontWeight: 400 }}> / {d.container_name}</span>}
                    </span>
                    <span style={{ fontSize: 12, color: '#64748b' }}>{d.namespace}</span>
                    <span style={{
                      padding: '2px 8px', borderRadius: 4, fontSize: 11, fontWeight: 600,
                      background: d.scaling_type === 'vertical' ? '#14532d44' : '#7c3aed22',
                      color: d.scaling_type === 'vertical' ? '#4ade80' : '#a78bfa',
                    }}>
                      {d.scaling_type === 'vertical' ? 'Vertical' : 'Horizontal'}
                    </span>
                    <span style={{
                      padding: '2px 8px', borderRadius: 4, fontSize: 11,
                      background: '#1e2535', color: '#94a3b8',
                    }}>
                      {d.reason}
                    </span>
                  </div>

                  {d.scaling_type === 'vertical' ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', alignItems: 'center' }}>
                        {d.current_cpu && (
                          <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13, color: '#94a3b8' }}>
                            <input
                              type="checkbox"
                              checked={getSel(d.id).cpu}
                              onChange={() => toggleSel(d.id, 'cpu')}
                              style={{ accentColor: '#4ade80', width: 14, height: 14 }}
                            />
                            CPU: <strong style={{ color: getSel(d.id).cpu ? '#f1f5f9' : '#475569' }}>{d.current_cpu}</strong>
                            <span style={{ color: '#475569' }}>→</span>
                            <strong style={{ color: getSel(d.id).cpu ? '#4ade80' : '#475569' }}>{d.recommended_cpu}</strong>
                          </label>
                        )}
                        {d.current_memory && (
                          <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13, color: '#94a3b8' }}>
                            <input
                              type="checkbox"
                              checked={getSel(d.id).memory}
                              onChange={() => toggleSel(d.id, 'memory')}
                              style={{ accentColor: '#4ade80', width: 14, height: 14 }}
                            />
                            Memory: <strong style={{ color: getSel(d.id).memory ? '#f1f5f9' : '#475569' }}>{d.current_memory}</strong>
                            <span style={{ color: '#475569' }}>→</span>
                            <strong style={{ color: getSel(d.id).memory ? '#4ade80' : '#475569' }}>{d.recommended_memory}</strong>
                          </label>
                        )}
                      </div>
                      <div style={{ display: 'flex', gap: 16, fontSize: 12, color: '#94a3b8' }}>
                        {d.confidence != null && <span>Confidence: <strong style={{ color: '#f1f5f9' }}>{d.confidence.toFixed(0)}%</strong></span>}
                        {d.savings_per_month != null && d.savings_per_month > 0 && <span style={{ color: '#4ade80' }}>Savings: <strong>${d.savings_per_month.toFixed(2)}/mo</strong></span>}
                        <span>{new Date(d.created_at).toLocaleString()}</span>
                      </div>
                    </div>
                  ) : (
                    <div style={{ display: 'flex', gap: 24, fontSize: 13, color: '#94a3b8' }}>
                      <span>
                        Replicas:{' '}
                        <strong style={{ color: '#f1f5f9' }}>{d.current_replicas}</strong>
                        <span style={{ color: '#475569', margin: '0 6px' }}>→</span>
                        <strong style={{ color: (d.desired_replicas ?? 0) > (d.current_replicas ?? 0) ? '#22c55e' : '#ef4444' }}>
                          {d.desired_replicas}
                        </strong>
                      </span>
                      {d.decision && d.decision.PeakCPU > 0 && (
                        <span>Peak CPU: <strong style={{ color: '#f1f5f9' }}>{(d.decision.PeakCPU * 100).toFixed(0)}%</strong></span>
                      )}
                      <span style={{ fontSize: 12 }}>{new Date(d.created_at).toLocaleString()}</span>
                    </div>
                  )}
                </div>

                <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
                  <ActionButton
                    label="Approve"
                    variant="approve"
                    loading={loading[d.id] === 'approve'}
                    onClick={() => handle(d.id, 'approve')}
                  />
                  <ActionButton
                    label="Reject"
                    variant="reject"
                    loading={loading[d.id] === 'reject'}
                    onClick={() => handle(d.id, 'reject')}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
