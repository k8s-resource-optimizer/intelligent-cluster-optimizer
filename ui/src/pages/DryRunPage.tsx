import { useEffect, useState } from 'react'
import { api, type DryRunDecision } from '../api'
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

export function DryRunPage() {
  const [decisions, setDecisions] = useState<DryRunDecision[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState<Record<string, 'approve' | 'reject' | null>>({})

  const load = () =>
    api.dryRunPending()
      .then(data => { setDecisions(data ?? []); setError(null) })
      .catch(e => setError(String(e)))

  useEffect(() => { load() }, [])

  const handle = async (id: string, action: 'approve' | 'reject') => {
    setLoading(prev => ({ ...prev, [id]: action }))
    try {
      if (action === 'approve') {
        await api.approve(id)
        toast(`Approved and applied scaling decision`, 'success')
      } else {
        await api.reject(id)
        toast(`Scaling decision rejected`, 'info')
      }
      await load()
    } catch (e) {
      toast(`Failed to ${action}: ${e}`, 'error')
    } finally {
      setLoading(prev => ({ ...prev, [id]: null }))
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Dry-Run Queue</h2>
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

      {error && (
        <div style={{ color: '#fca5a5', background: '#450a0a', padding: '10px 14px', borderRadius: 6, marginBottom: 16, fontSize: 13 }}>
          {error}
        </div>
      )}

      {decisions.length === 0 && !error ? (
        <div style={{ color: '#64748b', textAlign: 'center', padding: 48 }}>No pending dry-run decisions</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {decisions.map(d => (
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
                    <div style={{ display: 'flex', gap: 24, fontSize: 13, color: '#94a3b8', flexWrap: 'wrap' }}>
                      {d.current_cpu && <span>
                        CPU: <strong style={{ color: '#f1f5f9' }}>{d.current_cpu}</strong>
                        <span style={{ color: '#475569', margin: '0 6px' }}>→</span>
                        <strong style={{ color: '#4ade80' }}>{d.recommended_cpu}</strong>
                      </span>}
                      {d.current_memory && <span>
                        Memory: <strong style={{ color: '#f1f5f9' }}>{d.current_memory}</strong>
                        <span style={{ color: '#475569', margin: '0 6px' }}>→</span>
                        <strong style={{ color: '#4ade80' }}>{d.recommended_memory}</strong>
                      </span>}
                      {d.confidence != null && <span>
                        Confidence: <strong style={{ color: '#f1f5f9' }}>{(d.confidence * 100).toFixed(0)}%</strong>
                      </span>}
                      {d.savings_per_month != null && d.savings_per_month > 0 && <span style={{ color: '#4ade80' }}>
                        Savings: <strong>${d.savings_per_month.toFixed(2)}/mo</strong>
                      </span>}
                      <span style={{ fontSize: 12 }}>{new Date(d.created_at).toLocaleString()}</span>
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
