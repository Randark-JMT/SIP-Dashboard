package capture

import (
	"log"
	"net"
	"sync"

	"sip-dashboard/internal/rtp"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// RTPCapture 监听 RTP 端口范围，将 UDP 数据分发给对应的 RTP 流
type RTPCapture struct {
	iface    string
	portMin  int
	portMax  int
	stopChan chan struct{}

	mu      sync.RWMutex
	streams map[string]*rtp.Stream // key: "ip:port"（RTP 发送端地址）
}

// NewRTPCapture 创建新的 RTP 抓包器
func NewRTPCapture(iface string, portMin, portMax int) *RTPCapture {
	return &RTPCapture{
		iface:    iface,
		portMin:  portMin,
		portMax:  portMax,
		stopChan: make(chan struct{}),
		streams:  make(map[string]*rtp.Stream),
	}
}

// RegisterStream 注册一个 RTP 流，与指定远端地址绑定。
// 仅按 RTP 包的源地址匹配，这样每个 stream 只接收一个方向的音频，
// 避免同一腿的收发两个方向被合并进一个 stream。
func (c *RTPCapture) RegisterStream(addr string, stream *rtp.Stream) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streams[addr] = stream
}

// UnregisterStream 注销 RTP 流
func (c *RTPCapture) UnregisterStream(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.streams, addr)
}

// Start 启动 RTP 抓包（阻塞，建议在 goroutine 中调用）
func (c *RTPCapture) Start() error {
	handle, err := pcap.OpenLive(c.iface, 65535, false, pcap.BlockForever)
	if err != nil {
		return err
	}
	defer handle.Close()

	filter := buildRTPFilter(c.portMin, c.portMax)
	if err := handle.SetBPFFilter(filter); err != nil {
		return err
	}

	log.Printf("[RTP Capture] listening on %s ports %d-%d", c.iface, c.portMin, c.portMax)

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
func (c *RTPCapture) Stop() {
	close(c.stopChan)
}

func (c *RTPCapture) processPacket(pkt gopacket.Packet) {
	ipLayer := pkt.Layer(layers.LayerTypeIPv4)
	udpLayer := pkt.Layer(layers.LayerTypeUDP)
	if ipLayer == nil || udpLayer == nil {
		return
	}

	ip, _ := ipLayer.(*layers.IPv4)
	udp, _ := udpLayer.(*layers.UDP)

	if len(udp.Payload) == 0 {
		return
	}

	srcKey := net.JoinHostPort(ip.SrcIP.String(), udp.SrcPort.String())

	c.mu.RLock()
	stream, ok := c.streams[srcKey]
	c.mu.RUnlock()

	if !ok {
		return
	}

	stream.AddRawUDP(udp.Payload)
}

func buildRTPFilter(min, max int) string {
	return "udp and portrange " + itoa(min) + "-" + itoa(max)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
