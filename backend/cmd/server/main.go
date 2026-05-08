package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"sip-dashboard/internal/api"
	"sip-dashboard/internal/audio"
	"sip-dashboard/internal/capture"
	"sip-dashboard/internal/rtp"
	sipmod "sip-dashboard/internal/sip"
	"sip-dashboard/internal/store"
	"sip-dashboard/internal/ws"

	"github.com/emiago/sipgo/sip"
	"github.com/gin-gonic/gin"
)

func main() {
	iface := flag.String("interface", "eth0", "Network interface to capture on")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "sip-dashboard.db", "SQLite database path")
	recDir := flag.String("recordings", "recordings", "Directory to store WAV recordings")
	rtpMin := flag.Int("rtp-min", 10000, "RTP port range minimum")
	rtpMax := flag.Int("rtp-max", 20000, "RTP port range maximum")
	flag.Parse()

	// 确保录音目录存在
	if err := os.MkdirAll(*recDir, 0755); err != nil {
		log.Fatalf("failed to create recordings dir: %v", err)
	}

	// 打开数据库
	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// 初始化组件
	hub := ws.NewHub()
	eventChan := make(chan sipmod.SIPEvent, 256)
	sm := sipmod.NewStateMachine(eventChan)

	rtpCapture := capture.NewRTPCapture(*iface, *rtpMin, *rtpMax)

	apiServer := api.NewServer(db, hub, sm, *recDir)

	// Gin 引擎
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	apiServer.RegisterRoutes(r)

	// 静态文件服务（嵌入的前端构建产物）
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("embed static sub: %v", err)
	}
	r.NoRoute(func(c *gin.Context) {
		// API 路径不回退到 SPA
		if strings.HasPrefix(c.Request.URL.Path, "/api/") ||
			strings.HasPrefix(c.Request.URL.Path, "/ws/") {
			c.Status(http.StatusNotFound)
			return
		}
		http.FileServer(http.FS(staticFS)).ServeHTTP(c.Writer, c.Request)
	})

	// 启动 WebSocket Hub
	go hub.Run()

	// 启动 SIP 抓包
	sipMsgChan := make(chan sip.Message, 256)
	sipCap := capture.NewSIPCapture(*iface, sipMsgChan)
	go func() {
		if err := sipCap.Start(); err != nil {
			log.Fatalf("SIP capture failed: %v", err)
		}
	}()

	// 启动 RTP 抓包
	go func() {
		if err := rtpCapture.Start(); err != nil {
			log.Fatalf("RTP capture failed: %v", err)
		}
	}()

	// 处理 SIP 消息
	go func() {
		for msg := range sipMsgChan {
			sm.HandleMessage(msg)
		}
	}()

	// 处理 SIP 事件（广播到 WebSocket + 写数据库 + 管理 RTP 流）
	go func() {
		// callID -> RTP stream
		rtpStreams := make(map[string]*rtp.Stream)
		// callID -> PCM buffer
		pcmBuffers := make(map[string][]int16)

		for event := range eventChan {
			hub.BroadcastEvent(event)

			switch event.Type {
			case "CALL_START":
				payload := event.Payload.(sipmod.ActiveCallPayload)
				// 写数据库
				db.CreateCall(&store.Call{
					CallID:     payload.CallID,
					FromNumber: payload.From,
					ToNumber:   payload.To,
					StartTime:  payload.StartTime,
					Status:     "active",
				})
				pcmBuffers[payload.CallID] = nil

			case "CALL_STATUS":
				// 当通话进入 connected 状态时，注册 RTP 流
				payload := event.Payload.(sipmod.ActiveCallPayload)
				if payload.Status == sipmod.StatusConnected {
					sess, ok := sm.GetSession(payload.CallID)
					if ok && sess.RTPAddressB != "" {
						stream := rtp.NewStream(payload.CallID, sess.CodecPayload)
						rtpStreams[payload.CallID] = stream
						rtpCapture.RegisterStream(sess.RTPAddressB, stream)
						if sess.RTPAddressA != "" {
							rtpCapture.RegisterStream(sess.RTPAddressA, stream)
						}
						log.Printf("[Main] RTP stream registered for call %s", payload.CallID)

						// 消费 PCM 数据
						go func(callID string, s *rtp.Stream) {
							for pcm := range s.PCMOut() {
								pcmBuffers[callID] = append(pcmBuffers[callID], pcm...)
								// 同时推送实时音频（int16 little-endian bytes）
								bts := int16SliceToBytes(pcm)
								hub.PushAudio(callID, bts)
							}
						}(payload.CallID, stream)
					}
				}

			case "CALL_END":
				payload := event.Payload.(sipmod.CallEndPayload)
				callID := payload.CallID

				// 关闭 RTP 流
				if stream, ok := rtpStreams[callID]; ok {
					stream.Close()
					delete(rtpStreams, callID)
				}
				hub.UnregisterAudioStream(callID)

				// 写 WAV 文件
				recPath := ""
				if pcm, ok := pcmBuffers[callID]; ok && len(pcm) > 0 {
					safeID := strings.ReplaceAll(callID, "@", "_")
					safeID = strings.ReplaceAll(safeID, "/", "_")
					filename := fmt.Sprintf("%s_%s.wav",
						time.Now().Format("20060102_150405"), safeID[:min(len(safeID), 20)])
					recPath = filepath.Join(*recDir, filename)
					if err := audio.WriteWAV(recPath, pcm); err != nil {
						log.Printf("[Main] write WAV error: %v", err)
						recPath = ""
					} else {
						log.Printf("[Main] WAV written: %s", recPath)
					}
					delete(pcmBuffers, callID)
				}

				// 更新数据库
				status := "completed"
				db.UpdateCallEnd(callID, time.Now(), payload.Duration, recPath, status)
			}
		}
	}()

	// HTTP 服务
	srv := &http.Server{
		Addr:    *listen,
		Handler: r,
	}

	go func() {
		log.Printf("[HTTP] Server listening on %s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[Main] Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func int16SliceToBytes(samples []int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(s))
	}
	return buf
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
