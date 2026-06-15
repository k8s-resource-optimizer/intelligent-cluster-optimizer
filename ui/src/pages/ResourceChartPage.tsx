import { useEffect, useRef, useState } from 'react'
import {
  Area,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { api, type MetricsDataPoint, type ScalingRecord } from '../api'

const DEPLOYMENTS = [
  { namespace: 'workloads', deployment: 'stress-cyclic' },
  { namespace: 'workloads', deployment: 'stress-master' },
  { namespace: 'workloads', deployment: 'app-cpu-intensive' },
]

const WINDOW_OPTIONS = [
  { label: '2h',  ms: 2  * 60 * 60 * 1000 },
  { label: '6h',  ms: 6  * 60 * 60 * 1000 },
  { label: '12h', ms: 12 * 60 * 60 * 1000 },
  { label: '24h', ms: 24 * 60 * 60 * 1000 },
  { label: '48h', ms: 48 * 60 * 60 * 1000 },
]

function fmtTime(ts: string) {
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function fmtDateTime(d: Date) {
  return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function fmtMem(bytes: number): string {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
  return `${(bytes / 1024).toFixed(0)} KiB`
}

function CustomTooltip({ active, payload, label, fmt }: {
  active?: boolean
  payload?: { name: string; value: number; color: string; strokeDasharray?: string }[]
  label?: string
  fmt: (v: number) => string
}) {
  if (!active || !payload?.length) return null
  return (
    <div style={{
      background: '#0d1117',
      border: '1px solid #2d3748',
      borderRadius: 10,
      padding: '10px 14px',
      boxShadow: '0 8px 24px rgba(0,0,0,0.6)',
      minWidth: 160,
    }}>
      <div style={{ color: '#475569', fontSize: 11, marginBottom: 8, fontWeight: 500 }}>{label}</div>
      {payload.map(p => (
        <div key={p.name} style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <span style={{
            width: 20, height: 2,
            background: p.color,
            display: 'inline-block',
            borderRadius: 1,
            opacity: p.strokeDasharray ? 0.7 : 1,
          }} />
          <span style={{ color: '#94a3b8', fontSize: 12, flex: 1 }}>{p.name}</span>
          <span style={{ color: '#f1f5f9', fontSize: 12, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
            {p.value != null ? fmt(p.value) : '—'}
          </span>
        </div>
      ))}
    </div>
  )
}

function Chart({
  id,
  title,
  data,
  areaKey,
  lineKey,
  areaLabel,
  lineLabel,
  areaColor,
  lineColor,
  tickFmt,
  tooltipFmt,
}: {
  id: string
  title: string
  data: object[]
  areaKey: string
  lineKey: string
  areaLabel: string
  lineLabel: string
  areaColor: string
  lineColor: string
  tickFmt: (v: number) => string
  tooltipFmt: (v: number) => string
}) {
  const gradId = `grad-${id}`

  return (
    <div style={{
      background: '#0d1117',
      border: '1px solid #1e2535',
      borderRadius: 12,
      padding: '20px 16px 12px',
      overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16, paddingLeft: 4 }}>
        <span style={{ fontSize: 13, fontWeight: 600, color: '#cbd5e1', letterSpacing: '0.02em' }}>{title}</span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <svg width="20" height="2"><line x1="0" y1="1" x2="20" y2="1" stroke={areaColor} strokeWidth="2" /></svg>
            <span style={{ fontSize: 11, color: '#64748b' }}>{areaLabel}</span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <svg width="20" height="2"><line x1="0" y1="1" x2="20" y2="1" stroke={lineColor} strokeWidth="1.5" strokeDasharray="4 3" /></svg>
            <span style={{ fontSize: 11, color: '#64748b' }}>{lineLabel}</span>
          </div>
        </div>
      </div>

      <ResponsiveContainer width="100%" height={220}>
        <ComposedChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={areaColor} stopOpacity={0.2} />
              <stop offset="100%" stopColor={areaColor} stopOpacity={0} />
            </linearGradient>
          </defs>

          <CartesianGrid
            vertical={false}
            stroke="#1e2535"
            strokeOpacity={0.8}
          />

          <XAxis
            dataKey="time"
            tick={{ fill: '#475569', fontSize: 10, fontFamily: 'inherit' }}
            axisLine={false}
            tickLine={false}
            interval="preserveStartEnd"
            tickMargin={8}
          />

          <YAxis
            tickFormatter={tickFmt}
            tick={{ fill: '#475569', fontSize: 10, fontFamily: 'inherit' }}
            axisLine={false}
            tickLine={false}
            width={48}
            tickMargin={4}
          />

          <Tooltip
            content={<CustomTooltip fmt={tooltipFmt} />}
            cursor={{ stroke: '#2d3748', strokeWidth: 1 }}
          />

          <Area
            type="monotone"
            dataKey={areaKey}
            name={areaLabel}
            stroke={areaColor}
            strokeWidth={2}
            fill={`url(#${gradId})`}
            dot={false}
            activeDot={{ r: 4, fill: areaColor, strokeWidth: 0 }}
            isAnimationActive={false}
          />

          <Line
            type="stepAfter"
            dataKey={lineKey}
            name={lineLabel}
            stroke={lineColor}
            strokeWidth={1.5}
            strokeDasharray="5 4"
            dot={false}
            activeDot={{ r: 3, fill: lineColor, strokeWidth: 0 }}
            isAnimationActive={false}
          />
        </ComposedChart>
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
  const [windowMs, setWindowMs] = useState(WINDOW_OPTIONS[0].ms)
  const [windowEnd, setWindowEnd] = useState<number | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = (ns: string, dep: string) =>
    Promise.all([api.metricsHistory(ns, dep), api.scalingHistory()])
      .then(([metrics, hist]) => {
        setPoints(metrics ?? [])
        setHistory(hist ?? [])
        setLastUpdated(new Date())
        setError(null)
      })
      .catch(e => setError(String(e)))

  useEffect(() => {
    setWindowEnd(null)
    load(selected.namespace, selected.deployment)
    timerRef.current = setInterval(() => load(selected.namespace, selected.deployment), 30_000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [selected])

  const sorted = [...points].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
  )

  const dataStart = sorted.length > 0 ? new Date(sorted[0].timestamp).getTime() : 0
  const dataEnd   = sorted.length > 0 ? new Date(sorted[sorted.length - 1].timestamp).getTime() : 0
  const effectiveEnd   = windowEnd ?? dataEnd
  const effectiveStart = effectiveEnd - windowMs
  const isLive = windowEnd === null || windowEnd >= dataEnd - 30_000

  const windowed = sorted.filter(p => {
    const t = new Date(p.timestamp).getTime()
    return t >= effectiveStart && t <= effectiveEnd
  })

  const cpuData = windowed.map(p => ({
    time: fmtTime(p.timestamp),
    usage: p.usage_cpu,
    request: p.request_cpu,
  }))

  const memData = windowed.map(p => ({
    time: fmtTime(p.timestamp),
    usage:   p.usage_memory  > 0 ? p.usage_memory  : undefined,
    request: p.request_memory > 0 ? p.request_memory : undefined,
  }))

  const canGoBack    = effectiveStart > dataStart
  const canGoForward = !isLive

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

  const navBtnStyle = (enabled: boolean): React.CSSProperties => ({
    background: 'transparent',
    color: enabled ? '#94a3b8' : '#2d3748',
    border: '1px solid',
    borderColor: enabled ? '#2d3748' : '#1e2535',
    borderRadius: 6,
    padding: '4px 10px',
    fontSize: 13,
    cursor: enabled ? 'pointer' : 'default',
    lineHeight: 1,
  })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

      {/* Top bar */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div>
          <div style={{ fontSize: 18, fontWeight: 700, color: '#f1f5f9' }}>Resource Usage</div>
          <div style={{ fontSize: 12, color: '#475569', marginTop: 2 }}>
            {lastUpdated ? `Updated ${lastUpdated.toLocaleTimeString()} · auto-refresh 30s` : 'Loading…'}
          </div>
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
            border: '1px solid #2d3748',
            borderRadius: 8,
            padding: '6px 12px',
            fontSize: 13,
            cursor: 'pointer',
            outline: 'none',
          }}
        >
          {DEPLOYMENTS.map(d => (
            <option key={`${d.namespace}/${d.deployment}`} value={`${d.namespace}/${d.deployment}`}>
              {d.deployment}
            </option>
          ))}
        </select>
      </div>

      {error && (
        <div style={{ color: '#fca5a5', background: '#450a0a', padding: '10px 14px', borderRadius: 8, fontSize: 13 }}>
          {error}
        </div>
      )}

      {/* Savings + time nav */}
      <div style={{ display: 'flex', gap: 12, alignItems: 'stretch' }}>
        {/* Savings card */}
        <div style={{
          background: savings24h > 0 ? 'rgba(20,83,45,0.4)' : '#0d1117',
          border: `1px solid ${savings24h > 0 ? '#166534' : '#1e2535'}`,
          borderRadius: 12,
          padding: '14px 20px',
          minWidth: 180,
        }}>
          <div style={{ fontSize: 11, color: '#475569', marginBottom: 4, fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.06em' }}>
            Est. savings · 24h
          </div>
          <div style={{ fontSize: 26, fontWeight: 700, color: savings24h > 0 ? '#4ade80' : '#334155', lineHeight: 1 }}>
            {savings24h > 0 ? `$${savings24h.toFixed(2)}` : '—'}
          </div>
          {savings24h > 0 && <div style={{ fontSize: 11, color: '#4ade80', marginTop: 4 }}>/ month</div>}
        </div>

        {/* Time navigation */}
        {sorted.length > 0 && (
          <div style={{
            flex: 1,
            background: '#0d1117',
            border: '1px solid #1e2535',
            borderRadius: 12,
            padding: '12px 16px',
            display: 'flex',
            alignItems: 'center',
            gap: 10,
          }}>
            <button style={navBtnStyle(canGoBack)} onClick={canGoBack ? () => setWindowEnd(prev => {
              const end = prev ?? dataEnd
              return Math.max(dataStart + windowMs, end - windowMs)
            }) : undefined}>←</button>

            <div style={{ flex: 1, textAlign: 'center' }}>
              <div style={{ fontSize: 12, color: '#64748b' }}>
                {fmtDateTime(new Date(effectiveStart))} — {fmtDateTime(new Date(effectiveEnd))}
              </div>
              {isLive && (
                <div style={{ display: 'inline-flex', alignItems: 'center', gap: 5, marginTop: 3 }}>
                  <span style={{
                    width: 6, height: 6, borderRadius: '50%', background: '#4ade80',
                    boxShadow: '0 0 6px #4ade80',
                    display: 'inline-block',
                  }} />
                  <span style={{ fontSize: 11, color: '#4ade80', fontWeight: 600, letterSpacing: '0.06em' }}>LIVE</span>
                </div>
              )}
            </div>

            <div style={{ display: 'flex', gap: 4 }}>
              {WINDOW_OPTIONS.map(opt => (
                <button
                  key={opt.label}
                  onClick={() => { setWindowMs(opt.ms); setWindowEnd(null) }}
                  style={{
                    padding: '4px 9px', borderRadius: 6, border: '1px solid',
                    borderColor: windowMs === opt.ms ? '#3b82f6' : '#1e2535',
                    background: windowMs === opt.ms ? 'rgba(59,130,246,0.15)' : 'transparent',
                    color: windowMs === opt.ms ? '#60a5fa' : '#475569',
                    fontSize: 11, fontWeight: 600, cursor: 'pointer',
                  }}
                >{opt.label}</button>
              ))}
            </div>

            <button style={navBtnStyle(canGoForward)} onClick={canGoForward ? () => setWindowEnd(prev => {
              const end = prev ?? dataEnd
              const next = end + windowMs
              return next >= dataEnd ? null : next
            }) : undefined}>→</button>

            {!isLive && (
              <button
                onClick={() => setWindowEnd(null)}
                style={{ ...navBtnStyle(true), color: '#4ade80', borderColor: '#166534', padding: '4px 10px', fontSize: 11, fontWeight: 600 }}
              >Live</button>
            )}
          </div>
        )}
      </div>

      {points.length === 0 && !error ? (
        <div style={{
          color: '#334155', textAlign: 'center', padding: 80, fontSize: 14,
          border: '1px dashed #1e2535', borderRadius: 12,
        }}>
          No metrics data yet
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <Chart
            id="cpu"
            title="CPU"
            data={cpuData}
            areaKey="usage"
            lineKey="request"
            areaLabel="Actual Usage"
            lineLabel="Request"
            areaColor="#38bdf8"
            lineColor="#818cf8"
            tickFmt={v => `${v}m`}
            tooltipFmt={v => `${v}m`}
          />
          <Chart
            id="mem"
            title="Memory"
            data={memData}
            areaKey="usage"
            lineKey="request"
            areaLabel="Actual Usage"
            lineLabel="Request"
            areaColor="#34d399"
            lineColor="#818cf8"
            tickFmt={v => fmtMem(v)}
            tooltipFmt={v => fmtMem(v)}
          />
        </div>
      )}
    </div>
  )
}
