package jira

type JiraProject struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Lead        *struct {
		AccountID string `json:"accountId"`
	} `json:"lead"`
	ProjectCategory *struct {
		Name string `json:"name"`
	} `json:"projectCategory"`
}

type JiraUser struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	AvatarUrls   struct {
		Small string `json:"48x48"`
	} `json:"avatarUrls"`
	Active bool `json:"active"`
}

type JiraIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
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
	} `json:"fields"`
}

type JiraType struct {
	Name string `json:"name"`
}

type JiraStatus struct {
	Name           string `json:"name"`
	StatusCategory struct {
		Key string `json:"key"`
	} `json:"statusCategory"`
}

type JiraPrio struct {
	Name string `json:"name"`
}

type JiraComponent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type JiraSprint struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	State         string  `json:"state"`
	StartDate     *string `json:"startDate"`
	EndDate       *string `json:"endDate"`
	CompleteDate  *string `json:"completeDate"`
	OriginBoardID int     `json:"originBoardId"`
}

type JiraBoard struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type jiraSearchResult struct {
	StartAt    int         `json:"startAt"`
	MaxResults int         `json:"maxResults"`
	Total      int         `json:"total"`
	Issues     []JiraIssue `json:"issues"`
}

type jiraProjectList struct {
	Values     []JiraProject `json:"values"`
	IsLast     bool          `json:"isLast"`
	MaxResults int           `json:"maxResults"`
	StartAt    int           `json:"startAt"`
}

type jiraBoardList struct {
	Values     []JiraBoard `json:"values"`
	IsLast     bool        `json:"isLast"`
	MaxResults int         `json:"maxResults"`
	StartAt    int         `json:"startAt"`
}

type jiraSprintList struct {
	Values     []JiraSprint `json:"values"`
	IsLast     bool         `json:"isLast"`
	MaxResults int          `json:"maxResults"`
	StartAt    int          `json:"startAt"`
}

type jiraUserList []JiraUser
