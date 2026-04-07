package collab

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type PersistWorkerPool struct {
	service Service

	// Kafka consumer 把任务投递到本地 jobs 队列，再由固定数量的 worker 并发执行落库。
	jobs chan persistJob

	workers    int
	jobTimeout time.Duration

	wg sync.WaitGroup
}

type persistJob struct {
	event  SnapshotPersistEvent
	result chan error
}

func NewPersistWorkerPool(service Service, queueSize, workers int, jobTimeout time.Duration) *PersistWorkerPool {
	if queueSize <= 0 {
		queueSize = 1024
	}
	if workers <= 0 {
		workers = 4
	}
	if jobTimeout <= 0 {
		jobTimeout = 3 * time.Second
	}
	return &PersistWorkerPool{
		service:    service,
		jobs:       make(chan persistJob, queueSize),
		workers:    workers,
		jobTimeout: jobTimeout,
	}
}

func (p *PersistWorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.workerLoop(ctx, i)
	}
}

func (p *PersistWorkerPool) Wait() {
	p.wg.Wait()
}

// Submit 把 Kafka 消费到的持久化事件交给本地 worker 执行，
// 调用方会等待本次任务完成后再提交 Kafka offset。
func (p *PersistWorkerPool) Submit(ctx context.Context, evt SnapshotPersistEvent) error {
	job := persistJob{
		event:  evt,
		result: make(chan error, 1),
	}

	select {
	case p.jobs <- job:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-job.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *PersistWorkerPool) workerLoop(ctx context.Context, workerID int) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-p.jobs:
			// 单个任务超时后视为一次失败，让 consumer 决定是否重试或写入死信队列。
			jobCtx, cancel := context.WithTimeout(ctx, p.jobTimeout)
			err := p.service.PersistSnapshot(jobCtx, job.event.DocID, job.event.Revision)
			cancel()
			if err != nil {
				err = fmt.Errorf("persist snapshot doc=%s rev=%d worker=%d: %w", job.event.DocID, job.event.Revision, workerID, err)
				log.Print(err)
			}
			job.result <- err
		}
	}
}
