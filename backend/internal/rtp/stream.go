package rtp

import (
	"encoding/binary"
	"log"
	"sync"
)

const (
	// RTP 头部最小长度 (字节)
	rtpHeaderMinLen = 12
	// 乱序容忍窗口大小
	reorderWindowSize = 10
)

// rtpPacket 内部 RTP 包表示
type rtpPacket struct {
	seq     uint16
	payload []byte
}

// ssrcInfo 每个 SSRC 独立的序列号追踪状态
type ssrcInfo struct {
	lastSeq     uint16
	initialized bool
	buf         []rtpPacket
}

// Stream RTP 流重组器，负责按序收集 RTP payload 并解码为 PCM
// 支持同一通话中主叫和被叫两个方向（不同 SSRC）的数据
type Stream struct {
	mu          sync.Mutex
	callID      string
	payloadType int
	ssrcs       map[uint32]*ssrcInfo // 按 SSRC 独立追踪序列号
	pcmOut      chan []int16         // 输出 PCM 数据
	rawOut      chan []byte          // 输出原始 RTP payload (用于实时流)
	closed      bool                 // 是否已关闭，受 mu 保护
	closeOnce   sync.Once            // 确保通道只关闭一次
}

// NewStream 创建新的 RTP 流重组器
func NewStream(callID string, payloadType int) *Stream {
	return &Stream{
		callID:      callID,
		payloadType: payloadType,
		ssrcs:       make(map[uint32]*ssrcInfo),
		pcmOut:      make(chan []int16, 512),
		rawOut:      make(chan []byte, 512),
	}
}

// PCMOut 返回解码 PCM 数据通道
func (s *Stream) PCMOut() <-chan []int16 {
	return s.pcmOut
}

// RawOut 返回原始 RTP payload 通道（用于实时 WebSocket 流）
func (s *Stream) RawOut() <-chan []byte {
	return s.rawOut
}

// AddRawUDP 从原始 UDP payload 提取 RTP 头并处理
func (s *Stream) AddRawUDP(udpPayload []byte) {
	if len(udpPayload) < rtpHeaderMinLen {
		return
	}

	// 解析 RTP 头部
	// Byte 0: V(2) P(1) X(1) CC(4)
	// Byte 1: M(1) PT(7)
	// Bytes 2-3: Sequence Number
	// Bytes 4-7: Timestamp
	// Bytes 8-11: SSRC
	pt := int(udpPayload[1] & 0x7F)
	seq := binary.BigEndian.Uint16(udpPayload[2:4])
	ssrc := binary.BigEndian.Uint32(udpPayload[8:12])

	// CSRC 偏移
	cc := int(udpPayload[0] & 0x0F)
	offset := rtpHeaderMinLen + cc*4

	// Extension header
	if udpPayload[0]&0x10 != 0 {
		if offset+4 > len(udpPayload) {
			return
		}
		extLen := int(binary.BigEndian.Uint16(udpPayload[offset+2 : offset+4]))
		offset += 4 + extLen*4
	}

	if offset > len(udpPayload) {
		return
	}

	payload := make([]byte, len(udpPayload)-offset)
	copy(payload, udpPayload[offset:])

	// 忽略非音频 payload type（如 DTMF=101）
	if pt != 0 && pt != 8 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 按 SSRC 获取或创建独立的序列号追踪状态
	si, ok := s.ssrcs[ssrc]
	if !ok {
		si = &ssrcInfo{}
		s.ssrcs[ssrc] = si
	}

	if !si.initialized {
		si.lastSeq = seq - 1
		si.initialized = true
	}

	pkt := rtpPacket{seq: seq, payload: payload}
	si.buf = append(si.buf, pkt)

	// 尝试按序输出
	s.flushSSRC(si)
}

// flushSSRC 将指定 SSRC 缓冲区中按序的包输出
func (s *Stream) flushSSRC(si *ssrcInfo) {
	for {
		next := si.lastSeq + 1
		found := -1
		for i, p := range si.buf {
			if p.seq == next {
				found = i
				break
			}
		}
		if found < 0 {
			// 在容忍窗口内等待，超出则跳过
			if len(si.buf) >= reorderWindowSize {
				log.Printf("[RTP] stream %s: gap at seq %d, skipping", s.callID, next)
				si.buf = si.buf[1:]
				si.lastSeq = next
			}
			return
		}

		pkt := si.buf[found]
		si.buf = append(si.buf[:found], si.buf[found+1:]...)
		si.lastSeq = pkt.seq

		// 流已关闭则丢弃后续包（close 在 mu 外执行，但 closed 标志在 mu 内设置）
		if s.closed {
			return
		}

		// 输出原始 payload（用于实时流）
		rawCopy := make([]byte, len(pkt.payload))
		copy(rawCopy, pkt.payload)
		select {
		case s.rawOut <- rawCopy:
		default:
		}

		// 解码 PCM 并输出（用于 WAV 录制）
		pcm := DecodeToPCM(pkt.payload, s.payloadType)
		select {
		case s.pcmOut <- pcm:
		default:
		}
	}
}

// Close 关闭流通道，可安全多次调用
func (s *Stream) Close() {
	// 先在锁内设置 closed，使 flushSSRC 不再向通道发送
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	// 再关闭通道（只执行一次）
	s.closeOnce.Do(func() {
		close(s.pcmOut)
		close(s.rawOut)
	})
}
