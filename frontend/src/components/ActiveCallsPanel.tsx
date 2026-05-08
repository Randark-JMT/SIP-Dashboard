import { useState, useEffect } from "react"
import type { ActiveCall, WSEvent, CallEndPayload } from "../types/call"
import { useWebSocket } from "../hooks/useWebSocket"
import { useLiveAudio } from "../hooks/useLiveAudio"
import dayjs from "dayjs"

function useElapsed(startTime: string) {
  const [elapsed, setElapsed] = useState(0)
  useEffect(() => {
    const start = dayjs(startTime).valueOf()
    const timer = setInterval(() => {
      setElapsed(Math.floor((Date.now() - start) / 1000))
    }, 1000)
    return () => clearInterval(timer)
  }, [startTime])
  const m = Math.floor(elapsed / 60).toString().padStart(2, "0")
  const s = (elapsed % 60).toString().padStart(2, "0")
  return `${m}:${s}`
}

const STATUS_COLORS: Record<string, string> = {
  trying: "bg-yellow-100 text-yellow-800",
  ringing: "bg-blue-100 text-blue-800",
  connected: "bg-green-100 text-green-800",
  ended: "bg-gray-100 text-gray-600",
  cancelled: "bg-red-100 text-red-700",
}

const STATUS_LABELS: Record<string, string> = {
  trying: "拨号中",
  ringing: "振铃",
  connected: "通话中",
  ended: "已结束",
  cancelled: "已取消",
}

function CallCard({ call }: { call: ActiveCall }) {
  const elapsed = useElapsed(call.startTime)
  const { isPlaying, start, stop } = useLiveAudio(call.callId)

  return (
    <div className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
      <div className="flex items-center justify-between mb-3">
        <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_COLORS[call.status] ?? "bg-gray-100 text-gray-600"}`}>
          {call.status === "connected" && (
            <span className="mr-1 h-2 w-2 rounded-full bg-green-500 animate-pulse inline-block" />
          )}
          {STATUS_LABELS[call.status] ?? call.status}
        </span>
        <span className="font-mono text-sm text-gray-500">{elapsed}</span>
      </div>

      <div className="space-y-1 mb-4">
        <div className="flex items-center gap-2 text-sm">
          <span className="text-gray-400 w-10">主叫</span>
          <span className="font-medium text-gray-800">{call.from}</span>
        </div>
        <div className="flex items-center gap-2 text-sm">
          <span className="text-gray-400 w-10">被叫</span>
          <span className="font-medium text-gray-800">{call.to}</span>
        </div>
      </div>

      {call.status === "connected" && (
        <button
          onClick={isPlaying ? stop : start}
          className={`w-full rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
            isPlaying
              ? "bg-red-50 text-red-600 hover:bg-red-100"
              : "bg-indigo-50 text-indigo-600 hover:bg-indigo-100"
          }`}
        >
          {isPlaying ? "■ 停止监听" : "▶ 实时监听"}
        </button>
      )}
    </div>
  )
}

export default function ActiveCallsPanel() {
  const [calls, setCalls] = useState<Map<string, ActiveCall>>(new Map())

  const handleEvent = (event: WSEvent) => {
    if (event.type === "CALL_START") {
      const p = event.payload as ActiveCall
      setCalls(prev => new Map(prev).set(p.callId, p))
    } else if (event.type === "CALL_STATUS") {
      const p = event.payload as ActiveCall
      setCalls(prev => {
        const next = new Map(prev)
        if (next.has(p.callId)) {
          next.set(p.callId, { ...next.get(p.callId)!, status: p.status })
        }
        return next
      })
    } else if (event.type === "CALL_END") {
      const p = event.payload as CallEndPayload
      setCalls(prev => {
        const next = new Map(prev)
        next.delete(p.callId)
        return next
      })
    }
  }

  useWebSocket(handleEvent)

  const callList = Array.from(calls.values())

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-base font-semibold text-gray-800">活跃通话</h2>
        <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
          {callList.length} 路
        </span>
      </div>

      {callList.length === 0 ? (
        <div className="flex flex-1 items-center justify-center text-sm text-gray-400">
          暂无通话
        </div>
      ) : (
        <div className="space-y-3 overflow-y-auto">
          {callList.map(call => (
            <CallCard key={call.callId} call={call} />
          ))}
        </div>
      )}
    </div>
  )
}
