import { useEffect, useRef, useState } from 'react'
import { api, type PodInfo } from '../api'

function statusColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'running':     return '#22c55e'
    case 'pending':     return '#f59e0b'
    case 'failed':      return '#ef4444'
    case 'terminating': return '#f97316'
    default:            return '#94a3b8'
  }
}

function fmtCPU(millicores: number): string {
  if (millicores === 0) return '—'
  return millicores >= 1000
    ? `${(millicores / 1000).toFixed(2)} cores`
    : `${millicores}m`
}

function fmtMem(bytes: number): string {
  if (bytes === 0) return '—'
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
  return `${(bytes / 1024).toFixed(0)} KiB`
}

export function PodsPage() {
  const [pods, setPods] = useState<PodInfo[]>([])
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [selectedNamespace, setSelectedNamespace] = useState<string>('all')
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = () =>
    api.pods()
      .then(data => { setPods(data); setLastUpdated(new Date()); setError(null) })
      .catch(e => setError(String(e)))

  useEffect(() => {
    load()
    timerRef.current = setInterval(load, 10_000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [])

  const namespaces = ['all', ...Array.from(new Set(pods.map(p => p.namespace))).sort()]
  const visiblePods = selectedNamespace === 'all' ? pods : pods.filter(p => p.namespace === selectedNamespace)

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Pods</h2>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{ fontSize: 12, color: '#64748b' }}>
            {lastUpdated ? `Updated ${lastUpdated.toLocaleTimeString()} · auto-refresh 10s` : 'Loading…'}
          </div>
          <select
            value={selectedNamespace}
            onChange={e => setSelectedNamespace(e.target.value)}
            style={{
              background: '#161b22',
              color: '#e2e8f0',
              border: '1px solid #30363d',
              borderRadius: 6,
              padding: '4px 10px',
              fontSize: 13,
              cursor: 'pointer',
            }}
          >
            {namespaces.map(ns => (
              <option key={ns} value={ns}>{ns === 'all' ? 'All namespaces' : ns}</option>
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
              <th>Pod</th>
              <th>Namespace</th>
              <th>Status</th>
              <th>Restarts</th>
              <th>CPU</th>
              <th>Memory</th>
              <th>Node</th>
            </tr>
          </thead>
          <tbody>
            {visiblePods.length === 0 && !error ? (
              <tr><td colSpan={7} style={{ color: '#64748b', textAlign: 'center', padding: 32 }}>No pods found</td></tr>
            ) : visiblePods.map(pod => (
              <tr key={`${pod.namespace}/${pod.name}`}>
                <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{pod.name}</td>
                <td style={{ color: '#94a3b8' }}>{pod.namespace}</td>
                <td>
                  <span style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    padding: '2px 8px',
                    borderRadius: 4,
                    fontSize: 12,
                    fontWeight: 600,
                    background: statusColor(pod.status) + '1a',
                    color: statusColor(pod.status),
                  }}>
                    <span style={{ width: 6, height: 6, borderRadius: '50%', background: statusColor(pod.status) }} />
                    {pod.status}
                  </span>
                </td>
                <td style={{ color: pod.restart_count > 0 ? '#f59e0b' : '#94a3b8' }}>
                  {pod.restart_count}
                </td>
                <td style={{ fontFamily: 'monospace' }}>{fmtCPU(pod.cpu_usage_millicores)}</td>
                <td style={{ fontFamily: 'monospace' }}>{fmtMem(pod.memory_usage_bytes)}</td>
                <td style={{ color: '#94a3b8', fontSize: 12 }}>{pod.node || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
