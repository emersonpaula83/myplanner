package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

func TestGetProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/project/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		user, pass, ok := r.BasicAuth()
		if !ok || user != "test@test.com" || pass != "token123" {
			t.Error("missing or wrong basic auth")
		}

		json.NewEncoder(w).Encode(jiraProjectList{
			Values: []JiraProject{
				{ID: "10001", Key: "TCLOUD", Name: "TOTVS Cloud"},
				{ID: "10002", Key: "INFRA", Name: "Infraestrutura"},
			},
			IsLast: true,
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test@test.com", "token123", 10, testLogger())
	projects, err := client.GetProjects(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Key != "TCLOUD" {
		t.Errorf("expected TCLOUD, got %s", projects[0].Key)
	}
}

func TestGetProjectIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jiraSearchResult{
			Total: 1,
			Issues: []JiraIssue{
				{
					ID:  "20001",
					Key: "TCLOUD-42",
					Fields: struct {
						Summary        string      `json:"summary"`
						IssueType      JiraType    `json:"issuetype"`
						Status         JiraStatus  `json:"status"`
						Priority       *JiraPrio   `json:"priority"`
						Assignee       *JiraUser   `json:"assignee"`
						Reporter       *JiraUser   `json:"reporter"`
						Project        JiraProject `json:"project"`
						Created        string      `json:"created"`
						Updated        string      `json:"updated"`
						DueDate        *string     `json:"duedate"`
						ResolutionDate *string     `json:"resolutiondate"`
						TimeTracking   *struct {
							OriginalEstimateSeconds int `json:"originalEstimateSeconds"`
							TimeSpentSeconds        int `json:"timeSpentSeconds"`
						} `json:"timetracking"`
						StoryPoints *float64    `json:"story_points"`
						Sprint      *JiraSprint `json:"sprint"`
						Parent      *struct {
							ID  string `json:"id"`
							Key string `json:"key"`
						} `json:"parent"`
						Labels       []string        `json:"labels"`
						Components   []JiraComponent `json:"components"`
						CustomFields map[string]any  `json:"-"`
					}{
						Summary:   "Fix login bug",
						IssueType: JiraType{Name: "Bug"},
						Status: JiraStatus{Name: "In Progress", StatusCategory: struct {
							Key string `json:"key"`
						}{Key: "indeterminate"}},
						Created: "2026-07-01T10:00:00.000-0300",
						Updated: "2026-07-10T14:30:00.000-0300",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test@test.com", "token123", 10, testLogger())
	issues, err := client.GetProjectIssues(context.Background(), "TCLOUD", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Key != "TCLOUD-42" {
		t.Errorf("expected TCLOUD-42, got %s", issues[0].Key)
	}
}

func TestGetProjects_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(jiraProjectList{
				Values: []JiraProject{{ID: "1", Key: "A", Name: "Alpha"}},
				IsLast: false,
			})
		} else {
			json.NewEncoder(w).Encode(jiraProjectList{
				Values: []JiraProject{{ID: "2", Key: "B", Name: "Beta"}},
				IsLast: true,
			})
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	projects, err := client.GetProjects(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects across pages, got %d", len(projects))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestExtractCustomFields(t *testing.T) {
	raw := json.RawMessage(`{
		"summary": "Test",
		"customfield_10100": {"value": "Meta"},
		"customfield_10200": "some string",
		"status": {"name": "Done"}
	}`)

	result := extractCustomFields(raw)
	if result == nil {
		t.Fatal("expected non-nil custom fields")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 custom fields, got %d", len(result))
	}
	if _, ok := result["customfield_10100"]; !ok {
		t.Error("expected customfield_10100")
	}
	if _, ok := result["customfield_10200"]; !ok {
		t.Error("expected customfield_10200")
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	_, err := client.GetProjects(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestMoveToSprint(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())
	err := client.MoveToSprint(context.Background(), 42, "PROJ-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/rest/agile/1.0/sprint/42/issue" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	issues, ok := gotBody["issues"].([]any)
	if !ok || len(issues) != 1 || issues[0] != "PROJ-123" {
		t.Errorf("unexpected body: %v", gotBody)
	}
}

func TestMoveToSprint_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Sprint does not exist"]}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())
	err := client.MoveToSprint(context.Background(), 999, "PROJ-123")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestUpdateTimeEstimate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())

	tests := []struct {
		seconds  int
		expected string
	}{
		{3600, "1h"},
		{7200, "2h"},
		{28800, "1d"},
		{57600, "2d"},
		{30600, "1d 0.5h"},
		{1800, "0.5h"},
	}

	for _, tt := range tests {
		err := client.UpdateTimeEstimate(context.Background(), "PROJ-456", tt.seconds)
		if err != nil {
			t.Fatalf("unexpected error for %d seconds: %v", tt.seconds, err)
		}
		if gotMethod != http.MethodPut {
			t.Errorf("expected PUT, got %s", gotMethod)
		}
		if gotPath != "/rest/api/3/issue/PROJ-456" {
			t.Errorf("unexpected path: %s", gotPath)
		}
		fields, _ := gotBody["fields"].(map[string]any)
		tt2, _ := fields["timetracking"].(map[string]any)
		estimate, _ := tt2["originalEstimate"].(string)
		if estimate != tt.expected {
			t.Errorf("seconds=%d: expected %q, got %q", tt.seconds, tt.expected, estimate)
		}
	}
}

func TestUpdateTimeEstimate_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errors":{"timetracking":"invalid"}}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())
	err := client.UpdateTimeEstimate(context.Background(), "PROJ-456", 3600)
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(&APIError{StatusCode: http.StatusNotFound, Body: "not found"}) {
		t.Error("expected true for 404 APIError")
	}
	if IsNotFound(&APIError{StatusCode: http.StatusInternalServerError, Body: "boom"}) {
		t.Error("expected false for 500 APIError")
	}
	if IsNotFound(errors.New("some other error")) {
		t.Error("expected false for non-APIError")
	}
	if IsNotFound(nil) {
		t.Error("expected false for nil error")
	}
}

func TestTryParseSprint(t *testing.T) {
	t.Run("valid sprint", func(t *testing.T) {
		data := json.RawMessage(`{"id": 5, "name": "Sprint 5", "state": "active"}`)
		sp := tryParseSprint(data)
		if sp == nil {
			t.Fatal("expected non-nil sprint")
		}
		if sp.ID != 5 || sp.Name != "Sprint 5" || sp.State != "active" {
			t.Errorf("unexpected sprint: %+v", sp)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		data := json.RawMessage(`{"id": 5, "state": "active"}`)
		if sp := tryParseSprint(data); sp != nil {
			t.Errorf("expected nil, got %+v", sp)
		}
	})

	t.Run("missing state", func(t *testing.T) {
		data := json.RawMessage(`{"id": 5, "name": "Sprint 5"}`)
		if sp := tryParseSprint(data); sp != nil {
			t.Errorf("expected nil, got %+v", sp)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		data := json.RawMessage(`not json`)
		if sp := tryParseSprint(data); sp != nil {
			t.Errorf("expected nil, got %+v", sp)
		}
	})

	t.Run("id zero", func(t *testing.T) {
		data := json.RawMessage(`{"id": 0, "name": "Sprint 0", "state": "active"}`)
		if sp := tryParseSprint(data); sp != nil {
			t.Errorf("expected nil, got %+v", sp)
		}
	})
}

func TestTryParseSprintValue(t *testing.T) {
	t.Run("single sprint object", func(t *testing.T) {
		v := json.RawMessage(`{"id": 7, "name": "Sprint 7", "state": "closed"}`)
		sp := tryParseSprintValue(v)
		if sp == nil || sp.ID != 7 {
			t.Fatalf("unexpected result: %+v", sp)
		}
	})

	t.Run("array with active sprint", func(t *testing.T) {
		v := json.RawMessage(`[
			{"id": 1, "name": "Sprint 1", "state": "closed"},
			{"id": 2, "name": "Sprint 2", "state": "active"},
			{"id": 3, "name": "Sprint 3", "state": "closed"}
		]`)
		sp := tryParseSprintValue(v)
		if sp == nil || sp.ID != 2 {
			t.Fatalf("expected active sprint id 2, got %+v", sp)
		}
	})

	t.Run("array with no active returns latest", func(t *testing.T) {
		v := json.RawMessage(`[
			{"id": 1, "name": "Sprint 1", "state": "closed"},
			{"id": 3, "name": "Sprint 3", "state": "closed"},
			{"id": 2, "name": "Sprint 2", "state": "closed"}
		]`)
		sp := tryParseSprintValue(v)
		if sp == nil || sp.ID != 3 {
			t.Fatalf("expected latest sprint id 3, got %+v", sp)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		v := json.RawMessage(`[]`)
		if sp := tryParseSprintValue(v); sp != nil {
			t.Errorf("expected nil, got %+v", sp)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		v := json.RawMessage(`not json`)
		if sp := tryParseSprintValue(v); sp != nil {
			t.Errorf("expected nil, got %+v", sp)
		}
	})
}

func TestExtractSprintField(t *testing.T) {
	t.Run("known sprintFieldID", func(t *testing.T) {
		c := &HTTPClient{logger: zap.NewNop(), sprintFieldID: "customfield_10020"}
		raw := json.RawMessage(`{
			"customfield_10020": {"id": 9, "name": "Sprint 9", "state": "active"},
			"customfield_99999": "irrelevant"
		}`)
		sp := c.extractSprintField(raw)
		if sp == nil || sp.ID != 9 {
			t.Fatalf("expected sprint id 9, got %+v", sp)
		}
	})

	t.Run("scans customfield_* without known sprintFieldID", func(t *testing.T) {
		c := &HTTPClient{logger: zap.NewNop()}
		raw := json.RawMessage(`{
			"summary": "test",
			"customfield_10030": {"id": 11, "name": "Sprint 11", "state": "closed"}
		}`)
		sp := c.extractSprintField(raw)
		if sp == nil || sp.ID != 11 {
			t.Fatalf("expected sprint id 11, got %+v", sp)
		}
	})

	t.Run("no sprint data", func(t *testing.T) {
		c := &HTTPClient{logger: zap.NewNop(), sprintFieldID: "customfield_10020"}
		raw := json.RawMessage(`{
			"summary": "test",
			"customfield_10030": "not a sprint"
		}`)
		if sp := c.extractSprintField(raw); sp != nil {
			t.Errorf("expected nil, got %+v", sp)
		}
	})
}

func TestExtractCustomFields_NoCustomFields(t *testing.T) {
	raw := json.RawMessage(`{"summary": "Test", "status": {"name": "Done"}}`)
	if result := extractCustomFields(raw); result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestExtractCustomFields_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not json`)
	if result := extractCustomFields(raw); result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSetCustomFieldIDs(t *testing.T) {
	c := &HTTPClient{}
	ids := []string{"customfield_10001", "customfield_10002"}
	c.SetCustomFieldIDs(ids)
	if len(c.customFieldIDs) != 2 || c.customFieldIDs[0] != "customfield_10001" {
		t.Errorf("unexpected customFieldIDs: %v", c.customFieldIDs)
	}
}

func TestSetSprintFieldID(t *testing.T) {
	c := &HTTPClient{}
	c.SetSprintFieldID("customfield_10020")
	if c.sprintFieldID != "customfield_10020" {
		t.Errorf("expected customfield_10020, got %q", c.sprintFieldID)
	}
}

func TestGetUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/user/assignable/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(jiraUserList{
			{AccountID: "acc1", DisplayName: "Alice"},
			{AccountID: "acc2", DisplayName: "Bob"},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	users, err := client.GetUsers(context.Background(), "TCLOUD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestGetUsers_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	_, err := client.GetUsers(context.Background(), "TCLOUD")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetSprintFieldID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "customfield_10001", "name": "Other", "custom": true},
			{"id": "customfield_10020", "name": "Sprint", "custom": true, "schema": map[string]any{
				"type":   "array",
				"custom": "com.pyxis.greenhopper.jira:gh-sprint",
			}},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	id, err := client.GetSprintFieldID(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "customfield_10020" {
		t.Errorf("expected customfield_10020, got %q", id)
	}
}

func TestGetSprintFieldID_ByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "customfield_10099", "name": "Sprint", "custom": true},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	id, err := client.GetSprintFieldID(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "customfield_10099" {
		t.Errorf("expected customfield_10099, got %q", id)
	}
}

func TestGetSprintFieldID_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "customfield_10001", "name": "Other", "custom": true},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	_, err := client.GetSprintFieldID(context.Background())
	if err == nil {
		t.Fatal("expected error when sprint field not found")
	}
}

func TestGetSprintFieldID_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	_, err := client.GetSprintFieldID(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDiscoverCustomFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "customfield_12930", "name": "Tipo de Demanda", "custom": true},
			{"id": "customfield_99999", "name": "Unrelated", "custom": true},
			{"id": "customfield_10021", "name": "Flagged", "custom": true},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	result, err := client.DiscoverCustomFields(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["customfield_12930"] != "tipo_demanda" {
		t.Errorf("expected tipo_demanda mapping, got %v", result)
	}
	if result["customfield_10021"] != "flagged" {
		t.Errorf("expected flagged mapping, got %v", result)
	}
	if _, ok := result["customfield_99999"]; ok {
		t.Errorf("unexpected mapping for unrelated field: %v", result)
	}
}

func TestDiscoverCustomFields_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	_, err := client.DiscoverCustomFields(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetBoards(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(jiraBoardList{
			Values: []JiraBoard{{ID: 1, Name: "Board A", Type: "scrum"}},
			IsLast: true,
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	boards, err := client.GetBoards(context.Background(), "TCLOUD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(boards) != 1 || boards[0].Name != "Board A" {
		t.Errorf("unexpected boards: %v", boards)
	}
}

func TestGetBoards_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	_, err := client.GetBoards(context.Background(), "TCLOUD")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetBoardSprints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board/7/sprint" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(jiraSprintList{
			Values: []JiraSprint{{ID: 1, Name: "Sprint 1", State: "active"}},
			IsLast: true,
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	sprints, err := client.GetBoardSprints(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sprints) != 1 || sprints[0].Name != "Sprint 1" {
		t.Errorf("unexpected sprints: %v", sprints)
	}
}

func TestGetBoardSprints_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	_, err := client.GetBoardSprints(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateSprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/sprint" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(JiraSprint{ID: 55, Name: "New Sprint", State: "future"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	sprint, err := client.CreateSprint(context.Background(), 7, "New Sprint", time.Now(), time.Now().Add(14*24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sprint.ID != 55 {
		t.Errorf("expected sprint id 55, got %d", sprint.ID)
	}
}

func TestCreateSprint_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	_, err := client.CreateSprint(context.Background(), 7, "New Sprint", time.Now(), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAssignIssue(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	err := client.AssignIssue(context.Background(), "PROJ-1", "acc1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/rest/api/3/issue/PROJ-1/assignee" {
		t.Errorf("unexpected request: %s %s", gotMethod, gotPath)
	}
}

func TestAssignIssue_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	err := client.AssignIssue(context.Background(), "PROJ-1", "acc1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAddComment(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	err := client.AddComment(context.Background(), "PROJ-1", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/rest/api/3/issue/PROJ-1/comment" {
		t.Errorf("unexpected request: %s %s", gotMethod, gotPath)
	}
}

func TestAddComment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	err := client.AddComment(context.Background(), "PROJ-1", "hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetIssuesByProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jiraSearchResult{
			Total: 1,
			Issues: []JiraIssue{
				{ID: "1", Key: "A-1"},
			},
		})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	issues, err := client.GetIssuesByProjects(context.Background(), []string{"A", "B"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Key != "A-1" {
		t.Errorf("unexpected issues: %v", issues)
	}
}

func TestGetIssuesByProjects_Empty(t *testing.T) {
	client := NewHTTPClient("http://unused", "t@t.com", "tok", 10, testLogger())
	issues, err := client.GetIssuesByProjects(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issues != nil {
		t.Errorf("expected nil issues, got %v", issues)
	}
}

func TestGetIssuesByProjects_WithUpdatedSince(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(jiraSearchResult{Total: 0, Issues: []JiraIssue{}})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, testLogger())
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := client.GetIssuesByProjects(context.Background(), []string{"A"}, &since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jql, _ := gotBody["jql"].(string)
	if jql == "" {
		t.Error("expected jql in request body")
	}
}

func TestGetIssuesByProjects_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "t@t.com", "tok", 10, zap.NewNop())
	_, err := client.GetIssuesByProjects(context.Background(), []string{"A"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewOAuthClient(t *testing.T) {
	client := NewOAuthClient("https://example.atlassian.net", "access-token-123", 10, zap.NewNop())
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.authType != "oauth2" {
		t.Errorf("expected oauth2 auth type, got %q", client.authType)
	}
}

func TestNewOAuthClient_DefaultRate(t *testing.T) {
	client := NewOAuthClient("https://example.atlassian.net", "tok", 0, zap.NewNop())
	if client.limiter == nil {
		t.Fatal("expected non-nil limiter")
	}
}

func TestJiraSprint_GetBoardID(t *testing.T) {
	s := &JiraSprint{OriginBoardID: 5, BoardID: 9}
	if got := s.GetBoardID(); got != 5 {
		t.Errorf("expected origin board id 5, got %d", got)
	}

	s2 := &JiraSprint{BoardID: 9}
	if got := s2.GetBoardID(); got != 9 {
		t.Errorf("expected board id 9, got %d", got)
	}
}
