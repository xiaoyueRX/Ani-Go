package scheduler

import (
	"context"
	"sync"
	"time"
)

type schedulerRun struct {
	ctx             context.Context
	done            chan struct{}
	settingsChanged chan struct{}
	workers         sync.WaitGroup
	closing         bool // 受 Scheduler.runMu 保护，与 workers.Add 串行。
}

// StartAsync 登记一次运行并返回其结束信号。它包含所有定时工作及接纳的手工补全。
func (s *Scheduler) StartAsync(ctx context.Context) <-chan struct{} {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.run != nil {
		select {
		case <-s.run.done:
		default:
			return s.run.done
		}
	}
	run := &schedulerRun{ctx: ctx, done: make(chan struct{}), settingsChanged: make(chan struct{}, 1)}
	s.run = run
	go func() {
		defer close(run.done)
		if ctx.Err() == nil {
			s.runTimers(ctx, run.settingsChanged)
		}
		s.runMu.Lock()
		run.closing = true
		s.runMu.Unlock()
		run.workers.Wait()
	}()
	return run.done
}

// 在生命周期锁内接纳工作，先停接再等待，避免 Add 与 Wait 交错。
func (s *Scheduler) beginWork(parent context.Context) (context.Context, func(), error) {
	if err := parent.Err(); err != nil {
		return nil, nil, err
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	run := s.run
	if run == nil {
		return parent, func() {}, nil
	}
	if run.closing || run.ctx.Err() != nil {
		return nil, nil, context.Canceled
	}
	run.workers.Add(1)
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(run.ctx, cancel)
	return ctx, func() { stop(); cancel(); run.workers.Done() }, nil
}

func (s *Scheduler) launch(parent context.Context, task func(context.Context)) {
	ctx, finish, err := s.beginWork(parent)
	if err != nil {
		return
	}
	go func() {
		defer finish()
		if ctx.Err() == nil {
			task(ctx)
		}
	}()
}

// ReloadIntervals 唤醒长周期定时器，使已经提交的调度设置立即生效。
func (s *Scheduler) ReloadIntervals() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.run == nil || s.run.ctx.Err() != nil {
		return
	}
	select {
	case s.run.settingsChanged <- struct{}{}:
	default:
	}
}

type scheduleTicker struct {
	*time.Ticker
	key                string
	fallback, interval time.Duration
}

func (s *Scheduler) scheduleTicker(key string, fallback time.Duration) *scheduleTicker {
	interval := s.getInterval(key, fallback)
	return &scheduleTicker{Ticker: jitteredTicker(interval), key: key, fallback: fallback, interval: interval}
}

func (t *scheduleTicker) refresh(s *Scheduler, ticked bool) {
	interval := s.getInterval(t.key, t.fallback)
	if ticked || interval != t.interval {
		t.interval = interval
		resetWithJitter(t.Ticker, interval)
	}
}
