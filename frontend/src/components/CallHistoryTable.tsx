import { useState, useEffect, useCallback } from 'react'
import type { Call } from '../types/call'
import { fetchCalls } from '../api/client'
import AudioPlayer from './AudioPlayer'
import dayjs from 'dayjs'

const PAGE_SIZE = 20

const STATUS_COLORS: Record<string, string> = {
  active: 'bg-green-100 text-green-700',
  completed: 'bg-gray-100 text-gray-600',
  missed: 'bg-red-100 text-red-600',
  cancelled: 'bg-orange-100 text-orange-600',
}

const STATUS_LABELS: Record<string, string> = {
  active: '通话中',
  completed: '已完成',
  missed: '未接',
  cancelled: '已取消',
}

function formatDuration(secs?: number): string {
  if (secs == null) return '-'
  const m = Math.floor(secs / 60).toString().padStart(2, '0')
  const s = (secs % 60).toString().padStart(2, '0')
  return `${m}:${s}`
}

export default function CallHistoryTable() {
  const [calls, setCalls] = useState<Call[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('')
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetchCalls({ page, limit: PAGE_SIZE, status: statusFilter || undefined })
      setCalls(res.data ?? [])
      setTotal(res.total)
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [page, statusFilter])

  useEffect(() => { load() }, [load])

  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-base font-semibold text-gray-800">通话历史</h2>
        <div className="flex items-center gap-3">
          <select
            value={statusFilter}
            onChange={e => { setStatusFilter(e.target.value); setPage(1) }}
            className="rounded-lg border border-gray-200 px-2.5 py-1.5 text-sm text-gray-700 focus:outline-none focus:ring-2 focus:ring-indigo-300"
          >
            <option value="">全部状态</option>
            <option value="completed">已完成</option>
            <option value="active">通话中</option>
            <option value="missed">未接</option>
            <option value="cancelled">已取消</option>
          </select>
          <button
            onClick={load}
            className="rounded-lg bg-indigo-50 px-3 py-1.5 text-sm font-medium text-indigo-600 hover:bg-indigo-100 transition-colors"
          >
            刷新
          </button>
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto rounded-xl border border-gray-200 bg-white">
        <table className="min-w-full text-sm">
          <thead className="bg-gray-50 sticky top-0">
            <tr>
              {['主叫', '被叫', '开始时间', '时长', '状态', '录音'].map(h => (
                <th key={h} className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {loading ? (
              <tr>
                <td colSpan={6} className="text-center py-8 text-gray-400">加载中...</td>
              </tr>
            ) : calls.length === 0 ? (
              <tr>
                <td colSpan={6} className="text-center py-8 text-gray-400">暂无记录</td>
              </tr>
            ) : calls.map(call => (
              <tr key={call.id} className="hover:bg-gray-50 transition-colors">
                <td className="px-4 py-3 font-medium text-gray-800">{call.fromNumber}</td>
                <td className="px-4 py-3 text-gray-700">{call.toNumber}</td>
                <td className="px-4 py-3 text-gray-500">
                  {dayjs(call.startTime).format('MM-DD HH:mm:ss')}
                </td>
                <td className="px-4 py-3 font-mono text-gray-600">
                  {formatDuration(call.durationSecs)}
                </td>
                <td className="px-4 py-3">
                  <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_COLORS[call.status] ?? 'bg-gray-100 text-gray-600'}`}>
                    {STATUS_LABELS[call.status] ?? call.status}
                  </span>
                </td>
                <td className="px-4 py-3">
                  {call.recordingPath ? (
                    <AudioPlayer path={call.recordingPath} />
                  ) : (
                    <span className="text-gray-300 text-xs">-</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between mt-4">
          <span className="text-xs text-gray-400">共 {total} 条</span>
          <div className="flex gap-1">
            <button
              disabled={page === 1}
              onClick={() => setPage(p => p - 1)}
              className="rounded-lg px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed"
            >
              上一页
            </button>
            <span className="px-3 py-1.5 text-sm text-gray-700">
              {page} / {totalPages}
            </span>
            <button
              disabled={page === totalPages}
              onClick={() => setPage(p => p + 1)}
              className="rounded-lg px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed"
            >
              下一页
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
