package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lionparcel/eng-reminder/internal/jira"
)

// Discord color constants (decimal representation of hex).
const (
	colorRed    = 15548997 // #ED4245
	colorOrange = 15105570 // #E67E22
	colorYellow = 16776960 // #FFFF00
	colorGreen  = 5763719  // #57F287
	colorBlue   = 3447003  // #3498DB
)

// Discord sends notifications to a Discord channel via Incoming Webhook.
type Discord struct {
	webhookURL string
	httpClient *http.Client
}

// NewDiscord creates a new Discord notifier.
func NewDiscord(webhookURL string) *Discord {
	return &Discord{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// discordPayload is the Discord webhook payload structure.
type discordPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []discordEmbed `json:"embeds,omitempty"`
}

type discordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []discordEmbedField `json:"fields,omitempty"`
	Footer      *discordEmbedFooter `json:"footer,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordEmbedFooter struct {
	Text string `json:"text"`
}

// mentionFormat builds a Discord mention string from a user ID.
// "here"     → @here
// "everyone" → @everyone
// numeric ID → <@ID>
func mentionFormat(userID string) string {
	switch strings.ToLower(userID) {
	case "here":
		return "@here"
	case "everyone":
		return "@everyone"
	default:
		return fmt.Sprintf("<@%s>", userID)
	}
}

// buildMentionText builds the mention content string.
func buildMentionText(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, mentionFormat(id))
	}
	return strings.Join(parts, " ")
}

// priorityEmoji returns an emoji based on Jira priority name.
func priorityEmoji(priority string) string {
	switch strings.ToLower(priority) {
	case "critical", "blocker":
		return "🔴"
	case "high":
		return "🟠"
	case "medium":
		return "🟡"
	case "low":
		return "🟢"
	default:
		return "⚪"
	}
}

// severityEmoji returns an emoji for the given severity level.
func severityEmoji(sev string) string {
	switch sev {
	case "HIGH":
		return "🔴"
	case "MIDDLE":
		return "🟠"
	default:
		return "🟡"
	}
}

// severityColor returns a Discord embed color for the given severity.
func severityColor(sev string) int {
	switch sev {
	case "HIGH":
		return colorRed
	case "MIDDLE":
		return colorOrange
	default:
		return colorYellow
	}
}

// severityColorWord returns the English color word for the severity label in titles.
func severityColorWord(sev string) string {
	switch sev {
	case "HIGH":
		return "RED"
	case "MIDDLE":
		return "ORANGE"
	default:
		return "YELLOW"
	}
}

// buildAssigneeBreakdown returns a breakdown string of bug counts per assignee, sorted by count desc.
func buildAssigneeBreakdown(bugs []jira.Issue) string {
	counts := map[string]int{}
	order := []string{}
	for _, bug := range bugs {
		if _, exists := counts[bug.Assignee]; !exists {
			order = append(order, bug.Assignee)
		}
		counts[bug.Assignee]++
	}
	sort.Slice(order, func(i, j int) bool {
		return counts[order[i]] > counts[order[j]]
	})
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, fmt.Sprintf("**%s**: %d Bug", name, counts[name]))
	}
	return strings.Join(parts, " | ")
}

// epicField returns a formatted epic string from the bugs list.
// Shows the most common epic; notes additional epics if multiple are present.
func epicField(bugs []jira.Issue) string {
	counts := map[string]int{}
	info := map[string][2]string{} // epicKey -> [summary, url]
	order := []string{}
	for _, bug := range bugs {
		if bug.EpicKey == "" {
			continue
		}
		if _, exists := counts[bug.EpicKey]; !exists {
			order = append(order, bug.EpicKey)
		}
		counts[bug.EpicKey]++
		info[bug.EpicKey] = [2]string{bug.EpicSummary, bug.EpicURL}
	}
	if len(counts) == 0 {
		return "–"
	}
	// Sort by count descending
	sort.Slice(order, func(i, j int) bool {
		return counts[order[i]] > counts[order[j]]
	})
	// Build list, respecting Discord field value limit (1024 chars)
	const maxLen = 1024
	var sb strings.Builder
	for i, key := range order {
		v := info[key]
		line := fmt.Sprintf("[%s] %s (%d bug)\n%s", key, truncate(v[0], 60), counts[key], v[1])
		if i < len(order)-1 {
			line += "\n\n"
		}
		if sb.Len()+len(line) > maxLen {
			sb.WriteString(fmt.Sprintf("*...+%d epic lainnya*", len(order)-i))
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// buildBugFields converts issues to Discord embed fields (max 25 per embed).
func buildBugFields(bugs []jira.Issue) []discordEmbedField {
	const maxFields = 25
	count := len(bugs)
	if count > maxFields {
		count = maxFields
	}
	fields := make([]discordEmbedField, 0, count)
	for _, bug := range bugs[:count] {
		emoji := priorityEmoji(bug.Priority)
		stuck := time.Since(bug.Created).Round(time.Minute)
		value := fmt.Sprintf(
			"**Status:** %s | **Priority:** %s\n**Assignee:** %s | **Reporter:** %s\n**Dibuat:** %s (%s yang lalu)\n[🔗 Buka di Jira](%s)",
			bug.Status, bug.Priority,
			bug.Assignee, bug.Reporter,
			bug.Created.Format("02 Jan 2006 15:04"),
			stuck.String(),
			bug.URL,
		)
		fields = append(fields, discordEmbedField{
			Name:  fmt.Sprintf("%s [%s] %s", emoji, bug.Key, truncate(bug.Summary, 200)),
			Value: value,
		})
	}
	return fields
}

// truncate cuts a string to maxLen chars, appending "…" if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// send marshals the payload and POSTs it to the Discord webhook URL.
func (d *Discord) send(payload discordPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := d.httpClient.Post(d.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post to discord: %w", err)
	}
	defer resp.Body.Close()

	// Discord returns 204 No Content on success
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord webhook returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SendBugReminder sends a formatted Discord embed listing open Jira bug issues.
func (d *Discord) SendBugReminder(bugs []jira.Issue, mentionIDs []string, jiraBaseURL string) error {
	const threshLow, threshMid, threshHigh = 6, 10, 15
	triggeredBy := bugs[len(bugs)-1]

	fields := []discordEmbedField{
		{
			Name:  "📌 Epic",
			Value: epicField(bugs),
		},
		{
			Name:   "🦎 Jumlah Bug Open",
			Value:  fmt.Sprintf("**%d** bug", len(bugs)),
			Inline: true,
		},
		{
			Name:   "📊 Threshold",
			Value:  fmt.Sprintf("🔴 High: %d | 🟠 Mid: %d | 🟡 Low: %d", threshHigh, threshMid, threshLow),
			Inline: true,
		},
		{
			Name:  "👤 Breakdown per Assignee",
			Value: buildAssigneeBreakdown(bugs),
		},
		{
			Name:  "🎯 Triggered By",
			Value: fmt.Sprintf("[%s] %s\n%s", triggeredBy.Key, truncate(triggeredBy.Summary, 200), triggeredBy.URL),
		},
		{
			Name:  "📋 Status yang Dihitung",
			Value: "`Todo` | `In Progress` | `Code Review` | `Rejected` | `Reject`",
		},
	}

	embed := discordEmbed{
		Title:       "⚙️ YELLOW ALERT — Open Bug Reminder",
		Color:       colorBlue,
		Description: fmt.Sprintf("Terdapat **%d** bug yang masih open dan belum diselesaikan.", len(bugs)),
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "Eng Ngebug • Open Bug Reminder"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	return d.send(discordPayload{
		Content: buildMentionText(mentionIDs),
		Embeds:  []discordEmbed{embed},
	})
}

// SendNewBugAlert notifies leads when new bug(s) with status "To Do" are created.
func (d *Discord) SendNewBugAlert(bugs []jira.Issue, mentionIDs []string, jiraBaseURL string) error {
	const threshLow, threshMid, threshHigh = 6, 10, 15
	triggeredBy := bugs[len(bugs)-1]

	fields := []discordEmbedField{
		{
			Name:  "📌 Epic",
			Value: epicField(bugs),
		},
		{
			Name:   "🦎 Jumlah Bug Baru",
			Value:  fmt.Sprintf("**%d** bug", len(bugs)),
			Inline: true,
		},
		{
			Name:   "📊 Threshold",
			Value:  fmt.Sprintf("🔴 High: %d | 🟠 Mid: %d | 🟡 Low: %d", threshHigh, threshMid, threshLow),
			Inline: true,
		},
		{
			Name:  "👤 Breakdown per Assignee",
			Value: buildAssigneeBreakdown(bugs),
		},
		{
			Name:  "🎯 Triggered By",
			Value: fmt.Sprintf("[%s] %s\n%s", triggeredBy.Key, truncate(triggeredBy.Summary, 200), triggeredBy.URL),
		},
		{
			Name:  "📋 Status yang Dihitung",
			Value: "`Todo`",
		},
	}

	embed := discordEmbed{
		Title:       "⚙️ NEW BUG ALERT — Ada Bug Baru Masuk!",
		Color:       colorOrange,
		Description: fmt.Sprintf("**%d** bug baru dengan status **To Do** ditemukan — mohon segera ditindaklanjuti!", len(bugs)),
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "Eng Ngebug • New Bug Alert"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	return d.send(discordPayload{
		Content: buildMentionText(mentionIDs),
		Embeds:  []discordEmbed{embed},
	})
}

// SendHangingBugAlert notifies leads when bugs are stuck in "To Do" too long.
func (d *Discord) SendHangingBugAlert(bugs []jira.Issue, severity string, stuckMinutes int, mentionIDs []string, jiraBaseURL string) error {
	sEmoji := severityEmoji(severity)
	colorWord := severityColorWord(severity)

	const threshLow, threshMid, threshHigh = 6, 10, 15

	// Triggered By = bug paling baru (list sorted ASC, ambil terakhir)
	fields := []discordEmbedField{
		{
			Name:  "📌 Epic",
			Value: epicField(bugs),
		},
		{
			Name:   "🦎 Jumlah Bug (Dev Phase)",
			Value:  fmt.Sprintf("**%d** bug", len(bugs)),
			Inline: true,
		},
		{
			Name:   "📊 Threshold",
			Value:  fmt.Sprintf("🔴 High: %d | 🟠 Mid: %d | 🟡 Low: %d", threshHigh, threshMid, threshLow),
			Inline: true,
		},
		{
			Name:  "👤 Breakdown per Assignee",
			Value: buildAssigneeBreakdown(bugs),
		},
		{
			Name:  "🎯 Triggered By",
			Value: "Reminder Hanging Bug",
		},
		{
			Name:  "📋 Status yang Dicek",
			Value: "`Todo` yang terlalu lama tidak berpindah ke status lain",
		},
	}

	embed := discordEmbed{
		Title:       fmt.Sprintf("⚙️ %s ALERT — Bug Menunggu Fixing Engineer", colorWord),
		Color:       severityColor(severity),
		Description: fmt.Sprintf("Bug dalam fase development telah mencapai batas **%s** %s", severity, sEmoji),
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "Eng Ngebug • Development Phase Alert"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	return d.send(discordPayload{
		Content: buildMentionText(mentionIDs),
		Embeds:  []discordEmbed{embed},
	})
}

// SendHangingCodeReviewAlert notifies leads when bugs are stuck in "Code Review" too long.
func (d *Discord) SendHangingCodeReviewAlert(bugs []jira.Issue, severity string, stuckMinutes int, mentionIDs []string, jiraBaseURL string) error {
	sEmoji := severityEmoji(severity)
	colorWord := severityColorWord(severity)

	const threshLow, threshMid, threshHigh = 6, 10, 15

	// Triggered By = bug paling baru (list sorted ASC, ambil terakhir)

	fields := []discordEmbedField{
		{
			Name:  "📌 Epic",
			Value: epicField(bugs),
		},
		{
			Name:   "🦎 Jumlah Bug (Code Review Phase)",
			Value:  fmt.Sprintf("**%d** bug", len(bugs)),
			Inline: true,
		},
		{
			Name:   "📊 Threshold",
			Value:  fmt.Sprintf("🔴 High: %d | 🟠 Mid: %d | 🟡 Low: %d", threshHigh, threshMid, threshLow),
			Inline: true,
		},
		{
			Name:  "👤 Breakdown per Assignee",
			Value: buildAssigneeBreakdown(bugs),
		},
		{
			Name:  "🎯 Triggered By",
			Value: "Code Review Monitoring",
		},
		{
			Name:  "📋 Status yang Dicek",
			Value: "`Code Review` yang terlalu lama tidak berpindah ke status lain",
		},
	}

	embed := discordEmbed{
		Title:       fmt.Sprintf("⚙️ %s ALERT — Bug Menunggu Code Review Lead", colorWord),
		Color:       severityColor(severity),
		Description: fmt.Sprintf("Bug dalam fase code review telah mencapai batas **%s** %s", severity, sEmoji),
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "Eng Ngebug • Code Review Phase Alert"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	return d.send(discordPayload{
		Content: buildMentionText(mentionIDs),
		Embeds:  []discordEmbed{embed},
	})
}
