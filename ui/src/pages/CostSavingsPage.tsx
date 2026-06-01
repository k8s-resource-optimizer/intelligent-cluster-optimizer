import { useEffect, useState } from 'react'
import { api, type DryRunDecision } from '../api'
import { toast } from '../components/Toast'

function dedup(decisions: DryRunDecision[]): DryRunDecision[] {
  const map = new Map<string, DryRunDecision>()
  for (const d of decisions) {
    if ((d.savings_per_month ?? 0) <= 0) continue
    const key = `${d.deployment_name}/${d.container_name}`
    const existing = map.get(key)
    if (!existing || (d.savings_per_month ?? 0) > (existing.savings_per_month ?? 0)) {
      map.set(key, d)
    }
  }
  return [...map.values()].sort((a, b) => (b.savings_per_month ?? 0) - (a.savings_per_month ?? 0))
}

export function CostSavingsPage() {
  const [decisions, setDecisions] = useState<DryRunDecision[]>([])
  const [loading, setLoading] = useState<Record<string, 'approve' | 'reject' | null>>({})
  const [error, setError] = useState<string | null>(null)

  const load = () =>
    api.dryRunPending()
      .then(data => { setDecisions(data ?? []); setError(null) })
      .catch(e => setError(String(e)))

  useEffect(() => {
    load()
    const t = setInterval(load, 30_000)
    return () => clearInterval(t)
  }, [])

  const handle = async (id: string, action: 'approve' | 'reject') => {
    setLoading(prev => ({ ...prev, [id]: action }))
    try {
      if (action === 'approve') {
        await api.approve(id)
        toast('Approved and applied', 'success')
      } else {
        await api.reject(id)
        toast('Rejected', 'info')
      }
      await load()
    } catch (e) {
      toast(`Failed: ${e}`, 'error')
      await load()
    } finally {
      setLoading(prev => ({ ...prev, [id]: null }))
    }
  }

  const rows = dedup(decisions)
  const total = rows.reduce((s, d) => s + (d.savings_per_month ?? 0), 0)

  return (
    <div>
      {/* Summary cards */}
      <div style={{ display: 'flex', gap: 16, marginBottom: 24 }}>
        <div style={{
          flex: 1, background: '#0d1117', border: '1px solid #166534',
          borderRadius: 10, padding: '18px 22px',
        }}>
          <div style={{ fontSize: 12, color: '#4ade80', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 6 }}>
            Potential Monthly Savings
          </div>
          <div style={{ fontSize: 28, fontWeight: 700, color: '#4ade80' }}>
            ${total.toFixed(2)}
            <span style={{ fontSize: 14, fontWeight: 400, color: '#64748b', marginLeft: 4 }}>/mo</span>
          </div>
        </div>
        <div style={{
          flex: 1, background: '#0d1117', border: '1px solid #1e2535',
          borderRadius: 10, padding: '18px 22px',
        }}>
          <div style={{ fontSize: 12, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 6 }}>
            Recommendations
          </div>
          <div style={{ fontSize: 28, fontWeight: 700, color: '#f1f5f9' }}>
            {rows.length}
            <span style={{ fontSize: 14, fontWeight: 400, color: '#64748b', marginLeft: 4 }}>workloads</span>
          </div>
        </div>
        <div style={{
          flex: 1, background: '#0d1117', border: '1px solid #1e2535',
          borderRadius: 10, padding: '18px 22px',
        }}>
          <div style={{ fontSize: 12, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 6 }}>
            Annual Projection
          </div>
          <div style={{ fontSize: 28, fontWeight: 700, color: '#f1f5f9' }}>
            ${(total * 12).toFixed(2)}
            <span style={{ fontSize: 14, fontWeight: 400, color: '#64748b', marginLeft: 4 }}>/yr</span>
          </div>
        </div>
      </div>

      {error && (
        <div style={{ color: '#fca5a5', background: '#450a0a', padding: '10px 14px', borderRadius: 6, marginBottom: 16, fontSize: 13 }}>
          {error}
        </div>
      )}

      {rows.length === 0 ? (
        <div style={{ color: '#64748b', textAlign: 'center', padding: 48 }}>
          No cost-saving recommendations yet — metrics are still being collected.
        </div>
      ) : (
        <div style={{ background: '#0d1117', border: '1px solid #1e2535', borderRadius: 10, overflow: 'hidden' }}>
          {/* Table header */}
          <div style={{
            display: 'grid',
            gridTemplateColumns: '2fr 1.2fr 1.2fr 1fr 1fr 160px',
            padding: '10px 20px',
            background: '#0a0f18',
            borderBottom: '1px solid #1e2535',
            fontSize: 11,
            fontWeight: 700,
            color: '#475569',
            textTransform: 'uppercase',
            letterSpacing: '0.06em',
          }}>
            <span>Workload</span>
            <span>CPU</span>
            <span>Memory</span>
            <span>Confidence</span>
            <span>Savings/mo</span>
            <span></span>
          </div>

          {/* Table rows */}
          {rows.map((d, i) => (
            <div key={d.id} style={{
              display: 'grid',
              gridTemplateColumns: '2fr 1.2fr 1.2fr 1fr 1fr 160px',
              padding: '14px 20px',
              alignItems: 'center',
              borderBottom: i < rows.length - 1 ? '1px solid #1e2535' : 'none',
              transition: 'background 0.1s',
            }}
              onMouseEnter={e => (e.currentTarget.style.background = '#111827')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
            >
              {/* Workload */}
              <div>
                <span style={{ fontFamily: 'monospace', fontSize: 13, fontWeight: 600, color: '#f1f5f9' }}>
                  {d.deployment_name}
                </span>
                {d.container_name && (
                  <span style={{ fontSize: 12, color: '#475569', marginLeft: 6 }}>/ {d.container_name}</span>
                )}
                <div style={{ fontSize: 11, color: '#475569', marginTop: 2 }}>{d.namespace}</div>
              </div>

              {/* CPU */}
              <div style={{ fontSize: 13 }}>
                {d.current_cpu ? (
                  <>
                    <span style={{ color: '#94a3b8' }}>{d.current_cpu}</span>
                    <span style={{ color: '#475569', margin: '0 6px' }}>→</span>
                    <span style={{ color: '#4ade80', fontWeight: 600 }}>{d.recommended_cpu}</span>
                  </>
                ) : '—'}
              </div>

              {/* Memory */}
              <div style={{ fontSize: 13 }}>
                {d.current_memory ? (
                  <>
                    <span style={{ color: '#94a3b8' }}>{d.current_memory}</span>
                    <span style={{ color: '#475569', margin: '0 6px' }}>→</span>
                    <span style={{ color: '#4ade80', fontWeight: 600 }}>{d.recommended_memory}</span>
                  </>
                ) : '—'}
              </div>

              {/* Confidence */}
              <div>
                <div style={{
                  display: 'inline-flex', alignItems: 'center', gap: 6,
                  padding: '3px 8px', borderRadius: 4,
                  background: (d.confidence ?? 0) > 70 ? '#14532d44' : '#1e2535',
                  fontSize: 12, fontWeight: 600,
                  color: (d.confidence ?? 0) > 70 ? '#4ade80' : '#94a3b8',
                }}>
                  {(d.confidence ?? 0).toFixed(0)}%
                </div>
              </div>

              {/* Savings */}
              <div style={{ fontSize: 15, fontWeight: 700, color: '#4ade80' }}>
                ${d.savings_per_month!.toFixed(2)}
              </div>

              {/* Actions */}
              <div style={{ display: 'flex', gap: 8 }}>
                <button
                  onClick={() => handle(d.id, 'approve')}
                  disabled={loading[d.id] === 'approve'}
                  style={{
                    padding: '5px 12px', borderRadius: 5, border: 'none',
                    background: loading[d.id] === 'approve' ? '#1e2535' : '#166534',
                    color: loading[d.id] === 'approve' ? '#64748b' : '#bbf7d0',
                    fontSize: 12, fontWeight: 600, cursor: 'pointer',
                  }}
                >
                  {loading[d.id] === 'approve' ? '…' : 'Approve'}
                </button>
                <button
                  onClick={() => handle(d.id, 'reject')}
                  disabled={loading[d.id] === 'reject'}
                  style={{
                    padding: '5px 12px', borderRadius: 5, border: 'none',
                    background: loading[d.id] === 'reject' ? '#1e2535' : '#7f1d1d',
                    color: loading[d.id] === 'reject' ? '#64748b' : '#fecaca',
                    fontSize: 12, fontWeight: 600, cursor: 'pointer',
                  }}
                >
                  {loading[d.id] === 'reject' ? '…' : 'Reject'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
