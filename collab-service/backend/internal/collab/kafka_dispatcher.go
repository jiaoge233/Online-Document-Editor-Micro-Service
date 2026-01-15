package collab

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/IBM/sarama"
)

// KafkaDispatcher：本地有界队列 + worker 异步发送 + 有限重试。
// 目标：
// - 不阻塞主提交流程（Submit 只负责入队）
// - Kafka 短暂阻塞时靠队列吸收，后台慢慢补发
// - 队列满时允许降级（丢弃），避免内存无限增长
type KafkaDispatcher struct {
	producer sarama.SyncProducer
	topic    string

	queue chan DocOpEvent

	// sem 限制并发的 SendMessage 数量。
	kafkatSem *SemaphoreControl

	workers     int
	maxRetry    int
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

type KafkaDispatcherOptions struct {
	QueueSize   int
	Workers     int
	MaxRetry    int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

func NewKafkaDispatcher(producer sarama.SyncProducer, topic string, kafkatSem *SemaphoreControl, opt KafkaDispatcherOptions) *KafkaDispatcher {
	d := &KafkaDispatcher{
		producer:    producer,
		topic:       topic,
		queue:       make(chan DocOpEvent, opt.QueueSize),
		kafkatSem:   kafkatSem,
		workers:     opt.Workers,
		maxRetry:    opt.MaxRetry,
		baseBackoff: opt.BaseBackoff,
		maxBackoff:  opt.MaxBackoff,
	}

	d.Start()
	return d
}

// Enqueue：把事件放入本地队列。
// - 队列满时，等待直到 ctx 超时
// - ctx 超时返回错误 （kafka不要求强一致性，不是每个事件都必须送达）
func (d *KafkaDispatcher) Enqueue(ctx context.Context, evt DocOpEvent) error {
	select {
	case d.queue <- evt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *KafkaDispatcher) Start() {
	for i := 0; i < d.workers; i++ {
		go d.workerLoop(i)
	}
}

func (d *KafkaDispatcher) workerLoop(workerID int) {
	for evt := range d.queue {
		d.sendWithRetry(workerID, evt)
	}
}

func (d *KafkaDispatcher) sendWithRetry(workerID int, evt DocOpEvent) {
	for attempt := 0; attempt <= d.maxRetry; attempt++ {
		if d.kafkatSem != nil {
			// worker 允许一直等待（不会影响主链路）
			_ = d.kafkatSem.Acquire(context.Background())
		}

		err := d.sendOnce(evt)

		if d.kafkatSem != nil {
			_ = d.kafkatSem.Release()
		}

		if err == nil {
			return
		}

		if attempt == d.maxRetry {
			log.Printf("kafka send failed, drop event doc=%s op=%s rev=%d worker=%d err=%v",
				evt.DocID, evt.OperationID, evt.Revision, workerID, err)
			return
		}

		// 退避，每次退避时间X2
		backoff := d.baseBackoff * time.Duration(1<<attempt)
		if backoff > d.maxBackoff {
			backoff = d.maxBackoff
		}
		time.Sleep(backoff)
	}
}

func (d *KafkaDispatcher) sendOnce(evt DocOpEvent) error {
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
