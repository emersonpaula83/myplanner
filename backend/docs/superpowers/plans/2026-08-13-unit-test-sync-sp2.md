# SP2: SyncService Unit Tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Achieve ≥70% unit test coverage for `internal/service/sync.go` (~1,033 lines) using stdlib `testing` + function-field mocks from SP1.

**Architecture:** Each public SyncService method gets its own test function covering happy path, error propagation, and edge cases. The existing `mockJiraClient` (hardcoded returns) must first be refactored to function-field pattern to enable error-path testing. Private helpers (`buildClient`, `getFonte`, `syncOne`, `executSync`, `processIssue`, etc.) are tested indirectly through the public methods that call them.

**Tech Stack:** Go stdlib `testing`, `go.uber.org/zap` (nop logger), function-field mocks from `mocks_test.go`

## Global Constraints

- Framework: stdlib `testing` only — no testify, gomock, or external test deps
- Mock pattern: function-field injection per test (matches existing `mockSyncRepoStore`, `mockFonteDadosStore`)
- File: all tests in `internal/service/sync_test.go`
- No commits until everything passes 100%
- Coverage target: ≥70% of `sync.go` lines
- Logger: use `zap.NewNop()` in all tests

---

### Task 1: Refactor mockJiraClient + GetStatus/ListLogs Tests

**Files:**
- Modify: `internal/service/sync_test.go:16-64` (refactor mockJiraClient to function fields)
- Modify: `internal/service/sync_test.go` (add test functions)

**Interfaces:**
- Consumes: `jira.Client` interface (15 methods, `internal/jira/client.go:34-50`)
- Consumes: `mockSyncRepoStore` from `mocks_test.go` (function-field pattern)
- Produces: `mockJiraClient` with function fields, reusable by all subsequent tasks

**Context:** The existing `mockJiraClient` at line 16-64 of `sync_test.go` uses hardcoded return values. All 15 methods must be converted to function-field pattern (`GetProjectsFn`, `GetIssuesByProjectsFn`, etc.) to enable per-test error injection. GetStatus and ListLogs are simple pass-through methods — good warmup tests.

- [ ] **Step 1: Refactor mockJiraClient to function-field pattern**

Replace the existing `mockJiraClient` struct (lines 16-64) with function-field pattern:

```go
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
```

- [ ] **Step 2: Write helper to build default no-op mockJiraClient**

Add a helper that returns a `mockJiraClient` with all fields set to safe defaults (return zero values, nil errors). Tests override only what they need:

```go
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
		createSprintFn:         func(ctx context.Context, bid int, n string, s, e time.Time) (*jira.JiraSprint, error) { return nil, nil },
		assignIssueFn:          func(ctx context.Context, ik, aid string) error { return nil },
		addCommentFn:           func(ctx context.Context, ik, b string) error { return nil },
		moveToSprintFn:         func(ctx context.Context, sid int, ik string) error { return nil },
		updateTimeEstimateFn:   func(ctx context.Context, ik string, s int) error { return nil },
	}
}
```

- [ ] **Step 3: Update TestSyncServiceStructure to use new mock**

Update the existing `TestSyncServiceStructure` test to use `newDefaultMockJiraClient()` instead of `&mockJiraClient{}`:

```go
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
```

- [ ] **Step 4: Write TestGetStatus tests**

```go
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
```

- [ ] **Step 5: Write TestListLogs tests**

```go
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
```

- [ ] **Step 6: Remove the empty TestExecutSyncCountsTotals placeholder**

Delete the `TestExecutSyncCountsTotals` test (lines 130-135) — it asserts nothing and will be replaced by real tests in Task 5.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/service/ -run "TestGetStatus|TestListLogs|TestSyncServiceStructure|TestParseJiraTime|TestParseOptionalDate|TestNilIfEmpty" -v`
Expected: all PASS

---

### Task 2: ListJiraProjects Tests (covers buildClient + getFonte)

**Files:**
- Modify: `internal/service/sync_test.go` (add test functions)

**Interfaces:**
- Consumes: `mockJiraClient` (function-field, from Task 1)
- Consumes: `mockSyncRepoStore`, `mockFonteDadosStore` from `mocks_test.go`
- Consumes: `ClientFactory`, `OAuthClientFactory` types from `sync.go:21-22`
- Produces: `TestListJiraProjects` covering getFonte error, buildClient (API token + OAuth2 paths), GetProjects success/error

**Context:** `ListJiraProjects` calls `getFonte` → `buildClient` → `client.GetProjects`. Testing this method indirectly covers `buildClient` (OAuth2 token refresh, expired token, API token path) and `getFonte` (not-found, DB error). The `buildClient` method has 6 branch points: oauth2 with valid token, oauth2 with expired token + refresh success, oauth2 with expired token + no oauthSvc, oauth2 with missing tokens, api_token path, and refresh failure.

- [ ] **Step 1: Write TestListJiraProjects_GetFonteError**

```go
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
```

- [ ] **Step 2: Write TestListJiraProjects_BuildClientAPIToken**

Tests the API-token path of `buildClient` (non-oauth2 auth type):

```go
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
```

- [ ] **Step 3: Write TestListJiraProjects_BuildClientOAuth2**

Tests OAuth2 path — valid token (not expired):

```go
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
```

- [ ] **Step 4: Write TestListJiraProjects_OAuth2MissingTokens**

```go
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
```

- [ ] **Step 5: Write TestListJiraProjects_OAuth2ExpiredNoService**

```go
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
```

- [ ] **Step 6: Write TestListJiraProjects_GetProjectsError**

```go
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
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/service/ -run "TestListJiraProjects|TestGetStatus|TestListLogs" -v`
Expected: all PASS

---

### Task 3: SyncProject + SyncProjectScheduled Tests

**Files:**
- Modify: `internal/service/sync_test.go` (add test functions)

**Interfaces:**
- Consumes: `mockJiraClient`, `mockSyncRepoStore`, `mockFonteDadosStore` (all from Tasks 1-2)
- Produces: `TestSyncProject`, `TestSyncProjectScheduled` covering: already running, getFonte error, buildClient error, CreateSyncLog error, happy path (goroutine fires)

**Context:** `SyncProject` (line 122) and `SyncProjectScheduled` (line 159) are nearly identical — both check `HasRunningSyncForProject`, build client, create sync log, then fire `go runProjectSync`. Key difference: `Origem` field ("manual" vs "scheduled"). The goroutine runs async so we only verify the sync log is created and returned. Test must verify the returned `SyncLog` has correct fields (Tipo="project", Status="running", ProjectKey set).

- [ ] **Step 1: Write TestSyncProject subtests**

```go
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
			// These are called by the goroutine; set safe defaults
			getProjectKeysForSyncFn:    func(ctx context.Context, fid uuid.UUID) ([]string, error) { return nil, nil },
			getIssuesByProjectsFn:      nil,
			updateSyncLogFn:            func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error { return nil },
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
```

- [ ] **Step 2: Write TestSyncProjectScheduled subtests**

Same structure as SyncProject but verify `Origem == "scheduled"`:

```go
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

	t.Run("happy path returns scheduled origin", func(t *testing.T) {
		repo := &mockSyncRepoStore{
			hasRunningSyncForProjectFn: func(ctx context.Context, fid uuid.UUID, pk string) (bool, error) {
				return false, nil
			},
			createSyncLogFn:            func(ctx context.Context, log *domain.SyncLog) error { return nil },
			updateSyncLogFn:            func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error { return nil },
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
		if result.Origem != "scheduled" {
			t.Errorf("expected Origem 'scheduled', got %q", result.Origem)
		}
		time.Sleep(50 * time.Millisecond)
	})
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/ -run "TestSyncProject" -v`
Expected: all PASS

---

### Task 4: SyncAll + SyncFonteDados Tests

**Files:**
- Modify: `internal/service/sync_test.go` (add test functions)

**Interfaces:**
- Consumes: `mockJiraClient`, `mockSyncRepoStore`, `mockFonteDadosStore` (all from previous tasks)
- Produces: `TestSyncAll`, `TestSyncFonteDados` covering: GetFonteDadosAtivas error, empty fontes, sync errors logged but not returned, HasRunningSync guard, CreateSyncLog + executSync + UpdateSyncLog flow

**Context:** `SyncAll` (line 244) iterates active fontes, calling `syncOne` for each. Errors are logged but loop continues. `SyncFonteDados` (line 236) calls `getFonte` then `syncOne`. `syncOne` (line 273) is the core orchestration: HasRunningSync check → buildClient → CreateSyncLog → executSync → UpdateSyncLog → UpdateUltimoSync → AutoDetectEquipeBoardIDs. Testing these two methods covers `syncOne` thoroughly.

- [ ] **Step 1: Write TestSyncFonteDados subtests**

```go
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
```

- [ ] **Step 2: Write TestSyncAll subtests**

```go
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
			createSyncLogFn:            func(ctx context.Context, log *domain.SyncLog) error { return nil },
			getProjectKeysForSyncFn:    func(ctx context.Context, fid uuid.UUID) ([]string, error) { return nil, nil },
			updateSyncLogFn:            func(ctx context.Context, id uuid.UUID, s string, f time.Time, t repository.SyncTotals, e json.RawMessage, m *string) error { return nil },
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
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/ -run "TestSyncFonteDados|TestSyncAll" -v`
Expected: all PASS

---

### Task 5: executSync Integration Tests (syncOne → executSync path)

**Files:**
- Modify: `internal/service/sync_test.go` (add test functions)

**Interfaces:**
- Consumes: all mocks from previous tasks
- Produces: `TestSyncFonteDados_WithIssues` covering executSync internal paths: sprint field discovery, custom field discovery, project keys fallback, issue processing with members/sprints/components, soft-delete/undelete, parent resolution, partial error status

**Context:** `executSync` (line 384) is the heaviest method — processes issues, upserts projects/members/sprints/tasks, resolves parents, handles soft-delete/undelete. Testing through `SyncFonteDados` (which calls `syncOne` → `executSync`) exercises the full pipeline. Key branches: project keys empty → fallback to GetProjects; issues with components → UpsertProduto+LinkTarefaProduto; sync errors → status="partial"; sprint field discovery success/failure; custom field discovery.

- [ ] **Step 1: Write TestSyncFonteDados_WithIssues**

Tests executSync with actual issues that exercise processIssue, ensureProject, ensureMember:

```go
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
		hasRunningSyncFn:    func(ctx context.Context, fid uuid.UUID) (bool, error) { return false, nil },
		createSyncLogFn:     func(ctx context.Context, log *domain.SyncLog) error { return nil },
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
	mockClient := newDefaultMockJiraClient()
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return []jira.JiraIssue{
			{
				ID:  "10001",
				Key: "PROJ-1",
				Fields: jira.JiraIssueFields{
					Summary:   "Test Task",
					IssueType: jira.JiraIssueType{Name: "Story"},
					Status:    jira.JiraStatus{Name: "To Do", StatusCategory: jira.JiraStatusCategory{Key: "new"}},
					Project:   jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"},
					Assignee:  &jira.JiraUser{AccountID: "acc1", DisplayName: "User 1"},
					Sprint:    &jira.JiraSprint{ID: 100, Name: "Sprint 1", State: sprintState},
					Created:   "2026-01-01T10:00:00.000Z",
				},
			},
		}, nil
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
```

- [ ] **Step 2: Write TestSyncFonteDados_ProjectKeysFallback**

When GetProjectKeysForSync returns empty, executSync should fall back to client.GetProjects:

```go
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
```

- [ ] **Step 3: Write TestSyncFonteDados_PartialErrors**

Tests that sync errors during issue processing result in status="partial":

```go
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
	mockClient := newDefaultMockJiraClient()
	mockClient.getIssuesByProjectsFn = func(ctx context.Context, pks []string, u *time.Time) ([]jira.JiraIssue, error) {
		return []jira.JiraIssue{
			{
				ID:  "10001",
				Key: "PROJ-1",
				Fields: jira.JiraIssueFields{
					Summary:   "Task",
					IssueType: jira.JiraIssueType{Name: "Story"},
					Status:    jira.JiraStatus{Name: "To Do", StatusCategory: jira.JiraStatusCategory{Key: "new"}},
					Project:   jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"},
					Created:   "2026-01-01T10:00:00.000Z",
				},
			},
		}, nil
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -run "TestSyncFonteDados|TestSyncAll" -v`
Expected: all PASS

---

### Task 6: SyncEpicTasks + Helper Function Tests

**Files:**
- Modify: `internal/service/sync_test.go` (add test functions)

**Interfaces:**
- Consumes: all mocks from previous tasks
- Produces: `TestSyncEpicTasks` covering: getFonte error, buildClient error, invalid epic key format, no matching issues, happy path with count, helper function tests for `extractSprintEntryDate`, `extractFirstInProgressDate`, `resolveParents`, `flushProgress`

**Context:** `SyncEpicTasks` (line 665) parses epic key to extract project key, fetches all project issues, then processes only issues whose parent key matches the epic. Also cover remaining private helper functions: `extractSprintEntryDate` (line 931), `extractFirstInProgressDate` (line 950), `resolveParents` (line 366), `flushProgress` (line 356).

- [ ] **Step 1: Write TestSyncEpicTasks subtests**

```go
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
			return []jira.JiraIssue{
				{
					ID:  "1",
					Key: "PROJ-1",
					Fields: jira.JiraIssueFields{
						Summary:   "Unrelated",
						IssueType: jira.JiraIssueType{Name: "Story"},
						Status:    jira.JiraStatus{Name: "To Do", StatusCategory: jira.JiraStatusCategory{Key: "new"}},
						Project:   jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"},
						Created:   "2026-01-01T10:00:00.000Z",
					},
				},
			}, nil
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
			return []jira.JiraIssue{
				{
					ID:  "1",
					Key: "PROJ-1",
					Fields: jira.JiraIssueFields{
						Summary:   "Child task",
						IssueType: jira.JiraIssueType{Name: "Sub-task"},
						Status:    jira.JiraStatus{Name: "Done", StatusCategory: jira.JiraStatusCategory{Key: "done"}},
						Project:   jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"},
						Parent:    &jira.JiraIssue{Key: "PROJ-100"},
						Created:   "2026-01-01T10:00:00.000Z",
					},
				},
				{
					ID:  "2",
					Key: "PROJ-2",
					Fields: jira.JiraIssueFields{
						Summary:   "Not a child",
						IssueType: jira.JiraIssueType{Name: "Story"},
						Status:    jira.JiraStatus{Name: "To Do", StatusCategory: jira.JiraStatusCategory{Key: "new"}},
						Project:   jira.JiraProject{ID: "P1", Key: "PROJ", Name: "Project"},
						Created:   "2026-01-01T10:00:00.000Z",
					},
				},
			}, nil
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
```

- [ ] **Step 2: Write TestExtractSprintEntryDate**

```go
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
```

- [ ] **Step 3: Write TestExtractFirstInProgressDate**

```go
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
```

- [ ] **Step 4: Write TestResolveParents**

```go
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
```

- [ ] **Step 5: Write TestFlushProgress**

```go
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
```

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/service/ -run "TestSyncEpicTasks|TestExtractSprintEntryDate|TestExtractFirstInProgressDate|TestResolveParents|TestFlushProgress" -v`
Expected: all PASS

- [ ] **Step 7: Run full test suite + coverage**

Run: `go test ./internal/service/ -v -coverprofile=coverage.out && go tool cover -func=coverage.out | grep sync.go`
Expected: ≥70% coverage on sync.go, all tests PASS
