package capture

import (
	"log"

	"github.com/emiago/sipgo/sip"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// SIPCapture 在指定网络接口上监听 UDP 5060 端口，解析 SIP 消息并送往 channel
type SIPCapture struct {
	iface    string
	msgChan  chan sip.Message
	stopChan chan struct{}
}

// NewSIPCapture 创建新的 SIP 抓包器
func NewSIPCapture(iface string, msgChan chan sip.Message) *SIPCapture {
	return &SIPCapture{
		iface:    iface,
		msgChan:  msgChan,
		stopChan: make(chan struct{}),
	}
}

// Start 启动抓包（阻塞，建议在 goroutine 中调用）
func (c *SIPCapture) Start() error {
	handle, err := pcap.OpenLive(c.iface, 65535, false, pcap.BlockForever)
	if err != nil {
		return err
	}
	defer handle.Close()

	if err := handle.SetBPFFilter("udp port 5060"); err != nil {
		return err
	}

	log.Printf("[SIP Capture] listening on %s:5060", c.iface)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packets := packetSource.Packets()

	for {
		select {
		case <-c.stopChan:
			return nil
		case pkt, ok := <-packets:
			if !ok {
				return nil
			}
			c.processPacket(pkt)
		}
	}
}

// Stop 停止抓包
func (c *SIPCapture) Stop() {
	close(c.stopChan)
}

func (c *SIPCapture) processPacket(pkt gopacket.Packet) {
	udpLayer := pkt.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return
	}
	udp, _ := udpLayer.(*layers.UDP)
	payload := udp.Payload
	if len(payload) == 0 {
		return
	}

	msg, err := sip.ParseMessage(payload)
	if err != nil {
		// 非 SIP 消息静默忽略
		return
	}

	select {
	case c.msgChan <- msg:
	default:
		log.Printf("[SIP Capture] msg channel full, dropping packet")
	}
}
