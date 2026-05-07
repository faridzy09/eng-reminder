package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lionparcel/eng-reminder/internal/engineer"
)

// Issue represents a simplified Jira issue.
type Issue struct {
	Key             string
	Summary         string
	Status          string
	Priority        string
	Assignee        string
	Reporter        string
	Created         time.Time
	CodeReviewSince time.Time // zero if not yet in Code Review, or if changelog unavailable
	URL             string
	EpicKey         string
	EpicSummary     string
	EpicURL         string
}

// EngineerTask is a simplified task used for daily SP capacity checking.
type EngineerTask struct {
	Key         string
	Summary     string
	Assignee    string
	StoryPoints float64
	URL         string
}

// EngineerSPSummary holds the daily SP totals for one engineer.
type EngineerSPSummary struct {
	EngineerName  string
	DailyCapacity int
	TotalSP       float64
	TaskCount     int
	Tasks         []EngineerTask
}

// Client is a minimal Jira REST API v3 client.
type Client struct {
	baseURL    string
	email      string
	apiToken   string
	httpClient *http.Client
}

// NewClient creates a new Jira client.
func NewClient(baseURL, email, apiToken string) *Client {
	return &Client{
		baseURL:  baseURL,
		email:    email,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// jiraSearchResponse mirrors the Jira search API response.
type jiraSearchResponse struct {
	Issues []struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
			Status  struct {
				Name string `json:"name"`
			} `json:"status"`
			Priority struct {
				Name string `json:"name"`
			} `json:"priority"`
			Assignee *struct {
				DisplayName string `json:"displayName"`
			} `json:"assignee"`
			Reporter *struct {
				DisplayName string `json:"displayName"`
			} `json:"reporter"`
			Created string `json:"created"`
			Parent  *struct {
				Key    string `json:"key"`
				Fields struct {
					Summary   string `json:"summary"`
					IssueType struct {
						Name string `json:"name"`
					} `json:"issuetype"`
				} `json:"fields"`
			} `json:"parent"`
		} `json:"fields"`
	} `json:"issues"`
}

// searchIssues executes a JQL query and returns the parsed issues.
func (c *Client) searchIssues(jql string, maxResults int) ([]Issue, error) {
	endpoint := fmt.Sprintf("%s/rest/api/3/search/jql", c.baseURL)

	payload, err := json.Marshal(map[string]interface{}{
		"jql":        jql,
		"maxResults": maxResults,
		"fields":     []string{"summary", "status", "priority", "assignee", "reporter", "created", "parent", "customfield_10195", "customfield_10196"},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira API returned %d: %s", resp.StatusCode, string(body))
	}

	var result jiraSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	issues := make([]Issue, 0, len(result.Issues))
	for _, raw := range result.Issues {
		created, _ := time.Parse("2006-01-02T15:04:05.000-0700", raw.Fields.Created)

		assignee := "_Unassigned_"
		if raw.Fields.Assignee != nil {
			assignee = raw.Fields.Assignee.DisplayName
		}

		reporter := "unknown"
		if raw.Fields.Reporter != nil {
			reporter = raw.Fields.Reporter.DisplayName
		}

		epicKey, epicSummary, epicURL := "", "", ""
		if raw.Fields.Parent != nil && raw.Fields.Parent.Fields.IssueType.Name == "Epic" {
			epicKey = raw.Fields.Parent.Key
			epicSummary = raw.Fields.Parent.Fields.Summary
			epicURL = fmt.Sprintf("%s/browse/%s", c.baseURL, raw.Fields.Parent.Key)
		}

		issues = append(issues, Issue{
			Key:         raw.Key,
			Summary:     raw.Fields.Summary,
			Status:      raw.Fields.Status.Name,
			Priority:    raw.Fields.Priority.Name,
			Assignee:    assignee,
			Reporter:    reporter,
			Created:     created,
			URL:         fmt.Sprintf("%s/browse/%s", c.baseURL, raw.Key),
			EpicKey:     epicKey,
			EpicSummary: epicSummary,
			EpicURL:     epicURL,
		})
	}

	return issues, nil
}

// GetHangingBugs fetches bugs stuck in "To Do" for longer than stuckMinutes, across all projects.
func (c *Client) GetHangingBugs(stuckMinutes int) ([]Issue, error) {
	jql := `issuetype = Bug AND status in ("Todo","To Do","Reject","Rejected") AND created>="2026/05/04" ORDER BY created DESC`

	return c.searchIssues(jql, 100)
}

// GetHangingCodeReviews fetches bug-type issues stuck in "Code Review" status for longer than stuckMinutes.
func (c *Client) GetHangingCodeReviews(stuckMinutes int) ([]Issue, error) {
	jql := `issuetype = Bug AND status in ("CODE REVIEW","Code Review") AND created>="2026/05/04" ORDER BY created ASC`
	return c.searchIssues(jql, 100)
}

// GetCodeReviewTasks fetches all sub-tasks/tasks that are in "Code Review" status,
// created on or after 2026/05/04, and assigned to one of the registered engineers.
// It also fetches each issue's changelog to determine when it transitioned to Code Review.
func (c *Client) GetCodeReviewTasks() ([]Issue, error) {
	names := make([]string, 0, len(engineer.Team))
	for _, eng := range engineer.Team {
		names = append(names, fmt.Sprintf(`"%s"`, eng.Name))
	}
	assigneeList := strings.Join(names, ",")

	jql := fmt.Sprintf(
		`issuetype in (Sub-task,"Sub-task Engineer",Task,Subtask) AND status in ("CODE REVIEW","Code Review") AND created>="2026/05/04" AND assignee in (%s) ORDER BY created ASC`,
		assigneeList,
	)
	issues, err := c.searchIssues(jql, 200)
	if err != nil {
		return nil, err
	}

	// Enrich each issue with the time it transitioned to Code Review
	for i := range issues {
		t, err := c.getCodeReviewTransitionTime(issues[i].Key)
		if err == nil && !t.IsZero() {
			issues[i].CodeReviewSince = t
		}
	}
	return issues, nil
}

// getCodeReviewTransitionTime fetches the changelog of an issue and returns the most recent
// time the status was changed to "Code Review" (or "CODE REVIEW"). Returns zero time if not found.
func (c *Client) getCodeReviewTransitionTime(issueKey string) (time.Time, error) {
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/changelog", c.baseURL, issueKey)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("build changelog request: %w", err)
	}
	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("do changelog request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return time.Time{}, fmt.Errorf("jira changelog API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Values []struct {
			Created string `json:"created"`
			Items   []struct {
				Field    string `json:"field"`
				ToString string `json:"toString"`
			} `json:"items"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, fmt.Errorf("decode changelog: %w", err)
	}

	var latest time.Time
	for _, entry := range result.Values {
		for _, item := range entry.Items {
			if strings.EqualFold(item.Field, "status") &&
				(strings.EqualFold(item.ToString, "code review") || strings.EqualFold(item.ToString, "CODE REVIEW")) {
				t, err := time.Parse("2006-01-02T15:04:05.000-0700", entry.Created)
				if err == nil && t.After(latest) {
					latest = t
				}
			}
		}
	}
	return latest, nil
}

// HangingSeverity returns an alert level based on the number of hanging bugs.
// <10 → LOW, 10–14 → MIDDLE, ≥15 → HIGH
func HangingSeverity(count int) string {
	switch {
	case count >= 15:
		return "HIGH"
	case count >= 10:
		return "MIDDLE"
	default:
		return "LOW"
	}
}

// GetTasksByExpectedStartDate fetches all sub-tasks/tasks whose "Expected Start Date"
// equals the given date (YYYY-MM-DD) and assignee is one of the registered engineers.
// Story points are read from customfield_10024.
func (c *Client) GetTasksByExpectedStartDate(date string) ([]EngineerTask, error) {
	// build quoted assignee list from engineer registry
	names := make([]string, 0, len(engineer.Team))
	for _, eng := range engineer.Team {
		names = append(names, fmt.Sprintf(`"%s"`, eng.Name))
	}
	assigneeList := strings.Join(names, ",")

	// Jira JQL expects date as YYYY/MM/DD
	jiraDate := strings.ReplaceAll(date, "-", "/")

	jql := fmt.Sprintf(
		`issuetype in (Sub-task,"Sub-task Engineer",Subtask,Task) AND "Expected Start Date[Date]" = "%s" AND assignee in (%s) ORDER BY assignee ASC`,
		jiraDate, assigneeList,
	)

	endpoint := fmt.Sprintf("%s/rest/api/3/search/jql", c.baseURL)
	payload, err := json.Marshal(map[string]interface{}{
		"jql":        jql,
		"maxResults": 200,
		"fields":     []string{"summary", "assignee", "customfield_10024", "customfield_10195"},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary  string `json:"summary"`
				Assignee *struct {
					DisplayName string `json:"displayName"`
				} `json:"assignee"`
				StoryPoints json.RawMessage `json:"customfield_10024"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	tasks := make([]EngineerTask, 0, len(result.Issues))
	for _, raw := range result.Issues {
		assignee := "_Unassigned_"
		if raw.Fields.Assignee != nil {
			assignee = raw.Fields.Assignee.DisplayName
		}
		sp := parseRawSP(raw.Fields.StoryPoints)
		tasks = append(tasks, EngineerTask{
			Key:         raw.Key,
			Summary:     raw.Fields.Summary,
			Assignee:    assignee,
			StoryPoints: sp,
			URL:         fmt.Sprintf("%s/browse/%s", c.baseURL, raw.Key),
		})
	}
	return tasks, nil
}

// parseRawSP parses a Jira story-points field that may be a JSON number, a quoted
// number string, or null. Also handles comma as decimal separator (e.g. "1,5").
func parseRawSP(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	// try direct number (most common)
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	// try quoted string (e.g. "5" or "1.5" or "1,5")
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.ReplaceAll(s, ",", ".")
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return 0
}
