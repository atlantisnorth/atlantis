package github

import (
	"errors"

	"github.com/google/go-github/v31/github"
	"github.com/runatlantis/atlantis/server/events/models"
)

type EventParser struct {
	User  string
	Token string
}

// ParseGithubRepo parses the response from the GitHub API endpoint that
// returns a repo into the Atlantis model.
// See EventParsing for return value docs.
func (e *EventParser) ParseGithubRepo(ghRepo *github.Repository) (models.Repo, error) {
	return models.NewRepo(models.Github, ghRepo.GetFullName(), ghRepo.GetCloneURL(), e.User, e.Token)
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
		// if the PR is in draft state we do not initiate actions proactively however,
		// we must still clean up locks in the event of a user initiated plan
		if pullEvent.GetAction() == "closed" {
			pullEventType = models.ClosedPullEvent
		} else {
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
