package ws

import (
	"encoding/json"
	"sync"

	"gateway/backend/internal/cache"
)

type Hub struct {
	//  接口实例（一般是 Redis 实现的客户端句柄）。它本身不“存数据”，
	// 而是提供对外部存储的读写能力，用来落地/共享在线状态与光标信息
	presence cache.PresenceCache
	// 读写锁，用来保护 Hub 的内部状态在并发下安全访问，
	// 特别是 rooms 这类 map，防止并发读写导致崩溃或数据竞争。加入/离开房间、广播时都会先加锁。
	mu sync.RWMutex
	// docID -> set of connections
	rooms map[string]map[*Conn]struct{}
}

func NewHub(p cache.PresenceCache) *Hub {
	return &Hub{presence: p, rooms: make(map[string]map[*Conn]struct{})}
}

// Join 将连接加入指定文档房间
func (h *Hub) Join(docID string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[docID] == nil {
		// 为什么房间里存的是 map[Conn]，而不是 map[userID]
		// - 一个用户可开多个标签页/设备（多连接）；广播要逐连接发，不能只按 userID 发一次。
		h.rooms[docID] = make(map[*Conn]struct{})
	}
	h.rooms[docID][c] = struct{}{}
}

// Leave 将连接从指定文档房间移除
func (h *Hub) Leave(docID string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.rooms[docID]; ok {
		delete(conns, c)
		if len(conns) == 0 {
			delete(h.rooms, docID)
		}
	}
}

func (h *Hub) BroadcastPresence(docID string, members []PresenceMember) {
	h.mu.RLock()
	conns := h.rooms[docID]
	h.mu.RUnlock()
	content, err := json.Marshal(members)
	if err != nil {
		return
	}
	msg := ServerMessage{Type: "presence", DocID: docID, Content: string(content)}
	msg.Members = members
	for c := range conns {
		c.SendMessage_Enqueue(msg)
	}
}

func (h *Hub) BroadcastCursor(docID string, userID uint64, rng interface{}) {
	h.mu.RLock()
	conns := h.rooms[docID]
	h.mu.RUnlock()
	msg := ServerMessage{Type: "cursor", DocID: docID, UserID: userID, Range: rng}
	for c := range conns {
		c.send <- msg
	}
}
