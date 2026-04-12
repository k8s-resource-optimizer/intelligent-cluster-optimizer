import { useEffect, useState } from 'react'
import { api, type OptimizerConfigSummary } from '../api'
import { toast } from '../components/Toast'

interface DeploymentOption {
  namespace: string
  name: string
}

export function ManualScalePage() {
  const [options, setOptions] = useState<DeploymentOption[]>([])
  const [selected, setSelected] = useState('')
  const [namespace, setNamespace] = useState('')
  const [deploymentName, setDeploymentName] = useState('')
  const [replicas, setReplicas] = useState<number | ''>('')
  const [reason, setReason] = useState('Manual')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    api.optimizerConfigs()
      .then((configs: OptimizerConfigSummary[]) => {
        const opts = configs.map(c => ({ namespace: c.namespace, name: c.name }))
        setOptions(opts)
        if (opts.length > 0) {
          const first = opts[0]
          setSelected(`${first.namespace}/${first.name}`)
          setNamespace(first.namespace)
          setDeploymentName(first.name)
        }
      })
      .catch(() => {
        // configs unavailable — user fills fields manually
      })
  }, [])

  const onSelectChange = (val: string) => {
    setSelected(val)
    if (val === '__manual__') {
      setNamespace('')
      setDeploymentName('')
      return
    }
    const opt = options.find(o => `${o.namespace}/${o.name}` === val)
    if (opt) {
      setNamespace(opt.namespace)
      setDeploymentName(opt.name)
    }
  }

  const handleScale = async () => {
    if (!namespace.trim() || !deploymentName.trim()) {
      toast('Namespace and deployment name are required', 'error')
      return
    }
    if (replicas === '' || replicas < 0) {
      toast('Enter a valid replica count (≥ 0)', 'error')
      return
    }
    setLoading(true)
    try {
      await api.scale(namespace.trim(), deploymentName.trim(), replicas, reason.trim() || 'Manual')
      toast(`Scaled ${deploymentName} to ${replicas} replicas`, 'success')
      setReplicas('')
    } catch (e) {
      toast(`Scale failed: ${e}`, 'error')
    } finally {
      setLoading(false)
    }
  }

  const isManual = selected === '__manual__' || options.length === 0

  return (
    <div>
      <h2>Manual Scale</h2>

      <div style={{
        background: '#0d1117',
        border: '1px solid #1e2535',
        borderRadius: 8,
        padding: 28,
        maxWidth: 480,
      }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>

          {options.length > 0 && (
            <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <span style={{ fontSize: 12, fontWeight: 600, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                Deployment
              </span>
              <select
                value={selected}
                onChange={e => onSelectChange(e.target.value)}
                style={inputStyle}
              >
                {options.map(o => (
                  <option key={`${o.namespace}/${o.name}`} value={`${o.namespace}/${o.name}`}>
                    {o.namespace} / {o.name}
                  </option>
                ))}
                <option value="__manual__">Enter manually…</option>
              </select>
            </label>
          )}

          {isManual && (
            <>
              <label style={labelStyle}>
                <span style={labelTextStyle}>Namespace</span>
                <input
                  value={namespace}
                  onChange={e => setNamespace(e.target.value)}
                  placeholder="default"
                  style={inputStyle}
                />
              </label>
              <label style={labelStyle}>
                <span style={labelTextStyle}>Deployment Name</span>
                <input
                  value={deploymentName}
                  onChange={e => setDeploymentName(e.target.value)}
                  placeholder="my-deployment"
                  style={inputStyle}
                />
              </label>
            </>
          )}

          <label style={labelStyle}>
            <span style={labelTextStyle}>Replica Count</span>
            <input
              type="number"
              min={0}
              value={replicas}
              onChange={e => setReplicas(e.target.value === '' ? '' : parseInt(e.target.value, 10))}
              placeholder="3"
              style={inputStyle}
            />
          </label>

          <label style={labelStyle}>
            <span style={labelTextStyle}>Reason (optional)</span>
            <input
              value={reason}
              onChange={e => setReason(e.target.value)}
              placeholder="Manual"
              style={inputStyle}
            />
          </label>

          <button
            onClick={handleScale}
            disabled={loading}
            style={{
              marginTop: 4,
              padding: '10px 0',
              borderRadius: 6,
              border: 'none',
              background: loading ? '#1e2535' : '#7c3aed',
              color: loading ? '#64748b' : '#fff',
              fontWeight: 700,
              fontSize: 14,
              transition: 'background 0.15s',
            }}
            onMouseEnter={e => { if (!loading) (e.target as HTMLButtonElement).style.background = '#6d28d9' }}
            onMouseLeave={e => { if (!loading) (e.target as HTMLButtonElement).style.background = '#7c3aed' }}
          >
            {loading ? 'Scaling…' : 'Scale'}
          </button>
        </div>
      </div>
    </div>
  )
}

const labelStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 6,
}

const labelTextStyle: React.CSSProperties = {
  fontSize: 12,
  fontWeight: 600,
  color: '#94a3b8',
  textTransform: 'uppercase',
  letterSpacing: '0.04em',
}

const inputStyle: React.CSSProperties = {
  padding: '8px 12px',
  borderRadius: 5,
  border: '1px solid #1e2535',
  background: '#141824',
  color: '#f1f5f9',
  outline: 'none',
  width: '100%',
}
