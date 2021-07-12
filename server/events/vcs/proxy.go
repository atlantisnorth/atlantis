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

package vcs

import (
	"github.com/runatlantis/atlantis/server/events/models"
)

// ClientProxy proxies calls to the correct VCS client depending on which
// VCS host is required.
type ClientProxy struct {
	// clients maps from the vcs host type to the client that implements the
	// api for that host type, ex. github -> github client.
	clients map[models.VCSHostType]Client
}

func NewClientProxy(githubClient Client, gitlabClient Client, bitbucketCloudClient Client, bitbucketServerClient Client, azuredevopsClient Client) *ClientProxy {
	if githubClient == nil {
		githubClient = &NotConfiguredVCSClient{}
	}
	if gitlabClient == nil {
		gitlabClient = &NotConfiguredVCSClient{}
	}
	if bitbucketCloudClient == nil {
		bitbucketCloudClient = &NotConfiguredVCSClient{}
	}
	if bitbucketServerClient == nil {
		bitbucketServerClient = &NotConfiguredVCSClient{}
	}
	if azuredevopsClient == nil {
		azuredevopsClient = &NotConfiguredVCSClient{}
	}
	return &ClientProxy{
		clients: map[models.VCSHostType]Client{
			models.Github:          githubClient,
			models.Gitlab:          gitlabClient,
			models.BitbucketCloud:  bitbucketCloudClient,
			models.BitbucketServer: bitbucketServerClient,
			models.AzureDevops:     azuredevopsClient,
		},
	}
}

func (d *ClientProxy) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	return d.clients[repo.VCSHost.Type].GetModifiedFiles(repo, pull)
}

func (d *ClientProxy) CreateComment(repo models.Repo, pullNum int, comment string, command string) error {
	return d.clients[repo.VCSHost.Type].CreateComment(repo, pullNum, comment, command)
}

func (d *ClientProxy) HidePrevCommandComments(repo models.Repo, pullNum int, command string) error {
	return d.clients[repo.VCSHost.Type].HidePrevCommandComments(repo, pullNum, command)
}

func (d *ClientProxy) PullIsApproved(repo models.Repo, pull models.PullRequest) (bool, error) {
	return d.clients[repo.VCSHost.Type].PullIsApproved(repo, pull)
}

func (d *ClientProxy) PullIsMergeable(repo models.Repo, pull models.PullRequest) (bool, error) {
	return d.clients[repo.VCSHost.Type].PullIsMergeable(repo, pull)
}

func (d *ClientProxy) UpdateStatus(repo models.Repo, pull models.PullRequest, state models.CommitStatus, src string, description string, url string) error {
	return d.clients[repo.VCSHost.Type].UpdateStatus(repo, pull, state, src, description, url)
}

func (d *ClientProxy) MergePull(pull models.PullRequest, pullOptions models.PullRequestOptions) error {
	return d.clients[pull.BaseRepo.VCSHost.Type].MergePull(pull, pullOptions)
}

func (d *ClientProxy) MarkdownPullLink(pull models.PullRequest) (string, error) {
	return d.clients[pull.BaseRepo.VCSHost.Type].MarkdownPullLink(pull)
}

func (d *ClientProxy) GetTeamNamesForUser(repo models.Repo, user models.User) ([]string, error) {
	return d.clients[repo.VCSHost.Type].GetTeamNamesForUser(repo, user)
}

func (d *ClientProxy) DownloadRepoConfigFile(pull models.PullRequest) (bool, []byte, error) {
	return d.clients[pull.BaseRepo.VCSHost.Type].DownloadRepoConfigFile(pull)
}

func (d *ClientProxy) SupportsSingleFileDownload(repo models.Repo) bool {
	return d.clients[repo.VCSHost.Type].SupportsSingleFileDownload(repo)
}
