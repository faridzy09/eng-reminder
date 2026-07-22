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
	CodeReviewSince time.Time     // zero if not yet in Code Review, or if changelog unavailable
	BusinessHang    time.Duration // working-hours age (09:00–18:00 WIB); zero means "use wall-clock"
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

// FELeadSupervisor is the supervisor (PIC lead) whose engineers form the FE team.
const FELeadSupervisor = "Faridho"

// feAssigneeClause builds a quoted, comma-separated JQL assignee list from the FE
// engineers registered under FELeadSupervisor in engineer.go. Returns ok=false
// when no FE engineer is registered.
func feAssigneeClause() (string, bool) {
	names := engineer.NamesBySupervisor(FELeadSupervisor)
	if len(names) == 0 {
		return "", false
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf(`"%s"`, n))
	}
	return strings.Join(quoted, ","), true
}

// GetFEHangingBugs fetches FE-team bugs stuck in "To Do" across the FE project
// boards (GENESIS, CORB, WEBLP, CUST), assigned to an FE engineer.
func (c *Client) GetFEHangingBugs(stuckMinutes int) ([]Issue, error) {
	assignees, ok := feAssigneeClause()
	if !ok {
		return nil, nil
	}
	// Include the FE lead (Faridho) himself as an assignee, in addition to his engineers.
	assignees += `,"Faridho"`
	jql := fmt.Sprintf(
		`issuetype = Bug AND project in (Genesis,"Customer App Team","Corporare Business","Lion Parcel Web") AND status in ("Todo","To Do","Reject","Rejected") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`, assignees,
	)
	return c.searchIssues(jql, 100)
}

// GetFEHangingCodeReviews fetches FE-team bugs stuck in "Code Review" across the
// FE project boards, assigned to an FE engineer.
func (c *Client) GetFEHangingCodeReviews(stuckMinutes int) ([]Issue, error) {
	assignees, ok := feAssigneeClause()
	if !ok {
		return nil, nil
	}
	assignees += `,"Faridho"`
	jql := fmt.Sprintf(
		`project in (Genesis,"Customer App Team","Corporare Business","Lion Parcel Web") AND status in ("CODE REVIEW","Code Review") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`,
		assignees,
	)
	issues, err := c.searchIssues(jql, 100)
	if err != nil {
		return nil, err
	}
	c.enrichCodeReviewSince(issues)
	return issues, nil
}

// CorbLeadSupervisor is the supervisor (PIC lead) whose engineers form the CORB team.
const CorbLeadSupervisor = "Sholahuddin Alisyahbana"

// corbAssigneeClause builds a quoted, comma-separated JQL assignee list from the
// CORB engineers registered under CorbLeadSupervisor in engineer.go. Returns
// ok=false when no CORB engineer is registered.
func corbAssigneeClause() (string, bool) {
	names := engineer.NamesBySupervisor(CorbLeadSupervisor)
	if len(names) == 0 {
		return "", false
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf(`"%s"`, n))
	}
	return strings.Join(quoted, ","), true
}

// GetCorbHangingBugs fetches CORB-team bugs stuck in "To Do" across the FE project
// boards (GENESIS, CORB, WEBLP, CUST), assigned to a CORB engineer.
func (c *Client) GetCorbHangingBugs(stuckMinutes int) ([]Issue, error) {
	assignees, ok := corbAssigneeClause()
	if !ok {
		return nil, nil
	}
	// Include the CORB lead (Sholahuddin Alisyahbana) himself as an assignee, in addition to his engineers.
	assignees += `,"Sholahuddin Alisyahbana"`
	jql := fmt.Sprintf(
		`issuetype = Bug AND project in ("Corporare Business","Lion Parcel Web") AND status in ("Todo","To Do","Reject","Rejected") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`, assignees,
	)
	return c.searchIssues(jql, 100)
}

// GetCorbHangingCodeReviews fetches CORB-team bugs stuck in "Code Review" across
// the FE project boards, assigned to a CORB engineer.
func (c *Client) GetCorbHangingCodeReviews(stuckMinutes int) ([]Issue, error) {
	assignees, ok := corbAssigneeClause()
	if !ok {
		return nil, nil
	}
	assignees += `,"Sholahuddin Alisyahbana"`
	jql := fmt.Sprintf(
		`project in ("Corporare Business","Lion Parcel Web") AND status in ("CODE REVIEW","Code Review") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`,
		assignees,
	)
	issues, err := c.searchIssues(jql, 100)
	if err != nil {
		return nil, err
	}
	c.enrichCodeReviewSince(issues)
	return issues, nil
}

// GenesisLeadSupervisor is the supervisor (PIC lead) whose engineers form the Genesis team.
const GenesisLeadSupervisor = "Irvan Resna Hadiyana"

// genesisAssigneeClause builds a quoted, comma-separated JQL assignee list from the
// Genesis engineers registered under GenesisLeadSupervisor in engineer.go. Returns
// ok=false when no Genesis engineer is registered.
func genesisAssigneeClause() (string, bool) {
	names := engineer.NamesBySupervisor(GenesisLeadSupervisor)
	if len(names) == 0 {
		return "", false
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf(`"%s"`, n))
	}
	return strings.Join(quoted, ","), true
}

// GetGenesisHangingBugs fetches Genesis-team bugs stuck in "To Do" on the Genesis
// project board, assigned to a Genesis engineer.
func (c *Client) GetGenesisHangingBugs(stuckMinutes int) ([]Issue, error) {
	assignees, ok := genesisAssigneeClause()
	if !ok {
		return nil, nil
	}
	// Include the Genesis lead (Irvan Resna Hadiyana) himself as an assignee, in addition to his engineers.
	assignees += `,"Irvan Resna Hadiyana"`
	jql := fmt.Sprintf(
		`issuetype = Bug AND project = Genesis AND status in ("Todo","To Do","Reject","Rejected") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`, assignees,
	)
	return c.searchIssues(jql, 100)
}

// GetGenesisHangingCodeReviews fetches Genesis-team bugs stuck in "Code Review" on
// the Genesis project board, assigned to a Genesis engineer.
func (c *Client) GetGenesisHangingCodeReviews(stuckMinutes int) ([]Issue, error) {
	assignees, ok := genesisAssigneeClause()
	if !ok {
		return nil, nil
	}
	assignees += `,"Irvan Resna Hadiyana"`
	jql := fmt.Sprintf(
		`project = Genesis AND status in ("CODE REVIEW","Code Review") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`,
		assignees,
	)
	issues, err := c.searchIssues(jql, 100)
	if err != nil {
		return nil, err
	}
	c.enrichCodeReviewSince(issues)
	return issues, nil
}

// GenesisTwoLeadSupervisor is the supervisor (PIC lead) whose engineers form the GenesisTwo team.
const GenesisTwoLeadSupervisor = "DeriKurniawan"

// genesisTwoAssigneeClause builds a quoted, comma-separated JQL assignee list from the
// GenesisTwo engineers registered under GenesisTwoLeadSupervisor in engineer.go. Returns
// ok=false when no GenesisTwo engineer is registered.
func genesisTwoAssigneeClause() (string, bool) {
	names := engineer.NamesBySupervisor(GenesisTwoLeadSupervisor)
	if len(names) == 0 {
		return "", false
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf(`"%s"`, n))
	}
	return strings.Join(quoted, ","), true
}

// GetGenesisTwoHangingBugs fetches GenesisTwo-team bugs stuck in "To Do" on the
// GenesisTwo project board, assigned to a GenesisTwo engineer.
func (c *Client) GetGenesisTwoHangingBugs(stuckMinutes int) ([]Issue, error) {
	assignees, ok := genesisTwoAssigneeClause()
	if !ok {
		return nil, nil
	}
	// Include the GenesisTwo lead (DeriKurniawan) himself as an assignee, in addition to his engineers.
	assignees += `,"DeriKurniawan"`
	jql := fmt.Sprintf(
		`issuetype = Bug AND project = Genesis AND status in ("Todo","To Do","Reject","Rejected") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`, assignees,
	)
	return c.searchIssues(jql, 100)
}

// GetGenesisTwoHangingCodeReviews fetches GenesisTwo-team bugs stuck in "Code Review"
// on the GenesisTwo project board, assigned to a GenesisTwo engineer.
func (c *Client) GetGenesisTwoHangingCodeReviews(stuckMinutes int) ([]Issue, error) {
	assignees, ok := genesisTwoAssigneeClause()
	if !ok {
		return nil, nil
	}
	assignees += `,"DeriKurniawan"`
	jql := fmt.Sprintf(
		`project = Genesis AND status in ("CODE REVIEW","Code Review") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`,
		assignees,
	)
	issues, err := c.searchIssues(jql, 100)
	if err != nil {
		return nil, err
	}
	c.enrichCodeReviewSince(issues)
	return issues, nil
}

// GenesisThreeLeadSupervisor is the supervisor (PIC lead) whose engineers form the GenesisThree team.
const GenesisThreeLeadSupervisor = "Susi Cahyati"

// genesisThreeAssigneeClause builds a quoted, comma-separated JQL assignee list from the
// GenesisThree engineers registered under GenesisThreeLeadSupervisor in engineer.go. Returns
// ok=false when no GenesisThree engineer is registered.
func genesisThreeAssigneeClause() (string, bool) {
	names := engineer.NamesBySupervisor(GenesisThreeLeadSupervisor)
	if len(names) == 0 {
		return "", false
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf(`"%s"`, n))
	}
	return strings.Join(quoted, ","), true
}

// GetGenesisThreeHangingBugs fetches GenesisThree-team bugs stuck in "To Do" on the
// GenesisThree project board, assigned to a GenesisThree engineer.
func (c *Client) GetGenesisThreeHangingBugs(stuckMinutes int) ([]Issue, error) {
	assignees, ok := genesisThreeAssigneeClause()
	if !ok {
		return nil, nil
	}
	// Include the GenesisThree lead (Susi Cahyati) himself as an assignee, in addition to his engineers.
	assignees += `,"Susi Cahyati"`
	jql := fmt.Sprintf(
		`issuetype = Bug AND project = Genesis AND status in ("Todo","To Do","Reject","Rejected") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`, assignees,
	)
	return c.searchIssues(jql, 100)
}

// GetGenesisThreeHangingCodeReviews fetches GenesisThree-team bugs stuck in "Code Review"
// on the GenesisThree project board, assigned to a GenesisThree engineer.
func (c *Client) GetGenesisThreeHangingCodeReviews(stuckMinutes int) ([]Issue, error) {
	assignees, ok := genesisThreeAssigneeClause()
	if !ok {
		return nil, nil
	}
	assignees += `,"Susi Cahyati"`
	jql := fmt.Sprintf(
		`project = Genesis AND status in ("CODE REVIEW","Code Review") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`,
		assignees,
	)
	issues, err := c.searchIssues(jql, 100)
	if err != nil {
		return nil, err
	}
	c.enrichCodeReviewSince(issues)
	return issues, nil
}

// CustomerAppsLeadSupervisor is the supervisor (PIC lead) whose engineers form the CustomerApps team.
const CustomerAppsLeadSupervisor = "Falih Mulyana"

// customerAppsAssigneeClause builds a quoted, comma-separated JQL assignee list from the
// CustomerApps engineers registered under CustomerAppsLeadSupervisor in engineer.go. Returns
// ok=false when no CustomerApps engineer is registered.
func customerAppsAssigneeClause() (string, bool) {
	names := engineer.NamesBySupervisor(CustomerAppsLeadSupervisor)
	if len(names) == 0 {
		return "", false
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf(`"%s"`, n))
	}
	return strings.Join(quoted, ","), true
}

// GetCustomerAppsHangingBugs fetches CustomerApps-team bugs stuck in "To Do" on the
// CustomerApps project board, assigned to a CustomerApps engineer.
func (c *Client) GetCustomerAppsHangingBugs(stuckMinutes int) ([]Issue, error) {
	assignees, ok := customerAppsAssigneeClause()
	if !ok {
		return nil, nil
	}
	// Include the CustomerApps lead (Falih Mulyana) himself as an assignee, in addition to his engineers.
	assignees += `,"Falih Mulyana"`
	jql := fmt.Sprintf(
		`issuetype = Bug AND project in ("Customer App Team") AND status in ("Todo","To Do","Reject","Rejected") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`, assignees,
	)
	return c.searchIssues(jql, 100)
}

// GetCustomerAppsHangingCodeReviews fetches CustomerApps-team bugs stuck in "Code Review"
// on the CustomerApps project board, assigned to a CustomerApps engineer.
func (c *Client) GetCustomerAppsHangingCodeReviews(stuckMinutes int) ([]Issue, error) {
	assignees, ok := customerAppsAssigneeClause()
	if !ok {
		return nil, nil
	}
	assignees += `,"Falih Mulyana"`
	jql := fmt.Sprintf(
		`project in ("Customer App Team") AND status in ("CODE REVIEW","Code Review") AND assignee in (%s) AND created>="2026/05/04" ORDER BY created ASC`,
		assignees,
	)
	issues, err := c.searchIssues(jql, 100)
	if err != nil {
		return nil, err
	}
	c.enrichCodeReviewSince(issues)
	return issues, nil
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

	c.enrichCodeReviewSince(issues)
	return issues, nil
}

// enrichCodeReviewSince fills in each issue's CodeReviewSince by reading its
// changelog for the most recent transition into "Code Review". Issues whose
// changelog is unavailable or lacks such a transition keep a zero CodeReviewSince,
// so callers can fall back to Created. This must be applied to any issue list
// that feeds a Code Review hang alert, otherwise the hang time is wrongly
// measured from Created instead of from when the task entered Code Review.
func (c *Client) enrichCodeReviewSince(issues []Issue) {
	for i := range issues {
		t, err := c.getCodeReviewTransitionTime(issues[i].Key)
		if err == nil && !t.IsZero() {
			issues[i].CodeReviewSince = t
		}
	}
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
