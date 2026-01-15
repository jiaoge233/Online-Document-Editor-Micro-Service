package collab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"gateway/backend/internal/ot/delta"
)

// 协作引擎接口
type Service interface {
	Submit(ctx context.Context, docID string, authorID uint64,
		baseRevision uint64, clientID string, clientSeq uint64,
		ops delta.Delta) (AppliedOp, error)

	CurrentRevision(ctx context.Context, docID string) (uint64, error)

	LoadDocumentContent(ctx context.Context, docID string) (string, uint64, error)

	// 可选：用于握手/追平
	OpsSince(ctx context.Context, docID string, fromRevision uint64, limit int) ([]AppliedOp, error)

	SaveSnapshot(ctx context.Context, docID string) error

	GetDocumentID(ctx context.Context, title string) (string, error)
	CreateDocument(ctx context.Context, ownerID uint64, title string) error

	GetUserID(ctx context.Context, username string) (uint64, error)
}

// 快照存储接口
type SnapshotStore interface {
	SaveDocumentSnapshot(ctx context.Context, docID string, rev uint64, content string) error
}

type DocumentStore interface {
	GetDocumentID(ctx context.Context, title string) (string, error)
	CreateDocument(ctx context.Context, ownerID uint64, title string) error
}

type UserStore interface {
	GetUserID(ctx context.Context, username string) (uint64, error)
}

type AppliedOp struct {
	OperationId string // 本次操作的唯一ID（用于幂等/追踪）
	Revision    uint64 // 全局版本号
	AuthorId    uint64
	// 用户操作序列，注意不是[]
	Ops       delta.Delta
	AppliedAt time.Time
}

var (
	ErrRevisionConflict      = errors.New("REVISION_CONFLICT")
	ErrDuplicateOrOutOfOrder = errors.New("DUPLICATE_OR_OUT_OF_ORDER")
)

type docState struct {
	mu       sync.RWMutex
	revision uint64
	opsRing  []AppliedOp
	// 去重窗口：记录某 clientId 最近的最大 clientSeq（或滑动窗口集合）
	lastSeqByClient map[string]uint64
	// 文档内容缓冲区
	buf Buffer
}

// 内存实现：持有所有文档的状态
type InMemoryService struct {
	mu      sync.RWMutex
	docs    map[string]*docState
	ringCap int

	// 依赖注入
	// 只声明，实现在store中
	store         SnapshotStore
	documentStore DocumentStore
	userStore     UserStore

	kafka      sarama.SyncProducer
	kafkaTopic string
}

// NewInMemoryService 返回一个满足 Service 接口的实例
func NewInMemoryService(store SnapshotStore, documentStore DocumentStore, userStore UserStore, kafka sarama.SyncProducer, kafkaTopic string) Service {
	return &InMemoryService{
		docs:          make(map[string]*docState),
		ringCap:       1024, // 近期操作环形缓冲容量，可按需调整
		store:         store,
		documentStore: documentStore,
		userStore:     userStore,
		kafka:         kafka,
		kafkaTopic:    "doc-ops",
	}
}

func (s *InMemoryService) LoadDocumentContent(ctx context.Context, docID string) (string, uint64, error) {
	s.mu.RLock()
	ds := s.docs[docID]
	s.mu.RUnlock()
	if ds == nil {
		return "", 0, errors.New("document not found")
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.buf.String(), ds.revision, nil
}

// 获取或创建指定文档的状态
func (s *InMemoryService) getOrCreateDoc(docID string) *docState {
	s.mu.RLock()
	ds := s.docs[docID]
	s.mu.RUnlock()
	if ds != nil {
		return ds
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ds = s.docs[docID]; ds == nil {
		capacity := s.ringCap
		if capacity <= 0 {
			capacity = 1024
		}
		ds = &docState{
			lastSeqByClient: make(map[string]uint64),
			opsRing:         make([]AppliedOp, 0, capacity),
			buf:             NewPieceTable(""),
		}
		s.docs[docID] = ds
	}
	return ds
}

// 提交操作（InMemoryService 实现）
func (s *InMemoryService) Submit(ctx context.Context, docID string, authorID uint64, baseRevision uint64, clientId string, clientSeq uint64, ops delta.Delta) (AppliedOp, error) {
	ds := s.getOrCreateDoc(docID)
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// 幂等/去重（最小实现：只允许递增）
	if last := ds.lastSeqByClient[clientId]; clientSeq <= last {
		// 已处理过或乱序，最小实现可直接返回冲突
		return AppliedOp{}, ErrDuplicateOrOutOfOrder
	}
	// 版本校验
	if baseRevision != ds.revision {
		return AppliedOp{}, ErrRevisionConflict
	}

	if ds.buf == nil {
		ds.buf = NewPieceTable("")
	}
	if err := ds.buf.Apply(ops); err != nil {
		return AppliedOp{}, err
	}

	// 推进版本
	ds.revision++
	appliedOp := AppliedOp{
		OperationId: fmt.Sprintf("o-%d", time.Now().UnixNano()),
		Revision:    ds.revision,
		AuthorId:    authorID,
		Ops:         ops,
		AppliedAt:   time.Now(),
	}

	// 保存到环形缓冲（如果达到容量则丢弃最老的一条）
	if cap(ds.opsRing) > 0 && len(ds.opsRing) == cap(ds.opsRing) {
		copy(ds.opsRing[0:], ds.opsRing[1:])
		ds.opsRing = ds.opsRing[:len(ds.opsRing)-1]
	}
	ds.opsRing = append(ds.opsRing, appliedOp)

	// 更新去重窗口
	ds.lastSeqByClient[clientId] = clientSeq

	// 异步发 Kafka（不阻塞主流程）
	if s.kafka != nil && s.kafkaTopic != "" {
		evt := DocOpEvent{
			EventType:    "OP_APPLIED",
			DocID:        docID,
			OperationID:  appliedOp.OperationId,
			Revision:     appliedOp.Revision,
			AuthorID:     appliedOp.AuthorId,
			ClientID:     clientId,
			ClientSeq:    clientSeq,
			BaseRevision: baseRevision,
			Ops:          appliedOp.Ops,
			AppliedAt:    appliedOp.AppliedAt,
		}
		go func() {
			b, err := json.Marshal(evt)
			if err != nil {
				return
			}
			msg := &sarama.ProducerMessage{
				Topic: s.kafkaTopic,
				Key:   sarama.StringEncoder(docID), // 以 docId 做 key，便于按文档分区
				Value: sarama.ByteEncoder(b),
			}
			_, _, _ = s.kafka.SendMessage(msg) // 先忽略错误，有需要再打 log/retry
		}()
	}

	return appliedOp, nil
}

// 返回当前文档版本（InMemoryService 实现）
func (s *InMemoryService) CurrentRevision(ctx context.Context, docID string) (uint64, error) {
	s.mu.RLock()
	ds := s.docs[docID]
	s.mu.RUnlock()
	if ds == nil {
		return 0, nil
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.revision, nil
}

// 返回 fromRevision 之后的已应用操作（InMemoryService 实现）
func (s *InMemoryService) OpsSince(ctx context.Context, docID string, fromRevision uint64, limit int) ([]AppliedOp, error) {
	s.mu.RLock()
	ds := s.docs[docID]
	s.mu.RUnlock()
	if ds == nil {
		return nil, nil
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var out []AppliedOp
	for _, op := range ds.opsRing {
		if op.Revision > fromRevision {
			out = append(out, op)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *InMemoryService) SaveSnapshot(ctx context.Context, docID string) error {
	if s.store == nil {
		return errors.New("snapshot store not initialized")
	}
	s.mu.RLock()
	ds := s.docs[docID]
	s.mu.RUnlock()
	if ds == nil || ds.buf == nil {
		return errors.New("document not found or buffer not initialized")
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	content := ds.buf.String()
	rev := ds.revision
	return s.store.SaveDocumentSnapshot(ctx, docID, rev, content)
}

func (s *InMemoryService) GetDocumentID(ctx context.Context, title string) (string, error) {
	if s.documentStore == nil {
		return "", errors.New("document store not initialized")
	}
	return s.documentStore.GetDocumentID(ctx, title)
}

func (s *InMemoryService) CreateDocument(ctx context.Context, ownerID uint64, title string) error {
	if s.documentStore == nil {
		return errors.New("document store not initialized")
	}
	return s.documentStore.CreateDocument(ctx, ownerID, title)
}

func (s *InMemoryService) GetUserID(ctx context.Context, username string) (uint64, error) {
	if s.userStore == nil {
		return 0, errors.New("user store not initialized")
	}
	return s.userStore.GetUserID(ctx, username)
}
