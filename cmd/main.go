package main

import (
	"log"
	"os"
	"strings"

	"github.com/lionparcel/eng-reminder/internal/config"
	"github.com/lionparcel/eng-reminder/internal/jira"
	"github.com/lionparcel/eng-reminder/internal/notifier"
)

func main() {
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("[eng-reminder] invalid config: %v", err)
	}

	jiraClient := jira.NewClient(cfg.JiraBaseURL, cfg.JiraEmail, cfg.JiraAPIToken)
	discord := notifier.NewDiscord(cfg.DiscordWebhookURL)

	mentionIDs := parseMentions(cfg.DiscordLeadIDs)

	// ── 1. New bug alert: semua bug baru dengan status To Do ─────────────
	log.Println("[eng-reminder] checking for new bugs in To Do...")
	newBugs, err := jiraClient.GetNewBugs(cfg.JiraNewBugWindowMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch new bugs: %v", err)
	} else if len(newBugs) > 0 {
		log.Printf("[eng-reminder] found %d new bug(s), sending new bug alert...", len(newBugs))
		if err := discord.SendNewBugAlert(newBugs, mentionIDs, cfg.JiraBaseURL); err != nil {
			log.Printf("[eng-reminder] failed to send new bug alert: %v", err)
		}
	} else {
		log.Println("[eng-reminder] no new bugs found")
	}

	// ── 2. Hanging bug alert: bug stuck di To Do > threshold menit ────────
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

	// ── 3. Hanging Code Review alert ────────────────────────────────────
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

	// ── 4. General open-bug reminder ─────────────────────────────────────
	log.Println("[eng-reminder] checking Jira for latest bug issues...")
	bugs, err := jiraClient.GetLatestBugs(cfg.JiraMaxResults, cfg.JiraNewBugWindowMinutes)
	if err != nil {
		log.Fatalf("[eng-reminder] failed to fetch Jira bugs: %v", err)
	}

	if len(bugs) == 0 {
		log.Println("[eng-reminder] no open bug issues found, skipping reminder")
		os.Exit(0)
	}

	log.Printf("[eng-reminder] found %d bug issue(s), sending reminder...", len(bugs))
	if err := discord.SendBugReminder(bugs, mentionIDs, cfg.JiraBaseURL); err != nil {
		log.Fatalf("[eng-reminder] failed to send Slack notification: %v", err)
	}

	log.Printf("[eng-reminder] all notifications sent successfully")
	os.Exit(0)
}

// parseMentions splits a comma-separated string of Slack user IDs.
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
