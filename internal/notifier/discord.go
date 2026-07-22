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

	"github.com/lionparcel/eng-reminder/internal/engineer"
	"github.com/lionparcel/eng-reminder/internal/holiday"
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
		stuck := holiday.BusinessDuration(bug.Created, time.Now()).Round(time.Minute)
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

// hangingEmoji returns a colored indicator based on how long a bug has been hanging.
// > 2 jam   → 🔴 (danger)
// > 1 jam   → 🟠 (warning)
// < 1 jam   → 🟡 (caution)
func hangingEmoji(d time.Duration) string {
	switch {
	case d > 2*time.Hour:
		return "🔴"
	case d > 1*time.Hour:
		return "🟠"
	default:
		return "🟡"
	}
}

// hangDuration returns how long a bug has been hanging, always measured in
// working hours (09:00–18:00 WIB on non-holiday weekdays). It prefers the
// precomputed BusinessHang when set, otherwise derives the working-hours age
// from Created so callers never fall back to raw wall-clock time (which would
// wrongly count overnight, weekend, and holiday hours).
func hangDuration(b jira.Issue) time.Duration {
	if b.BusinessHang > 0 {
		return b.BusinessHang
	}
	if b.Created.IsZero() {
		return 0
	}
	return holiday.BusinessDuration(b.Created, time.Now())
}

// hangingWorstColor returns the embed color matching the longest-hanging bug.
func hangingWorstColor(bugs []jira.Issue) int {
	var worst time.Duration
	for _, b := range bugs {
		if b.Created.IsZero() {
			continue
		}
		if dur := hangDuration(b); dur > worst {
			worst = dur
		}
	}
	switch {
	case worst > 2*time.Hour:
		return colorRed
	case worst > 1*time.Hour:
		return colorOrange
	default:
		return colorYellow
	}
}

// buildHangingBugList builds a list of bug lines with hang duration & color indicator.
// Honors Discord's 1024 char field-value limit.
func buildHangingBugList(bugs []jira.Issue) string {
	// sort by longest hanging first
	sorted := make([]jira.Issue, len(bugs))
	copy(sorted, bugs)
	sort.Slice(sorted, func(i, j int) bool {
		return hangDuration(sorted[i]) > hangDuration(sorted[j])
	})

	const maxLen = 1024
	var sb strings.Builder
	for i, bug := range sorted {
		dur := hangDuration(bug)
		emoji := hangingEmoji(dur)
		line := fmt.Sprintf(
			"%s [[%s]](%s) `%s` · %s · _%s_",
			emoji, bug.Key, bug.URL, friendlyDuration(dur), bug.Assignee, truncate(bug.Summary, 60),
		)
		if i < len(sorted)-1 {
			line += "\n"
		}
		if sb.Len()+len(line) > maxLen {
			sb.WriteString(fmt.Sprintf("\n*...+%d bug lainnya*", len(sorted)-i))
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// buildHangingTaskList builds a list of task lines with hang duration & color indicator.
// Honors Discord's 1024 char field-value limit.
func buildHangingTaskList(task []jira.Issue) string {
	// sort by longest hanging first
	sorted := make([]jira.Issue, len(task))
	copy(sorted, task)
	sort.Slice(sorted, func(i, j int) bool {
		return hangDuration(sorted[i]) > hangDuration(sorted[j])
	})

	const maxLen = 1024
	var sb strings.Builder
	for i, task := range sorted {
		dur := hangDuration(task)
		emoji := hangingEmoji(dur)
		line := fmt.Sprintf(
			"%s [[%s]](%s) `%s` · %s · _%s_",
			emoji, task.Key, task.URL, friendlyDuration(dur), task.Assignee, truncate(task.Summary, 60),
		)
		if i < len(sorted)-1 {
			line += "\n"
		}
		if sb.Len()+len(line) > maxLen {
			sb.WriteString(fmt.Sprintf("\n*...+%d task lainnya*", len(sorted)-i))
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
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
			Name:  "⏱️ Daftar Bug Hanging (urut terlama)",
			Value: buildHangingBugList(bugs),
		},
		{
			Name:  "🎨 Indikator Hang Time",
			Value: "🔴 > 2 jam (danger) · 🟠 > 1 jam (warning) · 🟡 < 1 jam",
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

	// Warna embed mengikuti bug paling lama hanging (bukan severity jumlah).
	embedColor := hangingWorstColor(bugs)
	embed := discordEmbed{
		Title:       fmt.Sprintf("⚙️ %s ALERT — Bug Menunggu Fixing Engineer", colorWord),
		Color:       embedColor,
		Description: fmt.Sprintf("Bug dalam fase development berada pada range **%s** %s", severity, sEmoji),
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
			Name:  "⏱️ Daftar Bug Hanging (urut terlama)",
			Value: buildHangingBugList(bugs),
		},
		{
			Name:  "🎨 Indikator Hang Time",
			Value: "🔴 > 2 jam (danger) · 🟠 > 1 jam (warning) · 🟡 < 1 jam",
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

	// Warna embed mengikuti bug paling lama hanging (bukan severity jumlah).
	embedColor := hangingWorstColor(bugs)
	embed := discordEmbed{
		Title:       fmt.Sprintf("⚙️ %s ALERT — Bug Menunggu Code Review Lead", colorWord),
		Color:       embedColor,
		Description: fmt.Sprintf("Bug dalam fase code review berada pada range **%s** %s", severity, sEmoji),
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "Eng Ngebug • Code Review Phase Alert"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	return d.send(discordPayload{
		Content: buildMentionText(mentionIDs),
		Embeds:  []discordEmbed{embed},
	})
}

// SendSPCapacityAlert sends a daily SP capacity check notification to a dedicated channel.
// Engineers are grouped by supervisor so each SPV's team status is clearly visible.
// aboveCapacity = engineers at or above their daily SP target.
// belowCapacity = engineers who have tasks but total SP is below their daily target.
// noTasks       = engineers with no tasks scheduled for today (Expected Start Date is empty/unset).
func (d *Discord) SendSPCapacityAlert(
	date string,
	aboveCapacity []jira.EngineerSPSummary,
	belowCapacity []jira.EngineerSPSummary,
	noTasks []engineer.Engineer,
	mentionIDs []string,
) error {
	// build lookup maps keyed by engineer name
	aboveMap := make(map[string]jira.EngineerSPSummary, len(aboveCapacity))
	for _, e := range aboveCapacity {
		aboveMap[e.EngineerName] = e
	}
	belowMap := make(map[string]jira.EngineerSPSummary, len(belowCapacity))
	for _, e := range belowCapacity {
		belowMap[e.EngineerName] = e
	}
	noTaskSet := make(map[string]bool, len(noTasks))
	for _, e := range noTasks {
		noTaskSet[e.Name] = true
	}

	// collect unique supervisor names in insertion order
	spvOrder := []string{}
	spvSeen := map[string]bool{}
	for _, eng := range engineer.Team {
		if !spvSeen[eng.Supervisor] {
			spvOrder = append(spvOrder, eng.Supervisor)
			spvSeen[eng.Supervisor] = true
		}
	}

	totalEngineers := len(aboveCapacity) + len(belowCapacity) + len(noTasks)
	needsAction := len(belowCapacity) + len(noTasks)
	overallColor := colorGreen
	if needsAction > totalEngineers/2 {
		overallColor = colorRed
	} else if needsAction > 0 {
		overallColor = colorOrange
	}

	// hitung total SP aktual dan total kapasitas harian dari semua engineer
	var totalActualSP float64
	var totalCapacitySP int
	for _, e := range aboveCapacity {
		totalActualSP += e.TotalSP
		totalCapacitySP += e.DailyCapacity
	}
	for _, e := range belowCapacity {
		totalActualSP += e.TotalSP
		totalCapacitySP += e.DailyCapacity
	}
	for _, eng := range engineer.Team {
		if noTaskSet[eng.Name] {
			totalCapacitySP += eng.StoryPointsPerDay
		}
	}
	totalTaskCount := 0
	for _, e := range aboveCapacity {
		totalTaskCount += e.TaskCount
	}
	for _, e := range belowCapacity {
		totalTaskCount += e.TaskCount
	}

	// summary embed
	summaryEmbed := discordEmbed{
		Title: fmt.Sprintf("📊 SP Capacity Check — %s", date),
		Color: overallColor,
		Description: fmt.Sprintf(
			"Dari **%d** engineer: ✅ **%d** sesuai/melebihi target · ⚠️ **%d** kurang · 🚫 **%d** belum ada task.",
			totalEngineers, len(aboveCapacity), len(belowCapacity), len(noTasks),
		),
		Fields: []discordEmbedField{
			{
				Name:   "🎯 Total SP Harian (Aktual)",
				Value:  fmt.Sprintf("**%g SP** dari %d task", totalActualSP, totalTaskCount),
				Inline: true,
			},
			{
				Name:   "📦 Total Kapasitas SP (Max)",
				Value:  fmt.Sprintf("**%d SP** dari %d engineer", totalCapacitySP, totalEngineers),
				Inline: true,
			},
			{
				Name: "📉 Utilisasi",
				Value: fmt.Sprintf("**%.1f%%**", func() float64 {
					if totalCapacitySP == 0 {
						return 0
					}
					return totalActualSP / float64(totalCapacitySP) * 100
				}()),
				Inline: true,
			},
		},
		Footer:    &discordEmbedFooter{Text: "Eng Reminder • Daily SP Capacity Check"},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	embeds := []discordEmbed{summaryEmbed}

	// one embed per supervisor
	for _, spv := range spvOrder {
		var aboveLines, belowLines, noTaskLines []string

		for _, eng := range engineer.Team {
			if eng.Supervisor != spv {
				continue
			}
			if s, ok := aboveMap[eng.Name]; ok {
				aboveLines = append(aboveLines, fmt.Sprintf("✅ **%s** — %g / %d SP (%d task)", s.EngineerName, s.TotalSP, s.DailyCapacity, s.TaskCount))
			} else if s, ok := belowMap[eng.Name]; ok {
				belowLines = append(belowLines, fmt.Sprintf("⚠️ **%s** — %g / %d SP (%d task)", s.EngineerName, s.TotalSP, s.DailyCapacity, s.TaskCount))
			} else if noTaskSet[eng.Name] {
				noTaskLines = append(noTaskLines, fmt.Sprintf("🚫 **%s** — belum ada task", eng.Name))
			}
		}

		var sb strings.Builder
		for _, l := range aboveLines {
			sb.WriteString(l + "\n")
		}
		for _, l := range belowLines {
			sb.WriteString(l + "\n")
		}
		for _, l := range noTaskLines {
			sb.WriteString(l + "\n")
		}
		body := strings.TrimRight(sb.String(), "\n")
		if body == "" {
			body = "_–_"
		}

		spvColor := colorGreen
		if len(belowLines)+len(noTaskLines) > len(aboveLines) {
			spvColor = colorRed
		} else if len(belowLines)+len(noTaskLines) > 0 {
			spvColor = colorOrange
		}

		embeds = append(embeds, discordEmbed{
			Title: fmt.Sprintf("👤 SPV: %s", spv),
			Color: spvColor,
			Description: fmt.Sprintf(
				"✅ %d sesuai · ⚠️ %d kurang · 🚫 %d belum ada task",
				len(aboveLines), len(belowLines), len(noTaskLines),
			),
			Fields: []discordEmbedField{
				{Name: "Engineer", Value: truncate(body, 1024)},
			},
		})
	}

	// Discord allows max 10 embeds per request; split if needed
	const maxEmbedsPerReq = 10
	for i := 0; i < len(embeds); i += maxEmbedsPerReq {
		end := i + maxEmbedsPerReq
		if end > len(embeds) {
			end = len(embeds)
		}
		// mention leads only on the first request
		content := ""
		if i == 0 {
			content = buildMentionText(mentionIDs)
		}
		if err := d.send(discordPayload{Content: content, Embeds: embeds[i:end]}); err != nil {
			return err
		}
	}
	return nil
}

// friendlyDuration formats a duration as "Xj Ym" (jam & menit) in Indonesian.
// Seconds are ignored. Examples: 2h15m → "2j 15m", 45m → "45m", 3h → "3j".
func friendlyDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dj %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dj", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// SendCodeReviewTaskAlert notifies leads about engineer tasks that are in Code Review,
// grouped by supervisor so each lead can see which of their engineers needs a review.
func (d *Discord) SendCodeReviewTaskAlert(issues []jira.Issue, mentionIDs []string) error {
	// group issues by assignee
	tasksByAssignee := make(map[string][]jira.Issue)
	assigneeOrder := []string{}
	for _, iss := range issues {
		if _, exists := tasksByAssignee[iss.Assignee]; !exists {
			assigneeOrder = append(assigneeOrder, iss.Assignee)
		}
		tasksByAssignee[iss.Assignee] = append(tasksByAssignee[iss.Assignee], iss)
	}

	// collect unique supervisor order from engineer registry
	spvOrder := []string{}
	spvSeen := map[string]bool{}
	for _, eng := range engineer.Team {
		if !spvSeen[eng.Supervisor] {
			spvOrder = append(spvOrder, eng.Supervisor)
			spvSeen[eng.Supervisor] = true
		}
	}

	// summary embed
	wib := time.FixedZone("WIB", 7*60*60)
	summaryEmbed := discordEmbed{
		Title: fmt.Sprintf("🔍 Code Review Needed — %s", time.Now().In(wib).Format("2006-01-02 15:04")),
		Color: colorOrange,
		Description: fmt.Sprintf(
			"Ada **%d** task engineer dalam status **Code Review** yang perlu direview.\n_Dikelompokkan per SPV di bawah ini._",
			len(issues),
		),
		Fields: []discordEmbedField{
			{
				Name:   "📋 Total Task",
				Value:  fmt.Sprintf("**%d** task", len(issues)),
				Inline: true,
			},
			{
				Name:   "👥 Total Engineer",
				Value:  fmt.Sprintf("**%d** engineer", len(assigneeOrder)),
				Inline: true,
			},
		},
		Footer:    &discordEmbedFooter{Text: "Eng Reminder • Code Review Task Alert"},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	embeds := []discordEmbed{summaryEmbed}

	// one embed per supervisor that has engineers with code-review tasks
	for _, spv := range spvOrder {
		var lines []string
		taskCount := 0
		engCount := 0
		for _, eng := range engineer.Team {
			if eng.Supervisor != spv {
				continue
			}
			tasks, ok := tasksByAssignee[eng.Name]
			if !ok {
				continue
			}
			engCount++
			for _, t := range tasks {
				taskCount++
				var durLabel string
				if !t.CodeReviewSince.IsZero() {
					durLabel = friendlyDuration(holiday.BusinessDuration(t.CodeReviewSince, time.Now())) + " di CR"
				} else {
					durLabel = friendlyDuration(holiday.BusinessDuration(t.Created, time.Now())) + " sejak dibuat"
				}
				lines = append(lines, fmt.Sprintf(
					"🔸 **%s** · `%s` · _%s_\n　[[%s] %s](%s)",
					eng.Name, durLabel, t.Status, t.Key, truncate(t.Summary, 70), t.URL,
				))
			}
		}
		if len(lines) == 0 {
			continue
		}
		body := strings.Join(lines, "\n\n")
		embeds = append(embeds, discordEmbed{
			Title:       fmt.Sprintf("👤 SPV: %s  (%d task · %d engineer)", spv, taskCount, engCount),
			Color:       colorOrange,
			Description: truncate(body, 4096),
		})
	}

	// split into batches of 10 embeds (Discord limit)
	const maxEmbedsPerReq = 10
	for i := 0; i < len(embeds); i += maxEmbedsPerReq {
		end := i + maxEmbedsPerReq
		if end > len(embeds) {
			end = len(embeds)
		}
		content := ""
		if i == 0 {
			content = buildMentionText(mentionIDs)
		}
		if err := d.send(discordPayload{Content: content, Embeds: embeds[i:end]}); err != nil {
			return err
		}
	}
	return nil
}

func (d *Discord) SendHangingBugAlertV2(bugs []jira.Issue, severity string, stuckMinutes int, mentionIDs []string, jiraBaseURL string) error {
	sEmoji := severityEmoji(severity)
	colorWord := severityColorWord(severity)

	const threshLow, threshMid, threshHigh = 6, 10, 15

	// Triggered By = bug paling baru (list sorted ASC, ambil terakhir)
	fields := []discordEmbedField{
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
			Name:  "⏱️ Daftar Bug Hanging (urut terlama)",
			Value: buildHangingBugList(bugs),
		},
		{
			Name:  "🎨 Indikator Hang Time",
			Value: "🔴 > 2 jam (danger) · 🟠 > 1 jam (warning) · 🟡 < 1 jam",
		},
	}

	// Warna embed mengikuti bug paling lama hanging (bukan severity jumlah).
	embedColor := hangingWorstColor(bugs)
	embed := discordEmbed{
		Title:       fmt.Sprintf("⚙️ %s ALERT — Menunggu Fixing Engineer", colorWord),
		Color:       embedColor,
		Description: fmt.Sprintf("Bug dalam fase development berada pada range **%s** %s", severity, sEmoji),
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "EBS • Development Phase Alert"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	return d.send(discordPayload{
		Content: buildMentionText(mentionIDs),
		Embeds:  []discordEmbed{embed},
	})
}

func (d *Discord) SendHangingCodeReviewAlertV2(task []jira.Issue, severity string, stuckMinutes int, mentionIDs []string, jiraBaseURL string) error {
	sEmoji := severityEmoji(severity)
	colorWord := severityColorWord(severity)

	const threshLow, threshMid, threshHigh = 6, 10, 15

	// Triggered By = bug paling baru (list sorted ASC, ambil terakhir)

	fields := []discordEmbedField{
		{
			Name:   "🦎 Jumlah Task (Code Review Phase)",
			Value:  fmt.Sprintf("**%d** task", len(task)),
			Inline: true,
		},
		{
			Name:   "📊 Threshold",
			Value:  fmt.Sprintf("🔴 High: %d | 🟠 Mid: %d | 🟡 Low: %d", threshHigh, threshMid, threshLow),
			Inline: true,
		},
		{
			Name:  "⏱️ Daftar Task Hanging (urut terlama)",
			Value: buildHangingTaskList(task),
		},
		{
			Name:  "🎨 Indikator Hang Time",
			Value: "🔴 > 2 jam (danger) · 🟠 > 1 jam (warning) · 🟡 < 1 jam",
		},
	}

	// Warna embed mengikuti task paling lama hanging (bukan severity jumlah).
	embedColor := hangingWorstColor(task)
	embed := discordEmbed{
		Title:       fmt.Sprintf("⚙️ %s ALERT — Task Menunggu Code Review Lead", colorWord),
		Color:       embedColor,
		Description: fmt.Sprintf("Task dalam fase code review berada pada range **%s** %s", severity, sEmoji),
		Fields:      fields,
		Footer:      &discordEmbedFooter{Text: "EBS • Code Review Phase Alert"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	return d.send(discordPayload{
		Content: buildMentionText(mentionIDs),
		Embeds:  []discordEmbed{embed},
	})
}
