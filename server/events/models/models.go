// Copyright 2017 HootSuite Media Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// Modified hereafter by contributors to runatlantis/atlantis.
//
// Package models holds all models that are needed across packages.
// We place these models in their own package so as to avoid circular
// dependencies between packages (which is a compile error).
package models

import (
	"fmt"
	"net/url"
	paths "path"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/runatlantis/atlantis/server/logging"

	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
)

// Repo is a VCS repository.
type Repo struct {
	// FullName is the owner and repo name separated
	// by a "/", ex. "runatlantis/atlantis", "gitlab/subgroup/atlantis",
	// "Bitbucket Server/atlantis", "azuredevops/project/atlantis".
	FullName string
	// Owner is just the repo owner, ex. "runatlantis" or "gitlab/subgroup"
	// or azuredevops/project. This may contain /'s in the case of GitLab
	// subgroups or Azure DevOps Team Projects. This may contain spaces in
	// the case of Bitbucket Server.
	Owner string
	// Name is just the repo name, ex. "atlantis". This will never have
	// /'s in it.
	Name string
	// CloneURL is the full HTTPS url for cloning with username and token string
	// ex. "https://username:token@github.com/atlantis/atlantis.git".
	CloneURL string
	// SanitizedCloneURL is the full HTTPS url for cloning with the password
	// redacted.
	// ex. "https://user:<redacted>@github.com/atlantis/atlantis.git".
	SanitizedCloneURL string
	// VCSHost is where this repo is hosted.
	VCSHost VCSHost
}

// ID returns the atlantis ID for this repo.
// ID is in the form: {vcs hostname}/{repoFullName}.
func (r Repo) ID() string {
	return fmt.Sprintf("%s/%s", r.VCSHost.Hostname, r.FullName)
}

// NewRepo constructs a Repo object. repoFullName is the owner/repo form,
// cloneURL can be with or without .git at the end
// ex. https://github.com/runatlantis/atlantis.git OR
//     https://github.com/runatlantis/atlantis
func NewRepo(vcsHostType VCSHostType, repoFullName string, cloneURL string, vcsUser string, vcsToken string) (Repo, error) {
	if repoFullName == "" {
		return Repo{}, errors.New("repoFullName can't be empty")
	}
	if cloneURL == "" {
		return Repo{}, errors.New("cloneURL can't be empty")
	}

	// Azure DevOps doesn't work with .git suffix on clone URLs
	if !strings.HasSuffix(cloneURL, ".git") && vcsHostType != AzureDevops {
		cloneURL += ".git"
	}

	cloneURLParsed, err := url.Parse(cloneURL)
	if err != nil {
		return Repo{}, errors.Wrap(err, "invalid clone url")
	}

	// Ensure the Clone URL is for the same repo to avoid something malicious.
	// We skip this check for Bitbucket Server because its format is different
	// and because the caller in that case actually constructs the clone url
	// from the repo name and so there's no point checking if they match.
	// Azure DevOps also does not require .git at the end of clone urls.
	if vcsHostType != BitbucketServer && vcsHostType != AzureDevops {
		expClonePath := fmt.Sprintf("/%s.git", repoFullName)
		if expClonePath != cloneURLParsed.Path {
			return Repo{}, fmt.Errorf("expected clone url to have path %q but had %q", expClonePath, cloneURLParsed.Path)
		}
	}

	// We url encode because we're using them in a URL and weird characters can
	// mess up git.
	cloneURL = strings.Replace(cloneURL, " ", "%20", -1)
	escapedVCSUser := url.QueryEscape(vcsUser)
	escapedVCSToken := url.QueryEscape(vcsToken)
	auth := fmt.Sprintf("%s:%s@", escapedVCSUser, escapedVCSToken)
	redactedAuth := fmt.Sprintf("%s:<redacted>@", escapedVCSUser)

	// Construct clone urls with http and https auth. Need to do both
	// because Bitbucket supports http.
	authedCloneURL := strings.Replace(cloneURL, "https://", "https://"+auth, -1)
	authedCloneURL = strings.Replace(authedCloneURL, "http://", "http://"+auth, -1)
	sanitizedCloneURL := strings.Replace(cloneURL, "https://", "https://"+redactedAuth, -1)
	sanitizedCloneURL = strings.Replace(sanitizedCloneURL, "http://", "http://"+redactedAuth, -1)

	// Get the owner and repo names from the full name.
	owner, repo := SplitRepoFullName(repoFullName)
	if owner == "" || repo == "" {
		return Repo{}, fmt.Errorf("invalid repo format %q, owner %q or repo %q was empty", repoFullName, owner, repo)
	}
	// Only GitLab and AzureDevops repos can have /'s in their owners.
	// This is for GitLab subgroups and Azure DevOps Team Projects.
	if strings.Contains(owner, "/") && vcsHostType != Gitlab && vcsHostType != AzureDevops {
		return Repo{}, fmt.Errorf("invalid repo format %q, owner %q should not contain any /'s", repoFullName, owner)
	}
	if strings.Contains(repo, "/") {
		return Repo{}, fmt.Errorf("invalid repo format %q, repo %q should not contain any /'s", repoFullName, owner)
	}

	return Repo{
		FullName:          repoFullName,
		Owner:             owner,
		Name:              repo,
		CloneURL:          authedCloneURL,
		SanitizedCloneURL: sanitizedCloneURL,
		VCSHost: VCSHost{
			Type:     vcsHostType,
			Hostname: cloneURLParsed.Hostname(),
		},
	}, nil
}

// PullRequest is a VCS pull request.
// GitLab calls these Merge Requests.
type PullRequest struct {
	// Num is the pull request number or ID.
	Num int
	// HeadCommit is a sha256 that points to the head of the branch that is being
	// pull requested into the base. If the pull request is from Bitbucket Cloud
	// the string will only be 12 characters long because Bitbucket Cloud
	// truncates its commit IDs.
	HeadCommit string
	// URL is the url of the pull request.
	// ex. "https://github.com/runatlantis/atlantis/pull/1"
	URL string
	// HeadBranch is the name of the head branch (the branch that is getting
	// merged into the base).
	HeadBranch string
	// BaseBranch is the name of the base branch (the branch that the pull
	// request is getting merged into).
	BaseBranch string
	// Author is the username of the pull request author.
	Author string
	// State will be one of Open or Closed.
	// Gitlab supports an additional "merged" state but Github doesn't so we map
	// merged to Closed.
	State PullRequestState
	// BaseRepo is the repository that the pull request will be merged into.
	BaseRepo Repo
}

type PullRequestState int

const (
	OpenPullState PullRequestState = iota
	ClosedPullState
)

type PullRequestEventType int

const (
	OpenedPullEvent PullRequestEventType = iota
	UpdatedPullEvent
	ClosedPullEvent
	OtherPullEvent
)

func (p PullRequestEventType) String() string {
	switch p {
	case OpenedPullEvent:
		return "opened"
	case UpdatedPullEvent:
		return "updated"
	case ClosedPullEvent:
		return "closed"
	case OtherPullEvent:
		return "other"
	}
	return "<missing String() implementation>"
}

// User is a VCS user.
// During an autoplan, the user will be the Atlantis API user.
type User struct {
	Username string
}

// ProjectLock represents a lock on a project.
type ProjectLock struct {
	// Project is the project that is being locked.
	Project Project
	// Pull is the pull request from which the command was run that
	// created this lock.
	Pull PullRequest
	// User is the username of the user that ran the command
	// that created this lock.
	User User
	// Workspace is the Terraform workspace that this
	// lock is being held against.
	Workspace string
	// Time is the time at which the lock was first created.
	Time time.Time
}

// Project represents a Terraform project. Since there may be multiple
// Terraform projects in a single repo we also include Path to the project
// root relative to the repo root.
type Project struct {
	// RepoFullName is the owner and repo name, ex. "runatlantis/atlantis"
	RepoFullName string
	// Path to project root in the repo.
	// If "." then project is at root.
	// Never ends in "/".
	// todo: rename to RepoRelDir to match rest of project once we can separate
	// out how this is saved in boltdb vs. its usage everywhere else so we don't
	// break existing dbs.
	Path string
}

func (p Project) String() string {
	return fmt.Sprintf("repofullname=%s path=%s", p.RepoFullName, p.Path)
}

// Plan is the result of running an Atlantis plan command.
// This model is used to represent a plan on disk.
type Plan struct {
	// Project is the project this plan is for.
	Project Project
	// LocalPath is the absolute path to the plan on disk
	// (versus the relative path from the repo root).
	LocalPath string
}

// NewProject constructs a Project. Use this constructor because it
// sets Path correctly.
func NewProject(repoFullName string, path string) Project {
	path = paths.Clean(path)
	if path == "/" {
		path = "."
	}
	return Project{
		RepoFullName: repoFullName,
		Path:         path,
	}
}

// VCSHost is a Git hosting provider, for example GitHub.
type VCSHost struct {
	// Hostname is the hostname of the VCS provider, ex. "github.com" or
	// "github-enterprise.example.com".
	Hostname string

	// Type is which type of VCS host this is, ex. GitHub or GitLab.
	Type VCSHostType
}

type VCSHostType int

const (
	Github VCSHostType = iota
	Gitlab
	BitbucketCloud
	BitbucketServer
	AzureDevops
)

func (h VCSHostType) String() string {
	switch h {
	case Github:
		return "Github"
	case Gitlab:
		return "Gitlab"
	case BitbucketCloud:
		return "BitbucketCloud"
	case BitbucketServer:
		return "BitbucketServer"
	case AzureDevops:
		return "AzureDevops"
	}
	return "<missing String() implementation>"
}

// ProjectCommandContext defines the context for a plan or apply stage that will
// be executed for a project.
type ProjectCommandContext struct {
	// ApplyCmd is the command that users should run to apply this plan. If
	// this is an apply then this will be empty.
	ApplyCmd string
	// ApplyRequirements is the list of requirements that must be satisfied
	// before we will run the apply stage.
	ApplyRequirements []string
	// AutomergeEnabled is true if automerge is enabled for the repo that this
	// project is in.
	AutomergeEnabled bool
	// ParallelApplyEnabled is true if parallel apply is enabled for this project.
	ParallelApplyEnabled bool
	// ParallelPlanEnabled is true if parallel plan is enabled for this project.
	ParallelPlanEnabled bool
	// AutoplanEnabled is true if autoplanning is enabled for this project.
	AutoplanEnabled bool
	// BaseRepo is the repository that the pull request will be merged into.
	BaseRepo Repo
	// EscapedCommentArgs are the extra arguments that were added to the atlantis
	// command, ex. atlantis plan -- -target=resource. We then escape them
	// by adding a \ before each character so that they can be used within
	// sh -c safely, i.e. sh -c "terraform plan $(touch bad)".
	EscapedCommentArgs []string
	// HeadRepo is the repository that is getting merged into the BaseRepo.
	// If the pull request branch is from the same repository then HeadRepo will
	// be the same as BaseRepo.
	HeadRepo Repo
	// Log is a logger that's been set up for this context.
	Log *logging.SimpleLogger
	// PullMergeable is true if the pull request for this project is able to be merged.
	PullMergeable bool
	// Pull is the pull request we're responding to.
	Pull PullRequest
	// ProjectName is the name of the project set in atlantis.yaml. If there was
	// no name this will be an empty string.
	ProjectName string
	// RepoConfigVersion is the version of the repo's atlantis.yaml file. If
	// there was no file, this will be 0.
	RepoConfigVersion int
	// RePlanCmd is the command that users should run to re-plan this project.
	// If this is an apply then this will be empty.
	RePlanCmd string
	// RepoRelDir is the directory of this project relative to the repo root.
	RepoRelDir string
	// Steps are the sequence of commands we need to run for this project and this
	// stage.
	Steps []valid.Step
	// TerraformVersion is the version of terraform we should use when executing
	// commands for this project. This can be set to nil in which case we will
	// use the default Atlantis terraform version.
	TerraformVersion *version.Version
	// User is the user that triggered this command.
	User User
	// Verbose is true when the user would like verbose output.
	Verbose bool
	// Workspace is the Terraform workspace this project is in. It will always
	// be set.
	Workspace string
}

// The next two functions are kind of hacks. In a few different places
// we need to get a unique identifier for the current pull request as
// a single string (and it has to be constructed the same way each
// time). Same for getting a string that describes the current
// Atlantis workspace (or "project", which I think is the technically
// correct term). The easy way is to put this shared code as methods
// on the ProjectCommandContext struct, so that it's easy to get the
// relevant strings from anywhere. There might be a more appropriate
// way to do it; I haven't looked very hard.

func (c *ProjectCommandContext) PullSlug() string {
	return fmt.Sprintf("%s/%d", c.BaseRepo.FullName, c.Pull.Num)
}

// ProjectDesc returns a string that describes the current project in
// human-readable terms, as concisely as possible.
func (c *ProjectCommandContext) ProjectDesc() string {
	// If project name is set, it's supposed to be a unique
	// string, we'll just use that.
	if c.ProjectName != "" {
		return c.ProjectName
	}
	// Otherwise we want to report the Terraform working directory
	// which is the primary disambiguator.
	desc := c.RepoRelDir
	// Only if the configuration has specified custom workspaces
	// do we want to tack those on (because multiple projects
	// might then use the same working directory).
	if c.Workspace != "default" {
		desc += " (" + c.Workspace + ")"
	}
	return desc
}

// SplitRepoFullName splits a repo full name up into its owner and repo
// name segments. If the repoFullName is malformed, may return empty
// strings for owner or repo.
// Ex. runatlantis/atlantis => (runatlantis, atlantis)
//     gitlab/subgroup/runatlantis/atlantis => (gitlab/subgroup/runatlantis, atlantis)
//     azuredevops/project/atlantis => (azuredevops/project, atlantis)
func SplitRepoFullName(repoFullName string) (owner string, repo string) {
	lastSlashIdx := strings.LastIndex(repoFullName, "/")
	if lastSlashIdx == -1 || lastSlashIdx == len(repoFullName)-1 {
		return "", ""
	}

	return repoFullName[:lastSlashIdx], repoFullName[lastSlashIdx+1:]
}

// ProjectResult is the result of executing a plan/apply for a specific project.
type ProjectResult struct {
	Command      CommandName
	RepoRelDir   string
	Workspace    string
	Error        error
	Failure      string
	PlanSuccess  *PlanSuccess
	ApplySuccess string
	ProjectName  string
}

// CommitStatus returns the vcs commit status of this project result.
func (p ProjectResult) CommitStatus() CommitStatus {
	if p.Error != nil {
		return FailedCommitStatus
	}
	if p.Failure != "" {
		return FailedCommitStatus
	}
	return SuccessCommitStatus
}

// PlanStatus returns the plan status.
func (p ProjectResult) PlanStatus() ProjectPlanStatus {
	switch p.Command {

	case PlanCommand:
		if p.Error != nil {
			return ErroredPlanStatus
		} else if p.Failure != "" {
			return ErroredPlanStatus
		}
		return PlannedPlanStatus

	case ApplyCommand:
		if p.Error != nil {
			return ErroredApplyStatus
		} else if p.Failure != "" {
			return ErroredApplyStatus
		}
		return AppliedPlanStatus
	}

	panic("PlanStatus() missing a combination")
}

// IsSuccessful returns true if this project result had no errors.
func (p ProjectResult) IsSuccessful() bool {
	return p.PlanSuccess != nil || p.ApplySuccess != ""
}

// PlanSuccess is the result of a successful plan.
type PlanSuccess struct {
	// TerraformOutput is the output from Terraform of running plan.
	TerraformOutput string
	// LockURL is the full URL to the lock held by this plan.
	LockURL string
	// RePlanCmd is the command that users should run to re-plan this project.
	RePlanCmd string
	// ApplyCmd is the command that users should run to apply this plan.
	ApplyCmd string
	// HasDiverged is true if we're using the checkout merge strategy and the
	// branch we're merging into has been updated since we cloned and merged
	// it.
	HasDiverged bool
}

// PullStatus is the current status of a pull request that is in progress.
type PullStatus struct {
	// Projects are the projects that have been modified in this pull request.
	Projects []ProjectStatus
	// Pull is the original pull request model.
	Pull PullRequest
}

// StatusCount returns the number of projects that have status.
func (p PullStatus) StatusCount(status ProjectPlanStatus) int {
	c := 0
	for _, pr := range p.Projects {
		if pr.Status == status {
			c++
		}
	}
	return c
}

// ProjectStatus is the status of a specific project.
type ProjectStatus struct {
	Workspace   string
	RepoRelDir  string
	ProjectName string
	// Status is the status of where this project is at in the planning cycle.
	Status ProjectPlanStatus
}

// ProjectPlanStatus is the status of where this project is at in the planning
// cycle.
type ProjectPlanStatus int

const (
	// ErroredPlanStatus means that this plan has an error or the apply has an
	// error.
	ErroredPlanStatus ProjectPlanStatus = iota
	// PlannedPlanStatus means that a plan has been successfully generated but
	// not yet applied.
	PlannedPlanStatus
	// ErroredApplyStatus means that a plan has been generated but there was an
	// error while applying it.
	ErroredApplyStatus
	// AppliedPlanStatus means that a plan has been generated and applied
	// successfully.
	AppliedPlanStatus
	// DiscardedPlanStatus means that there was an unapplied plan that was
	// discarded due to a project being unlocked
	DiscardedPlanStatus
)

// String returns a string representation of the status.
func (p ProjectPlanStatus) String() string {
	switch p {
	case ErroredPlanStatus:
		return "plan_errored"
	case PlannedPlanStatus:
		return "planned"
	case ErroredApplyStatus:
		return "apply_errored"
	case AppliedPlanStatus:
		return "applied"
	case DiscardedPlanStatus:
		return "plan_discarded"
	default:
		panic("missing String() impl for ProjectPlanStatus")
	}
}

// CommandName is which command to run.
type CommandName int

const (
	// ApplyCommand is a command to run terraform apply.
	ApplyCommand CommandName = iota
	// PlanCommand is a command to run terraform plan.
	PlanCommand
	// UnlockCommand is a command to discard previous plans as well as the atlantis locks.
	UnlockCommand
	// Adding more? Don't forget to update String() below
)

// String returns the string representation of c.
func (c CommandName) String() string {
	switch c {
	case ApplyCommand:
		return "apply"
	case PlanCommand:
		return "plan"
	case UnlockCommand:
		return "unlock"
	}
	return ""
}

type TerraformOutputLine struct {
	// pull request that generated this output line
	PullSlug string

	// thing to print in console on web interface, no trailing newline
	Line string

	// true means we should clear the buffer of accumulated log
	// lines before (resp. after) logging the given line, so that
	// new clients connecting to the web interface don't see them
	ClearBefore bool
	ClearAfter  bool
}
