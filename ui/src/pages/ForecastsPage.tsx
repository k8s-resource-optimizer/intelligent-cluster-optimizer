import { useEffect, useState } from 'react'
import {
  Area,
  ComposedChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import { api, type ForecastEntry } from '../api'

function buildChartData(entry: ForecastEntry) {
  return entry.points.map(p => ({
    step: `+${p.step}`,
    low: +(p.low * 100).toFixed(1),
    median: +(p.median * 100).toFixed(1),
    high: +(p.high * 100).toFixed(1),
    band: [+(p.low * 100).toFixed(1), +(p.high * 100).toFixed(1)] as [number, number],
  }))
}

function ForecastCard({ entry }: { entry: ForecastEntry }) {
  const data = buildChartData(entry)
  const updated = new Date(entry.timestamp).toLocaleString()

  return (
    <div style={{
      background: '#0d1117',
      border: '1px solid #1e2535',
      borderRadius: 8,
      padding: 20,
      marginBottom: 20,
    }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 16 }}>
        <div>
          <h3 style={{ margin: 0 }}>{entry.deployment_name}</h3>
          <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>
            {entry.namespace} · {entry.cpu_samples} samples · {entry.inference_ms.toFixed(1)}ms inference · {updated}
          </div>
        </div>
        {entry.decision && (
          <div style={{
            padding: '4px 10px',
            borderRadius: 4,
            fontSize: 12,
            fontWeight: 600,
            background: entry.decision.ScaleUp ? '#14532d' : entry.decision.ScaleDown ? '#450a0a' : '#1e2535',
            color: entry.decision.ScaleUp ? '#86efac' : entry.decision.ScaleDown ? '#fca5a5' : '#94a3b8',
          }}>
            {entry.decision.ScaleUp
              ? `Scale Up → ${entry.decision.DesiredReplicas} replicas`
              : entry.decision.ScaleDown
              ? `Scale Down → ${entry.decision.DesiredReplicas} replicas`
              : 'No Change'}
          </div>
        )}
      </div>

      <ResponsiveContainer width="100%" height={220}>
        <ComposedChart data={data} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#1e2535" />
          <XAxis dataKey="step" tick={{ fill: '#64748b', fontSize: 11 }} axisLine={{ stroke: '#1e2535' }} tickLine={false} />
          <YAxis
            domain={[0, 100]}
            tickFormatter={v => `${v}%`}
            tick={{ fill: '#64748b', fontSize: 11 }}
            axisLine={{ stroke: '#1e2535' }}
            tickLine={false}
            width={40}
          />
          <Tooltip
            contentStyle={{ background: '#141824', border: '1px solid #1e2535', borderRadius: 6, fontSize: 12 }}
            labelStyle={{ color: '#94a3b8' }}
            formatter={(v: number) => [`${v}%`]}
          />
          <Legend wrapperStyle={{ fontSize: 12, color: '#94a3b8' }} />
          <Area
            dataKey="band"
            fill="#1d4ed8"
            fillOpacity={0.15}
            stroke="none"
            name="Confidence (p10-p90)"
          />
          <Line
            type="monotone"
            dataKey="median"
            stroke="#f97316"
            strokeWidth={2}
            strokeDasharray="6 3"
            dot={false}
            name="Forecast (p50)"
          />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  )
}

export function ForecastsPage() {
  const [entries, setEntries] = useState<ForecastEntry[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.forecasts()
      .then(data => { setEntries(data); setError(null) })
      .catch(e => setError(String(e)))
  }, [])

  return (
    <div>
      <h2>CPU Forecasts</h2>
      {error && (
        <div style={{ color: '#fca5a5', background: '#450a0a', padding: '10px 14px', borderRadius: 6, marginBottom: 16, fontSize: 13 }}>
          {error}
        </div>
      )}
      {entries.length === 0 && !error ? (
        <div style={{ color: '#64748b', textAlign: 'center', padding: 48 }}>No forecasts cached yet</div>
      ) : (
        entries.map(e => <ForecastCard key={`${e.namespace}/${e.deployment_name}`} entry={e} />)
      )}
    </div>
  )
}
