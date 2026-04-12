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
              <th>Deployment</th>
              <th>Namespace</th>
              <th>Replicas</th>
              <th>Reason</th>
              <th>Peak CPU</th>
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
                <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{r.deployment_name}</td>
                <td style={{ color: '#94a3b8' }}>{r.namespace}</td>
                <td>
                  <span style={{ color: '#94a3b8' }}>{r.old_replicas}</span>
                  <span style={{ color: '#475569', margin: '0 6px' }}>→</span>
                  <span style={{
                    fontWeight: 700,
                    color: r.new_replicas > r.old_replicas ? '#22c55e'
                      : r.new_replicas < r.old_replicas ? '#ef4444'
                      : '#94a3b8',
                  }}>
                    {r.new_replicas}
                  </span>
                </td>
                <td>{reasonBadge(r.reason)}</td>
                <td style={{ fontFamily: 'monospace', fontSize: 12, color: '#94a3b8' }}>
                  {r.peak_cpu > 0 ? `${(r.peak_cpu * 100).toFixed(0)}%` : '—'}
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
