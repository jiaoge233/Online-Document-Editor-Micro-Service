package collab

import (
	"collabServer/backend/internal/ot/delta"
	"time"
)

type DocOpEvent struct {
	EventType    string      `json:"eventType"` // 固定 "OP_APPLIED"
	DocID        string      `json:"docId"`
	OperationID  string      `json:"operationId"`
	Revision     uint64      `json:"revision"`
	AuthorID     uint64      `json:"authorId"`
	ClientID     string      `json:"clientId"`
	ClientSeq    uint64      `json:"clientSeq"` // 针对同一个 clientId 的“本地递增序号”
	BaseRevision uint64      `json:"baseRevision"`
	Ops          delta.Delta `json:"ops"`
	AppliedAt    time.Time   `json:"appliedAt"`
}
