package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return true // 内网环境，允许所有来源
	},
}

// client 代表一个 WebSocket 连接
type client struct {
	conn   *websocket.Conn
	send   chan []byte
	callID string // 非空表示仅接收指定通话的音频
	isBin  bool   // 是否为二进制音频流连接
}

// Hub 管理所有 WebSocket 连接的发布-订阅中心
type Hub struct {
	mu        sync.RWMutex
	clients   map[*client]struct{}
	broadcast chan []byte // JSON 事件广播
}

// NewHub 创建新的 Hub
func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*client]struct{}),
		broadcast: make(chan []byte, 256),
	}
}

// Run 启动 Hub 事件循环（阻塞）
func (h *Hub) Run() {
	for msg := range h.broadcast {
		h.mu.RLock()
		for c := range h.clients {
			if !c.isBin {
				select {
				case c.send <- msg:
				default:
					log.Printf("[WS Hub] client send buffer full")
				}
			}
		}
		h.mu.RUnlock()
	}
}

// BroadcastEvent 向所有事件订阅者广播 JSON 消息
func (h *Hub) BroadcastEvent(event interface{}) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[WS Hub] marshal error: %v", err)
		return
	}
	select {
	case h.broadcast <- data:
	default:
	}
}

// PushAudio 向指定通话的音频流推送 PCM 帧（转发给所有订阅该 callID 的 WebSocket 客户端）
func (h *Hub) PushAudio(callID string, pcm []byte) {
	h.mu.RLock()
	for c := range h.clients {
		if c.isBin && c.callID == callID {
			select {
			case c.send <- pcm:
			default:
			}
		}
	}
	h.mu.RUnlock()
}

// ServeEvents 处理事件 WebSocket 连接
func (h *Hub) ServeEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] upgrade error: %v", err)
		return
	}

	c := &client{conn: conn, send: make(chan []byte, 256), isBin: false}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	go c.writePump()
	c.readPump() // 阻塞直到连接断开

	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.send)
}

// ServeAudio 处理实时音频 WebSocket 连接
func (h *Hub) ServeAudio(w http.ResponseWriter, r *http.Request, callID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS Audio] upgrade error: %v", err)
		return
	}

	c := &client{conn: conn, send: make(chan []byte, 1024), callID: callID, isBin: true}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	go c.writePump()
	c.readPump()

	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.send)
}

// writePump 将 send channel 中的消息写入 WebSocket
func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		msgType := websocket.TextMessage
		if c.isBin {
			msgType = websocket.BinaryMessage
		}
		if err := c.conn.WriteMessage(msgType, msg); err != nil {
			return
		}
	}
}

// readPump 消费来自客户端的消息（仅用于保持连接，忽略内容）
func (c *client) readPump() {
	defer c.conn.Close()
	c.conn.SetReadLimit(512)
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
	}
}
