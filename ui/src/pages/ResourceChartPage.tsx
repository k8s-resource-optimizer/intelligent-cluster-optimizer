import { useEffect, useRef, useState } from 'react'
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { api, type MetricsDataPoint } from '../api'

const DEPLOYMENTS = [
  { namespace: 'workloads', deployment: 'stress-master' },
]

function fmt(ts: string) {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

export function ResourceChartPage() {
  const [selected, setSelected] = useState(DEPLOYMENTS[0])
  const [points, setPoints] = useState<MetricsDataPoint[]>([])
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = (ns: string, dep: string) =>
    api.metricsHistory(ns, dep)
      .then(data => { setPoints(data ?? []); setLastUpdated(new Date()); setError(null) })
      .catch(e => setError(String(e)))

  useEffect(() => {
    load(selected.namespace, selected.deployment)
    timerRef.current = setInterval(() => load(selected.namespace, selected.deployment), 30_000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [selected])

  const chartData = points.map(p => ({
    time: fmt(p.timestamp),
    'Actual Usage (m)': p.usage_cpu,
    'Request (m)': p.request_cpu,
  }))

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
        <h2 style={{ margin: 0 }}>Resource Usage</h2>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{ fontSize: 12, color: '#64748b' }}>
            {lastUpdated ? `Updated ${lastUpdated.toLocaleTimeString()} · auto-refresh 30s` : 'Loading…'}
          </div>
          <select
            value={`${selected.namespace}/${selected.deployment}`}
            onChange={e => {
              const [ns, dep] = e.target.value.split('/')
              setSelected({ namespace: ns, deployment: dep })
            }}
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
            {DEPLOYMENTS.map(d => (
              <option key={`${d.namespace}/${d.deployment}`} value={`${d.namespace}/${d.deployment}`}>
                {d.deployment} ({d.namespace})
              </option>
            ))}
          </select>
        </div>
      </div>

      {error && (
        <div style={{ color: '#fca5a5', background: '#450a0a', padding: '10px 14px', borderRadius: 6, marginBottom: 16, fontSize: 13 }}>
          {error}
        </div>
      )}

      {points.length === 0 && !error ? (
        <div style={{ color: '#64748b', textAlign: 'center', padding: 48 }}>
          No metrics data yet (last 2 hours)
        </div>
      ) : (
        <div style={{ background: '#0d1117', border: '1px solid #1e2535', borderRadius: 8, padding: '20px 8px 8px' }}>
          <ResponsiveContainer width="100%" height={340}>
            <LineChart data={chartData} margin={{ top: 4, right: 24, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#1e2535" />
              <XAxis
                dataKey="time"
                tick={{ fill: '#64748b', fontSize: 11 }}
                axisLine={{ stroke: '#1e2535' }}
                tickLine={false}
                interval="preserveStartEnd"
              />
              <YAxis
                tickFormatter={v => `${v}m`}
                tick={{ fill: '#64748b', fontSize: 11 }}
                axisLine={{ stroke: '#1e2535' }}
                tickLine={false}
                width={48}
              />
              <Tooltip
                contentStyle={{ background: '#141824', border: '1px solid #1e2535', borderRadius: 6, fontSize: 12 }}
                labelStyle={{ color: '#94a3b8' }}
                formatter={(v: number) => [`${v}m`]}
              />
              <Legend wrapperStyle={{ fontSize: 12, color: '#94a3b8', paddingTop: 12 }} />
              <Line
                type="monotone"
                dataKey="Actual Usage (m)"
                stroke="#f97316"
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 4 }}
              />
              <Line
                type="stepAfter"
                dataKey="Request (m)"
                stroke="#7c3aed"
                strokeWidth={2}
                strokeDasharray="6 3"
                dot={false}
                activeDot={{ r: 4 }}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}

      <div style={{ marginTop: 16, display: 'flex', gap: 24, fontSize: 13, color: '#64748b' }}>
        <span><span style={{ color: '#f97316' }}>—</span> Actual CPU usage collected every 30s</span>
        <span><span style={{ color: '#7c3aed' }}>- -</span> CPU request set by optimizer (step = applied change)</span>
      </div>
    </div>
  )
}
