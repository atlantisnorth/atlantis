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
package events

import (
	"regexp"

	"github.com/google/go-github/github"
	"github.com/lkysow/go-gitlab"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
)

const gitlabPullOpened = "opened"
const usagesCols = 90

// multiLineRegex is used to ignore multi-line comments since those aren't valid
// Atlantis commands.
var multiLineRegex = regexp.MustCompile(`.*\r?\n.+`)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_event_parsing.go EventParsing

type Command struct {
	Name      CommandName
	Workspace string
	Verbose   bool
	Flags     []string
	// Dir is the path relative to the repo root to run the command in.
	// If empty string then it wasn't specified. "." is the root of the repo.
	// Dir will never end in "/".
	Dir string
}

type EventParsing interface {
	ParseGithubIssueCommentEvent(comment *github.IssueCommentEvent) (baseRepo models.Repo, user models.User, pullNum int, err error)
	ParseGithubPull(pull *github.PullRequest) (models.PullRequest, models.Repo, error)
	ParseGithubRepo(ghRepo *github.Repository) (models.Repo, error)
	ParseGitlabMergeEvent(event gitlab.MergeEvent) (models.PullRequest, models.Repo, error)
	ParseGitlabMergeCommentEvent(event gitlab.MergeCommentEvent) (baseRepo models.Repo, headRepo models.Repo, user models.User, err error)
	ParseGitlabMergeRequest(mr *gitlab.MergeRequest) models.PullRequest
}

type EventParser struct {
	GithubUser  string
	GithubToken string
	GitlabUser  string
	GitlabToken string
}

func (e *EventParser) ParseGithubIssueCommentEvent(comment *github.IssueCommentEvent) (baseRepo models.Repo, user models.User, pullNum int, err error) {
	baseRepo, err = e.ParseGithubRepo(comment.Repo)
	if err != nil {
		return
	}
	if comment.Comment == nil || comment.Comment.User.GetLogin() == "" {
		err = errors.New("comment.user.login is null")
		return
	}
	commentorUsername := comment.Comment.User.GetLogin()
	user = models.User{
		Username: commentorUsername,
	}
	pullNum = comment.Issue.GetNumber()
	if pullNum == 0 {
		err = errors.New("issue.number is null")
		return
	}
	return
}

func (e *EventParser) ParseGithubPull(pull *github.PullRequest) (models.PullRequest, models.Repo, error) {
	var pullModel models.PullRequest
	var headRepoModel, baseRepoModel models.Repo

	commit := pull.Head.GetSHA()
	if commit == "" {
		return pullModel, headRepoModel, errors.New("head.sha is null")
	}
	url := pull.GetHTMLURL()
	if url == "" {
		return pullModel, headRepoModel, errors.New("html_url is null")
	}
	branch := pull.Head.GetRef()
	if branch == "" {
		return pullModel, headRepoModel, errors.New("head.ref is null")
	}
	authorUsername := pull.User.GetLogin()
	if authorUsername == "" {
		return pullModel, headRepoModel, errors.New("user.login is null")
	}
	num := pull.GetNumber()
	if num == 0 {
		return pullModel, headRepoModel, errors.New("number is null")
	}

	headRepoModel, err := e.ParseGithubRepo(pull.Head.Repo)
	if err != nil {
		return pullModel, headRepoModel, err
	}

	baseRepoModel, err = e.ParseGithubRepo(pull.Base.Repo)
	if err != nil {
		return pullModel, headRepoModel, err
	}

	pullState := models.Closed
	if pull.GetState() == "open" {
		pullState = models.Open
	}

	return models.PullRequest{
		Author:     authorUsername,
		Branch:     branch,
		HeadCommit: commit,
		URL:        url,
		Num:        num,
		State:      pullState,
		HeadRepo:   headRepoModel,
		BaseRepo:   baseRepoModel,
	}, headRepoModel, nil
}

func (e *EventParser) ParseGithubRepo(ghRepo *github.Repository) (models.Repo, error) {
	return models.NewRepo(models.Github, ghRepo.GetFullName(), ghRepo.GetCloneURL(), e.GithubUser, e.GithubToken)
}

func (e *EventParser) ParseGitlabMergeEvent(event gitlab.MergeEvent) (models.PullRequest, models.Repo, error) {
	modelState := models.Closed
	if event.ObjectAttributes.State == gitlabPullOpened {
		modelState = models.Open
	}
	// GitLab also has a "merged" state, but we map that to Closed so we don't
	// need to check for it.

	pull := models.PullRequest{
		URL:        event.ObjectAttributes.URL,
		Author:     event.User.Username,
		Num:        event.ObjectAttributes.IID,
		HeadCommit: event.ObjectAttributes.LastCommit.ID,
		Branch:     event.ObjectAttributes.SourceBranch,
		State:      modelState,
	}

	repo, err := models.NewRepo(models.Gitlab, event.Project.PathWithNamespace, event.Project.GitHTTPURL, e.GitlabUser, e.GitlabToken)
	return pull, repo, err
}

// ParseGitlabMergeCommentEvent creates Atlantis models out of a GitLab event.
func (e *EventParser) ParseGitlabMergeCommentEvent(event gitlab.MergeCommentEvent) (baseRepo models.Repo, headRepo models.Repo, user models.User, err error) {
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

func (e *EventParser) ParseGitlabMergeRequest(mr *gitlab.MergeRequest) models.PullRequest {
	pullState := models.Closed
	if mr.State == gitlabPullOpened {
		pullState = models.Open
	}
	// GitLab also has a "merged" state, but we map that to Closed so we don't
	// need to check for it.

	return models.PullRequest{
		URL:        mr.WebURL,
		Author:     mr.Author.Username,
		Num:        mr.IID,
		HeadCommit: mr.SHA,
		Branch:     mr.SourceBranch,
		State:      pullState,
	}
}
