export type CallStatus = 'trying' | 'ringing' | 'connected' | 'ended' | 'cancelled' | 'active' | 'completed' | 'missed'

export interface Call {
  id: number
  callId: string
  fromNumber: string
  toNumber: string
  startTime: string
  endTime?: string
  durationSecs?: number
  recordingPath?: string
  status: CallStatus
}

export interface ActiveCall {
  callId: string
  from: string
  to: string
  status: CallStatus
  startTime: string
}

export interface WSEvent {
  type: 'CALL_START' | 'CALL_STATUS' | 'CALL_END'
  payload: ActiveCall | CallEndPayload
}

export interface CallEndPayload {
  callId: string
  duration: number
  recordingPath?: string
}

export interface ListCallsResponse {
  data: Call[]
  total: number
  page: number
  limit: number
}
