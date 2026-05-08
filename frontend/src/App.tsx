import ActiveCallsPanel from './components/ActiveCallsPanel'
import CallHistoryTable from './components/CallHistoryTable'
import './index.css'

export default function App() {
  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      {/* Header */}
      <header className="bg-white border-b border-gray-200 px-6 py-4 flex items-center gap-3">
        <div className="h-8 w-8 rounded-lg bg-indigo-600 flex items-center justify-center">
          <svg className="h-4 w-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M3 5a2 2 0 012-2h3.28a1 1 0 01.948.684l1.498 4.493a1 1 0 01-.502 1.21l-2.257 1.13a11.042 11.042 0 005.516 5.516l1.13-2.257a1 1 0 011.21-.502l4.493 1.498a1 1 0 01.684.949V19a2 2 0 01-2 2h-1C9.716 21 3 14.284 3 6V5z" />
          </svg>
        </div>
        <div>
          <h1 className="text-lg font-bold text-gray-900 leading-none">SIP Dashboard</h1>
          <p className="text-xs text-gray-400 mt-0.5">FreePBX 实时通话监控</p>
        </div>
      </header>

      {/* Body */}
      <div className="flex flex-1 overflow-hidden p-6 gap-6">
        {/* Left: Active Calls */}
        <aside className="w-72 shrink-0 bg-white rounded-2xl border border-gray-200 shadow-sm p-5">
          <ActiveCallsPanel />
        </aside>

        {/* Main: History */}
        <main className="flex-1 min-w-0 flex flex-col">
          <CallHistoryTable />
        </main>
      </div>
    </div>
  )
}
