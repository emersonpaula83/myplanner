package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockJiraClient struct {
	getProjectsFn          func(ctx context.Context) ([]jira.JiraProject, error)
	getProjectIssuesFn     func(ctx context.Context, projectKey string, updatedSince *time.Time) ([]jira.JiraIssue, error)
	getIssuesByProjectsFn  func(ctx context.Context, projectKeys []string, updatedSince *time.Time) ([]jira.JiraIssue, error)
	getUsersFn             func(ctx context.Context, projectKey string) ([]jira.JiraUser, error)
	getBoardsFn            func(ctx context.Context, projectKey string) ([]jira.JiraBoard, error)
	getBoardSprintsFn      func(ctx context.Context, boardID int) ([]jira.JiraSprint, error)
	getSprintFieldIDFn     func(ctx context.Context) (string, error)
	setSprintFieldIDFn     func(id string)
	setCustomFieldIDsFn    func(ids []string)
	discoverCustomFieldsFn func(ctx context.Context) (map[string]string, error)
	createSprintFn         func(ctx context.Context, boardID int, name string, startDate, endDate time.Time) (*jira.JiraSprint, error)
	assignIssueFn          func(ctx context.Context, issueKey, accountID string) error
	addCommentFn           func(ctx context.Context, issueKey, body string) error
	moveToSprintFn         func(ctx context.Context, sprintJiraID int, issueKey string) error
	updateTimeEstimateFn   func(ctx context.Context, issueKey string, seconds int) error
}

func (m *mockJiraClient) GetProjects(ctx context.Context) ([]jira.JiraProject, error) {
	return m.getProjectsFn(ctx)
}
func (m *mockJiraClient) GetProjectIssues(ctx context.Context, projectKey string, updatedSince *time.Time) ([]jira.JiraIssue, error) {
	return m.getProjectIssuesFn(ctx, projectKey, updatedSince)
}
func (m *mockJiraClient) GetIssuesByProjects(ctx context.Context, projectKeys []string, updatedSince *time.Time) ([]jira.JiraIssue, error) {
	return m.getIssuesByProjectsFn(ctx, projectKeys, updatedSince)
}
func (m *mockJiraClient) GetUsers(ctx context.Context, projectKey string) ([]jira.JiraUser, error) {
	return m.getUsersFn(ctx, projectKey)
}
func (m *mockJiraClient) GetBoards(ctx context.Context, projectKey string) ([]jira.JiraBoard, error) {
	return m.getBoardsFn(ctx, projectKey)
}
func (m *mockJiraClient) GetBoardSprints(ctx context.Context, boardID int) ([]jira.JiraSprint, error) {
	return m.getBoardSprintsFn(ctx, boardID)
}
func (m *mockJiraClient) GetSprintFieldID(ctx context.Context) (string, error) {
	return m.getSprintFieldIDFn(ctx)
}
func (m *mockJiraClient) SetSprintFieldID(id string) {
	if m.setSprintFieldIDFn != nil {
		m.setSprintFieldIDFn(id)
	}
}
func (m *mockJiraClient) SetCustomFieldIDs(ids []string) {
	if m.setCustomFieldIDsFn != nil {
		m.setCustomFieldIDsFn(ids)
	}
}
func (m *mockJiraClient) DiscoverCustomFields(ctx context.Context) (map[string]string, error) {
	return m.discoverCustomFieldsFn(ctx)
}
func (m *mockJiraClient) CreateSprint(ctx context.Context, boardID int, name string, startDate, endDate time.Time) (*jira.JiraSprint, error) {
	return m.createSprintFn(ctx, boardID, name, startDate, endDate)
}
func (m *mockJiraClient) AssignIssue(ctx context.Context, issueKey, accountID string) error {
	return m.assignIssueFn(ctx, issueKey, accountID)
}
func (m *mockJiraClient) AddComment(ctx context.Context, issueKey, body string) error {
	return m.addCommentFn(ctx, issueKey, body)
}
func (m *mockJiraClient) MoveToSprint(ctx context.Context, sprintJiraID int, issueKey string) error {
	return m.moveToSprintFn(ctx, sprintJiraID, issueKey)
}
func (m *mockJiraClient) UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error {
	return m.updateTimeEstimateFn(ctx, issueKey, seconds)
}

func newDefaultMockJiraClient() *mockJiraClient {
	return &mockJiraClient{
		getProjectsFn:          func(ctx context.Context) ([]jira.JiraProject, error) { return nil, nil },
		getProjectIssuesFn:     func(ctx context.Context, pk string, u *time.Time) ([]jira.JiraIssue, error) { return nil, nil },
		getIssuesByProjectsFn:  func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) { return nil, nil },
		getUsersFn:             func(ctx context.Context, pk string) ([]jira.JiraUser, error) { return nil, nil },
		getBoardsFn:            func(ctx context.Context, pk string) ([]jira.JiraBoard, error) { return nil, nil },
		getBoardSprintsFn:      func(ctx context.Context, bid int) ([]jira.JiraSprint, error) { return nil, nil },
		getSprintFieldIDFn:     func(ctx context.Context) (string, error) { return "", fmt.Errorf("not set") },
		discoverCustomFieldsFn: func(ctx context.Context) (map[string]string, error) { return nil, nil },
		createSprintFn: func(ctx context.Context, bid int, n string, s, e time.Time) (*jira.JiraSprint, error) {
			return nil, nil
		},
		assignIssueFn:        func(ctx context.Context, ik, aid string) error { return nil },
		addCommentFn:         func(ctx context.Context, ik, b string) error { return nil },
		moveToSprintFn:       func(ctx context.Context, sid int, ik string) error { return nil },
		updateTimeEstimateFn: func(ctx context.Context, ik string, s int) error { return nil },
	}
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
	logger := zap.NewNop()
	mockClient := newDefaultMockJiraClient()
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

func TestGetStatus(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()

	t.Run("returns latest sync log", func(t *testing.T) {
		expected := &domain.SyncLog{ID: uuid.New(), Status: "success"}
		repo := &mockSyncRepoStore{
			getLatestSyncLogFn: func(ctx context.Context, id uuid.UUID) (*domain.SyncLog, error) {
				if id != fonteID {
					t.Errorf("unexpected fonteID: %v", id)
				}
				return expected, nil
			},
		}
		svc := NewSyncService(repo, nil, nil, nil, nil, 0, zap.NewNop())
		result, err := svc.GetStatus(ctx, fonteID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ID != expected.ID {
			t.Errorf("expected log ID %v, got %v", expected.ID, result.ID)
		}
	})

	t.Run("returns error from repo", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			getLatestSyncLogFn: func(ctx context.Context, id uuid.UUID) (*domain.SyncLog, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSyncService(repo, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.GetStatus(ctx, fonteID)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestListLogs(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()

	t.Run("returns logs with limit", func(t *testing.T) {
		expected := []domain.SyncLog{{ID: uuid.New()}, {ID: uuid.New()}}
		repo := &mockSyncRepoStore{
			listSyncLogsFn: func(ctx context.Context, id uuid.UUID, limit int) ([]domain.SyncLog, error) {
				if limit != 10 {
					t.Errorf("expected limit 10, got %d", limit)
				}
				return expected, nil
			},
		}
		svc := NewSyncService(repo, nil, nil, nil, nil, 0, zap.NewNop())
		result, err := svc.ListLogs(ctx, fonteID, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 logs, got %d", len(result))
		}
	})

	t.Run("returns error from repo", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			listSyncLogsFn: func(ctx context.Context, id uuid.UUID, limit int) ([]domain.SyncLog, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSyncService(repo, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.ListLogs(ctx, fonteID, 10)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestListJiraProjects_GetFonteError(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()

	t.Run("fonte not found", func(t *testing.T) {
		fdRepo := &mockFonteDadosStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) {
				return nil, nil
			},
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(nil, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.ListJiraProjects(ctx, fonteID)
		if err == nil {
			t.Fatal("expected error for nil fonte")
		}
	})

	t.Run("db error", func(t *testing.T) {
		fdRepo := &mockFonteDadosStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) {
				return nil, fmt.Errorf("connection refused")
			},
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(nil, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.ListJiraProjects(ctx, fonteID)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestListJiraProjects_BuildClientAPIToken(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "user@test.com"
	token := "api-token"

	fonte := &domain.FonteDados{
		ID:        fonteID,
		Nome:      "Test",
		BaseURL:   "https://test.atlassian.net",
		AuthType:  "api_token",
		UserEmail: &email,
		APIToken:  &token,
	}

	mockClient := newDefaultMockJiraClient()
	mockClient.getProjectsFn = func(ctx context.Context) ([]jira.JiraProject, error) {
		return []jira.JiraProject{
			{ID: "1", Key: "PROJ", Name: "Project One"},
		}, nil
	}

	var capturedBaseURL, capturedEmail, capturedToken string
	factory := func(baseURL, email, apiToken string, rateLimit int, logger *zap.Logger) jira.Client {
		capturedBaseURL = baseURL
		capturedEmail = email
		capturedToken = apiToken
		return mockClient
	}

	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}

	svc := NewSyncService(nil, fdRepo, factory, nil, nil, 10, zap.NewNop())
	result, err := svc.ListJiraProjects(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result))
	}
	if result[0].Key != "PROJ" {
		t.Errorf("expected key PROJ, got %s", result[0].Key)
	}
	if capturedBaseURL != fonte.BaseURL {
		t.Errorf("factory got baseURL %q, want %q", capturedBaseURL, fonte.BaseURL)
	}
	if capturedEmail != email {
		t.Errorf("factory got email %q, want %q", capturedEmail, email)
	}
	if capturedToken != token {
		t.Errorf("factory got token %q, want %q", capturedToken, token)
	}
}

func TestListJiraProjects_BuildClientOAuth2(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	accessToken := "valid-access-token"
	refreshToken := "refresh-token"
	expiry := time.Now().Add(1 * time.Hour)

	fonte := &domain.FonteDados{
		ID:                 fonteID,
		Nome:               "OAuth Test",
		BaseURL:            "https://api.atlassian.com/ex/jira/cloud-id",
		AuthType:           "oauth2",
		OAuth2AccessToken:  &accessToken,
		OAuth2RefreshToken: &refreshToken,
		OAuth2TokenExpiry:  &expiry,
	}

	mockClient := newDefaultMockJiraClient()
	mockClient.getProjectsFn = func(ctx context.Context) ([]jira.JiraProject, error) {
		return []jira.JiraProject{{ID: "1", Key: "OA", Name: "OAuth Project"}}, nil
	}

	var capturedAccessToken string
	oauthFactory := func(baseURL, at string, rateLimit int, logger *zap.Logger) jira.Client {
		capturedAccessToken = at
		return mockClient
	}

	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}

	svc := NewSyncService(nil, fdRepo, nil, oauthFactory, nil, 10, zap.NewNop())
	result, err := svc.ListJiraProjects(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Key != "OA" {
		t.Fatalf("unexpected result: %v", result)
	}
	if capturedAccessToken != accessToken {
		t.Errorf("expected access token %q, got %q", accessToken, capturedAccessToken)
	}
}

func TestListJiraProjects_OAuth2MissingTokens(t *testing.T) {
	ctx := context.Background()
	fonte := &domain.FonteDados{
		ID:       uuid.New(),
		Nome:     "Missing Tokens",
		BaseURL:  "https://test.atlassian.net",
		AuthType: "oauth2",
	}
	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}
	svc := NewSyncService(nil, fdRepo, nil, nil, nil, 0, zap.NewNop())
	_, err := svc.ListJiraProjects(ctx, fonte.ID)
	if err == nil {
		t.Fatal("expected error for missing oauth2 tokens")
	}
}

func TestListJiraProjects_OAuth2ExpiredNoService(t *testing.T) {
	ctx := context.Background()
	accessToken := "expired"
	refreshToken := "refresh"
	expiry := time.Now().Add(-1 * time.Hour)
	fonte := &domain.FonteDados{
		ID:                 uuid.New(),
		Nome:               "Expired No Svc",
		BaseURL:            "https://test.atlassian.net",
		AuthType:           "oauth2",
		OAuth2AccessToken:  &accessToken,
		OAuth2RefreshToken: &refreshToken,
		OAuth2TokenExpiry:  &expiry,
	}
	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}
	svc := NewSyncService(nil, fdRepo, nil, nil, nil, 0, zap.NewNop())
	_, err := svc.ListJiraProjects(ctx, fonte.ID)
	if err == nil {
		t.Fatal("expected error for expired token with no oauth service")
	}
}

func TestListJiraProjects_GetProjectsError(t *testing.T) {
	ctx := context.Background()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID:        uuid.New(),
		Nome:      "Test",
		BaseURL:   "https://test.atlassian.net",
		AuthType:  "api_token",
		UserEmail: &email,
		APIToken:  &token,
	}
	mockClient := newDefaultMockJiraClient()
	mockClient.getProjectsFn = func(ctx context.Context) ([]jira.JiraProject, error) {
		return nil, fmt.Errorf("jira api error")
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}
	svc := NewSyncService(nil, fdRepo, factory, nil, nil, 0, zap.NewNop())
	_, err := svc.ListJiraProjects(ctx, fonte.ID)
	if err == nil {
		t.Fatal("expected error from GetProjects")
	}
}

func TestSyncProject(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	t.Run("already running", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return true, nil
			},
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProject(ctx, fonteID, "PROJ")
		if !errors.Is(err, ErrSyncAlreadyRunning) {
			t.Fatalf("expected ErrSyncAlreadyRunning, got %v", err)
		}
	})

	t.Run("HasRunningSyncForProject error", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, fmt.Errorf("db error")
			},
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProject(ctx, fonteID, "PROJ")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("getFonte error", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return nil, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProject(ctx, fonteID, "PROJ")
		if err == nil {
			t.Fatal("expected error for nil fonte")
		}
	})

	t.Run("buildClient error", func(t *testing.T) {
		oauthFonte := &domain.FonteDados{
			ID:       fonteID,
			Nome:     "OAuth Missing Tokens",
			BaseURL:  "https://test.atlassian.net",
			AuthType: "oauth2",
		}
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return oauthFonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProject(ctx, fonteID, "PROJ")
		if err == nil {
			t.Fatal("expected error from buildClient (missing oauth2 tokens)")
		}
	})

	t.Run("CreateSyncLog error", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
			createSyncLogFn: func(ctx context.Context, log *domain.SyncLog) error {
				return fmt.Errorf("insert failed")
			},
		}
		mockClient := newDefaultMockJiraClient()
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProject(ctx, fonteID, "PROJ")
		if err == nil {
			t.Fatal("expected error from CreateSyncLog")
		}
	})

	t.Run("happy path returns sync log", func(t *testing.T) {
		var capturedLog *domain.SyncLog
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
			createSyncLogFn: func(ctx context.Context, log *domain.SyncLog) error {
				capturedLog = log
				return nil
			},
			// These are called by the runProjectSync goroutine; set safe defaults
			updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error {
				return nil
			},
			updateSyncLogTotalsFn:      func(ctx context.Context, id uuid.UUID, t repository.SyncTotals) error { return nil },
			getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		}

		mockClient := newDefaultMockJiraClient()
		mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
			return nil, nil
		}
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
		result, err := svc.SyncProject(ctx, fonteID, "PROJ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Tipo != "project" {
			t.Errorf("expected Tipo 'project', got %q", result.Tipo)
		}
		if result.Status != "running" {
			t.Errorf("expected Status 'running', got %q", result.Status)
		}
		if result.Origem != "manual" {
			t.Errorf("expected Origem 'manual', got %q", result.Origem)
		}
		if capturedLog.ProjectKey == nil || *capturedLog.ProjectKey != "PROJ" {
			t.Error("expected ProjectKey to be set to PROJ")
		}
		// Give goroutine time to complete
		time.Sleep(50 * time.Millisecond)
	})
}

func TestSyncProjectScheduled(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	t.Run("already running", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return true, nil
			},
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProjectScheduled(ctx, fonteID, "PROJ")
		if !errors.Is(err, ErrSyncAlreadyRunning) {
			t.Fatalf("expected ErrSyncAlreadyRunning, got %v", err)
		}
	})

	t.Run("getFonte error", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return nil, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProjectScheduled(ctx, fonteID, "PROJ")
		if err == nil {
			t.Fatal("expected error for nil fonte")
		}
	})

	t.Run("buildClient error", func(t *testing.T) {
		oauthFonte := &domain.FonteDados{
			ID:       fonteID,
			Nome:     "OAuth Missing Tokens",
			BaseURL:  "https://test.atlassian.net",
			AuthType: "oauth2",
		}
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return oauthFonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProjectScheduled(ctx, fonteID, "PROJ")
		if err == nil {
			t.Fatal("expected error from buildClient (missing oauth2 tokens)")
		}
	})

	t.Run("CreateSyncLog error", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
			createSyncLogFn: func(ctx context.Context, log *domain.SyncLog) error {
				return fmt.Errorf("insert failed")
			},
		}
		mockClient := newDefaultMockJiraClient()
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProjectScheduled(ctx, fonteID, "PROJ")
		if err == nil {
			t.Fatal("expected error from CreateSyncLog")
		}
	})

	t.Run("happy path returns scheduled origin", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
			createSyncLogFn: func(ctx context.Context, log *domain.SyncLog) error { return nil },
			updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error {
				return nil
			},
			updateSyncLogTotalsFn:      func(ctx context.Context, id uuid.UUID, t repository.SyncTotals) error { return nil },
			getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		}
		mockClient := newDefaultMockJiraClient()
		mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
			return nil, nil
		}
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
		result, err := svc.SyncProjectScheduled(ctx, fonteID, "PROJ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Tipo != "project" {
			t.Errorf("expected Tipo 'project', got %q", result.Tipo)
		}
		if result.Status != "running" {
			t.Errorf("expected Status 'running', got %q", result.Status)
		}
		if result.Origem != "scheduled" {
			t.Errorf("expected Origem 'scheduled', got %q", result.Origem)
		}
		time.Sleep(50 * time.Millisecond)
	})
}

func TestSyncFonteDados(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	t.Run("getFonte error", func(t *testing.T) {
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return nil, fmt.Errorf("db down") },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(nil, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncFonteDados(ctx, fonteID)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("sync already running", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) { return true, nil },
		}
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncFonteDados(ctx, fonteID)
		if !errors.Is(err, ErrSyncAlreadyRunning) {
			t.Fatalf("expected ErrSyncAlreadyRunning, got %v", err)
		}
	})

	t.Run("happy path full sync", func(t *testing.T) {
		var updatedSyncLog bool
		var updatedUltimoSync bool

		repo := &mockSyncRepoStore{
			hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
			createSyncLogFn:  func(ctx context.Context, log *domain.SyncLog) error { return nil },
			getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) {
				return []string{"PROJ"}, nil
			},
			updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error {
				updatedSyncLog = true
				return nil
			},
			autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 0, nil },
			getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
			undeleteReappearedFn:       func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
			softDeleteAbsentFn:         func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		}

		mockClient := newDefaultMockJiraClient()
		mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
			return nil, nil
		}
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }

		fdRepo := &mockFonteDadosStore{
			getByIDFn:         func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn: func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error {
				updatedUltimoSync = true
				return nil
			},
		}

		svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
		result, err := svc.SyncFonteDados(ctx, fonteID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Tipo != "full" {
			t.Errorf("expected Tipo 'full', got %q", result.Tipo)
		}
		if result.Status != "success" {
			t.Errorf("expected Status 'success', got %q", result.Status)
		}
		if !updatedSyncLog {
			t.Error("expected UpdateSyncLog to be called")
		}
		if !updatedUltimoSync {
			t.Error("expected UpdateUltimoSync to be called")
		}
	})
}

func TestSyncAll(t *testing.T) {
	ctx := context.Background()

	t.Run("GetFonteDadosAtivas error", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			getFonteDadosAtivasFn: func(ctx context.Context) ([]domain.FonteDados, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSyncService(repo, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncAll(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty fontes returns empty logs", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			getFonteDadosAtivasFn: func(ctx context.Context) ([]domain.FonteDados, error) {
				return []domain.FonteDados{}, nil
			},
		}
		svc := NewSyncService(repo, nil, nil, nil, nil, 0, zap.NewNop())
		logs, err := svc.SyncAll(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logs) != 0 {
			t.Errorf("expected 0 logs, got %d", len(logs))
		}
	})

	t.Run("continues on per-fonte error", func(t *testing.T) {
		email := "u@t.com"
		token := "t"
		fontes := []domain.FonteDados{
			{ID: uuid.New(), Nome: "Fail", BaseURL: "https://fail.net", AuthType: "api_token", UserEmail: &email, APIToken: &token},
			{ID: uuid.New(), Nome: "OK", BaseURL: "https://ok.net", AuthType: "api_token", UserEmail: &email, APIToken: &token},
		}
		callCount := 0
		repo := &mockSyncRepoStore{
			getFonteDadosAtivasFn: func(ctx context.Context) ([]domain.FonteDados, error) {
				return fontes, nil
			},
			hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) {
				callCount++
				if callCount == 1 {
					return false, fmt.Errorf("db flake")
				}
				return false, nil
			},
			createSyncLogFn:         func(ctx context.Context, log *domain.SyncLog) error { return nil },
			getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) { return nil, nil },
			updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error {
				return nil
			},
			autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 0, nil },
			getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
			undeleteReappearedFn:       func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
			softDeleteAbsentFn:         func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		}
		mockClient := newDefaultMockJiraClient()
		mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
			return nil, nil
		}
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return nil, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
		logs, err := svc.SyncAll(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logs) != 1 {
			t.Errorf("expected 1 successful log (2nd fonte), got %d", len(logs))
		}
	})
}

func TestSyncFonteDados_WithIssues(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	projetoID := uuid.New()
	membroID := uuid.New()
	sprintUUID := uuid.New()
	tarefaID := uuid.New()

	var upsertedTarefas int

	repo := &mockSyncRepoStore{
		hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
		createSyncLogFn:  func(ctx context.Context, log *domain.SyncLog) error { return nil },
		getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) {
			return []string{"PROJ"}, nil
		},
		upsertProjetoFn: func(ctx context.Context, fid uuid.UUID, jiraID, chave, nome string, desc *string, leadID *uuid.UUID, cat *string) (uuid.UUID, error) {
			return projetoID, nil
		},
		upsertMembroFn: func(ctx context.Context, fid uuid.UUID, accountID, nome string, email, avatar, team *string) (uuid.UUID, error) {
			return membroID, nil
		},
		upsertSprintFn: func(ctx context.Context, fid uuid.UUID, jiraID int, nome string, estado *string, di, df, dc *time.Time, bid *int, pid *uuid.UUID) (uuid.UUID, error) {
			return sprintUUID, nil
		},
		upsertTarefaFn: func(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error) {
			upsertedTarefas++
			return tarefaID, nil
		},
		undeleteReappearedFn:       func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		softDeleteAbsentFn:         func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, tots repository.SyncTotals, e json.RawMessage, m *string) error {
			return nil
		},
		autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 0, nil },
	}

	sprintState := "active"
	issue := jira.JiraIssue{ID: "10001", Key: "PROJ-1"}
	issue.Fields.Summary = "Test Task"
	issue.Fields.IssueType = jira.JiraType{Name: "Story"}
	issue.Fields.Status = jira.JiraStatus{Name: "To Do"}
	issue.Fields.Status.StatusCategory.Key = "new"
	issue.Fields.Project = jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"}
	issue.Fields.Assignee = &jira.JiraUser{AccountID: "acc1", DisplayName: "User 1"}
	issue.Fields.Sprint = &jira.JiraSprint{ID: 100, Name: "Sprint 1", State: sprintState}
	issue.Fields.Created = "2026-01-01T10:00:00.000Z"

	mockClient := newDefaultMockJiraClient()
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return []jira.JiraIssue{issue}, nil
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }

	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}

	svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
	result, err := svc.SyncFonteDados(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got %q", result.Status)
	}
	if result.TotalTarefas != 1 {
		t.Errorf("expected 1 tarefa, got %d", result.TotalTarefas)
	}
	if upsertedTarefas != 1 {
		t.Errorf("expected 1 upsertTarefa call, got %d", upsertedTarefas)
	}
}

func TestSyncFonteDados_ProjectKeysFallback(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	var getProjectsCalled bool
	repo := &mockSyncRepoStore{
		hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
		createSyncLogFn:  func(ctx context.Context, log *domain.SyncLog) error { return nil },
		getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) {
			return []string{}, nil
		},
		updateSyncLogFn:            func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error { return nil },
		autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 0, nil },
		getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		undeleteReappearedFn:       func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		softDeleteAbsentFn:         func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
	}
	mockClient := newDefaultMockJiraClient()
	mockClient.getProjectsFn = func(ctx context.Context) ([]jira.JiraProject, error) {
		getProjectsCalled = true
		return []jira.JiraProject{{ID: "1", Key: "FB", Name: "Fallback"}}, nil
	}
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return nil, nil
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}
	svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
	_, err := svc.SyncFonteDados(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !getProjectsCalled {
		t.Error("expected fallback to GetProjects when project keys empty")
	}
}

func TestSyncFonteDados_PartialErrors(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	var capturedStatus string
	repo := &mockSyncRepoStore{
		hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
		createSyncLogFn:  func(ctx context.Context, log *domain.SyncLog) error { return nil },
		getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) {
			return []string{"PROJ"}, nil
		},
		upsertProjetoFn: func(ctx context.Context, fid uuid.UUID, jiraID, chave, nome string, desc *string, leadID *uuid.UUID, cat *string) (uuid.UUID, error) {
			return uuid.Nil, fmt.Errorf("upsert project failed")
		},
		updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error {
			capturedStatus = s
			return nil
		},
		autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 0, nil },
		getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		undeleteReappearedFn:       func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		softDeleteAbsentFn:         func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
	}
	issue := jira.JiraIssue{ID: "10001", Key: "PROJ-1"}
	issue.Fields.Summary = "Task"
	issue.Fields.IssueType = jira.JiraType{Name: "Story"}
	issue.Fields.Status = jira.JiraStatus{Name: "To Do"}
	issue.Fields.Status.StatusCategory.Key = "new"
	issue.Fields.Project = jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"}
	issue.Fields.Created = "2026-01-01T10:00:00.000Z"

	mockClient := newDefaultMockJiraClient()
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return []jira.JiraIssue{issue}, nil
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}
	svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
	result, err := svc.SyncFonteDados(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStatus != "partial" {
		t.Errorf("expected sync log status 'partial', got %q", capturedStatus)
	}
	if result.Status != "partial" {
		t.Errorf("expected result status 'partial', got %q", result.Status)
	}
}

func TestSyncEpicTasks(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	t.Run("getFonte error", func(t *testing.T) {
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return nil, fmt.Errorf("db error") },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(nil, fdRepo, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncEpicTasks(ctx, fonteID, "PROJ-100")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid epic key no dash", func(t *testing.T) {
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		mockClient := newDefaultMockJiraClient()
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		svc := NewSyncService(nil, fdRepo, factory, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncEpicTasks(ctx, fonteID, "INVALIDKEY")
		if err == nil {
			t.Fatal("expected error for invalid epic key")
		}
	})

	t.Run("no matching issues returns 0", func(t *testing.T) {
		mockClient := newDefaultMockJiraClient()
		mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
			issue := jira.JiraIssue{ID: "1", Key: "PROJ-1"}
			issue.Fields.Summary = "Unrelated"
			issue.Fields.IssueType = jira.JiraType{Name: "Story"}
			issue.Fields.Status = jira.JiraStatus{Name: "To Do"}
			issue.Fields.Status.StatusCategory.Key = "new"
			issue.Fields.Project = jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"}
			issue.Fields.Created = "2026-01-01T10:00:00.000Z"
			return []jira.JiraIssue{issue}, nil
		}
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(nil, fdRepo, factory, nil, nil, 0, zap.NewNop())
		count, err := svc.SyncEpicTasks(ctx, fonteID, "PROJ-100")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("processes matching epic children", func(t *testing.T) {
		projetoID := uuid.New()
		membroID := uuid.New()
		tarefaID := uuid.New()

		repo := &mockSyncRepoStore{
			upsertProjetoFn: func(ctx context.Context, fid uuid.UUID, jiraID, chave, nome string, desc *string, leadID *uuid.UUID, cat *string) (uuid.UUID, error) {
				return projetoID, nil
			},
			upsertMembroFn: func(ctx context.Context, fid uuid.UUID, accountID, nome string, email, avatar, team *string) (uuid.UUID, error) {
				return membroID, nil
			},
			upsertTarefaFn: func(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error) {
				return tarefaID, nil
			},
			upsertSprintFn: func(ctx context.Context, fid uuid.UUID, jiraID int, nome string, estado *string, di, df, dc *time.Time, bid *int, pid *uuid.UUID) (uuid.UUID, error) {
				return uuid.New(), nil
			},
		}

		mockClient := newDefaultMockJiraClient()
		mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
			child := jira.JiraIssue{ID: "1", Key: "PROJ-1"}
			child.Fields.Summary = "Child task"
			child.Fields.IssueType = jira.JiraType{Name: "Sub-task"}
			child.Fields.Status = jira.JiraStatus{Name: "Done"}
			child.Fields.Status.StatusCategory.Key = "done"
			child.Fields.Project = jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"}
			child.Fields.Parent = &struct {
				ID  string `json:"id"`
				Key string `json:"key"`
			}{Key: "PROJ-100"}
			child.Fields.Created = "2026-01-01T10:00:00.000Z"

			notChild := jira.JiraIssue{ID: "2", Key: "PROJ-2"}
			notChild.Fields.Summary = "Not a child"
			notChild.Fields.IssueType = jira.JiraType{Name: "Story"}
			notChild.Fields.Status = jira.JiraStatus{Name: "To Do"}
			notChild.Fields.Status.StatusCategory.Key = "new"
			notChild.Fields.Project = jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"}
			notChild.Fields.Created = "2026-01-01T10:00:00.000Z"

			return []jira.JiraIssue{child, notChild}, nil
		}
		factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
		fdRepo := &mockFonteDadosStore{
			getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
			saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
			updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
		}
		svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
		count, err := svc.SyncEpicTasks(ctx, fonteID, "PROJ-100")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 child processed, got %d", count)
		}
	})
}

func TestExtractSprintEntryDate(t *testing.T) {
	t.Run("no sprint field items", func(t *testing.T) {
		cl := &jira.JiraChangelog{
			Histories: []jira.JiraHistory{
				{Created: "2026-01-01T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "status", ToString: "In Progress"},
				}},
			},
		}
		result := extractSprintEntryDate(cl, "Sprint 1")
		if result != nil {
			t.Error("expected nil when no Sprint field changes")
		}
	})

	t.Run("finds matching sprint entry", func(t *testing.T) {
		cl := &jira.JiraChangelog{
			Histories: []jira.JiraHistory{
				{Created: "2026-03-01T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "Sprint", ToString: "Sprint 1"},
				}},
				{Created: "2026-04-01T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "Sprint", ToString: "Sprint 1, Sprint 2"},
				}},
			},
		}
		result := extractSprintEntryDate(cl, "Sprint 1")
		if result == nil {
			t.Fatal("expected non-nil date")
		}
		if result.Month() != 4 {
			t.Errorf("expected latest match (April), got month %d", result.Month())
		}
	})
}

func TestExtractFirstInProgressDate(t *testing.T) {
	t.Run("no in-progress transitions", func(t *testing.T) {
		cl := &jira.JiraChangelog{
			Histories: []jira.JiraHistory{
				{Created: "2026-01-01T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "status", ToString: "To Do"},
				}},
			},
		}
		result := extractFirstInProgressDate(cl)
		if result != nil {
			t.Error("expected nil")
		}
	})

	t.Run("finds earliest in-progress", func(t *testing.T) {
		cl := &jira.JiraChangelog{
			Histories: []jira.JiraHistory{
				{Created: "2026-03-01T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "status", ToString: "In Progress"},
				}},
				{Created: "2026-01-01T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "status", ToString: "Em Andamento"},
				}},
			},
		}
		result := extractFirstInProgressDate(cl)
		if result == nil {
			t.Fatal("expected non-nil")
		}
		if result.Month() != 1 {
			t.Errorf("expected earliest (January), got month %d", result.Month())
		}
	})
}

func TestResolveParents(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	fonte := &domain.FonteDados{ID: fonteID}
	tarefaID := uuid.New()
	parentID := uuid.New()

	t.Run("resolves parent successfully", func(t *testing.T) {
		var updatedParent bool
		repo := &mockSyncRepoStore{
			lookupTarefaIDByJiraIDFn: func(ctx context.Context, fid uuid.UUID, jiraID string) (uuid.UUID, error) {
				return parentID, nil
			},
			updateTarefaParentFn: func(ctx context.Context, tid, pid uuid.UUID) error {
				updatedParent = true
				return nil
			},
		}
		svc := &SyncService{repo: repo, logger: zap.NewNop()}
		errs := svc.resolveParents(ctx, fonte, []parentRef{{tarefaID: tarefaID, parentJiraID: "10001"}})
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d", len(errs))
		}
		if !updatedParent {
			t.Error("expected UpdateTarefaParent to be called")
		}
	})

	t.Run("skips when parent not found", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			lookupTarefaIDByJiraIDFn: func(ctx context.Context, fid uuid.UUID, jiraID string) (uuid.UUID, error) {
				return uuid.Nil, nil
			},
		}
		svc := &SyncService{repo: repo, logger: zap.NewNop()}
		errs := svc.resolveParents(ctx, fonte, []parentRef{{tarefaID: tarefaID, parentJiraID: "99999"}})
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d", len(errs))
		}
	})

	t.Run("collects lookup errors", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			lookupTarefaIDByJiraIDFn: func(ctx context.Context, fid uuid.UUID, jiraID string) (uuid.UUID, error) {
				return uuid.Nil, fmt.Errorf("lookup failed")
			},
		}
		svc := &SyncService{repo: repo, logger: zap.NewNop()}
		errs := svc.resolveParents(ctx, fonte, []parentRef{{tarefaID: tarefaID, parentJiraID: "10001"}})
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d", len(errs))
		}
	})
}

func TestFlushProgress(t *testing.T) {
	ctx := context.Background()

	t.Run("nil syncLogID is no-op", func(t *testing.T) {
		svc := &SyncService{logger: zap.NewNop()}
		svc.flushProgress(ctx, nil, repository.SyncTotals{})
	})

	t.Run("calls UpdateSyncLogTotals", func(t *testing.T) {
		var called bool
		logID := uuid.New()
		repo := &mockSyncRepoStore{
			updateSyncLogTotalsFn: func(ctx context.Context, id uuid.UUID, t repository.SyncTotals) error {
				called = true
				return nil
			},
		}
		svc := &SyncService{repo: repo, logger: zap.NewNop()}
		svc.flushProgress(ctx, &logID, repository.SyncTotals{Tarefas: 5})
		if !called {
			t.Error("expected UpdateSyncLogTotals to be called")
		}
	})
}

func TestSyncFonteDados_WithRichIssues(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	projetoID := uuid.New()
	membroID := uuid.New()
	sprintUUID := uuid.New()
	tarefaID := uuid.New()
	produtoID := uuid.New()

	var upsertedTarefas int
	var linkedProdutos int
	var upsertedProdutos int

	repo := &mockSyncRepoStore{
		hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
		createSyncLogFn:  func(ctx context.Context, log *domain.SyncLog) error { return nil },
		getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) {
			return []string{"PROJ"}, nil
		},
		upsertProjetoFn: func(ctx context.Context, fid uuid.UUID, jiraID, chave, nome string, desc *string, leadID *uuid.UUID, cat *string) (uuid.UUID, error) {
			return projetoID, nil
		},
		upsertMembroFn: func(ctx context.Context, fid uuid.UUID, accountID, nome string, emailP, avatar, team *string) (uuid.UUID, error) {
			return membroID, nil
		},
		upsertSprintFn: func(ctx context.Context, fid uuid.UUID, jiraID int, nome string, estado *string, di, df, dc *time.Time, bid *int, pid *uuid.UUID) (uuid.UUID, error) {
			return sprintUUID, nil
		},
		upsertTarefaFn: func(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error) {
			upsertedTarefas++
			if t.EstimativaTempo == nil || *t.EstimativaTempo != 3600 {
				return uuid.Nil, fmt.Errorf("expected EstimativaTempo 3600")
			}
			if t.TempoGasto == nil || *t.TempoGasto != 1800 {
				return uuid.Nil, fmt.Errorf("expected TempoGasto 1800")
			}
			return tarefaID, nil
		},
		upsertProdutoFn: func(ctx context.Context, fid uuid.UUID, jiraID, nome string, desc *string, projID *uuid.UUID) (uuid.UUID, error) {
			upsertedProdutos++
			return produtoID, nil
		},
		linkTarefaProdutoFn: func(ctx context.Context, tid, pid uuid.UUID) error {
			linkedProdutos++
			return nil
		},
		lookupTarefaIDByJiraIDFn: func(ctx context.Context, fid uuid.UUID, jiraID string) (uuid.UUID, error) {
			return uuid.New(), nil
		},
		updateTarefaParentFn: func(ctx context.Context, tid, pid uuid.UUID) error {
			return nil
		},
		undeleteReappearedFn:       func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 1, nil },
		softDeleteAbsentFn:         func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 2, nil },
		getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, tots repository.SyncTotals, e json.RawMessage, m *string) error {
			return nil
		},
		autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 1, nil },
	}

	mockClient := newDefaultMockJiraClient()
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		var issue jira.JiraIssue
		issue.ID = "10001"
		issue.Key = "PROJ-1"
		issue.Fields.Summary = "Rich Task"
		issue.Fields.IssueType.Name = "Story"
		issue.Fields.Status.Name = "In Progress"
		issue.Fields.Status.StatusCategory.Key = "indeterminate"
		issue.Fields.Project.ID = "P1"
		issue.Fields.Project.Key = "PROJ"
		issue.Fields.Project.Name = "Project"
		issue.Fields.Created = "2026-01-01T10:00:00.000Z"
		issue.Fields.Updated = "2026-02-01T10:00:00.000Z"
		due := "2026-03-01"
		issue.Fields.DueDate = &due
		resolved := "2026-02-15T10:00:00.000Z"
		issue.Fields.ResolutionDate = &resolved
		issue.Fields.Assignee = &jira.JiraUser{AccountID: "acc1", DisplayName: "User 1", EmailAddress: "u1@t.com"}
		issue.Fields.Reporter = &jira.JiraUser{AccountID: "acc2", DisplayName: "User 2"}
		issue.Fields.Priority = &jira.JiraPrio{Name: "High"}
		sp := float64(5)
		issue.Fields.StoryPoints = &sp
		issue.Fields.TimeTracking = &struct {
			OriginalEstimateSeconds int `json:"originalEstimateSeconds"`
			TimeSpentSeconds        int `json:"timeSpentSeconds"`
		}{OriginalEstimateSeconds: 3600, TimeSpentSeconds: 1800}
		issue.Fields.Sprint = &jira.JiraSprint{ID: 100, Name: "Sprint 1", State: "active", OriginBoardID: 5}
		issue.Fields.Parent = &struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		}{ID: "9999", Key: "PROJ-0"}
		issue.Fields.Components = []jira.JiraComponent{{ID: "C1", Name: "Backend"}}
		issue.Changelog = &jira.JiraChangelog{
			Histories: []jira.JiraHistory{
				{Created: "2026-01-15T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "status", ToString: "In Progress"},
				}},
				{Created: "2026-01-10T10:00:00.000Z", Items: []jira.JiraHistoryItem{
					{Field: "Sprint", ToString: "Sprint 1"},
				}},
			},
		}

		return []jira.JiraIssue{issue}, nil
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }

	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}

	svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
	result, err := svc.SyncFonteDados(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got %q", result.Status)
	}
	if upsertedTarefas != 1 {
		t.Errorf("expected 1 upserted tarefa, got %d", upsertedTarefas)
	}
	if upsertedProdutos != 1 {
		t.Errorf("expected 1 upserted produto, got %d", upsertedProdutos)
	}
	if linkedProdutos != 1 {
		t.Errorf("expected 1 linked produto, got %d", linkedProdutos)
	}
	if result.TotalTarefas != 1 {
		t.Errorf("expected TotalTarefas 1, got %d", result.TotalTarefas)
	}
}

func TestSyncFonteDados_CustomFieldDiscovery(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	var updatedCFMap bool
	repo := &mockSyncRepoStore{
		hasRunningSyncFn: func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
		createSyncLogFn:  func(ctx context.Context, log *domain.SyncLog) error { return nil },
		getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) {
			return []string{"PROJ"}, nil
		},
		updateCustomFieldMapFn: func(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error {
			updatedCFMap = true
			return nil
		},
		undeleteReappearedFn:       func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		softDeleteAbsentFn:         func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) { return 0, nil },
		getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		updateSyncLogFn:            func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error { return nil },
		autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 0, nil },
	}
	mockClient := newDefaultMockJiraClient()
	mockClient.getSprintFieldIDFn = func(ctx context.Context) (string, error) {
		return "customfield_10020", nil
	}
	mockClient.discoverCustomFieldsFn = func(ctx context.Context) (map[string]string, error) {
		return map[string]string{"customfield_10100": "tipo_demanda"}, nil
	}
	var capturedIDs []string
	mockClient.setCustomFieldIDsFn = func(ids []string) {
		capturedIDs = ids
	}
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return nil, nil
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }

	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}

	svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
	_, err := svc.SyncFonteDados(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updatedCFMap {
		t.Error("expected UpdateCustomFieldMap to be called")
	}
	if len(capturedIDs) != 1 || capturedIDs[0] != "customfield_10100" {
		t.Errorf("expected SetCustomFieldIDs with [customfield_10100], got %v", capturedIDs)
	}
}

func TestSyncFonteDados_SoftDeleteAndUndelete(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	var softDeleteCalled, undeleteCalled bool
	repo := &mockSyncRepoStore{
		hasRunningSyncFn:        func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
		createSyncLogFn:         func(ctx context.Context, log *domain.SyncLog) error { return nil },
		getProjectKeysForSyncFn: func(ctx context.Context, fid uuid.UUID) ([]string, error) { return []string{"P"}, nil },
		undeleteReappearedFn: func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) {
			undeleteCalled = true
			return 3, nil
		},
		softDeleteAbsentFn: func(ctx context.Context, fid uuid.UUID, ids []string) (int64, error) {
			softDeleteCalled = true
			return 5, nil
		},
		getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
		updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, tots repository.SyncTotals, e json.RawMessage, m *string) error {
			if tots.Removidos != 5 {
				t.Errorf("expected Removidos=5, got %d", tots.Removidos)
			}
			return nil
		},
		autoDetectEquipeBoardIDsFn: func(ctx context.Context, fid uuid.UUID) (int, error) { return 0, nil },
	}

	mockClient := newDefaultMockJiraClient()
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return nil, nil
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
	fdRepo := &mockFonteDadosStore{
		getByIDFn:          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn:  func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error { return nil },
	}

	svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
	_, err := svc.SyncFonteDados(ctx, fonteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !softDeleteCalled {
		t.Error("expected SoftDeleteAbsentTarefas to be called")
	}
	if !undeleteCalled {
		t.Error("expected UndeleteReappearedTarefas to be called")
	}
}

func TestSyncProject_RunProjectSyncFlow(t *testing.T) {
	ctx := context.Background()
	fonteID := uuid.New()
	email := "u@t.com"
	token := "t"
	fonte := &domain.FonteDados{
		ID: fonteID, Nome: "Test", BaseURL: "https://test.atlassian.net",
		AuthType: "api_token", UserEmail: &email, APIToken: &token,
	}

	var syncLogUpdated bool
	var ultimoSyncUpdated bool

	repo := &mockSyncRepoStore{
		hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
			return false, nil
		},
		createSyncLogFn: func(ctx context.Context, log *domain.SyncLog) error { return nil },
		updateSyncLogFn: func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error {
			syncLogUpdated = true
			return nil
		},
		updateSyncLogTotalsFn:      func(ctx context.Context, id uuid.UUID, t repository.SyncTotals) error { return nil },
		getDistinctBoardProjectsFn: func(ctx context.Context, fid uuid.UUID) (map[int]uuid.UUID, error) { return nil, nil },
	}
	mockClient := newDefaultMockJiraClient()
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return nil, nil
	}
	factory := func(b, e, a string, r int, l *zap.Logger) jira.Client { return mockClient }
	fdRepo := &mockFonteDadosStore{
		getByIDFn:         func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) { return fonte, nil },
		saveOAuthTokensFn: func(ctx context.Context, id uuid.UUID, b, a, r string, e time.Time) error { return nil },
		updateUltimoSyncFn: func(ctx context.Context, id uuid.UUID, t time.Time) error {
			ultimoSyncUpdated = true
			return nil
		},
	}

	svc := NewSyncService(repo, fdRepo, factory, nil, nil, 0, zap.NewNop())
	_, err := svc.SyncProject(ctx, fonteID, "PROJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if !syncLogUpdated {
		t.Error("expected UpdateSyncLog to be called by goroutine")
	}
	if !ultimoSyncUpdated {
		t.Error("expected UpdateUltimoSync to be called by goroutine")
	}
}

func TestTimeToPgDate(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := timeToPgDate(nil)
		if result != nil {
			t.Error("expected nil")
		}
	})
	t.Run("valid time", func(t *testing.T) {
		tm := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		result := timeToPgDate(&tm)
		if result == nil {
			t.Fatal("expected non-nil")
		}
		if !result.Valid {
			t.Error("expected Valid=true")
		}
		if result.Time.Day() != 15 {
			t.Errorf("expected day 15, got %d", result.Time.Day())
		}
	})
}

func TestParseOptionalTime(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := parseOptionalTime(nil)
		if result != nil {
			t.Error("expected nil")
		}
	})
	t.Run("empty string", func(t *testing.T) {
		s := ""
		result := parseOptionalTime(&s)
		if result != nil {
			t.Error("expected nil for empty string")
		}
	})
	t.Run("valid time", func(t *testing.T) {
		s := "2026-07-01T10:00:00.000Z"
		result := parseOptionalTime(&s)
		if result == nil {
			t.Fatal("expected non-nil")
		}
	})
}

func TestParseOptionalJiraTimePtr(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := parseOptionalJiraTimePtr(nil)
		if result != nil {
			t.Error("expected nil")
		}
	})
	t.Run("empty string", func(t *testing.T) {
		s := ""
		result := parseOptionalJiraTimePtr(&s)
		if result != nil {
			t.Error("expected nil for empty string")
		}
	})
	t.Run("valid time", func(t *testing.T) {
		s := "2026-07-01T10:00:00.000Z"
		result := parseOptionalJiraTimePtr(&s)
		if result == nil {
			t.Fatal("expected non-nil")
		}
	})
}
