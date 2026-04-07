package collab

import (
	"collabServer/backend/internal/ot/delta"
	"time"
)

const (
	// 操作事件用于审计/广播侧链路；快照事件用于异步持久化链路。
	DocOpAppliedEventType          = "OP_APPLIED"
	SnapshotPersistRequestedType   = "SNAPSHOT_PERSIST_REQUESTED"
	SnapshotPersistTriggerDebounce = "debounce"
	SnapshotPersistTriggerManual   = "manual_save"
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

// SnapshotPersistEvent 是落库任务事件。
// 这里只携带文档标识和目标 revision，真正的正文内容由 worker 在消费时从当前内存态读取。
type SnapshotPersistEvent struct {
	EventType string `json:"eventType"` // 固定 "SNAPSHOT_PERSIST_REQUESTED"
	DocID     string `json:"docId"`
	Revision  uint64 `json:"revision"`
	Trigger   string `json:"trigger"` // debounce / manual_save
	// RetryCount 表示这条落库任务已经被重新投递了多少次；
	// consumer 会据此判断继续进入 retry topic 还是进入 dead-letter queue。
	RetryCount int `json:"retryCount,omitempty"`
	// LastError 记录最近一次失败原因，方便排查为什么最终进入重试队列或死信队列。
	LastError   string    `json:"lastError,omitempty"`
	RequestedAt time.Time `json:"requestedAt"`
}
