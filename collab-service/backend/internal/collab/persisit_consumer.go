package collab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/IBM/sarama"
)

type PersistConsumer struct {
	group   sarama.ConsumerGroup
	groupID string
	// 同时消费主持久化队列和重试队列；死信队列只写不读，留给人工排查或补偿任务。
	topics     []string
	workerPool *PersistWorkerPool
	producer   sarama.SyncProducer
	retryTopic string
	dlqTopic   string
	// maxRetry 是业务落库失败后的最大重试次数，不包含 producer 向 Kafka 发消息时的网络重试。
	maxRetry int
}

func NewPersistConsumer(
	brokers []string,
	groupID string,
	topics []string,
	retryTopic string,
	dlqTopic string,
	maxRetry int,
	producer sarama.SyncProducer,
	workerPool *PersistWorkerPool,
) (*PersistConsumer, error) {
	if len(brokers) == 0 || groupID == "" || len(topics) == 0 || retryTopic == "" || dlqTopic == "" {
		return nil, errors.New("persist consumer config is incomplete")
	}
	if maxRetry <= 0 {
		maxRetry = 3
	}

	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_1_0_0
	cfg.Consumer.Return.Errors = true
	cfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, err
	}

	return &PersistConsumer{
		group:      group,
		groupID:    groupID,
		topics:     topics,
		workerPool: workerPool,
		producer:   producer,
		retryTopic: retryTopic,
		dlqTopic:   dlqTopic,
		maxRetry:   maxRetry,
	}, nil
}

func (c *PersistConsumer) Start(ctx context.Context) {
	if c.workerPool != nil {
		c.workerPool.Start(ctx)
	}

	go func() {
		handler := &persistConsumerGroupHandler{
			workerPool: c.workerPool,
			producer:   c.producer,
			retryTopic: c.retryTopic,
			dlqTopic:   c.dlqTopic,
			maxRetry:   c.maxRetry,
		}
		for {
			// Consume 返回通常意味着 rebalance 或短暂异常；外层循环负责自动重新进入消费。
			if err := c.group.Consume(ctx, c.topics, handler); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("persist consumer group=%s consume error: %v", c.groupID, err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	go func() {
		for err := range c.group.Errors() {
			log.Printf("persist consumer group=%s error: %v", c.groupID, err)
		}
	}()
}

func (c *PersistConsumer) Close() error {
	if c.group == nil {
		return nil
	}
	return c.group.Close()
}

type persistConsumerGroupHandler struct {
	workerPool *PersistWorkerPool
	producer   sarama.SyncProducer
	retryTopic string
	dlqTopic   string
	maxRetry   int
}

func (h *persistConsumerGroupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *persistConsumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *persistConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			var evt SnapshotPersistEvent
			if err := json.Unmarshal(msg.Value, &evt); err != nil {
				log.Printf("persist consumer unmarshal failed: %v", err)
				session.MarkMessage(msg, "")
				continue
			}
			if evt.EventType != SnapshotPersistRequestedType {
				session.MarkMessage(msg, "")
				continue
			}
			if h.workerPool == nil {
				log.Printf("persist worker pool is nil, skip doc=%s rev=%d", evt.DocID, evt.Revision)
				session.MarkMessage(msg, "")
				continue
			}

			if err := h.workerPool.Submit(session.Context(), evt); err != nil {
				// 落库失败后先重新投递，再提交当前消息 offset，避免同一条消息在当前分区无限阻塞。
				if rerouteErr := h.handleFailure(evt, err); rerouteErr != nil {
					return rerouteErr
				}
				session.MarkMessage(msg, "")
				continue
			}
			session.MarkMessage(msg, "")
		}
	}
}

func (h *persistConsumerGroupHandler) handleFailure(evt SnapshotPersistEvent, workerErr error) error {
	failedEvt := evt
	failedEvt.LastError = workerErr.Error()

	// 小于最大重试次数时继续进入 retry topic；否则进入死信队列等待人工介入。
	if failedEvt.RetryCount < h.maxRetry {
		failedEvt.RetryCount++
		return h.publishToTopic(h.retryTopic, failedEvt)
	}
	return h.publishToTopic(h.dlqTopic, failedEvt)
}

func (h *persistConsumerGroupHandler) publishToTopic(topic string, evt SnapshotPersistEvent) error {
	if h.producer == nil {
		return fmt.Errorf("persist consumer producer is nil for topic %s", topic)
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(evt.DocID),
		Value: sarama.ByteEncoder(b),
	}
	_, _, err = h.producer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("publish persist reroute topic=%s doc=%s rev=%d retry=%d: %w", topic, evt.DocID, evt.Revision, evt.RetryCount, err)
	}
	return nil
}
