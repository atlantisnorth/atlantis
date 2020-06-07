package fixtures

import "github.com/google/go-github/v31/github"

var PullEvent = github.PullRequestEvent{
	Sender: &github.User{
		Login: github.String("user"),
	},
	Repo:        &Repo,
	PullRequest: &Pull,
	Action:      github.String("opened"),
}

var Pull = github.PullRequest{
	Head: &github.PullRequestBranch{
		SHA:  github.String("sha256"),
		Ref:  github.String("ref"),
		Repo: &Repo,
	},
	Base: &github.PullRequestBranch{
		SHA:  github.String("sha256"),
		Repo: &Repo,
		Ref:  github.String("basebranch"),
	},
	HTMLURL: github.String("html-url"),
	User: &github.User{
		Login: github.String("user"),
	},
	Number: github.Int(1),
	State:  github.String("open"),
}

var Repo = github.Repository{
	FullName: github.String("owner/repo"),
	Owner:    &github.User{Login: github.String("owner")},
	Name:     github.String("repo"),
	CloneURL: github.String("https://github.com/owner/repo.git"),
}
