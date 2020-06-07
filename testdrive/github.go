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

package testdrive

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-github/v31/github"
)

var githubUsername string
var githubToken string

// Client used for GitHub interactions.
type Client struct {
	client *github.Client
	ctx    context.Context
}

// CreateFork forks a GitHub repo into the user's account that is authenticated.
func (g *Client) CreateFork(owner string, repoName string) error {
	_, _, err := g.client.Repositories.CreateFork(g.ctx, owner, repoName, nil)
	// The GitHub client returns an error even though the fork was successful.
	// In order to figure out the exact error we will need to check the message.
	if err != nil && !strings.Contains(err.Error(), "job scheduled on GitHub side; try again later") {
		return err
	}
	return nil
}

// CheckForkSuccess waits for github fork to complete.
// Forks can take up to 5 minutes to complete according to GitHub.
func (g *Client) CheckForkSuccess(ownerName string, forkRepoName string) bool {
	for i := 0; i < 5; i++ {
		if err := g.CreateFork(ownerName, forkRepoName); err == nil {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// CreateWebhook creates a GitHub webhook to send requests to our local ngrok.
func (g *Client) CreateWebhook(ownerName string, repoName string, hookURL string) error {
	atlantisHook := &github.Hook{
		Events: []string{"issue_comment", "pull_request", "pull_request_review", "push"},
		Config: map[string]interface{}{
			"url":          hookURL,
			"content_type": "json",
		},
		Active: github.Bool(true),
	}
	_, _, err := g.client.Repositories.CreateHook(g.ctx, ownerName, repoName, atlantisHook)
	return err
}

// CreatePullRequest creates a GitHub pull request with custom title and
// description. If there's already a pull request open for this branch it will
// return successfully.
func (g *Client) CreatePullRequest(ownerName string, repoName string, head string, base string) (string, error) {

	// First check if the pull request already exists.
	pulls, _, err := g.client.PullRequests.List(g.ctx, ownerName, repoName, nil)
	if err != nil {
		return "", err
	}
	for _, pull := range pulls {
		if pull.Head.GetRef() == head && pull.Base.GetRef() == base {
			return pull.GetHTMLURL(), nil
		}
	}

	// If not, create it.
	newPullRequest := &github.NewPullRequest{
		Title: github.String("Welcome to Atlantis!"),
		Head:  github.String(head),
		Body:  github.String(pullRequestBody),
		Base:  github.String(base),
	}
	pull, _, err := g.client.PullRequests.Create(g.ctx, ownerName, repoName, newPullRequest)
	if err != nil {
		return "", err
	}

	return pull.GetHTMLURL(), nil
}
