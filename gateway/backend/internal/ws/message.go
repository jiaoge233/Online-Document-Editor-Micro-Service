package ws

import "gateway/backend/internal/ot/delta"

type ClientMessage struct {
	Type         string      `json:"type"`
	UserID       uint64      `json:"userId"`
	Password     string      `json:"password"`
	Username     string      `json:"username"`
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
	BaseRevision    uint64
	CurrentRevision uint64
	// 客户端实例标识。同一用户可有多个 clientId（多端/多标签页）。
	ClientId string `json:"clientId"`
	// 针对同一个 clientId 的“本地递增序号”
	ClientSeq uint64      `json:"clientSeq"`
	Ops       delta.Delta `json:"ops"`
}
