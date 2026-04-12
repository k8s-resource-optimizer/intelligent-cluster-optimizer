import { useState } from 'react'
import { Layout } from './components/Layout'
import { ToastContainer } from './components/Toast'
import { PodsPage } from './pages/PodsPage'
import { ForecastsPage } from './pages/ForecastsPage'
import { ScalingHistoryPage } from './pages/ScalingHistoryPage'
import { DryRunPage } from './pages/DryRunPage'
import { ManualScalePage } from './pages/ManualScalePage'

type Page = 'pods' | 'forecasts' | 'scaling-history' | 'dry-run' | 'manual-scale'

export default function App() {
  const [page, setPage] = useState<Page>('pods')

  const content = {
    'pods':            <PodsPage />,
    'forecasts':       <ForecastsPage />,
    'scaling-history': <ScalingHistoryPage />,
    'dry-run':         <DryRunPage />,
    'manual-scale':    <ManualScalePage />,
  }[page]

  return (
    <>
      <Layout page={page} onNavigate={setPage}>
        {content}
      </Layout>
      <ToastContainer />
    </>
  )
}
