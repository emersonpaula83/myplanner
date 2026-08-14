package service

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestNewSchedulerService(t *testing.T) {
	svc := NewSchedulerService(nil, nil, zap.NewNop())
	if svc == nil {
		t.Fatal("expected non-nil SchedulerService")
	}
	if svc.lastFired == nil {
		t.Error("expected initialized lastFired map")
	}
	if len(svc.lastFired) != 0 {
		t.Errorf("expected empty lastFired map, got %d entries", len(svc.lastFired))
	}
}

func TestCleanLastFired(t *testing.T) {
	svc := &SchedulerService{
		lastFired: map[uuid.UUID]string{
			uuid.New(): "10:00",
			uuid.New(): "11:00",
		},
		mu: sync.Mutex{},
	}

	svc.cleanLastFired()

	if len(svc.lastFired) != 0 {
		t.Errorf("expected empty map after cleanLastFired, got %d entries", len(svc.lastFired))
	}
	if svc.lastFired == nil {
		t.Error("expected non-nil map after cleanLastFired")
	}
}

func TestTick_RepoError(t *testing.T) {
	repo := &mockSyncScheduleStore{
		getDueSchedulesFn: func(ctx context.Context, hora string) ([]domain.SyncSchedule, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	svc := &SchedulerService{
		scheduleRepo: repo,
		logger:       zap.NewNop(),
		lastFired:    make(map[uuid.UUID]string),
	}

	// Should not panic and should return early on repo error.
	svc.tick(context.Background())
}

func TestTick_EmptySchedules(t *testing.T) {
	repo := &mockSyncScheduleStore{
		getDueSchedulesFn: func(ctx context.Context, hora string) ([]domain.SyncSchedule, error) {
			return nil, nil
		},
	}
	svc := &SchedulerService{
		scheduleRepo: repo,
		logger:       zap.NewNop(),
		lastFired:    make(map[uuid.UUID]string),
	}

	// Should not panic and should return early with no schedules due.
	svc.tick(context.Background())
}
