package collab

import (
	"context"
	"errors"
)

var MaxSemaphore int = 100

type SemaphoreControl struct {
	ch chan struct{}
}

func NewSemaphoreControl() *SemaphoreControl {
	return &SemaphoreControl{ch: make(chan struct{}, MaxSemaphore)}
}

func (s *SemaphoreControl) Acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return errors.New("Acquire Reach time limit")
	}
}

func (s *SemaphoreControl) Release() error {
	select {
	case <-s.ch:
		return nil
	default:
		return errors.New("Release Failed, semaphore is not acquired")
	}
}
