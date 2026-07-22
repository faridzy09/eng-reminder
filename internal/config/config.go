package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all environment-driven configuration.
type Config struct {
	JiraBaseURL                      string
	JiraEmail                        string
	JiraAPIToken                     string
	JiraMaxResults                   int
	JiraNewBugWindowMinutes          int // lookback window for new-bug alerts (default 15 min)
	BugHangingMinutes                int // minutes before a To Do bug is considered hanging (default 10)
	CodeReviewHangingMinutes         int // minutes before a Code Review bug is considered hanging (default 10)
	DiscordWebhookURL                string
	DiscordLeadIDs                   string // comma-separated Discord user IDs (numeric) e.g. "123456789012345678,987654321098765432"
	DiscordSPAlertWebhookURL         string // webhook for the SP capacity alert channel (optional)
	DiscordCodeReviewWebhookURL      string // webhook for the engineer code review task alert channel (optional)
	DiscordFEBugWebhookURL           string // webhook for the FE-team hanging bug alert channel (optional)
	DiscordFELeadIDs                 string // comma-separated Discord user IDs for the FE lead (Faridho)
	DiscordCorbBugWebhookURL         string // webhook for the CORB-team hanging bug alert channel (optional)
	DiscordCorbLeadIDs               string // comma-separated Discord user IDs for the CORB lead (Sholahuddin Alisyahbana)
	DiscordGenesisBugWebhookURL      string // webhook for the Genesis-team hanging bug alert channel "genesis 1" (optional)
	DiscordGenesisLeadIDs            string // comma-separated Discord user IDs for the Genesis lead (Irvan Resna Hadiyana)
	DiscordGenesisTwoBugWebhookURL   string // webhook for the GenesisTwo-team hanging bug alert channel "genesis 2" (optional)
	DiscordGenesisTwoLeadIDs         string // comma-separated Discord user IDs for the GenesisTwo lead (DeriKurniawan)
	DiscordGenesisThreeBugWebhookURL string // webhook for the GenesisThree-team hanging bug alert channel "genesis 3" (optional)
	DiscordGenesisThreeLeadIDs       string // comma-separated Discord user IDs for the GenesisThree lead (Susi Cahyati)
	DiscordCustomerAppsBugWebhookURL string // webhook for the CustomerApps-team hanging bug alert channel "customer apps" (optional)
	DiscordCustomerAppsLeadIDs       string // comma-separated Discord user IDs for the CustomerApps lead (Falih Mulyana)
}

// Load reads configuration from environment variables.
// When running locally, it also loads a .env file if present.
func Load() *Config {
	// Load .env file if it exists (silently ignored in production/CI where env vars are set directly)
	_ = godotenv.Load()
	maxResults := 10
	if v := os.Getenv("JIRA_MAX_RESULTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxResults = n
		}
	}

	newBugWindow := 15
	if v := os.Getenv("JIRA_NEW_BUG_WINDOW_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			newBugWindow = n
		}
	}

	hangingMinutes := 10
	if v := os.Getenv("BUG_HANGING_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			hangingMinutes = n
		}
	}

	crHangingMinutes := 10
	if v := os.Getenv("CODE_REVIEW_HANGING_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			crHangingMinutes = n
		}
	}

	return &Config{
		JiraBaseURL:                      os.Getenv("JIRA_BASE_URL"),
		JiraEmail:                        os.Getenv("JIRA_EMAIL"),
		JiraAPIToken:                     os.Getenv("JIRA_API_TOKEN"),
		JiraMaxResults:                   maxResults,
		JiraNewBugWindowMinutes:          newBugWindow,
		BugHangingMinutes:                hangingMinutes,
		CodeReviewHangingMinutes:         crHangingMinutes,
		DiscordWebhookURL:                os.Getenv("DISCORD_WEBHOOK_URL"),
		DiscordLeadIDs:                   os.Getenv("DISCORD_LEAD_IDS"),
		DiscordSPAlertWebhookURL:         os.Getenv("DISCORD_SP_ALERT_WEBHOOK_URL"),
		DiscordCodeReviewWebhookURL:      os.Getenv("DISCORD_CODE_REVIEW_WEBHOOK_URL"),
		DiscordFEBugWebhookURL:           os.Getenv("DISCORD_FE_BUG_WEBHOOK_URL"),
		DiscordFELeadIDs:                 os.Getenv("DISCORD_FE_LEAD_IDS"),
		DiscordCorbBugWebhookURL:         os.Getenv("DISCORD_CORB_BUG_WEBHOOK_URL"),
		DiscordCorbLeadIDs:               os.Getenv("DISCORD_CORB_LEAD_IDS"),
		DiscordGenesisBugWebhookURL:      os.Getenv("DISCORD_GENESIS_BUG_WEBHOOK_URL"),
		DiscordGenesisLeadIDs:            os.Getenv("DISCORD_GENESIS_LEAD_IDS"),
		DiscordGenesisTwoBugWebhookURL:   os.Getenv("DISCORD_GENESIS2_BUG_WEBHOOK_URL"),
		DiscordGenesisTwoLeadIDs:         os.Getenv("DISCORD_GENESIS2_LEAD_IDS"),
		DiscordGenesisThreeBugWebhookURL: os.Getenv("DISCORD_GENESIS3_BUG_WEBHOOK_URL"),
		DiscordGenesisThreeLeadIDs:       os.Getenv("DISCORD_GENESIS3_LEAD_IDS"),
		DiscordCustomerAppsBugWebhookURL: os.Getenv("DISCORD_CUSTOMER_APPS_BUG_WEBHOOK_URL"),
		DiscordCustomerAppsLeadIDs:       os.Getenv("DISCORD_CUSTOMER_APPS_LEAD_IDS"),
	}
}

// Validate checks required fields are present.
func (c *Config) Validate() error {
	required := map[string]string{
		"JIRA_BASE_URL":       c.JiraBaseURL,
		"JIRA_EMAIL":          c.JiraEmail,
		"JIRA_API_TOKEN":      c.JiraAPIToken,
		"DISCORD_WEBHOOK_URL": c.DiscordWebhookURL,
	}
	for k, v := range required {
		if v == "" {
			return fmt.Errorf("missing required env var: %s", k)
		}
	}
	return nil
}
