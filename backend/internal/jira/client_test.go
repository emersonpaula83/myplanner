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
