import type { ListCallsResponse } from '../types/call'

const BASE = '/api'

export async function fetchCalls(params: {
  page?: number
  limit?: number
  status?: string
}): Promise<ListCallsResponse> {
  const p = new URLSearchParams()
  if (params.page) p.set('page', String(params.page))
  if (params.limit) p.set('limit', String(params.limit))
  if (params.status) p.set('status', params.status)

  const res = await fetch(`${BASE}/calls?${p}`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export async function fetchActiveCalls() {
  const res = await fetch(`${BASE}/active`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export function recordingUrl(filename: string): string {
  return `${BASE}/recordings/${encodeURIComponent(filename)}`
}
