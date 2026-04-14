import { useEffect, useState } from 'react'
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

  useEffect(() => {
    api.scalingHistory()
      .then(data => { setRecords(data); setError(null) })
      .catch(e => setError(String(e)))
  }, [])

  return (
    <div>
      <h2>Scaling History</h2>

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
            {records.length === 0 && !error ? (
              <tr><td colSpan={7} style={{ color: '#64748b', textAlign: 'center', padding: 32 }}>No scaling decisions yet</td></tr>
            ) : records.map(r => (
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
