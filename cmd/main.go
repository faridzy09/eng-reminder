package main

import (
	"log"
	"strings"
	"time"

	"github.com/lionparcel/eng-reminder/internal/config"
	"github.com/lionparcel/eng-reminder/internal/engineer"
	"github.com/lionparcel/eng-reminder/internal/holiday"
	"github.com/lionparcel/eng-reminder/internal/jira"
	"github.com/lionparcel/eng-reminder/internal/notifier"
)

// feMinHang is the minimum working-hours age a FE bug must reach before it is
// alerted: only bugs hanging longer than 1 working hour (09:00–18:00 WIB,
// excluding weekends & holidays) are notified.
const feMinHang = 1 * time.Hour

const (
	tickIntervalBug        = 30 * time.Minute
	tickIntervalSP         = 60 * time.Minute
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

	var discordFEBug *notifier.Discord
	if cfg.DiscordFEBugWebhookURL != "" {
		discordFEBug = notifier.NewDiscord(cfg.DiscordFEBugWebhookURL)
	}
	feMentionIDs := parseMentions(cfg.DiscordFELeadIDs)

	var discordCorbBug *notifier.Discord
	if cfg.DiscordCorbBugWebhookURL != "" {
		discordCorbBug = notifier.NewDiscord(cfg.DiscordCorbBugWebhookURL)
	}
	corbMentionIDs := parseMentions(cfg.DiscordCorbLeadIDs)

	// Genesis-team channel "genesis 1" (lead: Irvan Resna Hadiyana)
	var discordGenesisBug *notifier.Discord
	if cfg.DiscordGenesisBugWebhookURL != "" {
		discordGenesisBug = notifier.NewDiscord(cfg.DiscordGenesisBugWebhookURL)
	}
	genesisMentionIDs := parseMentions(cfg.DiscordGenesisLeadIDs)

	// GenesisTwo-team channel "genesis 2" (lead: DeriKurniawan)
	var discordGenesisTwoBug *notifier.Discord
	if cfg.DiscordGenesisTwoBugWebhookURL != "" {
		discordGenesisTwoBug = notifier.NewDiscord(cfg.DiscordGenesisTwoBugWebhookURL)
	}
	genesisTwoMentionIDs := parseMentions(cfg.DiscordGenesisTwoLeadIDs)

	// GenesisThree-team channel "genesis 3" (lead: Susi Cahyati)
	var discordGenesisThreeBug *notifier.Discord
	if cfg.DiscordGenesisThreeBugWebhookURL != "" {
		discordGenesisThreeBug = notifier.NewDiscord(cfg.DiscordGenesisThreeBugWebhookURL)
	}
	genesisThreeMentionIDs := parseMentions(cfg.DiscordGenesisThreeLeadIDs)

	// CustomerApps-team channel "customer apps" (lead: Falih Mulyana)
	var discordCustomerAppsBug *notifier.Discord
	if cfg.DiscordCustomerAppsBugWebhookURL != "" {
		discordCustomerAppsBug = notifier.NewDiscord(cfg.DiscordCustomerAppsBugWebhookURL)
	}
	customerAppsMentionIDs := parseMentions(cfg.DiscordCustomerAppsLeadIDs)

	// Jalankan langsung saat pertama kali start
	runBugAlerts(jiraClient, discord, mentionIDs, cfg)
	runFEBugAlerts(jiraClient, discordFEBug, feMentionIDs, cfg)
	runCorbBugAlerts(jiraClient, discordCorbBug, corbMentionIDs, cfg)
	runGenesisBugAlerts(jiraClient, discordGenesisBug, genesisMentionIDs, cfg)
	runGenesisTwoBugAlerts(jiraClient, discordGenesisTwoBug, genesisTwoMentionIDs, cfg)
	runGenesisThreeBugAlerts(jiraClient, discordGenesisThreeBug, genesisThreeMentionIDs, cfg)
	runCustomerAppsBugAlerts(jiraClient, discordCustomerAppsBug, customerAppsMentionIDs, cfg)
	runSPCheck(jiraClient, discordSP, mentionIDs, cfg)
	runCodeReviewCheck(jiraClient, discordCodeReview, mentionIDs)

	// Bug alerts setiap 30 menit
	bugTicker := time.NewTicker(tickIntervalBug)
	defer bugTicker.Stop()

	// SP capacity check setiap 60 menit
	spTicker := time.NewTicker(tickIntervalSP)
	defer spTicker.Stop()

	// Code review task alert setiap 15 menit
	crTaskTicker := time.NewTicker(tickIntervalCodeReview)
	defer crTaskTicker.Stop()

	for {
		select {
		case <-bugTicker.C:
			runBugAlerts(jiraClient, discord, mentionIDs, cfg)
			runFEBugAlerts(jiraClient, discordFEBug, feMentionIDs, cfg)
			runCorbBugAlerts(jiraClient, discordCorbBug, corbMentionIDs, cfg)
			runGenesisBugAlerts(jiraClient, discordGenesisBug, genesisMentionIDs, cfg)
			runGenesisTwoBugAlerts(jiraClient, discordGenesisTwoBug, genesisTwoMentionIDs, cfg)
			runGenesisThreeBugAlerts(jiraClient, discordGenesisThreeBug, genesisThreeMentionIDs, cfg)
			runCustomerAppsBugAlerts(jiraClient, discordCustomerAppsBug, customerAppsMentionIDs, cfg)
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

// runFEBugAlerts sends hanging bug alerts to the FE-team channel. It only covers
// bugs on the FE project boards (GENESIS, CORB, WEBLP, CUST) assigned to FE
// engineers (PIC lead Faridho, read from engineer.go).
func runFEBugAlerts(jiraClient *jira.Client, discordFE *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	if discordFE == nil {
		return
	}

	log.Printf("[eng-reminder] ── FE bug alerts started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–20:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping FE bug alerts", hour, nowWIB.Minute())
		return
	}

	now := time.Now()

	// ── 1. FE hanging bug alert: bug stuck di To Do > 1 jam kerja ─────────
	log.Printf("[eng-reminder] checking for FE bugs hanging in To Do > 1 working hour (09:00–18:00 WIB)...")
	hangingBugs, err := jiraClient.GetFEHangingBugs(cfg.BugHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch FE hanging bugs: %v", err)
	} else {
		hangingBugs = filterBusinessHanging(hangingBugs, feMinHang, now)
		if len(hangingBugs) > 0 {
			severity := jira.HangingSeverity(len(hangingBugs))
			log.Printf("[eng-reminder] found %d FE hanging bug(s) past 1 working hour — severity %s, sending alert...", len(hangingBugs), severity)
			if err := discordFE.SendHangingBugAlertV2(hangingBugs, severity, cfg.BugHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send FE hanging bug alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no FE hanging bugs past 1 working hour")
		}
	}

	// ── 2. FE hanging Code Review alert ──────────────────────────────────
	log.Printf("[eng-reminder] checking for FE bugs hanging in Code Review > 1 working hour (09:00–18:00 WIB)...")
	crBugs, err := jiraClient.GetFEHangingCodeReviews(cfg.CodeReviewHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch FE hanging code reviews: %v", err)
	} else {
		crBugs = filterBusinessHangingCR(crBugs, feMinHang, now)
		if len(crBugs) > 0 {
			severity := jira.HangingSeverity(len(crBugs))
			log.Printf("[eng-reminder] found %d FE code review hanging past 1 working hour — severity %s, sending alert...", len(crBugs), severity)
			if err := discordFE.SendHangingCodeReviewAlertV2(crBugs, severity, cfg.CodeReviewHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send FE code review hanging alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no FE hanging code reviews past 1 working hour")
		}
	}

	log.Println("[eng-reminder] ── FE bug alerts completed ──")
}

// runCorbBugAlerts sends hanging bug alerts to the CORB-team channel. It only
// covers bugs on the project boards (GENESIS, CORB, WEBLP, CUST) assigned to CORB
// engineers (PIC lead Sholahuddin Alisyahbana, read from engineer.go).
func runCorbBugAlerts(jiraClient *jira.Client, discordCorb *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	if discordCorb == nil {
		return
	}

	log.Printf("[eng-reminder] ── CORB bug alerts started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–20:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping CORB bug alerts", hour, nowWIB.Minute())
		return
	}

	now := time.Now()

	// ── 1. CORB hanging bug alert: bug stuck di To Do > 1 jam kerja ───────
	log.Printf("[eng-reminder] checking for CORB bugs hanging in To Do > 1 working hour (09:00–18:00 WIB)...")
	hangingBugs, err := jiraClient.GetCorbHangingBugs(cfg.BugHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch CORB hanging bugs: %v", err)
	} else {
		hangingBugs = filterBusinessHanging(hangingBugs, feMinHang, now)
		if len(hangingBugs) > 0 {
			severity := jira.HangingSeverity(len(hangingBugs))
			log.Printf("[eng-reminder] found %d CORB hanging bug(s) past 1 working hour — severity %s, sending alert...", len(hangingBugs), severity)
			if err := discordCorb.SendHangingBugAlertV2(hangingBugs, severity, cfg.BugHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send CORB hanging bug alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no CORB hanging bugs past 1 working hour")
		}
	}

	// ── 2. CORB hanging Code Review alert ────────────────────────────────
	log.Printf("[eng-reminder] checking for CORB bugs hanging in Code Review > 1 working hour (09:00–18:00 WIB)...")
	crBugs, err := jiraClient.GetCorbHangingCodeReviews(cfg.CodeReviewHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch CORB hanging code reviews: %v", err)
	} else {
		crBugs = filterBusinessHangingCR(crBugs, feMinHang, now)
		if len(crBugs) > 0 {
			severity := jira.HangingSeverity(len(crBugs))
			log.Printf("[eng-reminder] found %d CORB code review hanging past 1 working hour — severity %s, sending alert...", len(crBugs), severity)
			if err := discordCorb.SendHangingCodeReviewAlertV2(crBugs, severity, cfg.CodeReviewHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send CORB code review hanging alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no CORB hanging code reviews past 1 working hour")
		}
	}

	log.Println("[eng-reminder] ── CORB bug alerts completed ──")
}

// runGenesisBugAlerts sends hanging bug alerts to the Genesis-team channel
// "genesis 1". It only covers bugs on the Genesis project board assigned to
// Genesis engineers (PIC lead Irvan Resna Hadiyana, read from engineer.go).
func runGenesisBugAlerts(jiraClient *jira.Client, discordGenesis *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	if discordGenesis == nil {
		return
	}

	log.Printf("[eng-reminder] ── Genesis bug alerts started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–20:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping Genesis bug alerts", hour, nowWIB.Minute())
		return
	}

	now := time.Now()

	// ── 1. Genesis hanging bug alert: bug stuck di To Do > 1 jam kerja ───────
	log.Printf("[eng-reminder] checking for Genesis bugs hanging in To Do > 1 working hour (09:00–18:00 WIB)...")
	hangingBugs, err := jiraClient.GetGenesisHangingBugs(cfg.BugHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch Genesis hanging bugs: %v", err)
	} else {
		hangingBugs = filterBusinessHanging(hangingBugs, feMinHang, now)
		if len(hangingBugs) > 0 {
			severity := jira.HangingSeverity(len(hangingBugs))
			log.Printf("[eng-reminder] found %d Genesis hanging bug(s) past 1 working hour — severity %s, sending alert...", len(hangingBugs), severity)
			if err := discordGenesis.SendHangingBugAlertV2(hangingBugs, severity, cfg.BugHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send Genesis hanging bug alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no Genesis hanging bugs past 1 working hour")
		}
	}

	// ── 2. Genesis hanging Code Review alert ─────────────────────────────────
	log.Printf("[eng-reminder] checking for Genesis bugs hanging in Code Review > 1 working hour (09:00–18:00 WIB)...")
	crBugs, err := jiraClient.GetGenesisHangingCodeReviews(cfg.CodeReviewHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch Genesis hanging code reviews: %v", err)
	} else {
		crBugs = filterBusinessHangingCR(crBugs, feMinHang, now)
		if len(crBugs) > 0 {
			severity := jira.HangingSeverity(len(crBugs))
			log.Printf("[eng-reminder] found %d Genesis code review hanging past 1 working hour — severity %s, sending alert...", len(crBugs), severity)
			if err := discordGenesis.SendHangingCodeReviewAlertV2(crBugs, severity, cfg.CodeReviewHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send Genesis code review hanging alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no Genesis hanging code reviews past 1 working hour")
		}
	}

	log.Println("[eng-reminder] ── Genesis bug alerts completed ──")
}

// runGenesisTwoBugAlerts sends hanging bug alerts to the GenesisTwo-team channel
// "genesis 2". It only covers bugs on the GenesisTwo project board assigned to
// GenesisTwo engineers (PIC lead DeriKurniawan, read from engineer.go).
func runGenesisTwoBugAlerts(jiraClient *jira.Client, discordGenesisTwo *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	if discordGenesisTwo == nil {
		return
	}

	log.Printf("[eng-reminder] ── GenesisTwo bug alerts started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–20:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping GenesisTwo bug alerts", hour, nowWIB.Minute())
		return
	}

	now := time.Now()

	// ── 1. GenesisTwo hanging bug alert: bug stuck di To Do > 1 jam kerja ───────
	log.Printf("[eng-reminder] checking for GenesisTwo bugs hanging in To Do > 1 working hour (09:00–18:00 WIB)...")
	hangingBugs, err := jiraClient.GetGenesisTwoHangingBugs(cfg.BugHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch GenesisTwo hanging bugs: %v", err)
	} else {
		hangingBugs = filterBusinessHanging(hangingBugs, feMinHang, now)
		if len(hangingBugs) > 0 {
			severity := jira.HangingSeverity(len(hangingBugs))
			log.Printf("[eng-reminder] found %d GenesisTwo hanging bug(s) past 1 working hour — severity %s, sending alert...", len(hangingBugs), severity)
			if err := discordGenesisTwo.SendHangingBugAlertV2(hangingBugs, severity, cfg.BugHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send GenesisTwo hanging bug alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no GenesisTwo hanging bugs past 1 working hour")
		}
	}

	// ── 2. GenesisTwo hanging Code Review alert ─────────────────────────────────
	log.Printf("[eng-reminder] checking for GenesisTwo bugs hanging in Code Review > 1 working hour (09:00–18:00 WIB)...")
	crBugs, err := jiraClient.GetGenesisTwoHangingCodeReviews(cfg.CodeReviewHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch GenesisTwo hanging code reviews: %v", err)
	} else {
		crBugs = filterBusinessHangingCR(crBugs, feMinHang, now)
		if len(crBugs) > 0 {
			severity := jira.HangingSeverity(len(crBugs))
			log.Printf("[eng-reminder] found %d GenesisTwo code review hanging past 1 working hour — severity %s, sending alert...", len(crBugs), severity)
			if err := discordGenesisTwo.SendHangingCodeReviewAlertV2(crBugs, severity, cfg.CodeReviewHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send GenesisTwo code review hanging alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no GenesisTwo hanging code reviews past 1 working hour")
		}
	}

	log.Println("[eng-reminder] ── GenesisTwo bug alerts completed ──")
}

// runGenesisThreeBugAlerts sends hanging bug alerts to the GenesisThree-team channel
// "genesis 3". It only covers bugs on the GenesisThree project board assigned to
// GenesisThree engineers (PIC lead Susi Cahyati, read from engineer.go).
func runGenesisThreeBugAlerts(jiraClient *jira.Client, discordGenesisThree *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	if discordGenesisThree == nil {
		return
	}

	log.Printf("[eng-reminder] ── GenesisThree bug alerts started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–20:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping GenesisThree bug alerts", hour, nowWIB.Minute())
		return
	}

	now := time.Now()

	// ── 1. GenesisThree hanging bug alert: bug stuck di To Do > 1 jam kerja ───────
	log.Printf("[eng-reminder] checking for GenesisThree bugs hanging in To Do > 1 working hour (09:00–18:00 WIB)...")
	hangingBugs, err := jiraClient.GetGenesisThreeHangingBugs(cfg.BugHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch GenesisThree hanging bugs: %v", err)
	} else {
		hangingBugs = filterBusinessHanging(hangingBugs, feMinHang, now)
		if len(hangingBugs) > 0 {
			severity := jira.HangingSeverity(len(hangingBugs))
			log.Printf("[eng-reminder] found %d GenesisThree hanging bug(s) past 1 working hour — severity %s, sending alert...", len(hangingBugs), severity)
			if err := discordGenesisThree.SendHangingBugAlertV2(hangingBugs, severity, cfg.BugHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send GenesisThree hanging bug alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no GenesisThree hanging bugs past 1 working hour")
		}
	}

	// ── 2. GenesisThree hanging Code Review alert ─────────────────────────────────
	log.Printf("[eng-reminder] checking for GenesisThree bugs hanging in Code Review > 1 working hour (09:00–18:00 WIB)...")
	crBugs, err := jiraClient.GetGenesisThreeHangingCodeReviews(cfg.CodeReviewHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch GenesisThree hanging code reviews: %v", err)
	} else {
		crBugs = filterBusinessHangingCR(crBugs, feMinHang, now)
		if len(crBugs) > 0 {
			severity := jira.HangingSeverity(len(crBugs))
			log.Printf("[eng-reminder] found %d GenesisThree code review hanging past 1 working hour — severity %s, sending alert...", len(crBugs), severity)
			if err := discordGenesisThree.SendHangingCodeReviewAlertV2(crBugs, severity, cfg.CodeReviewHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send GenesisThree code review hanging alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no GenesisThree hanging code reviews past 1 working hour")
		}
	}

	log.Println("[eng-reminder] ── GenesisThree bug alerts completed ──")
}

// runCustomerAppsBugAlerts sends hanging bug alerts to the CustomerApps-team channel
// "customer apps". It only covers bugs on the CustomerApps project board assigned to
// CustomerApps engineers (PIC lead Falih Mulyana, read from engineer.go).
func runCustomerAppsBugAlerts(jiraClient *jira.Client, discordCustomerApps *notifier.Discord, mentionIDs []string, cfg *config.Config) {
	if discordCustomerApps == nil {
		return
	}

	log.Printf("[eng-reminder] ── CustomerApps bug alerts started at %s ──", time.Now().Format("2006-01-02 15:04:05"))

	// ── Cek jam kerja WIB (UTC+7): hanya kirim notif pukul 08:00–20:00 ──
	wib := time.FixedZone("WIB", 7*60*60)
	nowWIB := time.Now().In(wib)
	hour := nowWIB.Hour()
	if hour < 8 || hour >= 20 {
		log.Printf("[eng-reminder] outside working hours WIB (%02d:%02d), skipping CustomerApps bug alerts", hour, nowWIB.Minute())
		return
	}

	now := time.Now()

	// ── 1. CustomerApps hanging bug alert: bug stuck di To Do > 1 jam kerja ───────
	log.Printf("[eng-reminder] checking for CustomerApps bugs hanging in To Do > 1 working hour (09:00–18:00 WIB)...")
	hangingBugs, err := jiraClient.GetCustomerAppsHangingBugs(cfg.BugHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch CustomerApps hanging bugs: %v", err)
	} else {
		hangingBugs = filterBusinessHanging(hangingBugs, feMinHang, now)
		if len(hangingBugs) > 0 {
			severity := jira.HangingSeverity(len(hangingBugs))
			log.Printf("[eng-reminder] found %d CustomerApps hanging bug(s) past 1 working hour — severity %s, sending alert...", len(hangingBugs), severity)
			if err := discordCustomerApps.SendHangingBugAlertV2(hangingBugs, severity, cfg.BugHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send CustomerApps hanging bug alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no CustomerApps hanging bugs past 1 working hour")
		}
	}

	// ── 2. CustomerApps hanging Code Review alert ─────────────────────────────────
	log.Printf("[eng-reminder] checking for CustomerApps bugs hanging in Code Review > 1 working hour (09:00–18:00 WIB)...")
	crBugs, err := jiraClient.GetCustomerAppsHangingCodeReviews(cfg.CodeReviewHangingMinutes)
	if err != nil {
		log.Printf("[eng-reminder] failed to fetch CustomerApps hanging code reviews: %v", err)
	} else {
		crBugs = filterBusinessHangingCR(crBugs, feMinHang, now)
		if len(crBugs) > 0 {
			severity := jira.HangingSeverity(len(crBugs))
			log.Printf("[eng-reminder] found %d CustomerApps code review hanging past 1 working hour — severity %s, sending alert...", len(crBugs), severity)
			if err := discordCustomerApps.SendHangingCodeReviewAlertV2(crBugs, severity, cfg.CodeReviewHangingMinutes, mentionIDs, cfg.JiraBaseURL); err != nil {
				log.Printf("[eng-reminder] failed to send CustomerApps code review hanging alert: %v", err)
			}
		} else {
			log.Println("[eng-reminder] no CustomerApps hanging code reviews past 1 working hour")
		}
	}

	log.Println("[eng-reminder] ── CustomerApps bug alerts completed ──")
}

// filterBusinessHanging keeps only bugs whose working-hours age (measured from
// Created up to now) exceeds minHang. Working hours are 09:00–18:00 WIB on
// non-holiday weekdays, so time spent overnight, on weekends, or on national
// holidays does not count towards the hang duration.
func filterBusinessHanging(bugs []jira.Issue, minHang time.Duration, now time.Time) []jira.Issue {
	return filterBusinessHangingFrom(bugs, minHang, now, func(b jira.Issue) time.Time { return b.Created })
}

// filterBusinessHangingCR is like filterBusinessHanging but measures the hang
// duration from when the task entered Code Review (CodeReviewSince), falling
// back to Created when the changelog didn't yield a Code Review timestamp. Use
// this for Code Review alerts so the reported time reflects "waiting in Code
// Review", not "age since the bug was created".
func filterBusinessHangingCR(bugs []jira.Issue, minHang time.Duration, now time.Time) []jira.Issue {
	return filterBusinessHangingFrom(bugs, minHang, now, func(b jira.Issue) time.Time {
		if !b.CodeReviewSince.IsZero() {
			return b.CodeReviewSince
		}
		return b.Created
	})
}

// filterBusinessHangingFrom keeps only bugs whose working-hours age, measured
// from anchor(b) up to now, exceeds minHang. Working hours are 09:00–18:00 WIB
// on non-holiday weekdays, so time spent overnight, on weekends, or on national
// holidays does not count. The surviving bugs get BusinessHang set to that age.
func filterBusinessHangingFrom(bugs []jira.Issue, minHang time.Duration, now time.Time, anchor func(jira.Issue) time.Time) []jira.Issue {
	filtered := make([]jira.Issue, 0, len(bugs))
	for _, b := range bugs {
		start := anchor(b)
		if start.IsZero() {
			continue
		}
		hang := holiday.BusinessDuration(start, now)
		if hang > minHang {
			b.BusinessHang = hang
			filtered = append(filtered, b)
		}
	}
	return filtered
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
