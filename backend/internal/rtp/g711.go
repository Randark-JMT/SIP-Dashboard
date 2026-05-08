package rtp

// UlawToPCM 将 G.711 μ-law (PCMU, payload type 0) 字节解码为 16-bit PCM
func UlawToPCM(ulaw byte) int16 {
	ulaw ^= 0xFF
	sign := int16(1)
	if ulaw&0x80 != 0 {
		sign = -1
		ulaw &^= 0x80
	}
	exponent := (ulaw >> 4) & 0x07
	mantissa := ulaw & 0x0F
	sample := int16((mantissa<<3)+0x84) << exponent
	return sign * (sample - 0x84)
}

// AlawToPCM 将 G.711 A-law (PCMA, payload type 8) 字节解码为 16-bit PCM
func AlawToPCM(alaw byte) int16 {
	alaw ^= 0x55
	sign := int16(1)
	if alaw&0x80 != 0 {
		sign = -1
		alaw &^= 0x80
	}
	exponent := (alaw >> 4) & 0x07
	mantissa := alaw & 0x0F
	var sample int16
	if exponent == 0 {
		sample = int16(mantissa<<1) + 1
	} else {
		sample = int16((mantissa|0x10)<<1+1) << (exponent - 1)
	}
	return sign * sample
}

// DecodeToPCM 根据 payload type 解码 RTP payload
// payloadType: 0=PCMU, 8=PCMA
func DecodeToPCM(payload []byte, payloadType int) []int16 {
	pcm := make([]int16, len(payload))
	for i, b := range payload {
		if payloadType == 8 {
			pcm[i] = AlawToPCM(b)
		} else {
			pcm[i] = UlawToPCM(b)
		}
	}
	return pcm
}
