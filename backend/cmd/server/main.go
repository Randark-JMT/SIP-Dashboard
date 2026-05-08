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
	"sync"
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

// version 由 ldflags 在构建时注入（如 -X main.version=v1.0.0）
var version = "dev"

func main() {
	iface := flag.String("interface", "eth0", "Network interface to capture on")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "sip-dashboard.db", "SQLite database path")
	recDir := flag.String("recordings", "recordings", "Directory to store WAV recordings")
	rtpMin := flag.Int("rtp-min", 10000, "RTP port range minimum")
	rtpMax := flag.Int("rtp-max", 20000, "RTP port range maximum")
	showVer := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Println("sip-dashboard", version)
		os.Exit(0)
	}

	log.Printf("[Main] SIP Dashboard %s starting", version)

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
		// callID -> RTP streams (A leg, B leg)
		type rtpPair struct{ a, b *rtp.Stream }
		rtpStreams := make(map[string]rtpPair)
		// callID -> PCM 完成通道（goroutine 结束后写入完整 PCM 数据）
		pcmDones := make(map[string]chan []int16)

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

			case "CALL_STATUS":
				// 当通话进入 connected 状态时，注册 RTP 流
				payload := event.Payload.(sipmod.ActiveCallPayload)
				if payload.Status == sipmod.StatusConnected {
					sess, ok := sm.GetSession(payload.CallID)
					// 防止 B2BUA 两腿 200 OK 导致重复注册
					if _, alreadyRegistered := rtpStreams[payload.CallID]; alreadyRegistered {
						break
					}
					if ok && sess.RTPAddressB != "" {
						streamB := rtp.NewStream(payload.CallID, sess.CodecPayload)
						streamA := rtp.NewStream(payload.CallID, sess.CodecPayload)
						rtpStreams[payload.CallID] = rtpPair{a: streamA, b: streamB}
						rtpCapture.RegisterStream(sess.RTPAddressB, streamB)
						if sess.RTPAddressA != "" {
							rtpCapture.RegisterStream(sess.RTPAddressA, streamA)
						}
						log.Printf("[Main] RTP stream registered for call %s", payload.CallID)

						// 消费 PCM 数据：本地积累，完成后写入 done channel，避免与主循环竞争
						doneCh := make(chan []int16, 1)
						pcmDones[payload.CallID] = doneCh
						go func(callID string, sA, sB *rtp.Stream, done chan<- []int16) {
							var pcmA, pcmB []int16
							var wg sync.WaitGroup
							wg.Add(2)
							go func() {
								defer wg.Done()
								for pcm := range sA.PCMOut() {
									pcmA = append(pcmA, pcm...)
									// 实时推送 A 路（交错立体声左声道）
									hub.PushAudio(callID, int16SliceToBytes(pcm))
								}
							}()
							go func() {
								defer wg.Done()
								for pcm := range sB.PCMOut() {
									pcmB = append(pcmB, pcm...)
									// 实时推送 B 路（交错立体声右声道）
									hub.PushAudio(callID, int16SliceToBytes(pcm))
								}
							}()
							wg.Wait()
							// 将两路 PCM 合并为单条 []int16 传给 done：
							// 前半段 = pcmA，后半段 = pcmB，由 WriteWAVStereo 负责交错
							combined := append(pcmA, pcmB...)
							done <- combined
						}(payload.CallID, streamA, streamB, doneCh)
					}
				}

			case "CALL_END":
				payload := event.Payload.(sipmod.CallEndPayload)
				callID := payload.CallID

				// 关闭 RTP 流（会关闭 PCMOut channel，令 PCM goroutine 退出）
				if pair, ok := rtpStreams[callID]; ok {
					pair.a.Close()
					pair.b.Close()
					delete(rtpStreams, callID)
				}

				// 取出 done channel，在独立 goroutine 中等待 PCM 数据就绪后写 WAV 并更新数据库
				var doneCh <-chan []int16
				if ch, ok := pcmDones[callID]; ok {
					doneCh = ch
					delete(pcmDones, callID)
				}
				go func(callID string, dur int, done <-chan []int16) {
					var combined []int16
					if done != nil {
						combined = <-done
					}
					recPath := ""
					if len(combined) > 0 {
						// combined = pcmA + pcmB（前后拼接），拆分后分别作为左右声道
						half := len(combined) / 2
						pcmA := combined[:half]
						pcmB := combined[half:]
						safeID := strings.ReplaceAll(callID, "@", "_")
						safeID = strings.ReplaceAll(safeID, "/", "_")
						filename := fmt.Sprintf("%s_%s.wav",
							time.Now().Format("20060102_150405"), safeID[:min(len(safeID), 20)])
						recPath = filepath.Join(*recDir, filename)
						if err := audio.WriteWAVStereo(recPath, pcmA, pcmB); err != nil {
							log.Printf("[Main] write WAV error: %v", err)
							recPath = ""
						} else {
							log.Printf("[Main] WAV written: %s", recPath)
						}
					}
					db.UpdateCallEnd(callID, time.Now(), dur, recPath, "completed")
				}(callID, payload.Duration, doneCh)
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
