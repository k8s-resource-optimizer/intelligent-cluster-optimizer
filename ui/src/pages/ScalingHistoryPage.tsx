import { useEffect, useMemo, useState } from 'react'
import { api, type ScalingRecord } from '../api'

function reasonBadge(reason: string) {
  const color =
    reason.includes('ML') ? '#7c3aed' :
    reason.includes('HoltWinters') ? '#0891b2' :
    reason.includes('Manual') ? '#0d9488' : '#475569'

  return (
    <span style={{
      padding: '2px 8px',
      borderRadius: 4,
      fontSize: 11,
      fontWeight: 600,
      background: color + '22',
      color,
    }}>
      {reason}
    </span>
  )
}

function ChangeCell({ r }: { r: ScalingRecord }) {
  if (r.old_cpu || r.old_memory) {
    return (
      <div style={{ fontSize: 12, display: 'flex', flexDirection: 'column', gap: 2 }}>
        {r.old_cpu && (
          <span>
            CPU: <span style={{ color: '#94a3b8' }}>{r.old_cpu}</span>
            <span style={{ color: '#475569', margin: '0 4px' }}>→</span>
            <span style={{ color: '#4ade80', fontWeight: 600 }}>{r.new_cpu}</span>
          </span>
        )}
        {r.old_memory && (
          <span>
            Mem: <span style={{ color: '#94a3b8' }}>{r.old_memory}</span>
            <span style={{ color: '#475569', margin: '0 4px' }}>→</span>
            <span style={{ color: '#4ade80', fontWeight: 600 }}>{r.new_memory}</span>
          </span>
        )}
      </div>
    )
  }
  return (
    <span>
      <span style={{ color: '#94a3b8' }}>{r.old_replicas ?? '—'}</span>
      <span style={{ color: '#475569', margin: '0 6px' }}>→</span>
      <span style={{
        fontWeight: 700,
        color: (r.new_replicas ?? 0) > (r.old_replicas ?? 0) ? '#22c55e'
          : (r.new_replicas ?? 0) < (r.old_replicas ?? 0) ? '#ef4444'
          : '#94a3b8',
      }}>
        {r.new_replicas ?? '—'}
      </span>
    </span>
  )
}

export function ScalingHistoryPage() {
  const [records, setRecords] = useState<ScalingRecord[]>([])
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<string>('all')

  const load = () =>
    api.scalingHistory()
      .then(data => {
        const sorted = [...data].sort(
          (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
        )
        setRecords(sorted)
        setError(null)
      })
      .catch(e => setError(String(e)))

  useEffect(() => {
    load()
    const t = setInterval(load, 30_000)
    return () => clearInterval(t)
  }, [])

  // Unique deployment names for filter dropdown
  const deployments = useMemo(() => {
    const names = Array.from(new Set(records.map(r => r.deployment_name))).sort()
    return names
  }, [records])

  const filtered = useMemo(() => {
    if (filter === 'all') return records
    return records.filter(r => r.deployment_name === filter)
  }, [records, filter])

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Scaling History</h2>
        <div style={{ display: 'flex', gap: 8 }}>
        <button onClick={load} style={{ padding: '6px 14px', borderRadius: 5, border: '1px solid #1e2535', background: 'transparent', color: '#94a3b8', fontSize: 12, cursor: 'pointer' }}>
          Refresh
        </button>
        <select
          value={filter}
          onChange={e => setFilter(e.target.value)}
          style={{
            padding: '6px 12px',
            borderRadius: 5,
            border: '1px solid #1e2535',
            background: '#0d1117',
            color: '#94a3b8',
            fontSize: 13,
            cursor: 'pointer',
          }}
        >
          <option value="all">All deployments</option>
          {deployments.map(name => (
            <option key={name} value={name}>{name}</option>
          ))}
        </select>
        </div>
      </div>

      {error && (
        <div style={{ color: '#fca5a5', background: '#450a0a', padding: '10px 14px', borderRadius: 6, marginBottom: 16, fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ background: '#0d1117', borderRadius: 8, border: '1px solid #1e2535', overflow: 'hidden' }}>
        <table>
          <thead>
            <tr>
              <th>Time</th>
              <th>Deployment / Container</th>
              <th>Namespace</th>
              <th>Change</th>
              <th>Reason</th>
              <th>Savings/mo</th>
              <th>Applied</th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 && !error ? (
              <tr><td colSpan={7} style={{ color: '#64748b', textAlign: 'center', padding: 32 }}>No scaling decisions yet</td></tr>
            ) : filtered.map(r => (
              <tr key={r.id}>
                <td style={{ color: '#94a3b8', fontSize: 12, whiteSpace: 'nowrap' }}>
                  {new Date(r.timestamp).toLocaleString()}
                </td>
                <td style={{ fontFamily: 'monospace', fontSize: 13 }}>
                  {r.deployment_name}
                  {r.container_name && <span style={{ color: '#64748b', fontSize: 11 }}> / {r.container_name}</span>}
                </td>
                <td style={{ color: '#94a3b8' }}>{r.namespace}</td>
                <td><ChangeCell r={r} /></td>
                <td>{reasonBadge(r.reason)}</td>
                <td style={{ fontFamily: 'monospace', fontSize: 12, color: '#4ade80' }}>
                  {r.savings_per_month != null && r.savings_per_month > 0
                    ? `$${r.savings_per_month.toFixed(2)}`
                    : '—'}
                </td>
                <td>
                  <span style={{ color: r.applied ? '#22c55e' : '#f59e0b', fontSize: 12 }}>
                    {r.applied ? 'Yes' : 'Dry-run'}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
