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

package events

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/google/go-github/v31/github"
	"github.com/mcdafydd/go-azuredevops/azuredevops"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs/bitbucketcloud"
	"github.com/runatlantis/atlantis/server/events/vcs/bitbucketserver"
	"github.com/xanzy/go-gitlab"
	"gopkg.in/go-playground/validator.v9"
)

const gitlabPullOpened = "opened"
const usagesCols = 90

// PullCommand is a command to run on a pull request.
type PullCommand interface {
	// CommandName is the name of the command we're running.
	CommandName() models.CommandName
	// IsVerbose is true if the output of this command should be verbose.
	IsVerbose() bool
	// IsAutoplan is true if this is an autoplan command vs. a comment command.
	IsAutoplan() bool
}

// AutoplanCommand is a plan command that is automatically triggered when a
// pull request is opened or updated.
type AutoplanCommand struct{}

// CommandName is Plan.
func (c AutoplanCommand) CommandName() models.CommandName {
	return models.PlanCommand
}

// IsVerbose is false for autoplan commands.
func (c AutoplanCommand) IsVerbose() bool {
	return false
}

// IsAutoplan is true for autoplan commands (obviously).
func (c AutoplanCommand) IsAutoplan() bool {
	return true
}

// CommentCommand is a command that was triggered by a pull request comment.
type CommentCommand struct {
	// RepoRelDir is the path relative to the repo root to run the command in.
	// Will never end in "/". If empty then the comment specified no directory.
	RepoRelDir string
	// Flags are the extra arguments appended to the comment,
	// ex. atlantis plan -- -target=resource
	Flags []string
	// Name is the name of the command the comment specified.
	Name models.CommandName
	// Verbose is true if the command should output verbosely.
	Verbose bool
	// Workspace is the name of the Terraform workspace to run the command in.
	// If empty then the comment specified no workspace.
	Workspace string
	// ProjectName is the name of a project to run the command on. It refers to a
	// project specified in an atlantis.yaml file.
	// If empty then the comment specified no project.
	ProjectName string
}

// IsForSpecificProject returns true if the command is for a specific dir, workspace
// or project name. Otherwise it's a command like "atlantis plan" or "atlantis
// apply".
func (c CommentCommand) IsForSpecificProject() bool {
	return c.RepoRelDir != "" || c.Workspace != "" || c.ProjectName != ""
}

// CommandName returns the name of this command.
func (c CommentCommand) CommandName() models.CommandName {
	return c.Name
}

// IsVerbose is true if the command should give verbose output.
func (c CommentCommand) IsVerbose() bool {
	return c.Verbose
}

// IsAutoplan will be false for comment commands.
func (c CommentCommand) IsAutoplan() bool {
	return false
}

// String returns a string representation of the command.
func (c CommentCommand) String() string {
	return fmt.Sprintf("command=%q verbose=%t dir=%q workspace=%q project=%q flags=%q", c.Name.String(), c.Verbose, c.RepoRelDir, c.Workspace, c.ProjectName, strings.Join(c.Flags, ","))
}

// NewCommentCommand constructs a CommentCommand, setting all missing fields to defaults.
func NewCommentCommand(repoRelDir string, flags []string, name models.CommandName, verbose bool, workspace string, project string) *CommentCommand {
	// If repoRelDir was empty we want to keep it that way to indicate that it
	// wasn't specified in the comment.
	if repoRelDir != "" {
		repoRelDir = path.Clean(repoRelDir)
		if repoRelDir == "/" {
			repoRelDir = "."
		}
	}
	return &CommentCommand{
		RepoRelDir:  repoRelDir,
		Flags:       flags,
		Name:        name,
		Verbose:     verbose,
		Workspace:   workspace,
		ProjectName: project,
	}
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_event_parsing.go EventParsing

// EventParsing parses webhook events from different VCS hosts into their
// respective Atlantis models.
// todo: rename to VCSParsing or the like because this also parses API responses #refactor
type EventParsing interface {
	// ParseGithubIssueCommentEvent parses GitHub pull request comment events.
	// baseRepo is the repo that the pull request will be merged into.
	// user is the pull request author.
	// pullNum is the number of the pull request that triggered the webhook.
	ParseGithubIssueCommentEvent(comment *github.IssueCommentEvent) (
		baseRepo models.Repo, user models.User, pullNum int, err error)

	// ParseGithubPull parses the response from the GitHub API endpoint (not
	// from a webhook) that returns a pull request.
	// pull is the parsed pull request.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	ParseGithubPull(ghPull *github.PullRequest) (
		pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, err error)

	// ParseGithubPullEvent parses GitHub pull request events.
	// pull is the parsed pull request.
	// pullEventType is the type of event, for example opened/closed.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	// user is the pull request author.
	ParseGithubPullEvent(pullEvent *github.PullRequestEvent) (
		pull models.PullRequest, pullEventType models.PullRequestEventType,
		baseRepo models.Repo, headRepo models.Repo, user models.User, err error)

	// ParseGithubRepo parses the response from the GitHub API endpoint that
	// returns a repo into the Atlantis model.
	ParseGithubRepo(ghRepo *github.Repository) (models.Repo, error)

	// ParseGitlabMergeRequestEvent parses GitLab merge request events.
	// pull is the parsed merge request.
	// pullEventType is the type of event, for example opened/closed.
	// baseRepo is the repo the merge request will be merged into.
	// headRepo is the repo the merge request branch is from.
	// user is the pull request author.
	ParseGitlabMergeRequestEvent(event gitlab.MergeEvent) (
		pull models.PullRequest, pullEventType models.PullRequestEventType,
		baseRepo models.Repo, headRepo models.Repo, user models.User, err error)

	// ParseGitlabMergeRequestCommentEvent parses GitLab merge request comment
	// events.
	// baseRepo is the repo the merge request will be merged into.
	// headRepo is the repo the merge request branch is from.
	// user is the pull request author.
	ParseGitlabMergeRequestCommentEvent(event gitlab.MergeCommentEvent) (
		baseRepo models.Repo, headRepo models.Repo, user models.User, err error)

	// ParseGitlabMergeRequest parses the response from the GitLab API endpoint
	// that returns a merge request.
	ParseGitlabMergeRequest(mr *gitlab.MergeRequest, baseRepo models.Repo) models.PullRequest

	// ParseBitbucketCloudPullEvent parses a pull request event from Bitbucket
	// Cloud (bitbucket.org).
	// pull is the parsed pull request.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	// user is the pull request author.
	ParseBitbucketCloudPullEvent(body []byte) (
		pull models.PullRequest, baseRepo models.Repo,
		headRepo models.Repo, user models.User, err error)

	// ParseBitbucketCloudPullCommentEvent parses a pull request comment event
	// from Bitbucket Cloud (bitbucket.org).
	// pull is the parsed pull request.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	// user is the pull request author.
	// comment is the comment that triggered the event.
	ParseBitbucketCloudPullCommentEvent(body []byte) (
		pull models.PullRequest, baseRepo models.Repo,
		headRepo models.Repo, user models.User, comment string, err error)

	// GetBitbucketCloudPullEventType returns the type of the pull request
	// event given the Bitbucket Cloud header.
	GetBitbucketCloudPullEventType(eventTypeHeader string) models.PullRequestEventType

	// ParseBitbucketServerPullEvent parses a pull request event from Bitbucket
	// Server.
	// pull is the parsed pull request.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	// user is the pull request author.
	ParseBitbucketServerPullEvent(body []byte) (
		pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo,
		user models.User, err error)

	// ParseBitbucketServerPullCommentEvent parses a pull request comment event
	// from Bitbucket Server.
	// pull is the parsed pull request.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	// user is the pull request author.
	// comment is the comment that triggered the event.
	ParseBitbucketServerPullCommentEvent(body []byte) (
		pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo,
		user models.User, comment string, err error)

	// GetBitbucketServerPullEventType returns the type of the pull request
	// event given the Bitbucket Server header.
	GetBitbucketServerPullEventType(eventTypeHeader string) models.PullRequestEventType

	// ParseAzureDevopsPull parses the response from the Azure DevOps API endpoint (not
	// from a webhook) that returns a pull request.
	// pull is the parsed pull request.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	ParseAzureDevopsPull(adPull *azuredevops.GitPullRequest) (
		pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, err error)

	// ParseAzureDevopsPullEvent parses Azure DevOps pull request events.
	// pull is the parsed pull request.
	// pullEventType is the type of event, for example opened/closed.
	// baseRepo is the repo the pull request will be merged into.
	// headRepo is the repo the pull request branch is from.
	// user is the pull request author.
	ParseAzureDevopsPullEvent(pullEvent azuredevops.Event) (
		pull models.PullRequest, pullEventType models.PullRequestEventType,
		baseRepo models.Repo, headRepo models.Repo, user models.User, err error)

	// ParseAzureDevopsRepo parses the response from the Azure DevOps API endpoint that
	// returns a repo into the Atlantis model.
	ParseAzureDevopsRepo(adRepo *azuredevops.GitRepository) (models.Repo, error)
}

// EventParser parses VCS events.
type EventParser struct {
	GithubUser         string
	GithubToken        string
	GitlabUser         string
	GitlabToken        string
	AllowDraftPRs      bool
	BitbucketUser      string
	BitbucketToken     string
	BitbucketServerURL string
	AzureDevopsToken   string
	AzureDevopsUser    string
}

// GetBitbucketCloudPullEventType returns the type of the pull request
// event given the Bitbucket Cloud header.
func (e *EventParser) GetBitbucketCloudPullEventType(eventTypeHeader string) models.PullRequestEventType {
	switch eventTypeHeader {
	case bitbucketcloud.PullCreatedHeader:
		return models.OpenedPullEvent
	case bitbucketcloud.PullUpdatedHeader:
		return models.UpdatedPullEvent
	case bitbucketcloud.PullFulfilledHeader, bitbucketcloud.PullRejectedHeader:
		return models.ClosedPullEvent
	}
	return models.OtherPullEvent
}

// ParseBitbucketCloudPullCommentEvent parses a pull request comment event
// from Bitbucket Cloud (bitbucket.org).
// See EventParsing for return value docs.
func (e *EventParser) ParseBitbucketCloudPullCommentEvent(body []byte) (pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, user models.User, comment string, err error) {
	var event bitbucketcloud.CommentEvent
	if err = json.Unmarshal(body, &event); err != nil {
		err = errors.Wrap(err, "parsing json")
		return
	}
	if err = validator.New().Struct(event); err != nil {
		err = errors.Wrapf(err, "API response %q was missing fields", string(body))
		return
	}
	pull, baseRepo, headRepo, user, err = e.parseCommonBitbucketCloudEventData(event.CommonEventData)
	comment = *event.Comment.Content.Raw
	return
}

func (e *EventParser) parseCommonBitbucketCloudEventData(event bitbucketcloud.CommonEventData) (pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	var prState models.PullRequestState
	switch *event.PullRequest.State {
	case "OPEN":
		prState = models.OpenPullState
	case "MERGED":
		prState = models.ClosedPullState
	case "SUPERSEDED":
		prState = models.ClosedPullState
	case "DECLINED":
		prState = models.ClosedPullState
	default:
		err = fmt.Errorf("unable to determine pull request state from %q–this is a bug", *event.PullRequest.State)
		return
	}

	headRepo, err = models.NewRepo(
		models.BitbucketCloud,
		*event.PullRequest.Source.Repository.FullName,
		*event.PullRequest.Source.Repository.Links.HTML.HREF,
		e.BitbucketUser,
		e.BitbucketToken)
	if err != nil {
		return
	}
	baseRepo, err = models.NewRepo(
		models.BitbucketCloud,
		*event.Repository.FullName,
		*event.Repository.Links.HTML.HREF,
		e.BitbucketUser,
		e.BitbucketToken)
	if err != nil {
		return
	}

	pull = models.PullRequest{
		Num:        *event.PullRequest.ID,
		HeadCommit: *event.PullRequest.Source.Commit.Hash,
		URL:        *event.PullRequest.Links.HTML.HREF,
		HeadBranch: *event.PullRequest.Source.Branch.Name,
		BaseBranch: *event.PullRequest.Destination.Branch.Name,
		Author:     *event.Actor.Nickname,
		State:      prState,
		BaseRepo:   baseRepo,
	}
	user = models.User{
		Username: *event.Actor.Nickname,
	}
	return
}

// ParseBitbucketCloudPullEvent parses a pull request event from Bitbucket
// Cloud (bitbucket.org).
// See EventParsing for return value docs.
func (e *EventParser) ParseBitbucketCloudPullEvent(body []byte) (pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	var event bitbucketcloud.PullRequestEvent
	if err = json.Unmarshal(body, &event); err != nil {
		err = errors.Wrap(err, "parsing json")
		return
	}
	if err = validator.New().Struct(event); err != nil {
		err = errors.Wrapf(err, "API response %q was missing fields", string(body))
		return
	}
	pull, baseRepo, headRepo, user, err = e.parseCommonBitbucketCloudEventData(event.CommonEventData)
	return
}

// ParseGithubIssueCommentEvent parses GitHub pull request comment events.
// See EventParsing for return value docs.
func (e *EventParser) ParseGithubIssueCommentEvent(comment *github.IssueCommentEvent) (baseRepo models.Repo, user models.User, pullNum int, err error) {
	baseRepo, err = e.ParseGithubRepo(comment.Repo)
	if err != nil {
		return
	}
	if comment.Comment == nil || comment.Comment.User.GetLogin() == "" {
		err = errors.New("comment.user.login is null")
		return
	}
	commenterUsername := comment.Comment.User.GetLogin()
	user = models.User{
		Username: commenterUsername,
	}
	pullNum = comment.Issue.GetNumber()
	if pullNum == 0 {
		err = errors.New("issue.number is null")
		return
	}
	return
}

// ParseGithubPullEvent parses GitHub pull request events.
// See EventParsing for return value docs.
func (e *EventParser) ParseGithubPullEvent(pullEvent *github.PullRequestEvent) (pull models.PullRequest, pullEventType models.PullRequestEventType, baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	if pullEvent.PullRequest == nil {
		err = errors.New("pull_request is null")
		return
	}
	pull, baseRepo, headRepo, err = e.ParseGithubPull(pullEvent.PullRequest)
	if err != nil {
		return
	}
	if pullEvent.Sender == nil {
		err = errors.New("sender is null")
		return
	}
	senderUsername := pullEvent.Sender.GetLogin()
	if senderUsername == "" {
		err = errors.New("sender.login is null")
		return
	}

	if pullEvent.GetPullRequest().GetDraft() {
		// Even if the PR is in draft state users can manually run plan or may
		// be using the -allow-draft-prs flag. If so then we need to ensure locks are cleaned up.
		if pullEvent.GetAction() == "closed" {
			pullEventType = models.ClosedPullEvent
		} else if !e.AllowDraftPRs {
			pullEventType = models.OtherPullEvent
		}
	} else {
		switch pullEvent.GetAction() {
		case "opened":
			pullEventType = models.OpenedPullEvent
		case "ready_for_review":
			// when an author takes a PR out of 'draft' state a 'ready_for_review'
			// event is triggered. We want atlantis to treat this as a freshly opened PR
			pullEventType = models.OpenedPullEvent
		case "synchronize":
			pullEventType = models.UpdatedPullEvent
		case "closed":
			pullEventType = models.ClosedPullEvent
		default:
			pullEventType = models.OtherPullEvent
		}
	}
	user = models.User{Username: senderUsername}
	return
}

// ParseGithubPull parses the response from the GitHub API endpoint (not
// from a webhook) that returns a pull request.
// See EventParsing for return value docs.
func (e *EventParser) ParseGithubPull(pull *github.PullRequest) (pullModel models.PullRequest, baseRepo models.Repo, headRepo models.Repo, err error) {
	commit := pull.Head.GetSHA()
	if commit == "" {
		err = errors.New("head.sha is null")
		return
	}
	url := pull.GetHTMLURL()
	if url == "" {
		err = errors.New("html_url is null")
		return
	}
	headBranch := pull.Head.GetRef()
	if headBranch == "" {
		err = errors.New("head.ref is null")
		return
	}
	baseBranch := pull.Base.GetRef()
	if baseBranch == "" {
		err = errors.New("base.ref is null")
		return
	}

	authorUsername := pull.User.GetLogin()
	if authorUsername == "" {
		err = errors.New("user.login is null")
		return
	}
	num := pull.GetNumber()
	if num == 0 {
		err = errors.New("number is null")
		return
	}

	baseRepo, err = e.ParseGithubRepo(pull.Base.Repo)
	if err != nil {
		return
	}
	headRepo, err = e.ParseGithubRepo(pull.Head.Repo)
	if err != nil {
		return
	}

	pullState := models.ClosedPullState
	if pull.GetState() == "open" {
		pullState = models.OpenPullState
	}

	pullModel = models.PullRequest{
		Author:     authorUsername,
		HeadBranch: headBranch,
		HeadCommit: commit,
		URL:        url,
		Num:        num,
		State:      pullState,
		BaseRepo:   baseRepo,
		BaseBranch: baseBranch,
	}
	return
}

// ParseGithubRepo parses the response from the GitHub API endpoint that
// returns a repo into the Atlantis model.
// See EventParsing for return value docs.
func (e *EventParser) ParseGithubRepo(ghRepo *github.Repository) (models.Repo, error) {
	return models.NewRepo(models.Github, ghRepo.GetFullName(), ghRepo.GetCloneURL(), e.GithubUser, e.GithubToken)
}

// ParseGitlabMergeRequestEvent parses GitLab merge request events.
// pull is the parsed merge request.
// See EventParsing for return value docs.
func (e *EventParser) ParseGitlabMergeRequestEvent(event gitlab.MergeEvent) (pull models.PullRequest, eventType models.PullRequestEventType, baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	modelState := models.ClosedPullState
	if event.ObjectAttributes.State == gitlabPullOpened {
		modelState = models.OpenPullState
	}
	// GitLab also has a "merged" state, but we map that to Closed so we don't
	// need to check for it.

	baseRepo, err = models.NewRepo(models.Gitlab, event.Project.PathWithNamespace, event.Project.GitHTTPURL, e.GitlabUser, e.GitlabToken)
	if err != nil {
		return
	}
	headRepo, err = models.NewRepo(models.Gitlab, event.ObjectAttributes.Source.PathWithNamespace, event.ObjectAttributes.Source.GitHTTPURL, e.GitlabUser, e.GitlabToken)
	if err != nil {
		return
	}

	pull = models.PullRequest{
		URL:        event.ObjectAttributes.URL,
		Author:     event.User.Username,
		Num:        event.ObjectAttributes.IID,
		HeadCommit: event.ObjectAttributes.LastCommit.ID,
		HeadBranch: event.ObjectAttributes.SourceBranch,
		BaseBranch: event.ObjectAttributes.TargetBranch,
		State:      modelState,
		BaseRepo:   baseRepo,
	}

	switch event.ObjectAttributes.Action {
	case "open":
		eventType = models.OpenedPullEvent
	case "update":
		eventType = models.UpdatedPullEvent
	case "merge", "close":
		eventType = models.ClosedPullEvent
	default:
		eventType = models.OtherPullEvent
	}

	user = models.User{
		Username: event.User.Username,
	}

	return
}

// ParseGitlabMergeRequestCommentEvent parses GitLab merge request comment
// events.
// See EventParsing for return value docs.
func (e *EventParser) ParseGitlabMergeRequestCommentEvent(event gitlab.MergeCommentEvent) (baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	// Parse the base repo first.
	repoFullName := event.Project.PathWithNamespace
	cloneURL := event.Project.GitHTTPURL
	baseRepo, err = models.NewRepo(models.Gitlab, repoFullName, cloneURL, e.GitlabUser, e.GitlabToken)
	if err != nil {
		return
	}
	user = models.User{
		Username: event.User.Username,
	}

	// Now parse the head repo.
	headRepoFullName := event.MergeRequest.Source.PathWithNamespace
	headCloneURL := event.MergeRequest.Source.GitHTTPURL
	headRepo, err = models.NewRepo(models.Gitlab, headRepoFullName, headCloneURL, e.GitlabUser, e.GitlabToken)
	return
}

// ParseGitlabMergeRequest parses the merge requests and returns a pull request
// model. We require passing in baseRepo because we can't get this information
// from the merge request. The only caller of this function already has that
// data so we can construct the pull request object correctly.
func (e *EventParser) ParseGitlabMergeRequest(mr *gitlab.MergeRequest, baseRepo models.Repo) models.PullRequest {
	pullState := models.ClosedPullState
	if mr.State == gitlabPullOpened {
		pullState = models.OpenPullState
	}
	// GitLab also has a "merged" state, but we map that to Closed so we don't
	// need to check for it.

	return models.PullRequest{
		URL:        mr.WebURL,
		Author:     mr.Author.Username,
		Num:        mr.IID,
		HeadCommit: mr.SHA,
		HeadBranch: mr.SourceBranch,
		BaseBranch: mr.TargetBranch,
		State:      pullState,
		BaseRepo:   baseRepo,
	}
}

// GetBitbucketServerPullEventType returns the type of the pull request
// event given the Bitbucket Server header.
func (e *EventParser) GetBitbucketServerPullEventType(eventTypeHeader string) models.PullRequestEventType {
	switch eventTypeHeader {
	case bitbucketserver.PullCreatedHeader:
		return models.OpenedPullEvent
	case bitbucketserver.PullMergedHeader, bitbucketserver.PullDeclinedHeader, bitbucketserver.PullDeletedHeader:
		return models.ClosedPullEvent
	}
	return models.OtherPullEvent
}

// ParseBitbucketServerPullCommentEvent parses a pull request comment event
// from Bitbucket Server.
// See EventParsing for return value docs.
func (e *EventParser) ParseBitbucketServerPullCommentEvent(body []byte) (pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, user models.User, comment string, err error) {
	var event bitbucketserver.CommentEvent
	if err = json.Unmarshal(body, &event); err != nil {
		err = errors.Wrap(err, "parsing json")
		return
	}
	if err = validator.New().Struct(event); err != nil {
		err = errors.Wrapf(err, "API response %q was missing fields", string(body))
		return
	}
	pull, baseRepo, headRepo, user, err = e.parseCommonBitbucketServerEventData(event.CommonEventData)
	comment = *event.Comment.Text
	return
}

func (e *EventParser) parseCommonBitbucketServerEventData(event bitbucketserver.CommonEventData) (pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	var prState models.PullRequestState
	switch *event.PullRequest.State {
	case "OPEN":
		prState = models.OpenPullState
	case "MERGED":
		prState = models.ClosedPullState
	case "DECLINED":
		prState = models.ClosedPullState
	default:
		err = fmt.Errorf("unable to determine pull request state from %q–this is a bug", *event.PullRequest.State)
		return
	}

	headRepoSlug := *event.PullRequest.FromRef.Repository.Slug
	headRepoFullname := fmt.Sprintf("%s/%s", *event.PullRequest.FromRef.Repository.Project.Name, headRepoSlug)
	headRepoCloneURL := fmt.Sprintf("%s/scm/%s/%s.git", e.BitbucketServerURL, strings.ToLower(*event.PullRequest.FromRef.Repository.Project.Key), headRepoSlug)
	headRepo, err = models.NewRepo(
		models.BitbucketServer,
		headRepoFullname,
		headRepoCloneURL,
		e.BitbucketUser,
		e.BitbucketToken)
	if err != nil {
		return
	}

	baseRepoSlug := *event.PullRequest.ToRef.Repository.Slug
	baseRepoFullname := fmt.Sprintf("%s/%s", *event.PullRequest.ToRef.Repository.Project.Name, baseRepoSlug)
	baseRepoCloneURL := fmt.Sprintf("%s/scm/%s/%s.git", e.BitbucketServerURL, strings.ToLower(*event.PullRequest.ToRef.Repository.Project.Key), baseRepoSlug)
	baseRepo, err = models.NewRepo(
		models.BitbucketServer,
		baseRepoFullname,
		baseRepoCloneURL,
		e.BitbucketUser,
		e.BitbucketToken)
	if err != nil {
		return
	}

	pull = models.PullRequest{
		Num:        *event.PullRequest.ID,
		HeadCommit: *event.PullRequest.FromRef.LatestCommit,
		URL:        fmt.Sprintf("%s/projects/%s/repos/%s/pull-requests/%d", e.BitbucketServerURL, *event.PullRequest.ToRef.Repository.Project.Key, *event.PullRequest.ToRef.Repository.Slug, *event.PullRequest.ID),
		HeadBranch: *event.PullRequest.FromRef.DisplayID,
		BaseBranch: *event.PullRequest.ToRef.DisplayID,
		Author:     *event.Actor.Username,
		State:      prState,
		BaseRepo:   baseRepo,
	}
	user = models.User{
		Username: *event.Actor.Username,
	}
	return
}

// ParseBitbucketServerPullEvent parses a pull request event from Bitbucket
// Server.
// See EventParsing for return value docs.
func (e *EventParser) ParseBitbucketServerPullEvent(body []byte) (pull models.PullRequest, baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	var event bitbucketserver.PullRequestEvent
	if err = json.Unmarshal(body, &event); err != nil {
		err = errors.Wrap(err, "parsing json")
		return
	}
	if err = validator.New().Struct(event); err != nil {
		err = errors.Wrapf(err, "API response %q was missing fields", string(body))
		return
	}
	pull, baseRepo, headRepo, user, err = e.parseCommonBitbucketServerEventData(event.CommonEventData)
	return
}

// ParseAzureDevopsPullEvent parses Azure DevOps pull request events.
// See EventParsing for return value docs.
func (e *EventParser) ParseAzureDevopsPullEvent(event azuredevops.Event) (pull models.PullRequest, pullEventType models.PullRequestEventType, baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
	pullResource, ok := event.Resource.(*azuredevops.GitPullRequest)
	if !ok {
		err = errors.New("failed to type assert event.Resource")
		return
	}
	pull, baseRepo, headRepo, err = e.ParseAzureDevopsPull(pullResource)
	if err != nil {
		return
	}
	createdBy := pullResource.GetCreatedBy()
	if createdBy == nil {
		err = errors.New("CreatedBy is null")
		return
	}
	senderUsername := createdBy.GetUniqueName()
	if senderUsername == "" {
		err = errors.New("CreatedBy.UniqueName is null")
		return
	}
	switch event.EventType {
	case "git.pullrequest.created":
		pullEventType = models.OpenedPullEvent
	case "git.pullrequest.updated":
		pullEventType = models.UpdatedPullEvent
		if pull.State == models.ClosedPullState {
			pullEventType = models.ClosedPullEvent
		}
	default:
		pullEventType = models.OtherPullEvent
	}
	user = models.User{Username: senderUsername}
	return
}

// ParseAzureDevopsPull parses the response from the Azure DevOps API endpoint (not
// from a webhook) that returns a pull request.
// See EventParsing for return value docs.
func (e *EventParser) ParseAzureDevopsPull(pull *azuredevops.GitPullRequest) (pullModel models.PullRequest, baseRepo models.Repo, headRepo models.Repo, err error) {
	commit := pull.LastMergeSourceCommit.GetCommitID()
	if commit == "" {
		err = errors.New("lastMergeSourceCommit.commitID is null")
		return
	}
	url := pull.GetURL()
	if url == "" {
		err = errors.New("url is null")
		return
	}
	headBranch := pull.GetSourceRefName()
	if headBranch == "" {
		err = errors.New("sourceRefName (branch name) is null")
		return
	}
	baseBranch := pull.GetTargetRefName()
	if baseBranch == "" {
		err = errors.New("targetRefName (branch name) is null")
		return
	}
	num := pull.GetPullRequestID()
	if num == 0 {
		err = errors.New("pullRequestId is null")
		return
	}
	createdBy := pull.GetCreatedBy()
	if createdBy == nil {
		err = errors.New("CreatedBy is null")
		return
	}
	authorUsername := createdBy.GetUniqueName()
	if authorUsername == "" {
		err = errors.New("CreatedBy.UniqueName is null")
		return
	}
	baseRepo, err = e.ParseAzureDevopsRepo(pull.GetRepository())
	if err != nil {
		return
	}
	headRepo, err = e.ParseAzureDevopsRepo(pull.GetRepository())
	if err != nil {
		return
	}
	pullState := models.ClosedPullState
	if *pull.Status == azuredevops.PullActive.String() {
		pullState = models.OpenPullState
	}

	pullModel = models.PullRequest{
		Author: authorUsername,
		// Change webhook refs from "refs/heads/<branch>" to "<branch>"
		HeadBranch: strings.Replace(headBranch, "refs/heads/", "", 1),
		HeadCommit: commit,
		URL:        url,
		Num:        num,
		State:      pullState,
		BaseRepo:   baseRepo,
		BaseBranch: strings.Replace(baseBranch, "refs/heads/", "", 1),
	}
	return
}

// ParseAzureDevopsRepo parses the response from the Azure DevOps API endpoint that
// returns a repo into the Atlantis model.
// If the event payload doesn't contain a parent repository reference, extract the owner
// name from the URL. The URL will match one of two different formats:
//
// https://runatlantis.visualstudio.com/project/_git/repo
// https://dev.azure.com/runatlantis/project/_git/repo
//
// See EventParsing for return value docs.
func (e *EventParser) ParseAzureDevopsRepo(adRepo *azuredevops.GitRepository) (models.Repo, error) {
	teamProject := adRepo.GetProject()
	parent := adRepo.GetParentRepository()
	owner := ""
	if parent != nil {
		owner = parent.GetName()
	} else {
		uri, err := url.Parse(adRepo.GetWebURL())
		if err != nil {
			return models.Repo{}, err
		}
		if strings.Contains(uri.Host, "visualstudio.com") {
			owner = strings.Split(uri.Host, ".")[0]
		} else if strings.Contains(uri.Host, "dev.azure.com") {
			owner = strings.Split(uri.Path, "/")[1]
		} else {
			owner = ""
		}
	}

	// Construct our own clone URL so we always get the new dev.azure.com
	// hostname for now.
	// https://docs.microsoft.com/en-us/azure/devops/release-notes/2018/sep-10-azure-devops-launch#switch-existing-organizations-to-use-the-new-domain-name-url
	project := teamProject.GetName()
	repo := adRepo.GetName()
	cloneURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", owner, project, repo)
	fullName := fmt.Sprintf("%s/%s/%s", owner, project, repo)
	return models.NewRepo(models.AzureDevops, fullName, cloneURL, e.AzureDevopsUser, e.AzureDevopsToken)
}
