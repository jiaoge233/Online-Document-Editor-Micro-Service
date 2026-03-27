package collab

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/IBM/sarama"
)

// PersistDispatcher 把“需要落库”的任务事件异步发到 Kafka。
// 它与 DocOp 的 dispatcher 分开，便于后续独立调优 topic、重试和监控。
type PersistDispatcher struct {
	producer sarama.SyncProducer
	topic    string

	// 本地缓冲队列用于吸收短时间内大量“请求持久化”事件，避免阻塞主编辑链路。
	queue chan SnapshotPersistEvent

	sem *SemaphoreControl

	workers     int
	maxRetry    int
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

type PersistDispatcherOptions struct {
	QueueSize int
	Workers   int
	// 这里的 MaxRetry 只负责“发送到 Kafka”这一步的网络重试，
	// 业务落库失败后的 3 次重试走的是 retry topic / dlq 机制。
	MaxRetry    int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

func NewPersistDispatcher(producer sarama.SyncProducer, topic string, sem *SemaphoreControl, opt PersistDispatcherOptions) *PersistDispatcher {
	if opt.QueueSize <= 0 {
		opt.QueueSize = 1024
	}
	if opt.Workers <= 0 {
		opt.Workers = 4
	}
	if opt.MaxRetry <= 0 {
		opt.MaxRetry = 3
	}
	if opt.BaseBackoff <= 0 {
		opt.BaseBackoff = 50 * time.Millisecond
	}
	if opt.MaxBackoff <= 0 {
		opt.MaxBackoff = time.Second
	}
	d := &PersistDispatcher{
		producer:    producer,
		topic:       topic,
		queue:       make(chan SnapshotPersistEvent, opt.QueueSize),
		sem:         sem,
		workers:     opt.Workers,
		maxRetry:    opt.MaxRetry,
		baseBackoff: opt.BaseBackoff,
		maxBackoff:  opt.MaxBackoff,
	}

	d.Start()
	return d
}

func (d *PersistDispatcher) Enqueue(ctx context.Context, evt SnapshotPersistEvent) error {
	select {
	case d.queue <- evt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *PersistDispatcher) Start() {
	for i := 0; i < d.workers; i++ {
		go d.workerLoop(i)
	}
}

func (d *PersistDispatcher) workerLoop(workerID int) {
	for evt := range d.queue {
		// dispatcher 只保证把事件可靠送到 Kafka；
		// 真正的数据库持久化由 consumer 侧 worker pool 完成。
		d.sendWithRetry(workerID, evt)
	}
}

func (d *PersistDispatcher) sendWithRetry(workerID int, evt SnapshotPersistEvent) {
	for attempt := 0; attempt <= d.maxRetry; attempt++ {
		if d.sem != nil {
			_ = d.sem.Acquire(context.Background())
		}

		err := d.sendOnce(evt)

		if d.sem != nil {
			_ = d.sem.Release()
		}

		if err == nil {
			return
		}

		if attempt == d.maxRetry {
			log.Printf("persist event send failed, drop doc=%s rev=%d worker=%d err=%v",
				evt.DocID, evt.Revision, workerID, err)
			return
		}

		// 发送 Kafka 的瞬时失败用指数退避处理，避免 broker 抖动时疯狂重试。
		backoff := d.baseBackoff * time.Duration(1<<attempt)
		if backoff > d.maxBackoff {
			backoff = d.maxBackoff
		}
		time.Sleep(backoff)
	}
}

func (d *PersistDispatcher) sendOnce(evt SnapshotPersistEvent) error {
	if d.producer == nil || d.topic == "" {
		return nil
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := &sarama.ProducerMessage{
		Topic: d.topic,
		Key:   sarama.StringEncoder(evt.DocID),
		Value: sarama.ByteEncoder(b),
	}
	_, _, err = d.producer.SendMessage(msg)
	return err
}
