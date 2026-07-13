package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type Client interface {
	GetProjects(ctx context.Context) ([]JiraProject, error)
	GetProjectIssues(ctx context.Context, projectKey string, updatedSince *time.Time) ([]JiraIssue, error)
	GetUsers(ctx context.Context, projectKey string) ([]JiraUser, error)
	GetBoards(ctx context.Context, projectKey string) ([]JiraBoard, error)
	GetBoardSprints(ctx context.Context, boardID int) ([]JiraSprint, error)
}

type HTTPClient struct {
	baseURL    string
	email      string
	apiToken   string
	httpClient *http.Client
	limiter    *rate.Limiter
	logger     *zap.Logger
}

func NewHTTPClient(baseURL, email, apiToken string, ratePerSec int, logger *zap.Logger) *HTTPClient {
	if ratePerSec <= 0 {
		ratePerSec = 5
	}
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		email:      email,
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(ratePerSec), ratePerSec),
		logger:     logger,
	}
}

func (c *HTTPClient) do(ctx context.Context, path string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		c.logger.Warn("jira api error", zap.Int("status", resp.StatusCode), zap.String("path", path), zap.String("body", string(body[:min(len(body), 200)])))
		return nil, fmt.Errorf("jira api error: status %d", resp.StatusCode)
	}

	return body, nil
}

func (c *HTTPClient) GetProjects(ctx context.Context) ([]JiraProject, error) {
	all := make([]JiraProject, 0)
	startAt := 0
	for {
		path := fmt.Sprintf("/rest/api/3/project/search?startAt=%d&maxResults=50", startAt)
		body, err := c.do(ctx, path)
		if err != nil {
			return nil, err
		}
		var result jiraProjectList
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding projects: %w", err)
		}
		all = append(all, result.Values...)
		if result.IsLast || len(result.Values) == 0 {
			break
		}
		startAt += len(result.Values)
	}
	c.logger.Debug("fetched projects", zap.Int("count", len(all)))
	return all, nil
}

func (c *HTTPClient) GetProjectIssues(ctx context.Context, projectKey string, updatedSince *time.Time) ([]JiraIssue, error) {
	jql := fmt.Sprintf("project = %s", projectKey)
	if updatedSince != nil {
		jql += fmt.Sprintf(" AND updated >= \"%s\"", updatedSince.Format("2006-01-02 15:04"))
	}
	jql += " ORDER BY updated DESC"

	fields := "summary,issuetype,status,priority,assignee,reporter,project,created,updated,duedate,resolutiondate,timetracking,sprint,parent,labels,components"

	all := make([]JiraIssue, 0)
	startAt := 0
	for {
		path := fmt.Sprintf("/rest/api/3/search?jql=%s&startAt=%d&maxResults=100&fields=%s",
			url.QueryEscape(jql), startAt, fields)
		body, err := c.do(ctx, path)
		if err != nil {
			return nil, err
		}
		var result jiraSearchResult
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding issues: %w", err)
		}
		all = append(all, result.Issues...)
		if startAt+len(result.Issues) >= result.Total || len(result.Issues) == 0 {
			break
		}
		startAt += len(result.Issues)
	}
	c.logger.Debug("fetched issues", zap.String("project", projectKey), zap.Int("count", len(all)))
	return all, nil
}

func (c *HTTPClient) GetUsers(ctx context.Context, projectKey string) ([]JiraUser, error) {
	path := fmt.Sprintf("/rest/api/3/user/assignable/search?project=%s&maxResults=1000", projectKey)
	body, err := c.do(ctx, path)
	if err != nil {
		return nil, err
	}
	users := make(jiraUserList, 0)
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("decoding users: %w", err)
	}
	c.logger.Debug("fetched users", zap.String("project", projectKey), zap.Int("count", len(users)))
	return users, nil
}

func (c *HTTPClient) GetBoards(ctx context.Context, projectKey string) ([]JiraBoard, error) {
	all := make([]JiraBoard, 0)
	startAt := 0
	for {
		path := fmt.Sprintf("/rest/agile/1.0/board?projectKeyOrId=%s&startAt=%d&maxResults=50", projectKey, startAt)
		body, err := c.do(ctx, path)
		if err != nil {
			return nil, err
		}
		var result jiraBoardList
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding boards: %w", err)
		}
		all = append(all, result.Values...)
		if result.IsLast || len(result.Values) == 0 {
			break
		}
		startAt += len(result.Values)
	}
	return all, nil
}

func (c *HTTPClient) GetBoardSprints(ctx context.Context, boardID int) ([]JiraSprint, error) {
	all := make([]JiraSprint, 0)
	startAt := 0
	for {
		path := fmt.Sprintf("/rest/agile/1.0/board/%d/sprint?startAt=%d&maxResults=50", boardID, startAt)
		body, err := c.do(ctx, path)
		if err != nil {
			return nil, err
		}
		var result jiraSprintList
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding sprints: %w", err)
		}
		all = append(all, result.Values...)
		if result.IsLast || len(result.Values) == 0 {
			break
		}
		startAt += len(result.Values)
	}
	return all, nil
}
