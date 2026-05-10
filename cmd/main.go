package main

import (
	"log"
	"strings"
	"time"

	"github.com/lionparcel/eng-reminder/internal/config"
	"github.com/lionparcel/eng-reminder/internal/engineer"
	"github.com/lionparcel/eng-reminder/internal/jira"
	"github.com/lionparcel/eng-reminder/internal/notifier"
)

const (
	tickIntervalBug        = 15 * time.Minute
	tickIntervalSP         = 30 * time.Minute
	tickIntervalCodeReview = 60 * time.Minute
)

func main() {
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("[eng-reminder] invalid config: %v", err)
	}

	jiraClient := jira.NewClient(cfg.JiraBaseURL, cfg.JiraEmail, cfg.JiraAPIToken)
	discord := notifier.NewDiscord(cfg.DiscordWebhookURL)
	mentionIDs := parseMentions(cfg.DiscordLeadIDs)

	var discordSP *notifier.Discord
	if cfg.DiscordSPAlertWebhookURL != "" {
		discordSP = notifier.NewDiscord(cfg.DiscordSPAlertWebhookURL)
	}

	var discordCodeReview *notifier.Discord
	if cfg.DiscordCodeReviewWebhookURL != "" {
		discordCodeReview = notifier.NewDiscord(cfg.DiscordCodeReviewWebhookURL)
	}

	// Jalankan langsung saat pertama kali start
	runBugAlerts(jiraClient, discord, mentionIDs, cfg)
	runSPCheck(jiraClient, discordSP, mentionIDs, cfg)
	runCodeReviewCheck(jiraClient, discordCodeReview, mentionIDs)

	// Bug alerts setiap 15 menit
	bugTicker := time.NewTicker(tickIntervalBug)
	defer bugTicker.Stop()

	// SP capacity check setiap 30 menit
	spTicker := time.NewTicker(tickIntervalSP)
	defer spTicker.Stop()

	// Code review task alert setiap 15 menit
	crTaskTicker := time.NewTicker(tickIntervalCodeReview)
	defer crTaskTicker.Stop()

	for {
		select {
		case <-bugTicker.C:
			runBugAlerts(jiraClient, discord, mentionIDs, cfg)
		case <-spTicker.C:
			runSPCheck(jiraClient, discordSP, mentionIDs, cfg)
		case <-crTaskTicker.C:
			runCodeReviewCheck(jiraClient, discordCodeReview, mentionIDs)
		}
	}
}

func runBugAlerts(jiraClient *jira.Client, discord *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	log.Printf("[eng-reminder] ── bug alerts started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–18:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping bug alerts", hour, nowWIB.Minute())
		return
	}

	// ── 1. Hanging bug alert: bug stuck di To Do > threshold menit ────────
	log.Printf("[eng-reminder] checking for bugs hanging in To Do > %d minutes...", cfg.BugHangingMinutes)
	hangingBugs, err := jiraClient.GetHangingBugs(cfg.BugHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch hanging bugs: %v", err)
	} else if len(hangingBugs) > 0 {
		severity := jira.HangingSeverity(len(hangingBugs))
		log.Printf("[eng-reminder] found %d hanging bug(s) — severity %s, sending alert...", len(hangingBugs), severity)
		if err := discord.SendHangingBugAlert(hangingBugs, severity, cfg.BugHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
			log.Printf("[eng-reminder] failed to send hanging bug alert: %v", err)
		}
	} else {
		log.Println("[eng-reminder] no hanging bugs found")
	}

	// ── 2. Hanging Code Review alert ────────────────────────────────────
	log.Printf("[eng-reminder] checking for bugs hanging in Code Review > %d minutes...", cfg.CodeReviewHangingMinutes)
	crBugs, err := jiraClient.GetHangingCodeReviews(cfg.CodeReviewHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch hanging code reviews: %v", err)
	} else if len(crBugs) > 0 {
		severity := jira.HangingSeverity(len(crBugs))
		log.Printf("[eng-reminder] found %d code review hanging — severity %s, sending alert...", len(crBugs), severity)
		if err := discord.SendHangingCodeReviewAlert(crBugs, severity, cfg.CodeReviewHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
			log.Printf("[eng-reminder] failed to send code review hanging alert: %v", err)
		}
	} else {
		log.Println("[eng-reminder] no hanging code reviews found")
	}

	log.Println("[eng-reminder] ── bug alerts completed ──")
}

func runSPCheck(jiraClient *jira.Client, discordSP *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	if discordSP == nil {
		return
	}

	log.Printf("[eng-reminder] ── SP check started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–18:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping SP check", hour, nowWIB.Minute())
		return
	}

	today := nowWIB.Format("2006-01-02")
	log.Printf("[eng-reminder] checking SP capacity for engineers on %s...", today)
	tasks, err := jiraClient.GetTasksByExpectedStartDate(today)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch engineer tasks: %v", err)
		return
	}
	above, below, noTasks := categorizeEngineerSP(tasks)
	log.Printf("[eng-reminder] SP check: %d above/at target, %d below target, %d no tasks", len(above), len(below), len(noTasks))
	if err := discordSP.SendSPCapacityAlert(today, above, below, noTasks, mentionIDs); err != nil {
		log.Printf("[eng-reminder] failed to send SP capacity alert: %v", err)
	}

	log.Println("[eng-reminder] ── SP check completed ──")
}

// categorizeEngineerSP groups tasks by engineer and categorises each engineer
// into above-capacity, below-capacity, or no-tasks buckets.
func categorizeEngineerSP(tasks []jira.EngineerTask) (above, below []jira.EngineerSPSummary, noTasks []engineer.Engineer) {
	// group tasks by Jira assignee displayName
	tasksByAssignee := make(map[string][]jira.EngineerTask)
	for _, t := range tasks {
		tasksByAssignee[t.Assignee] = append(tasksByAssignee[t.Assignee], t)
	}

	for _, eng := range engineer.Team {
		var engTasks []jira.EngineerTask
		for assigneeName, taskList := range tasksByAssignee {
			matched := engineer.FindByJiraDisplayName(assigneeName)
			if matched != nil && matched.ID == eng.ID {
				engTasks = taskList
				break
			}
		}

		if len(engTasks) == 0 {
			noTasks = append(noTasks, eng)
			continue
		}

		totalSP := 0.0
		for _, t := range engTasks {
			totalSP += t.StoryPoints
		}
		summary := jira.EngineerSPSummary{
			EngineerName:  eng.Name,
			DailyCapacity: eng.StoryPointsPerDay,
			TotalSP:       totalSP,
			TaskCount:     len(engTasks),
			Tasks:         engTasks,
		}
		if totalSP >= float64(eng.StoryPointsPerDay) {
			above = append(above, summary)
		} else {
			below = append(below, summary)
		}
	}
	return
}

func runCodeReviewCheck(jiraClient *jira.Client, discordCR *notifier.Discord, mentionIDs []string) {
	if discordCR == nil {
		return
	}

	log.Printf("[eng-reminder] ── code review task check started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping code review task check", hour, nowWIB.Minute())
		return
	}

	issues, err := jiraClient.GetCodeReviewTasks()
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch code review tasks: %v", err)
		return
	}
	if len(issues) == 0 {
		log.Println("[eng-reminder] no code review tasks found for engineers")
		return
	}
	log.Printf("[eng-reminder] found %d code review task(s), sending alert...", len(issues))
	if err := discordCR.SendCodeReviewTaskAlert(issues, mentionIDs); err != nil {
		log.Printf("[eng-reminder] failed to send code review task alert: %v", err)
	}

	log.Println("[eng-reminder] ── code review task check completed ──")
}

// parseMentions splits a comma-separated string of Discord user IDs.
func parseMentions(raw string) []string {
	if raw == "" {
		return nil
	}
	ids := []string{}
	for _, u := range strings.Split(raw, ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			ids = append(ids, u)
		}
	}
	return ids
}
