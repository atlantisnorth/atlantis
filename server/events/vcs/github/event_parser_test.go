package github_test

import (
	"testing"

	"github.com/google/go-github/v31/github"
	"github.com/mohae/deepcopy"

	"github.com/runatlantis/atlantis/server/events/models"
	vcsgithub "github.com/runatlantis/atlantis/server/events/vcs/github"
	. "github.com/runatlantis/atlantis/server/events/vcs/github/fixtures"
	. "github.com/runatlantis/atlantis/testing"
)

var parser = vcsgithub.EventParser{
	User:  "github-user",
	Token: "github-token",
}

func TestParseGithubRepo(t *testing.T) {
	r, err := parser.ParseGithubRepo(&Repo)
	Ok(t, err)
	Equals(t, models.Repo{
		Owner:             "owner",
		FullName:          "owner/repo",
		CloneURL:          "https://github-user:github-token@github.com/owner/repo.git",
		SanitizedCloneURL: "https://github-user:<redacted>@github.com/owner/repo.git",
		Name:              "repo",
		VCSHost: models.VCSHost{
			Hostname: "github.com",
			Type:     models.Github,
		},
	}, r)
}

func TestParseGithubIssueCommentEvent(t *testing.T) {
	comment := github.IssueCommentEvent{
		Repo: &Repo,
		Issue: &github.Issue{
			Number:  github.Int(1),
			User:    &github.User{Login: github.String("issue_user")},
			HTMLURL: github.String("https://github.com/runatlantis/atlantis/issues/1"),
		},
		Comment: &github.IssueComment{
			User: &github.User{Login: github.String("comment_user")},
		},
	}

	testComment := deepcopy.Copy(comment).(github.IssueCommentEvent)
	testComment.Comment = nil
	_, _, _, err := parser.ParseGithubIssueCommentEvent(&testComment)
	ErrEquals(t, "comment.user.login is null", err)

	testComment = deepcopy.Copy(comment).(github.IssueCommentEvent)
	testComment.Comment.User = nil
	_, _, _, err = parser.ParseGithubIssueCommentEvent(&testComment)
	ErrEquals(t, "comment.user.login is null", err)

	testComment = deepcopy.Copy(comment).(github.IssueCommentEvent)
	testComment.Comment.User.Login = nil
	_, _, _, err = parser.ParseGithubIssueCommentEvent(&testComment)
	ErrEquals(t, "comment.user.login is null", err)

	testComment = deepcopy.Copy(comment).(github.IssueCommentEvent)
	testComment.Issue = nil
	_, _, _, err = parser.ParseGithubIssueCommentEvent(&testComment)
	ErrEquals(t, "issue.number is null", err)

	// this should be successful
	repo, user, pullNum, err := parser.ParseGithubIssueCommentEvent(&comment)
	Ok(t, err)
	Equals(t, models.Repo{
		Owner:             *comment.Repo.Owner.Login,
		FullName:          *comment.Repo.FullName,
		CloneURL:          "https://github-user:github-token@github.com/owner/repo.git",
		SanitizedCloneURL: "https://github-user:<redacted>@github.com/owner/repo.git",
		Name:              "repo",
		VCSHost: models.VCSHost{
			Hostname: "github.com",
			Type:     models.Github,
		},
	}, repo)
	Equals(t, models.User{
		Username: *comment.Comment.User.Login,
	}, user)
	Equals(t, *comment.Issue.Number, pullNum)
}

func TestParseGithubPullEvent(t *testing.T) {
	_, _, _, _, _, err := parser.ParseGithubPullEvent(&github.PullRequestEvent{})
	ErrEquals(t, "pull_request is null", err)

	testEvent := deepcopy.Copy(PullEvent).(github.PullRequestEvent)
	testEvent.PullRequest.HTMLURL = nil
	_, _, _, _, _, err = parser.ParseGithubPullEvent(&testEvent)
	ErrEquals(t, "html_url is null", err)

	testEvent = deepcopy.Copy(PullEvent).(github.PullRequestEvent)
	testEvent.Sender = nil
	_, _, _, _, _, err = parser.ParseGithubPullEvent(&testEvent)
	ErrEquals(t, "sender is null", err)

	testEvent = deepcopy.Copy(PullEvent).(github.PullRequestEvent)
	testEvent.Sender.Login = nil
	_, _, _, _, _, err = parser.ParseGithubPullEvent(&testEvent)
	ErrEquals(t, "sender.login is null", err)

	actPull, evType, actBaseRepo, actHeadRepo, actUser, err := parser.ParseGithubPullEvent(&PullEvent)
	Ok(t, err)
	expBaseRepo := models.Repo{
		Owner:             "owner",
		FullName:          "owner/repo",
		CloneURL:          "https://github-user:github-token@github.com/owner/repo.git",
		SanitizedCloneURL: "https://github-user:<redacted>@github.com/owner/repo.git",
		Name:              "repo",
		VCSHost: models.VCSHost{
			Hostname: "github.com",
			Type:     models.Github,
		},
	}
	Equals(t, expBaseRepo, actBaseRepo)
	Equals(t, expBaseRepo, actHeadRepo)
	Equals(t, models.PullRequest{
		URL:        Pull.GetHTMLURL(),
		Author:     Pull.User.GetLogin(),
		HeadBranch: Pull.Head.GetRef(),
		BaseBranch: Pull.Base.GetRef(),
		HeadCommit: Pull.Head.GetSHA(),
		Num:        Pull.GetNumber(),
		State:      models.OpenPullState,
		BaseRepo:   expBaseRepo,
	}, actPull)
	Equals(t, models.OpenedPullEvent, evType)
	Equals(t, models.User{Username: "user"}, actUser)
}
