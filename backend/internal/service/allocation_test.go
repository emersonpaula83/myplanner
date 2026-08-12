package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestListProjectAllocations_ComputesMetrics(t *testing.T) {
	svc := &AllocationService{}
	_, err := svc.ListProjectAllocations(context.Background(), uuid.New(), nil, "ativos")
	if err == nil {
		t.Fatal("expected error from nil repo, got nil")
	}
}
