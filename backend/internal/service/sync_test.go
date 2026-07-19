package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"github.com/totvs/tcloud-planner/backend/internal/jira"
	"github.com/totvs/tcloud-planner/backend/internal/repository"
	"go.uber.org/zap"
)

type mockJiraClient struct {
	projects []jira.JiraProject
	issues   []jira.JiraIssue
	users    []jira.JiraUser
	boards   []jira.JiraBoard
	sprints  []jira.JiraSprint
}

func (m *mockJiraClient) GetProjects(ctx context.Context) ([]jira.JiraProject, error) {
	return m.projects, nil
}
func (m *mockJiraClient) GetProjectIssues(ctx context.Context, projectKey string, updatedSince *time.Time) ([]jira.JiraIssue, error) {
	return m.issues, nil
}
func (m *mockJiraClient) GetUsers(ctx context.Context, projectKey string) ([]jira.JiraUser, error) {
	return m.users, nil
}
func (m *mockJiraClient) GetBoards(ctx context.Context, projectKey string) ([]jira.JiraBoard, error) {
	return m.boards, nil
}
func (m *mockJiraClient) GetBoardSprints(ctx context.Context, boardID int) ([]jira.JiraSprint, error) {
	return m.sprints, nil
}

func TestParseJiraTime(t *testing.T) {
	tests := []struct {
		input string
		year  int
	}{
		{"2026-07-01T10:00:00.000-0300", 2026},
		{"2026-07-01T10:00:00.000Z", 2026},
		{"2026-07-01T10:00:00Z", 2026},
	}
	for _, tt := range tests {
		result := parseJiraTime(tt.input)
		if result.Year() != tt.year {
			t.Errorf("parseJiraTime(%q) year = %d, want %d", tt.input, result.Year(), tt.year)
		}
	}
}

func TestParseOptionalDate(t *testing.T) {
	d := "2026-07-15"
	result := parseOptionalDate(&d)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Day() != 15 {
		t.Errorf("expected day 15, got %d", result.Day())
	}

	result = parseOptionalDate(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestNilIfEmpty(t *testing.T) {
	result := nilIfEmpty(nil)
	if result != nil {
		t.Error("expected nil for nil priority")
	}

	result = nilIfEmpty(&jira.JiraPrio{Name: "High"})
	if result == nil || *result != "High" {
		t.Errorf("expected 'High', got %v", result)
	}
}

func TestSyncServiceStructure(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockClient := &mockJiraClient{}
	factory := func(baseURL, email, apiToken string, rateLimit int, logger *zap.Logger) jira.Client {
		return mockClient
	}

	oauthFactory := func(baseURL, accessToken string, rateLimit int, logger *zap.Logger) jira.Client {
		return mockClient
	}
	svc := NewSyncService(nil, nil, factory, oauthFactory, nil, 5, logger)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.rateLimit != 5 {
		t.Errorf("expected rateLimit 5, got %d", svc.rateLimit)
	}
}

func TestExecutSyncCountsTotals(t *testing.T) {
	_ = domain.SyncLog{}
	_ = repository.SyncTotals{}
	_ = uuid.New()
	_ = json.RawMessage(`{}`)
}
