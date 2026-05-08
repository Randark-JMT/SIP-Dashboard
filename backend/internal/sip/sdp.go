package sip

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reConnection = regexp.MustCompile(`(?m)^c=IN IP4 (\S+)`)
	reMedia      = regexp.MustCompile(`(?m)^m=audio (\d+)\s+RTP/AVP\s+([\d ]+)`)
)

// ParseSDP 从 SIP body 中提取 RTP 地址、端口和编解码信息
func ParseSDP(body string) (*SDPInfo, error) {
	connMatch := reConnection.FindStringSubmatch(body)
	if connMatch == nil {
		return nil, fmt.Errorf("no c= line found in SDP")
	}
	ip := connMatch[1]

	mediaMatch := reMedia.FindStringSubmatch(body)
	if mediaMatch == nil {
		return nil, fmt.Errorf("no m=audio line found in SDP")
	}
	port, err := strconv.Atoi(mediaMatch[1])
	if err != nil {
		return nil, fmt.Errorf("invalid media port: %w", err)
	}

	// 从 payload type 列表中找到 PCMU(0) 或 PCMA(8)
	codecPayload := 0
	for _, pt := range strings.Fields(mediaMatch[2]) {
		n, _ := strconv.Atoi(pt)
		if n == 0 || n == 8 {
			codecPayload = n
			break
		}
	}

	return &SDPInfo{
		IP:           ip,
		AudioPort:    port,
		CodecPayload: codecPayload,
	}, nil
}
