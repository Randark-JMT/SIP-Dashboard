package sip

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// StateMachine 管理所有活跃通话的状态
type StateMachine struct {
	mu       sync.RWMutex
	sessions map[string]*CallSession // key: Call-ID
	events   chan SIPEvent
}

// NewStateMachine 创建新的状态机实例
func NewStateMachine(eventCh chan SIPEvent) *StateMachine {
	return &StateMachine{
		sessions: make(map[string]*CallSession),
		events:   eventCh,
	}
}

// HandleMessage 处理解析后的 SIP 消息
func (sm *StateMachine) HandleMessage(msg sip.Message) {
	switch m := msg.(type) {
	case *sip.Request:
		sm.handleRequest(m)
	case *sip.Response:
		sm.handleResponse(m)
	}
}

func (sm *StateMachine) handleRequest(req *sip.Request) {
	callID := req.CallID().Value()
	method := req.Method

	switch method {
	case sip.INVITE:
		sm.handleInvite(req, callID)
	case sip.BYE:
		sm.handleBye(callID)
	case sip.CANCEL:
		sm.handleCancel(callID)
	case sip.ACK:
		// ACK 之后通话正式建立，状态已在 200 OK 时设置
	}
}

func (sm *StateMachine) handleResponse(resp *sip.Response) {
	callID := resp.CallID().Value()
	code := resp.StatusCode

	sm.mu.Lock()
	session, exists := sm.sessions[callID]
	sm.mu.Unlock()

	if !exists {
		return
	}

	switch {
	case code == 100:
		sm.updateStatus(session, StatusTrying)
	case code == 180 || code == 183:
		sm.updateStatus(session, StatusRinging)
	case code == 200:
		// 若已经是 connected 状态（B2BUA 两腿各自发 200 OK），忽略重复处理
		sm.mu.RLock()
		alreadyConnected := session.Status == StatusConnected
		sm.mu.RUnlock()
		if alreadyConnected {
			return
		}
		// 200 OK 可能携带 SDP（被叫侧的 RTP 信息）
		body := string(resp.Body())
		if body != "" {
			sdp, err := ParseSDP(body)
			if err == nil {
				sm.mu.Lock()
				// 仅在尚未设置时才写入，避免 B2BUA 第二腿覆盖第一腿的地址
				if session.RTPAddressB == "" {
					session.RTPAddressB = fmt.Sprintf("%s:%d", sdp.IP, sdp.AudioPort)
				}
				sm.mu.Unlock()
				log.Printf("[SIP] CallID=%s callee RTP=%s codec=%d", callID, session.RTPAddressB, sdp.CodecPayload)
			}
		}
		now := time.Now()
		sm.mu.Lock()
		session.ConnectTime = &now
		sm.mu.Unlock()
		sm.updateStatus(session, StatusConnected)

	case code >= 400:
		sm.terminateSession(callID, StatusEnded)
	}
}

func (sm *StateMachine) handleInvite(req *sip.Request, callID string) {
	fromHeader := req.From()
	toHeader := req.To()

	fromNumber := extractNumber(fromHeader.Address.User)
	toNumber := extractNumber(toHeader.Address.User)

	sm.mu.Lock()
	session, exists := sm.sessions[callID]
	if !exists {
		// 检查是否已有相同主被叫号码的活跃会话（B2BUA 两腿去重）
		// 在 0.5 毫秒时间窗口内，From+To 或 To+From 相同的视为同一通话
		const dedupeWindow = 500 * time.Millisecond
		for existingID, s := range sm.sessions {
			if existingID == callID {
				continue
			}
			sameParties := (s.FromNumber == fromNumber && s.ToNumber == toNumber) ||
				(s.FromNumber == toNumber && s.ToNumber == fromNumber)
			if sameParties && time.Since(s.StartTime) <= dedupeWindow {
				// 将新 Call-ID 映射到已有会话，忽略此 INVITE
				sm.sessions[callID] = s
				log.Printf("[SIP] B2BUA dedup: CallID=%s merged into existing CallID=%s", callID, existingID)
				sm.mu.Unlock()
				return
			}
		}
		session = &CallSession{
			CallID:     callID,
			FromNumber: fromNumber,
			ToNumber:   toNumber,
			FromTag:    fromHeader.Params.String(),
			Status:     StatusTrying,
			StartTime:  time.Now(),
		}
		sm.sessions[callID] = session
	}
	sm.mu.Unlock()

	// 从 INVITE body 提取主叫 SDP（RTP 端点）
	body := string(req.Body())
	if body != "" {
		sdp, err := ParseSDP(body)
		if err == nil {
			sm.mu.Lock()
			session.RTPAddressA = fmt.Sprintf("%s:%d", sdp.IP, sdp.AudioPort)
			session.CodecPayload = sdp.CodecPayload
			sm.mu.Unlock()
			log.Printf("[SIP] CallID=%s caller RTP=%s codec=%d", callID, session.RTPAddressA, sdp.CodecPayload)
		}
	}

	if !exists {
		sm.emit(SIPEvent{
			Type: "CALL_START",
			Payload: ActiveCallPayload{
				CallID:    callID,
				From:      fromNumber,
				To:        toNumber,
				Status:    StatusTrying,
				StartTime: session.StartTime,
			},
		})
	}
}

func (sm *StateMachine) handleBye(callID string) {
	sm.terminateSession(callID, StatusEnded)
}

func (sm *StateMachine) handleCancel(callID string) {
	sm.terminateSession(callID, StatusCancelled)
}

func (sm *StateMachine) updateStatus(session *CallSession, status CallStatus) {
	sm.mu.Lock()
	session.Status = status
	callID := session.CallID
	from := session.FromNumber
	to := session.ToNumber
	start := session.StartTime
	sm.mu.Unlock()

	sm.emit(SIPEvent{
		Type: "CALL_STATUS",
		Payload: ActiveCallPayload{
			CallID:    callID,
			From:      from,
			To:        to,
			Status:    status,
			StartTime: start,
		},
	})
}

func (sm *StateMachine) terminateSession(callID string, status CallStatus) {
	sm.mu.Lock()
	session, exists := sm.sessions[callID]
	if !exists {
		sm.mu.Unlock()
		return
	}
	now := time.Now()
	session.EndTime = &now
	session.Status = status
	duration := 0
	if session.ConnectTime != nil {
		duration = int(now.Sub(*session.ConnectTime).Seconds())
	}
	recordingPath := session.RecordingPath
	// 删除所有指向同一 session 的 Call-ID（B2BUA 合并场景）
	for k, s := range sm.sessions {
		if s == session {
			delete(sm.sessions, k)
		}
	}
	sm.mu.Unlock()

	sm.emit(SIPEvent{
		Type: "CALL_END",
		Payload: CallEndPayload{
			CallID:        callID,
			Duration:      duration,
			RecordingPath: recordingPath,
		},
	})
}

// SetRecordingPath 在录音完成后更新路径
func (sm *StateMachine) SetRecordingPath(callID, path string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[callID]; ok {
		s.RecordingPath = path
	}
}

// GetActiveSessions 返回所有当前活跃通话
func (sm *StateMachine) GetActiveSessions() []*CallSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]*CallSession, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		cp := *s
		result = append(result, &cp)
	}
	return result
}

// GetSession 按 Call-ID 获取会话
func (sm *StateMachine) GetSession(callID string) (*CallSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[callID]
	if ok {
		cp := *s
		return &cp, true
	}
	return nil, false
}

func (sm *StateMachine) emit(event SIPEvent) {
	select {
	case sm.events <- event:
	default:
		log.Printf("[SM] event channel full, dropping event: %s", event.Type)
	}
}

// extractNumber 从 SIP URI 用户部分提取电话号码
func extractNumber(user string) string {
	// 去掉 sip: 前缀
	user = strings.TrimPrefix(user, "sip:")
	// 去掉 @domain 部分
	if idx := strings.Index(user, "@"); idx >= 0 {
		user = user[:idx]
	}
	return user
}
