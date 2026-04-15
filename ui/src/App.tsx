import { useState } from 'react'
import { Layout } from './components/Layout'
import { ToastContainer } from './components/Toast'
import { PodsPage } from './pages/PodsPage'
import { ForecastsPage } from './pages/ForecastsPage'
import { ScalingHistoryPage } from './pages/ScalingHistoryPage'
import { DryRunPage } from './pages/DryRunPage'
import { ManualScalePage } from './pages/ManualScalePage'
import { ResourceChartPage } from './pages/ResourceChartPage'

type Page = 'pods' | 'forecasts' | 'scaling-history' | 'dry-run' | 'manual-scale' | 'resource-chart'

export default function App() {
  const [page, setPage] = useState<Page>('pods')

  const content = {
    'pods':            <PodsPage />,
    'forecasts':       <ForecastsPage />,
    'scaling-history': <ScalingHistoryPage />,
    'dry-run':         <DryRunPage />,
    'manual-scale':    <ManualScalePage />,
    'resource-chart':  <ResourceChartPage />,
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
