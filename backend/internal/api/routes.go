package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"sip-dashboard/internal/sip"
	"sip-dashboard/internal/store"
	"sip-dashboard/internal/ws"

	"github.com/gin-gonic/gin"
)

// Server 包含所有 API 依赖
type Server struct {
	db     *store.DB
	hub    *ws.Hub
	sm     *sip.StateMachine
	recDir string
}

// NewServer 创建 API 服务
func NewServer(db *store.DB, hub *ws.Hub, sm *sip.StateMachine, recDir string) *Server {
	return &Server{db: db, hub: hub, sm: sm, recDir: recDir}
}

// RegisterRoutes 注册所有路由到 Gin Engine
func (s *Server) RegisterRoutes(r *gin.Engine) {
	// WebSocket
	r.GET("/ws/events", func(c *gin.Context) {
		s.hub.ServeEvents(c.Writer, c.Request)
	})
	r.GET("/ws/audio/:callId", func(c *gin.Context) {
		callID := c.Param("callId")
		s.hub.ServeAudio(c.Writer, c.Request, callID)
	})

	// REST API
	api := r.Group("/api")
	{
		api.GET("/calls", s.listCalls)
		api.GET("/calls/:id", s.getCall)
		api.GET("/active", s.getActiveCalls)
	}

	// 录音文件服务（支持 Range 请求）
	r.GET("/api/recordings/:filename", s.serveRecording)
}

// listCalls 分页查询历史通话记录
func (s *Server) listCalls(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	calls, total, err := s.db.ListCalls(status, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  calls,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// getCall 按 ID 查询单条通话记录
func (s *Server) getCall(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	call, err := s.db.GetCallByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.JSON(http.StatusOK, call)
}

// getActiveCalls 获取当前活跃通话（来自内存状态机）
func (s *Server) getActiveCalls(c *gin.Context) {
	sessions := s.sm.GetActiveSessions()
	result := make([]gin.H, 0, len(sessions))
	for _, sess := range sessions {
		result = append(result, gin.H{
			"callId":    sess.CallID,
			"from":      sess.FromNumber,
			"to":        sess.ToNumber,
			"status":    sess.Status,
			"startTime": sess.StartTime,
		})
	}
	c.JSON(http.StatusOK, result)
}

// serveRecording 提供录音文件下载，支持 Range 请求（断点续传）
func (s *Server) serveRecording(c *gin.Context) {
	filename := filepath.Base(c.Param("filename"))
	// 防止路径遍历
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	path := filepath.Join(s.recDir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	c.Header("Content-Type", "audio/wav")
	http.ServeFile(c.Writer, c.Request, path)
}
