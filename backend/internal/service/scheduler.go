package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type SchedulerService struct {
	syncSvc      *SyncService
	scheduleRepo *repository.SyncScheduleRepository
	logger       *zap.Logger
	mu           sync.Mutex
	lastFired    map[uuid.UUID]string
}

func NewSchedulerService(syncSvc *SyncService, scheduleRepo *repository.SyncScheduleRepository, logger *zap.Logger) *SchedulerService {
	return &SchedulerService{
		syncSvc:      syncSvc,
		scheduleRepo: scheduleRepo,
		logger:       logger,
		lastFired:    make(map[uuid.UUID]string),
	}
}

func (s *SchedulerService) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	s.logger.Info("scheduler started")

	cleanTick := 0
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
			cleanTick++
			if cleanTick >= 60 {
				s.cleanLastFired()
				cleanTick = 0
			}
		}
	}
}

func (s *SchedulerService) tick(ctx context.Context) {
	hora := time.Now().Format("15:04")

	schedules, err := s.scheduleRepo.GetDueSchedules(ctx, hora)
	if err != nil {
		s.logger.Error("scheduler: failed to get due schedules", zap.Error(err))
		return
	}

	if len(schedules) == 0 {
		return
	}

	s.logger.Info("scheduler: found due schedules", zap.String("hora", hora), zap.Int("count", len(schedules)))

	for _, sched := range schedules {
		s.mu.Lock()
		if s.lastFired[sched.ID] == hora {
			s.mu.Unlock()
			continue
		}
		s.lastFired[sched.ID] = hora
		s.mu.Unlock()

		for _, pk := range sched.ProjectKeys {
			go func(fonteID uuid.UUID, projectKey string) {
				s.logger.Info("scheduler: triggering sync",
					zap.String("project", projectKey),
					zap.String("hora", hora),
				)
				_, err := s.syncSvc.SyncProjectScheduled(ctx, fonteID, projectKey)
				if err != nil {
					if errors.Is(err, ErrSyncAlreadyRunning) {
						s.logger.Warn("scheduler: sync already running, skipping",
							zap.String("project", projectKey),
						)
						return
					}
					s.logger.Error("scheduler: sync failed",
						zap.String("project", projectKey),
						zap.Error(err),
					)
				}
			}(sched.FonteDadosID, pk)
		}
	}
}

func (s *SchedulerService) cleanLastFired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFired = make(map[uuid.UUID]string)
}
