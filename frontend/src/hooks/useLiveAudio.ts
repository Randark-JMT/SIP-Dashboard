import { useEffect, useRef, useState } from 'react'

const SAMPLE_RATE = 8000

export function useLiveAudio(callId: string | null) {
  const [isPlaying, setIsPlaying] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const nextTimeRef = useRef<number>(0)

  const start = () => {
    if (!callId || isPlaying) return

    const audioCtx = new AudioContext({ sampleRate: SAMPLE_RATE })
    audioCtxRef.current = audioCtx
    nextTimeRef.current = audioCtx.currentTime + 0.1

    const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${protocol}://${location.host}/ws/audio/${encodeURIComponent(callId)}`)
    wsRef.current = ws
    ws.binaryType = 'arraybuffer'

    ws.onmessage = (e: MessageEvent<ArrayBuffer>) => {
      const int16 = new Int16Array(e.data)
      const float32 = new Float32Array(int16.length)
      for (let i = 0; i < int16.length; i++) {
        float32[i] = int16[i] / 32768.0
      }

      const buffer = audioCtx.createBuffer(1, float32.length, SAMPLE_RATE)
      buffer.copyToChannel(float32, 0)

      const source = audioCtx.createBufferSource()
      source.buffer = buffer

      // 连接增益节点以支持音量控制
      const gain = audioCtx.createGain()
      gain.gain.value = 1.0
      source.connect(gain).connect(audioCtx.destination)

      const startAt = Math.max(nextTimeRef.current, audioCtx.currentTime + 0.05)
      source.start(startAt)
      nextTimeRef.current = startAt + buffer.duration
    }

    ws.onclose = () => setIsPlaying(false)
    ws.onerror = () => ws.close()

    setIsPlaying(true)
  }

  const stop = () => {
    wsRef.current?.close()
    audioCtxRef.current?.close()
    audioCtxRef.current = null
    wsRef.current = null
    setIsPlaying(false)
  }

  useEffect(() => {
    return () => {
      wsRef.current?.close()
      audioCtxRef.current?.close()
    }
  }, [])

  return { isPlaying, start, stop }
}
