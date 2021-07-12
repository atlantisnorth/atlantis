package server

import (
	"github.com/runatlantis/atlantis/server/logging"
)

// UserConfig holds config values passed in by the user.
// The mapstructure tags correspond to flags in cmd/server.go and are used when
// the config is parsed from a YAML file.
type UserConfig struct {
	AllowForkPRs               bool   `mapstructure:"allow-fork-prs"`
	AllowRepoConfig            bool   `mapstructure:"allow-repo-config"`
	AtlantisURL                string `mapstructure:"atlantis-url"`
	Automerge                  bool   `mapstructure:"automerge"`
	AutoplanFileList           string `mapstructure:"autoplan-file-list"`
	AzureDevopsToken           string `mapstructure:"azuredevops-token"`
	AzureDevopsUser            string `mapstructure:"azuredevops-user"`
	AzureDevopsWebhookPassword string `mapstructure:"azuredevops-webhook-password"`
	AzureDevopsWebhookUser     string `mapstructure:"azuredevops-webhook-user"`
	BitbucketBaseURL           string `mapstructure:"bitbucket-base-url"`
	BitbucketToken             string `mapstructure:"bitbucket-token"`
	BitbucketUser              string `mapstructure:"bitbucket-user"`
	BitbucketWebhookSecret     string `mapstructure:"bitbucket-webhook-secret"`
	CheckoutStrategy           string `mapstructure:"checkout-strategy"`
	DataDir                    string `mapstructure:"data-dir"`
	DisableApplyAll            bool   `mapstructure:"disable-apply-all"`
	DisableApply               bool   `mapstructure:"disable-apply"`
	DisableAutoplan            bool   `mapstructure:"disable-autoplan"`
	DisableMarkdownFolding     bool   `mapstructure:"disable-markdown-folding"`
	DisableRepoLocking         bool   `mapstructure:"disable-repo-locking"`
	EnablePolicyChecksFlag     bool   `mapstructure:"enable-policy-checks"`
	EnableRegExpCmd            bool   `mapstructure:"enable-regexp-cmd"`
	GithubHostname             string `mapstructure:"gh-hostname"`
	GithubToken                string `mapstructure:"gh-token"`
	GithubUser                 string `mapstructure:"gh-user"`
	GithubWebhookSecret        string `mapstructure:"gh-webhook-secret"`
	GithubOrg                  string `mapstructure:"gh-org"`
	GithubAppID                int64  `mapstructure:"gh-app-id"`
	GithubAppKey               string `mapstructure:"gh-app-key-file"`
	GithubAppSlug              string `mapstructure:"gh-app-slug"`
	GithubTeamAllowlist        string `mapstructure:"gh-team-allowlist"`
	GitlabHostname             string `mapstructure:"gitlab-hostname"`
	GitlabToken                string `mapstructure:"gitlab-token"`
	GitlabUser                 string `mapstructure:"gitlab-user"`
	GitlabWebhookSecret        string `mapstructure:"gitlab-webhook-secret"`
	HidePrevPlanComments       bool   `mapstructure:"hide-prev-plan-comments"`
	LogLevel                   string `mapstructure:"log-level"`
	ParallelPoolSize           int    `mapstructure:"parallel-pool-size"`
	PlanDrafts                 bool   `mapstructure:"allow-draft-prs"`
	Port                       int    `mapstructure:"port"`
	RepoConfig                 string `mapstructure:"repo-config"`
	RepoConfigJSON             string `mapstructure:"repo-config-json"`
	RepoAllowlist              string `mapstructure:"repo-allowlist"`
	// RepoWhitelist is deprecated in favour of RepoAllowlist.
	RepoWhitelist string `mapstructure:"repo-whitelist"`

	// RequireApproval is whether to require pull request approval before
	// allowing terraform apply's to be run.
	RequireApproval bool `mapstructure:"require-approval"`
	// RequireMergeable is whether to require pull requests to be mergeable before
	// allowing terraform apply's to run.
	RequireMergeable bool `mapstructure:"require-mergeable"`
	// SilenceNoProjects is whether Atlantis should respond to a PR if no projects are found.
	SilenceNoProjects bool `mapstructure:"silence-no-projects"`
	// RequireUnDiverged is whether to require pull requests to rebase default branch before
	// allowing terraform apply's to run.
	RequireUnDiverged   bool `mapstructure:"require-undiverged"`
	SilenceForkPRErrors bool `mapstructure:"silence-fork-pr-errors"`
	// SilenceVCSStatusNoPlans is whether autoplan should set commit status if no plans
	// are found.
	SilenceVCSStatusNoPlans bool `mapstructure:"silence-vcs-status-no-plans"`
	// SilenceVCSStatusNoProjects is whether autoplan should set commit status if no projects
	// are found.
	SilenceVCSStatusNoProjects bool `mapstructure:"silence-vcs-status-no-projects"`
	SilenceAllowlistErrors     bool `mapstructure:"silence-allowlist-errors"`
	// SilenceWhitelistErrors is deprecated in favour of SilenceAllowlistErrors
	SilenceWhitelistErrors bool            `mapstructure:"silence-whitelist-errors"`
	SkipCloneNoChanges     bool            `mapstructure:"skip-clone-no-changes"`
	SlackToken             string          `mapstructure:"slack-token"`
	SSLCertFile            string          `mapstructure:"ssl-cert-file"`
	SSLKeyFile             string          `mapstructure:"ssl-key-file"`
	TFDownloadURL          string          `mapstructure:"tf-download-url"`
	TFEHostname            string          `mapstructure:"tfe-hostname"`
	TFEToken               string          `mapstructure:"tfe-token"`
	VCSStatusName          string          `mapstructure:"vcs-status-name"`
	DefaultTFVersion       string          `mapstructure:"default-tf-version"`
	Webhooks               []WebhookConfig `mapstructure:"webhooks"`
	WriteGitCreds          bool            `mapstructure:"write-git-creds"`
}

// ToLogLevel returns the LogLevel object corresponding to the user-passed
// log level.
func (u UserConfig) ToLogLevel() logging.LogLevel {
	switch u.LogLevel {
	case "debug":
		return logging.Debug
	case "info":
		return logging.Info
	case "warn":
		return logging.Warn
	case "error":
		return logging.Error
	}
	return logging.Info
}
