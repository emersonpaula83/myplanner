package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
