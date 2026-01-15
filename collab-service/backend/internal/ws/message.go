package ws

import (
	"time"

	"collabServer/backend/internal/ot/delta"
)

type ClientMessage struct {
	Type         string      `json:"type"`
	DocID        string      `json:"docId"`
	DocTitle     string      `json:"docTitle"`
	Range        interface{} `json:"range,omitempty"`
	BaseRevision uint64      `json:"baseRevision"`
	ClientId     string      `json:"clientId"`
	ClientSeq    uint64      `json:"clientSeq"`
	Ops          delta.Delta `json:"ops"`
	Content      string      `json:"content,omitempty"`
}

type PresenceMember struct {
	UserID   uint64 `json:"userId"`
	Username string `json:"username,omitempty"`
}

type ServerMessage struct {
	Type     string           `json:"type"`
	UserID   uint64           `json:"userId,omitempty"`
	DocID    string           `json:"docId,omitempty"`
	Revision uint64           `json:"revision,omitempty"`
	Members  []PresenceMember `json:"members,omitempty"`
	Cursor   interface{}      `json:"cursor,omitempty"`
	Range    interface{}      `json:"range,omitempty"`
	Content  string           `json:"content,omitempty"`
}

type OpSubmitMessage struct {
	Type            string `json:"type"`
	DocID           string `json:"docId"`
	BaseRevision    uint64 `json:"baseRevision"`
	CurrentRevision uint64 `json:"currentRevision"`
	// 客户端实例标识。同一用户可有多个 clientId（多端/多标签页）。
	ClientId string `json:"clientId"`
	// 针对同一个 clientId 的“本地递增序号”
	ClientSeq uint64      `json:"clientSeq"`
	Ops       delta.Delta `json:"ops"`
}

// 广播给同文档房间内其他连接的“已应用操作”事件
// - 与 op_applied(ack) 区分：这里用于把变更推送给其他协作者（包括同用户的其他标签页）
// - 前端可按需实现：收到后在本地应用 ops，并将本地 revision 对齐到 revision
type OpBroadcastMessage struct {
	Type      string      `json:"type"` // 固定 "op_broadcast"
	DocID     string      `json:"docId"`
	Revision  uint64      `json:"revision"` // 服务端已应用后的最新版本
	AuthorID  uint64      `json:"authorId"`
	ClientId  string      `json:"clientId,omitempty"`
	ClientSeq uint64      `json:"clientSeq,omitempty"`
	Ops       delta.Delta `json:"ops"`
	AppliedAt time.Time   `json:"appliedAt,omitempty"`
}

type OpAppliedMessage struct {
	Type            string `json:"type"` // 固定 "op_applied"
	DocID           string `json:"docId"`
	BaseRevision    uint64 `json:"baseRevision"`    // 客户端提交时的 base
	CurrentRevision uint64 `json:"currentRevision"` // 服务端应用后的最新版本
	ClientId        string `json:"clientId"`
	ClientSeq       uint64 `json:"clientSeq"`
}
