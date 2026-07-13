package worker

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Syncer abstracts a full synchronization run.
type Syncer interface {
	SyncAll(ctx context.Context) error
}

type simpleSyncer struct {
	fn func(ctx context.Context) error
}

func (s *simpleSyncer) SyncAll(ctx context.Context) error {
	return s.fn(ctx)
}

// SyncWorker periodically triggers a sync operation on a fixed interval,
// running in its own goroutine until stopped or the context is cancelled.
type SyncWorker struct {
	syncer   Syncer
	interval time.Duration
	logger   *zap.Logger
	stopCh   chan struct{}
	once     sync.Once
}

// NewSyncWorker creates a SyncWorker that invokes syncFn every intervalMinutes.
// If intervalMinutes is not positive, it defaults to 30 minutes.
func NewSyncWorker(syncFn func(ctx context.Context) error, intervalMinutes int, logger *zap.Logger) *SyncWorker {
	if intervalMinutes <= 0 {
		intervalMinutes = 30
	}
	return &SyncWorker{
		syncer:   &simpleSyncer{fn: syncFn},
		interval: time.Duration(intervalMinutes) * time.Minute,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the periodic sync loop. It blocks until the context is
// cancelled or Stop is called, so callers typically invoke it in a
// dedicated goroutine.
func (w *SyncWorker) Start(ctx context.Context) {
	w.logger.Info("sync worker started", zap.Duration("interval", w.interval))
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("sync worker stopped (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("sync worker stopped")
			return
		case <-ticker.C:
			w.logger.Info("sync worker tick: starting sync")
			if err := w.syncer.SyncAll(ctx); err != nil {
				w.logger.Error("sync worker: sync failed", zap.Error(err))
			}
		}
	}
}

// Stop signals the worker to stop. It is safe to call multiple times.
func (w *SyncWorker) Stop() {
	w.once.Do(func() {
		close(w.stopCh)
	})
}
