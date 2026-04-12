export interface PodInfo {
  name: string
  namespace: string
  status: string
  restart_count: number
  cpu_usage_millicores: number
  memory_usage_bytes: number
  node: string
  start_time?: string
}

export interface ForecastPoint {
  step: number
  low: number
  median: number
  high: number
}

export interface ScaleDecision {
  ScaleUp: boolean
  ScaleDown: boolean
  DesiredReplicas: number
  PeakCPU: number
  SustainedCPU: number
}

export interface ForecastEntry {
  timestamp: string
  namespace: string
  deployment_name: string
  points: ForecastPoint[]
  decision?: ScaleDecision
  inference_ms: number
  cpu_samples: number
}

export interface ScalingRecord {
  id: string
  timestamp: string
  namespace: string
  deployment_name: string
  old_replicas: number
  new_replicas: number
  reason: string
  peak_cpu: number
  sustained_cpu: number
  applied: boolean
}

export interface OptimizerConfigSummary {
  name: string
  namespace: string
  enabled: boolean
  dry_run: boolean
  phase: string
  ml_forecaster_enabled: boolean
}

export interface DryRunDecision {
  id: string
  created_at: string
  namespace: string
  deployment_name: string
  current_replicas: number
  desired_replicas: number
  reason: string
  decision: ScaleDecision
  status: 'pending' | 'approved' | 'rejected'
  reviewed_at?: string
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export const api = {
  health: () => fetch('/health').then(r => r.ok),
  pods: () => get<PodInfo[]>('/api/pods'),
  forecasts: () => get<ForecastEntry[]>('/api/forecasts'),
  scalingHistory: () => get<ScalingRecord[]>('/api/scaling-history'),
  optimizerConfigs: () => get<OptimizerConfigSummary[]>('/api/optimizer-configs'),
  dryRunPending: () => get<DryRunDecision[]>('/api/dry-run/pending'),
  approve: (id: string) => post<DryRunDecision>('/api/dry-run/approve', { id }),
  reject: (id: string) => post<DryRunDecision>('/api/dry-run/reject', { id }),
  scale: (namespace: string, deployment_name: string, replicas: number, reason = 'Manual') =>
    post<ScalingRecord>('/api/scale', { namespace, deployment_name, replicas, reason }),
}
