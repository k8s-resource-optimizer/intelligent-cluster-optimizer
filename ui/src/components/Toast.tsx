import { useEffect, useState, useCallback } from 'react'

export type ToastKind = 'success' | 'error' | 'info'

export interface ToastMessage {
  id: number
  text: string
  kind: ToastKind
}

let nextId = 1

type Listener = (msg: ToastMessage) => void
const listeners: Listener[] = []

export function toast(text: string, kind: ToastKind = 'info') {
  const msg: ToastMessage = { id: nextId++, text, kind }
  listeners.forEach(l => l(msg))
}

export function ToastContainer() {
  const [messages, setMessages] = useState<ToastMessage[]>([])

  const add = useCallback((msg: ToastMessage) => {
    setMessages(prev => [...prev, msg])
    setTimeout(() => {
      setMessages(prev => prev.filter(m => m.id !== msg.id))
    }, 4000)
  }, [])

  useEffect(() => {
    listeners.push(add)
    return () => {
      const idx = listeners.indexOf(add)
      if (idx !== -1) listeners.splice(idx, 1)
    }
  }, [add])

  if (messages.length === 0) return null

  return (
    <div style={{
      position: 'fixed',
      bottom: 24,
      right: 24,
      display: 'flex',
      flexDirection: 'column',
      gap: 8,
      zIndex: 9999,
    }}>
      {messages.map(m => (
        <div key={m.id} style={{
          padding: '12px 18px',
          borderRadius: 8,
          maxWidth: 360,
          fontSize: 13,
          fontWeight: 500,
          boxShadow: '0 4px 16px rgba(0,0,0,0.4)',
          background: m.kind === 'success' ? '#166534'
            : m.kind === 'error' ? '#7f1d1d'
            : '#1e3a5f',
          color: m.kind === 'success' ? '#bbf7d0'
            : m.kind === 'error' ? '#fecaca'
            : '#bfdbfe',
          border: `1px solid ${m.kind === 'success' ? '#15803d'
            : m.kind === 'error' ? '#991b1b'
            : '#1d4ed8'}`,
          animation: 'slideIn 0.2s ease',
        }}>
          {m.text}
        </div>
      ))}
      <style>{`
        @keyframes slideIn {
          from { opacity: 0; transform: translateY(8px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}
