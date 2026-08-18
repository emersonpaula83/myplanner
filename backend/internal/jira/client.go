package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira api error: status %d: %s", e.StatusCode, e.Body)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

type Client interface {
	GetProjects(ctx context.Context) ([]JiraProject, error)
	GetProjectIssues(ctx context.Context, projectKey string, updatedSince *time.Time) ([]JiraIssue, error)
	GetIssuesByProjects(ctx context.Context, projectKeys []string, updatedSince *time.Time) ([]JiraIssue, error)
	GetUsers(ctx context.Context, projectKey string) ([]JiraUser, error)
	GetBoards(ctx context.Context, projectKey string) ([]JiraBoard, error)
	GetBoardSprints(ctx context.Context, boardID int) ([]JiraSprint, error)
	GetSprintFieldID(ctx context.Context) (string, error)
	SetSprintFieldID(id string)
	SetCustomFieldIDs(ids []string)
	DiscoverCustomFields(ctx context.Context) (map[string]string, error)
	CreateSprint(ctx context.Context, boardID int, name string, startDate, endDate time.Time) (*JiraSprint, error)
	AssignIssue(ctx context.Context, issueKey, accountID string) error
	AddComment(ctx context.Context, issueKey, body string) error
	MoveToSprint(ctx context.Context, sprintJiraID int, issueKey string) error
	UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error
	RemoveFromSprint(ctx context.Context, issueKey string) error
	SearchIssuesByKeys(ctx context.Context, projectKey string, issueKeys []string) ([]JiraIssue, error)
}

type HTTPClient struct {
	baseURL        string
	authType       string
	email          string
	apiToken       string
	accessToken    string
	httpClient     *http.Client
	limiter        *rate.Limiter
	logger         *zap.Logger
	sprintFieldID  string
	customFieldIDs []string
}

func (c *HTTPClient) SetCustomFieldIDs(ids []string) {
	c.customFieldIDs = ids
}

func NewHTTPClient(baseURL, email, apiToken string, ratePerSec int, logger *zap.Logger) *HTTPClient {
	if ratePerSec <= 0 {
		ratePerSec = 5
	}
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authType:   "basic",
		email:      email,
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(ratePerSec), ratePerSec),
		logger:     logger,
	}
}

func NewOAuthClient(baseURL, accessToken string, ratePerSec int, logger *zap.Logger) *HTTPClient {
	if ratePerSec <= 0 {
		ratePerSec = 5
	}
	return &HTTPClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		authType:    "oauth2",
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		limiter:     rate.NewLimiter(rate.Limit(ratePerSec), ratePerSec),
		logger:      logger,
	}
}

func (c *HTTPClient) do(ctx context.Context, path string) ([]byte, error) {
	return c.doRequest(ctx, http.MethodGet, path, nil)
}

func (c *HTTPClient) doPost(ctx context.Context, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling payload: %w", err)
	}
	return c.doRequest(ctx, http.MethodPost, path, data)
}

func (c *HTTPClient) doPut(ctx context.Context, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling payload: %w", err)
	}
	return c.doRequest(ctx, http.MethodPut, path, data)
}

func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	reqURL := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		reqBody = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if c.authType == "oauth2" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	} else {
		req.SetBasicAuth(c.email, c.apiToken)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		errBody := string(respBody[:min(len(respBody), 500)])
		c.logger.Warn("jira api error", zap.Int("status", resp.StatusCode), zap.String("path", path), zap.String("body", errBody))
		return nil, &APIError{StatusCode: resp.StatusCode, Body: errBody}
	}

	return respBody, nil
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

	fields := []string{"summary", "issuetype", "status", "priority", "assignee", "reporter",
		"project", "created", "updated", "duedate", "resolutiondate", "timetracking",
		"sprint", "parent", "labels", "components"}
	if c.sprintFieldID != "" && c.sprintFieldID != "sprint" {
		fields = append(fields, c.sprintFieldID)
	}
	fields = append(fields, c.customFieldIDs...)

	all := make([]JiraIssue, 0)
	var nextPageToken string
	for {
		payload := map[string]any{
			"jql":        jql,
			"maxResults": 100,
			"fields":     fields,
			"expand":     "changelog",
		}
		if nextPageToken != "" {
			payload["nextPageToken"] = nextPageToken
		}
		body, err := c.doPost(ctx, "/rest/api/3/search/jql", payload)
		if err != nil {
			return nil, err
		}

		var rawResult struct {
			NextPageToken string            `json:"nextPageToken"`
			IsLast        bool              `json:"isLast"`
			Issues        []json.RawMessage `json:"issues"`
		}
		if err := json.Unmarshal(body, &rawResult); err != nil {
			return nil, fmt.Errorf("decoding issues: %w", err)
		}

		for _, rawIssue := range rawResult.Issues {
			var issue JiraIssue
			if err := json.Unmarshal(rawIssue, &issue); err != nil {
				c.logger.Warn("skipping unparseable issue", zap.Error(err))
				continue
			}
			var issueMap map[string]json.RawMessage
			if err := json.Unmarshal(rawIssue, &issueMap); err == nil {
				if fieldsRaw, ok := issueMap["fields"]; ok {
					issue.Fields.CustomFields = extractCustomFields(fieldsRaw)
					if issue.Fields.Sprint == nil {
						issue.Fields.Sprint = c.extractSprintField(fieldsRaw)
					}
				}
			}
			all = append(all, issue)
		}

		if rawResult.IsLast || len(rawResult.Issues) == 0 || rawResult.NextPageToken == "" {
			break
		}
		nextPageToken = rawResult.NextPageToken
	}
	c.logger.Debug("fetched issues", zap.String("project", projectKey), zap.Int("count", len(all)))
	return all, nil
}

func (c *HTTPClient) SearchIssuesByKeys(ctx context.Context, projectKey string, issueKeys []string) ([]JiraIssue, error) {
	if len(issueKeys) == 0 {
		return nil, nil
	}

	quoted := make([]string, len(issueKeys))
	for i, k := range issueKeys {
		quoted[i] = fmt.Sprintf("%q", k)
	}
	jql := fmt.Sprintf("project = %s AND key IN (%s)", projectKey, strings.Join(quoted, ","))

	fields := []string{"summary", "issuetype", "status", "priority", "assignee", "reporter",
		"project", "created", "updated", "duedate", "resolutiondate", "timetracking",
		"sprint", "parent", "labels", "components"}
	if c.sprintFieldID != "" && c.sprintFieldID != "sprint" {
		fields = append(fields, c.sprintFieldID)
	}
	fields = append(fields, c.customFieldIDs...)

	all := make([]JiraIssue, 0)
	var nextPageToken string
	for {
		payload := map[string]any{
			"jql":        jql,
			"maxResults": 100,
			"fields":     fields,
		}
		if nextPageToken != "" {
			payload["nextPageToken"] = nextPageToken
		}
		body, err := c.doPost(ctx, "/rest/api/3/search/jql", payload)
		if err != nil {
			return nil, err
		}

		var rawResult struct {
			NextPageToken string            `json:"nextPageToken"`
			IsLast        bool              `json:"isLast"`
			Issues        []json.RawMessage `json:"issues"`
		}
		if err := json.Unmarshal(body, &rawResult); err != nil {
			return nil, fmt.Errorf("decoding issues: %w", err)
		}

		for _, rawIssue := range rawResult.Issues {
			var issue JiraIssue
			if err := json.Unmarshal(rawIssue, &issue); err != nil {
				c.logger.Warn("skipping unparseable issue", zap.Error(err))
				continue
			}
			var issueMap map[string]json.RawMessage
			if err := json.Unmarshal(rawIssue, &issueMap); err == nil {
				if fieldsRaw, ok := issueMap["fields"]; ok {
					issue.Fields.CustomFields = extractCustomFields(fieldsRaw)
					if issue.Fields.Sprint == nil {
						issue.Fields.Sprint = c.extractSprintField(fieldsRaw)
					}
				}
			}
			all = append(all, issue)
		}

		if rawResult.IsLast || len(rawResult.Issues) == 0 || rawResult.NextPageToken == "" {
			break
		}
		nextPageToken = rawResult.NextPageToken
	}
	c.logger.Debug("searched issues by keys", zap.String("project", projectKey), zap.Int("count", len(all)))
	return all, nil
}

func (c *HTTPClient) GetIssuesByProjects(ctx context.Context, projectKeys []string, updatedSince *time.Time) ([]JiraIssue, error) {
	if len(projectKeys) == 0 {
		return nil, nil
	}
	jql := fmt.Sprintf("project IN (%s)", strings.Join(projectKeys, ", "))
	if updatedSince != nil {
		jql += fmt.Sprintf(" AND updated >= \"%s\"", updatedSince.Format("2006-01-02 15:04"))
	}
	jql += " ORDER BY updated DESC"

	fields := []string{"summary", "issuetype", "status", "priority", "assignee", "reporter",
		"project", "created", "updated", "duedate", "resolutiondate", "timetracking",
		"sprint", "parent", "labels", "components"}
	if c.sprintFieldID != "" && c.sprintFieldID != "sprint" {
		fields = append(fields, c.sprintFieldID)
	}
	fields = append(fields, c.customFieldIDs...)

	all := make([]JiraIssue, 0)
	var nextPageToken string
	for {
		payload := map[string]any{
			"jql":        jql,
			"maxResults": 100,
			"fields":     fields,
			"expand":     "changelog",
		}
		if nextPageToken != "" {
			payload["nextPageToken"] = nextPageToken
		}
		body, err := c.doPost(ctx, "/rest/api/3/search/jql", payload)
		if err != nil {
			return nil, err
		}

		var rawResult struct {
			NextPageToken string            `json:"nextPageToken"`
			IsLast        bool              `json:"isLast"`
			Issues        []json.RawMessage `json:"issues"`
		}
		if err := json.Unmarshal(body, &rawResult); err != nil {
			return nil, fmt.Errorf("decoding issues: %w", err)
		}

		for _, rawIssue := range rawResult.Issues {
			var issue JiraIssue
			if err := json.Unmarshal(rawIssue, &issue); err != nil {
				c.logger.Warn("skipping unparseable issue", zap.Error(err))
				continue
			}
			var issueMap map[string]json.RawMessage
			if err := json.Unmarshal(rawIssue, &issueMap); err == nil {
				if fieldsRaw, ok := issueMap["fields"]; ok {
					issue.Fields.CustomFields = extractCustomFields(fieldsRaw)
					if issue.Fields.Sprint == nil {
						issue.Fields.Sprint = c.extractSprintField(fieldsRaw)
					}
				}
			}
			all = append(all, issue)
		}

		if rawResult.IsLast || len(rawResult.Issues) == 0 || rawResult.NextPageToken == "" {
			break
		}
		nextPageToken = rawResult.NextPageToken
	}
	c.logger.Debug("fetched issues by projects", zap.Int("projects", len(projectKeys)), zap.Int("issues", len(all)))
	return all, nil
}

func (c *HTTPClient) extractSprintField(raw json.RawMessage) *JiraSprint {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	// Try known sprint field ID first
	if c.sprintFieldID != "" {
		if v, ok := fields[c.sprintFieldID]; ok && len(v) > 0 && string(v) != "null" {
			if sp := tryParseSprintValue(v); sp != nil {
				return sp
			}
		}
	}
	// Fallback: scan all customfields
	for k, v := range fields {
		if len(k) <= 12 || k[:12] != "customfield_" {
			continue
		}
		if len(v) == 0 || string(v) == "null" {
			continue
		}
		if sp := tryParseSprintValue(v); sp != nil {
			c.logger.Debug("found sprint in custom field", zap.String("field", k), zap.Int("sprintID", sp.ID))
			if c.sprintFieldID == "" {
				c.sprintFieldID = k
			}
			return sp
		}
	}
	return nil
}

func tryParseSprintValue(v json.RawMessage) *JiraSprint {
	if sp := tryParseSprint(v); sp != nil {
		return sp
	}
	var arr []json.RawMessage
	if json.Unmarshal(v, &arr) == nil && len(arr) > 0 {
		var active, latest *JiraSprint
		for _, raw := range arr {
			sp := tryParseSprint(raw)
			if sp == nil {
				continue
			}
			if sp.State == "active" {
				active = sp
			}
			if latest == nil || sp.ID > latest.ID {
				latest = sp
			}
		}
		if active != nil {
			return active
		}
		return latest
	}
	return nil
}

func tryParseSprint(data json.RawMessage) *JiraSprint {
	var obj map[string]any
	if json.Unmarshal(data, &obj) != nil {
		return nil
	}
	if _, hasName := obj["name"]; !hasName {
		return nil
	}
	if _, hasState := obj["state"]; !hasState {
		return nil
	}
	var sprint JiraSprint
	if json.Unmarshal(data, &sprint) == nil && sprint.ID > 0 {
		return &sprint
	}
	return nil
}

// extractCustomFields scans the raw JIRA "fields" JSON object and pulls out
// any keys prefixed with "customfield_", since those are not represented as
// named struct fields on JiraIssue.
func extractCustomFields(raw json.RawMessage) map[string]any {
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range fields {
		if len(k) > 12 && k[:12] == "customfield_" {
			result[k] = v
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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

func (c *HTTPClient) GetSprintFieldID(ctx context.Context) (string, error) {
	body, err := c.do(ctx, "/rest/api/3/field")
	if err != nil {
		return "", err
	}
	var fields []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Custom bool   `json:"custom"`
		Schema *struct {
			Type   string `json:"type"`
			Custom string `json:"custom"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(body, &fields); err != nil {
		return "", fmt.Errorf("decoding fields: %w", err)
	}
	for _, f := range fields {
		if f.Schema != nil && f.Schema.Custom == "com.pyxis.greenhopper.jira:gh-sprint" {
			c.logger.Info("discovered sprint field", zap.String("id", f.ID), zap.String("name", f.Name))
			return f.ID, nil
		}
	}
	for _, f := range fields {
		if strings.EqualFold(f.Name, "Sprint") && f.Custom {
			c.logger.Info("discovered sprint field by name", zap.String("id", f.ID))
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("sprint field not found")
}

func (c *HTTPClient) SetSprintFieldID(id string) {
	c.sprintFieldID = id
}

var knownCustomFieldsByID = map[string]string{
	"customfield_12930": "tipo_demanda",
	"customfield_10021": "flagged",
}

var knownCustomFieldsByName = map[string]string{
	"tipo de demanda": "tipo_demanda",
	"flagged":         "flagged",
}

func (c *HTTPClient) DiscoverCustomFields(ctx context.Context) (map[string]string, error) {
	body, err := c.do(ctx, "/rest/api/3/field")
	if err != nil {
		return nil, err
	}
	var fields []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Custom bool   `json:"custom"`
	}
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("decoding fields: %w", err)
	}
	// DEBUG: log all custom fields to find correct tipo_demanda field
	for _, f := range fields {
		if f.Custom {
			nameLower := strings.ToLower(f.Name)
			if strings.Contains(nameLower, "tipo") || strings.Contains(nameLower, "demanda") || strings.Contains(nameLower, "type") {
				c.logger.Info("DEBUG candidate field for tipo_demanda", zap.String("id", f.ID), zap.String("name", f.Name))
			}
		}
	}

	result := make(map[string]string)
	for _, f := range fields {
		if mapped, ok := knownCustomFieldsByID[f.ID]; ok {
			result[f.ID] = mapped
			c.logger.Info("discovered custom field by ID", zap.String("id", f.ID), zap.String("mapped", mapped), zap.String("jiraName", f.Name))
		}
		if f.Custom {
			nameLower := strings.ToLower(f.Name)
			if mapped, ok := knownCustomFieldsByName[nameLower]; ok {
				if _, already := result[f.ID]; !already {
					result[f.ID] = mapped
					c.logger.Info("discovered custom field by name", zap.String("id", f.ID), zap.String("mapped", mapped), zap.String("jiraName", f.Name))
				}
			}
		}
	}
	return result, nil
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

func (c *HTTPClient) CreateSprint(ctx context.Context, boardID int, name string, startDate, endDate time.Time) (*JiraSprint, error) {
	payload := map[string]any{
		"name":          name,
		"originBoardId": boardID,
		"startDate":     startDate.Format(time.RFC3339),
		"endDate":       endDate.Format(time.RFC3339),
	}
	body, err := c.doPost(ctx, "/rest/agile/1.0/sprint", payload)
	if err != nil {
		return nil, fmt.Errorf("creating sprint %q: %w", name, err)
	}
	var sprint JiraSprint
	if err := json.Unmarshal(body, &sprint); err != nil {
		return nil, fmt.Errorf("decoding created sprint: %w", err)
	}
	c.logger.Info("created sprint in jira", zap.String("name", name), zap.Int("id", sprint.ID))
	return &sprint, nil
}

func (c *HTTPClient) AssignIssue(ctx context.Context, issueKey, accountID string) error {
	payload := map[string]string{"accountId": accountID}
	_, err := c.doPut(ctx, "/rest/api/3/issue/"+issueKey+"/assignee", payload)
	if err != nil {
		return fmt.Errorf("assigning issue %q: %w", issueKey, err)
	}
	return nil
}

func (c *HTTPClient) AddComment(ctx context.Context, issueKey, body string) error {
	payload := map[string]any{
		"body": map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []any{
				map[string]any{
					"type": "paragraph",
					"content": []any{
						map[string]any{"type": "text", "text": body},
					},
				},
			},
		},
	}
	_, err := c.doPost(ctx, "/rest/api/3/issue/"+issueKey+"/comment", payload)
	if err != nil {
		return fmt.Errorf("adding comment to issue %q: %w", issueKey, err)
	}
	return nil
}

func (c *HTTPClient) MoveToSprint(ctx context.Context, sprintJiraID int, issueKey string) error {
	path := fmt.Sprintf("/rest/agile/1.0/sprint/%d/issue", sprintJiraID)
	payload := map[string]any{"issues": []string{issueKey}}
	_, err := c.doPost(ctx, path, payload)
	if err != nil {
		return fmt.Errorf("moving issue %q to sprint %d: %w", issueKey, sprintJiraID, err)
	}
	return nil
}

func formatJiraDuration(seconds int) string {
	days := seconds / 28800
	remaining := seconds % 28800
	hours := float64(remaining) / 3600.0

	if days > 0 && hours > 0 {
		return fmt.Sprintf("%dd %.4gh", days, hours)
	}
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%.4gh", hours)
}

func (c *HTTPClient) UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error {
	estimate := formatJiraDuration(seconds)
	payload := map[string]any{
		"fields": map[string]any{
			"timetracking": map[string]any{
				"originalEstimate": estimate,
			},
		},
	}
	_, err := c.doPut(ctx, "/rest/api/3/issue/"+issueKey, payload)
	if err != nil {
		return fmt.Errorf("updating time estimate for %q: %w", issueKey, err)
	}
	return nil
}

func (c *HTTPClient) RemoveFromSprint(ctx context.Context, issueKey string) error {
	payload := map[string]any{"issues": []string{issueKey}}
	_, err := c.doPost(ctx, "/rest/agile/1.0/backlog/issue", payload)
	if err != nil {
		return fmt.Errorf("removing issue %q from sprint: %w", issueKey, err)
	}
	return nil
}
