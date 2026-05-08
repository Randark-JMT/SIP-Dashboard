import { useEffect, useRef, useCallback } from 'react'
import type { WSEvent } from '../types/call'

type Handler = (event: WSEvent) => void

export function useWebSocket(onMessage: Handler) {
  const wsRef = useRef<WebSocket | null>(null)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage

  const connect = useCallback(() => {
    const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${protocol}://${location.host}/ws/events`)
    wsRef.current = ws

    ws.onopen = () => console.log('[WS] connected')

    ws.onmessage = (e) => {
      try {
        const event: WSEvent = JSON.parse(e.data)
        onMessageRef.current(event)
      } catch {
        console.warn('[WS] non-JSON message', e.data)
      }
    }

    ws.onclose = () => {
      console.log('[WS] disconnected, reconnecting in 3s...')
      setTimeout(connect, 3000)
    }

    ws.onerror = (err) => {
      console.error('[WS] error', err)
      ws.close()
    }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      wsRef.current?.close()
    }
  }, [connect])
}
