package completiongates

import (
	"context"
	"sync"
)

// taskScheduler runs at most one evaluation per task at a time and coalesces
// requests that arrive while one runs into a single follow-up run.
type taskScheduler struct {
	mu      sync.Mutex
	running map[string]bool
	dirty   map[string]bool
	wg      sync.WaitGroup
	run     func(ctx context.Context, taskID string)
}

func newTaskScheduler(run func(ctx context.Context, taskID string)) *taskScheduler {
	return &taskScheduler{running: map[string]bool{}, dirty: map[string]bool{}, run: run}
}

func (s *taskScheduler) schedule(ctx context.Context, taskID string) {
	if taskID == "" || ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	if s.running[taskID] {
		s.dirty[taskID] = true
		s.mu.Unlock()
		return
	}
	s.running[taskID] = true
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			s.run(ctx, taskID)
			s.mu.Lock()
			if !s.dirty[taskID] || ctx.Err() != nil {
				delete(s.running, taskID)
				delete(s.dirty, taskID)
				s.mu.Unlock()
				return
			}
			delete(s.dirty, taskID)
			s.mu.Unlock()
		}
	}()
}

func (s *taskScheduler) wait() { s.wg.Wait() }
