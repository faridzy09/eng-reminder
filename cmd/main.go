package main

import (
	"log"
	"strings"
	"time"

	"github.com/lionparcel/eng-reminder/internal/config"
	"github.com/lionparcel/eng-reminder/internal/jira"
	"github.com/lionparcel/eng-reminder/internal/notifier"
)

const tickInterval = 5 * time.Minute

func main() {
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("[eng-reminder] invalid config: %v", err)
	}

	jiraClient := jira.NewClient(cfg.JiraBaseURL, cfg.JiraEmail, cfg.JiraAPIToken)
	discord := notifier.NewDiscord(cfg.DiscordWebhookURL)
	mentionIDs := parseMentions(cfg.DiscordLeadIDs)

	// Jalankan langsung saat pertama kali start
	run(jiraClient, discord, mentionIDs, cfg)

	// Kemudian ulangi setiap 10 menit
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for range ticker.C {
		run(jiraClient, discord, mentionIDs, cfg)
	}
}

func run(jiraClient *jira.Client, discord *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	log.Printf("[eng-reminder] ── run started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–18:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 18 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping notifications", hour, nowWIB.Minute())
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

	log.Println("[eng-reminder] ── run completed ──")
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
