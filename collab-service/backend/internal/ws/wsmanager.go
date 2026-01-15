package ws

import (
	"log"
	"net/http"
	"strings"

	"collabServer/backend/internal/collab"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	// "time"
)

// 全局的WebSocket upgrader（允许本地开发环境的来源）
var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" { // 一些环境可能不发送 Origin，或为 "null"
		return true
	}
	allowedPrefixes := []string{
		"http://localhost",
		"http://127.0.0.1",
		"https://localhost",
		"https://127.0.0.1",
		"",
	}
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(origin, p) {
			return true
		}
	}
	return false
}}

type Manager struct {
	h   *Hub
	svc collab.Service
	sem *collab.SemaphoreControl
}

func NewManager(h *Hub, svc collab.Service, sem *collab.SemaphoreControl) *Manager {
	return &Manager{h: h, svc: svc, sem: sem}
}

func (m *Manager) WebSocketConnect(c *gin.Context, h *Hub) {
	// 获取 lastKnownRevision, query:获取 URL 查询参数
	userIDUint64 := c.GetUint64("userId")
	username := c.GetString("username")
	//docID := c.Query("docId")
	//if docID == "" {
	//	c.String(http.StatusBadRequest, "missing docId")
	//	return
	//}
	// 鉴权......还不会写
	// lastRev := c.Query("lastKnownRevision")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v (origin=%s)", err, c.Request.Header.Get("Origin"))
		return
	}
	// defer：用于延迟执行（延迟至return处）
	defer conn.Close()

	// 发送 welcome（伪实现）
	msg := ServerMessage{Type: "welcome", Content: "有一个新成员加入了，欢迎"}
	// _ = conn.WriteJSON(gin.H{"type": "welcome", "docId": docID, "revision": 0})

	wsConn := NewConn(conn, m.h, "", userIDUint64, username, m.svc, m.sem)

	// 先启动写循环，确保后续写入 send 通道的消息可以被及时发送
	go wsConn.writeLoop()
	// 发送 welcome 消息
	wsConn.send <- msg

	// 最后再进入读循环（阻塞至连接关闭）
	wsConn.readLoop(c.Request.Context())
}
