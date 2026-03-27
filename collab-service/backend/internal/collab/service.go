package collab

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"collabServer/backend/internal/ot/delta"
)

// 协作引擎接口
type Service interface {
	Submit(ctx context.Context, docID string, authorID uint64,
		baseRevision uint64, clientID string, clientSeq uint64,
		ops delta.Delta) (AppliedOp, error)

	CurrentRevision(ctx context.Context, docID string) (uint64, error)

	LoadDocumentContent(ctx context.Context, docID string) (string, uint64, error)

	// 用于握手/追平
	OpsSince(ctx context.Context, docID string, fromRevision uint64, limit int) ([]AppliedOp, error)

	SaveSnapshot(ctx context.Context, docID string) error
	PersistSnapshot(ctx context.Context, docID string, minRevision uint64) error

	GetDocumentID(ctx context.Context, title string) (string, error)

	CreateDocument(ctx context.Context, ownerID uint64, title string) error
}

// 快照存储接口
type SnapshotStore interface {
	SaveDocumentSnapshot(ctx context.Context, docID string, rev uint64, content string) error
}

type DocumentStore interface {
	GetDocumentID(ctx context.Context, title string) (string, error)
	CreateDocument(ctx context.Context, ownerID uint64, title string) error
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
	// 去重窗口：记录某 clientId (string) 最近的最大 clientSeq (uint64)（或滑动窗口集合）
	lastSeqByClient map[string]uint64
	// 文档内容缓冲区
	buf Buffer
}

type snapshotPersistState struct {
	latestRevision uint64
	// timer 用于实现按文档粒度的 debounce：窗口内多次修改只会合并成一次落库请求。
	timer *time.Timer
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

	kafka      sarama.SyncProducer
	kafkaTopic string

	kafkaDispatcher   *KafkaDispatcher
	persistDispatcher *PersistDispatcher
	persistInterval   time.Duration

	persistMu sync.Mutex
	dirtyDocs map[string]*snapshotPersistState
}

// NewInMemoryService 返回一个满足 Service 接口的实例
func NewInMemoryService(
	store SnapshotStore,
	documentStore DocumentStore,
	kafka sarama.SyncProducer,
	kafkaTopic string,
	kafkaDispatcher *KafkaDispatcher,
	persistDispatcher *PersistDispatcher,
	persistInterval time.Duration,
) Service {
	return &InMemoryService{
		docs:              make(map[string]*docState),
		ringCap:           1024, // 近期操作环形缓冲容量，可按需调整
		store:             store,
		documentStore:     documentStore,
		kafka:             kafka,
		kafkaTopic:        kafkaTopic,
		kafkaDispatcher:   kafkaDispatcher,
		persistDispatcher: persistDispatcher,
		persistInterval:   persistInterval,
		dirtyDocs:         make(map[string]*snapshotPersistState),
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
	// 加锁，保护 ds 的并发访问（map）
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
	if s.kafkaDispatcher != nil && s.kafka != nil && s.kafkaTopic != "" {
		evt := DocOpEvent{
			EventType:    DocOpAppliedEventType,
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

		// 短等待把事件放入本地队列，后台重试发送 Kafka。
		enqueueCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		if err := s.kafkaDispatcher.Enqueue(enqueueCtx, evt); err != nil {
			// 超时：降级丢弃，但不影响主流程
			log.Printf("kafka queue busy, drop event doc=%s rev=%d: %v", docID, appliedOp.Revision, err)
		}
	}

	s.markSnapshotDirty(docID, appliedOp.Revision)

	return appliedOp, nil
}

// 返回当前文档版本
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

// 返回 fromRevision 之后的已应用操作
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

// SaveSnapshot 对外提供“请求保存”的入口：
// - 配置了 MQ 持久化链路时，写库请求会先入 Kafka
// - 未配置时退回到同步写库，保证旧逻辑仍可工作
func (s *InMemoryService) SaveSnapshot(ctx context.Context, docID string) error {
	if s.persistDispatcher != nil {
		rev, err := s.CurrentRevision(ctx, docID)
		if err != nil {
			return err
		}
		return s.enqueueSnapshotPersist(ctx, docID, rev, SnapshotPersistTriggerManual)
	}
	return s.PersistSnapshot(ctx, docID, 0)
}

// PersistSnapshot 由 Kafka consumer worker 调用，真正执行数据库快照落库。
// minRevision 用于避免消费到的旧事件把更早的版本错误当成“最新快照”。
func (s *InMemoryService) PersistSnapshot(ctx context.Context, docID string, minRevision uint64) error {
	if s.store == nil {
		return errors.New("snapshot store not initialized")
	}
	s.mu.RLock()
	ds := s.docs[docID]
	s.mu.RUnlock()
	if ds == nil {
		return errors.New("document not found")
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if ds.buf == nil {
		return errors.New("buffer not initialized")
	}
	content := ds.buf.String()
	rev := ds.revision
	if minRevision > 0 && rev < minRevision {
		return fmt.Errorf("document %s current revision %d is behind target revision %d", docID, rev, minRevision)
	}
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

func (s *InMemoryService) markSnapshotDirty(docID string, revision uint64) {
	if s.persistDispatcher == nil || s.persistInterval <= 0 {
		return
	}

	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	state := s.dirtyDocs[docID]
	if state == nil {
		state = &snapshotPersistState{}
		s.dirtyDocs[docID] = state
	}
	// 始终记住窗口内最新 revision，等定时器触发时只持久化这一版。
	state.latestRevision = revision
	if state.timer != nil {
		state.timer.Stop()
	}
	state.timer = time.AfterFunc(s.persistInterval, func() {
		s.flushSnapshotPersist(docID)
	})
}

func (s *InMemoryService) flushSnapshotPersist(docID string) {
	s.persistMu.Lock()
	state := s.dirtyDocs[docID]
	if state == nil {
		s.persistMu.Unlock()
		return
	}
	targetRevision := state.latestRevision
	state.timer = nil
	s.persistMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.enqueueSnapshotPersist(ctx, docID, targetRevision, SnapshotPersistTriggerDebounce); err != nil {
		log.Printf("enqueue snapshot persist failed doc=%s rev=%d: %v", docID, targetRevision, err)
		// 如果连“发持久化任务”都失败了，就把最新 revision 重新挂回 dirty 状态，等待下一轮定时补发。
		s.persistMu.Lock()
		current := s.dirtyDocs[docID]
		if current == nil {
			current = &snapshotPersistState{}
			s.dirtyDocs[docID] = current
		}
		if current.latestRevision < targetRevision {
			current.latestRevision = targetRevision
		}
		if current.timer == nil {
			current.timer = time.AfterFunc(s.persistInterval, func() {
				s.flushSnapshotPersist(docID)
			})
		}
		s.persistMu.Unlock()
		return
	}

	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	current := s.dirtyDocs[docID]
	if current == nil {
		return
	}
	if current.latestRevision == targetRevision && current.timer == nil {
		delete(s.dirtyDocs, docID)
	}
}

func (s *InMemoryService) enqueueSnapshotPersist(ctx context.Context, docID string, revision uint64, trigger string) error {
	if s.persistDispatcher == nil {
		// 没有 MQ 时退回同步持久化，保持服务仍然可用。
		return s.PersistSnapshot(ctx, docID, revision)
	}
	evt := SnapshotPersistEvent{
		EventType:   SnapshotPersistRequestedType,
		DocID:       docID,
		Revision:    revision,
		Trigger:     trigger,
		RetryCount:  0,
		RequestedAt: time.Now(),
	}
	return s.persistDispatcher.Enqueue(ctx, evt)
}
