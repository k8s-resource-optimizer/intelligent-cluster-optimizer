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
  container_name?: string
  reason: string
  applied: boolean
  // horizontal
  old_replicas?: number
  new_replicas?: number
  peak_cpu?: number
  sustained_cpu?: number
  // vertical
  old_cpu?: string
  new_cpu?: string
  old_memory?: string
  new_memory?: string
  confidence?: number
  savings_per_hour?: number
  savings_per_month?: number
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
  reason: string
  scaling_type: 'horizontal' | 'vertical'
  status: 'pending' | 'approved' | 'rejected'
  reviewed_at?: string
  // horizontal
  current_replicas?: number
  desired_replicas?: number
  decision?: ScaleDecision
  // vertical
  container_name?: string
  workload_kind?: string
  current_cpu?: string
  recommended_cpu?: string
  current_memory?: string
  recommended_memory?: string
  confidence?: number
  savings_per_hour?: number
  savings_per_month?: number
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

export interface MetricsDataPoint {
  timestamp: string
  usage_cpu: number
  request_cpu: number
  usage_memory: number
  request_memory: number
}

export const api = {
  health: () => fetch('/health').then(r => r.ok),
  pods: () => get<PodInfo[]>('/api/pods'),
  metricsHistory: (namespace: string, deployment: string) =>
    get<MetricsDataPoint[]>(`/api/metrics-history?namespace=${namespace}&deployment=${deployment}`),
  forecasts: () => get<ForecastEntry[]>('/api/forecasts'),
  scalingHistory: () => get<ScalingRecord[]>('/api/scaling-history'),
  optimizerConfigs: () => get<OptimizerConfigSummary[]>('/api/optimizer-configs'),
  dryRunPending: () => get<DryRunDecision[]>('/api/dry-run/pending'),
  approve: (id: string) => post<DryRunDecision>('/api/dry-run/approve', { id }),
  reject: (id: string) => post<DryRunDecision>('/api/dry-run/reject', { id }),
  scale: (namespace: string, deployment_name: string, replicas: number, reason = 'Manual') =>
    post<ScalingRecord>('/api/scale', { namespace, deployment_name, replicas, reason }),
}
