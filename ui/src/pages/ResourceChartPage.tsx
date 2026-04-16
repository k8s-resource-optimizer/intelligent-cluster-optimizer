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
import { api, type MetricsDataPoint, type ScalingRecord } from '../api'

const DEPLOYMENTS = [
  { namespace: 'workloads', deployment: 'stress-master' },
  { namespace: 'workloads', deployment: 'stress-cyclic' },
]

function fmt(ts: string) {
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function fmtMem(bytes: number): string {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
  return `${(bytes / 1024).toFixed(0)} KiB`
}

function MiniChart({
  data,
  yKey1,
  yKey2,
  label1,
  label2,
  color1,
  color2,
  tickFormatter,
  tooltipFormatter,
}: {
  data: object[]
  yKey1: string
  yKey2: string
  label1: string
  label2: string
  color1: string
  color2: string
  tickFormatter: (v: number) => string
  tooltipFormatter: (v: number) => string
}) {
  return (
    <div style={{ background: '#0d1117', border: '1px solid #1e2535', borderRadius: 8, padding: '20px 8px 8px' }}>
      <ResponsiveContainer width="100%" height={280}>
        <LineChart data={data} margin={{ top: 4, right: 24, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#1e2535" />
          <XAxis
            dataKey="time"
            tick={{ fill: '#64748b', fontSize: 11 }}
            axisLine={{ stroke: '#1e2535' }}
            tickLine={false}
            interval="preserveStartEnd"
          />
          <YAxis
            tickFormatter={tickFormatter}
            tick={{ fill: '#64748b', fontSize: 11 }}
            axisLine={{ stroke: '#1e2535' }}
            tickLine={false}
            width={52}
          />
          <Tooltip
            contentStyle={{ background: '#141824', border: '1px solid #1e2535', borderRadius: 6, fontSize: 12 }}
            labelStyle={{ color: '#94a3b8' }}
            formatter={(v: number) => [tooltipFormatter(v)]}
          />
          <Legend wrapperStyle={{ fontSize: 12, color: '#94a3b8', paddingTop: 8 }} />
          <Line type="monotone" dataKey={yKey1} name={label1} stroke={color1} strokeWidth={2} dot={false} activeDot={{ r: 4 }} />
          <Line type="stepAfter" dataKey={yKey2} name={label2} stroke={color2} strokeWidth={2} strokeDasharray="6 3" dot={false} activeDot={{ r: 4 }} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}

export function ResourceChartPage() {
  const [selected, setSelected] = useState(DEPLOYMENTS[0])
  const [points, setPoints] = useState<MetricsDataPoint[]>([])
  const [history, setHistory] = useState<ScalingRecord[]>([])
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = (ns: string, dep: string) =>
    Promise.all([
      api.metricsHistory(ns, dep),
      api.scalingHistory(),
    ]).then(([metrics, hist]) => {
      setPoints(metrics ?? [])
      setHistory(hist ?? [])
      setLastUpdated(new Date())
      setError(null)
    }).catch(e => setError(String(e)))

  useEffect(() => {
    load(selected.namespace, selected.deployment)
    timerRef.current = setInterval(() => load(selected.namespace, selected.deployment), 30_000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [selected])

  // Cost savings: last 24h, filtered to selected deployment
  const cutoff = Date.now() - 24 * 60 * 60 * 1000
  const savings24h = history
    .filter(r =>
      r.deployment_name === selected.deployment &&
      r.namespace === selected.namespace &&
      new Date(r.timestamp).getTime() > cutoff &&
      r.applied &&
      (r.savings_per_month ?? 0) > 0
    )
    .reduce((sum, r) => sum + (r.savings_per_month ?? 0), 0)

  const cpuData = points.map(p => ({
    time: fmt(p.timestamp),
    'Actual (m)': p.usage_cpu,
    'Request (m)': p.request_cpu,
  }))

  const memData = points.map(p => ({
    time: fmt(p.timestamp),
    'Actual': p.usage_memory,
    'Request': p.request_memory,
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

      {/* Cost savings banner */}
      <div style={{
        background: savings24h > 0 ? '#0d2818' : '#0d1117',
        border: `1px solid ${savings24h > 0 ? '#166534' : '#1e2535'}`,
        borderRadius: 8,
        padding: '14px 20px',
        marginBottom: 20,
        display: 'flex',
        alignItems: 'center',
        gap: 16,
      }}>
        <div>
          <div style={{ fontSize: 12, color: '#64748b', marginBottom: 2 }}>Estimated savings (last 24h)</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: savings24h > 0 ? '#4ade80' : '#94a3b8' }}>
            {savings24h > 0 ? `$${savings24h.toFixed(2)}/mo` : '—'}
          </div>
        </div>
        {savings24h > 0 && (
          <div style={{ fontSize: 12, color: '#4ade80', marginLeft: 8 }}>
            based on {history.filter(r =>
              r.deployment_name === selected.deployment &&
              r.namespace === selected.namespace &&
              new Date(r.timestamp).getTime() > cutoff &&
              r.applied &&
              (r.savings_per_month ?? 0) > 0
            ).length} applied optimizations
          </div>
        )}
      </div>

      {points.length === 0 && !error ? (
        <div style={{ color: '#64748b', textAlign: 'center', padding: 48 }}>
          No metrics data yet (last 2 hours)
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          <div>
            <div style={{ fontSize: 13, fontWeight: 600, color: '#94a3b8', marginBottom: 10 }}>CPU (millicores)</div>
            <MiniChart
              data={cpuData}
              yKey1="Actual (m)"
              yKey2="Request (m)"
              label1="Actual Usage"
              label2="Request"
              color1="#fef08a"
              color2="#7c3aed"
              tickFormatter={v => `${v}m`}
              tooltipFormatter={v => `${v}m`}
            />
          </div>

          <div>
            <div style={{ fontSize: 13, fontWeight: 600, color: '#94a3b8', marginBottom: 10 }}>Memory</div>
            <MiniChart
              data={memData}
              yKey1="Actual"
              yKey2="Request"
              label1="Actual Usage"
              label2="Request"
              color1="#fef08a"
              color2="#7c3aed"
              tickFormatter={v => fmtMem(v)}
              tooltipFormatter={v => fmtMem(v)}
            />
          </div>
        </div>
      )}
    </div>
  )
}
