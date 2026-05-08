package sip

import "time"

// CallStatus 通话状态
type CallStatus string

const (
	StatusTrying    CallStatus = "trying"
	StatusRinging   CallStatus = "ringing"
	StatusConnected CallStatus = "connected"
	StatusEnded     CallStatus = "ended"
	StatusCancelled CallStatus = "cancelled"
)

// CallSession 代表一个 SIP 通话会话
type CallSession struct {
	CallID        string
	FromNumber    string
	ToNumber      string
	FromTag       string
	ToTag         string
	Status        CallStatus
	StartTime     time.Time
	ConnectTime   *time.Time
	EndTime       *time.Time
	RTPAddressA   string // 主叫 RTP 地址:端口
	RTPAddressB   string // 被叫 RTP 地址:端口
	CodecPayload  int    // 0=PCMU, 8=PCMA
	RecordingPath string
	LastSeq       int
}

// SDPInfo SDP 解析结果
type SDPInfo struct {
	IP           string
	AudioPort    int
	CodecPayload int // 0=PCMU, 8=PCMA
}

// SIPEvent SIP 事件，送往 WebSocket Hub
type SIPEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// ActiveCallPayload 活跃通话信息
type ActiveCallPayload struct {
	CallID    string     `json:"callId"`
	From      string     `json:"from"`
	To        string     `json:"to"`
	Status    CallStatus `json:"status"`
	StartTime time.Time  `json:"startTime"`
}

// CallEndPayload 通话结束信息
type CallEndPayload struct {
	CallID        string `json:"callId"`
	Duration      int    `json:"duration"`
	RecordingPath string `json:"recordingPath,omitempty"`
}
