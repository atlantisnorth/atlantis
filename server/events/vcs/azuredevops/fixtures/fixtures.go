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

package fixtures

import (
	"github.com/mcdafydd/go-azuredevops/azuredevops"
)

var ADPullEvent = azuredevops.Event{
	EventType: "git.pullrequest.created",
	Resource:  &ADPull,
}

var ADPullUpdatedEvent = azuredevops.Event{
	EventType: "git.pullrequest.updated",
	Resource:  &ADPull,
}

var ADPullClosedEvent = azuredevops.Event{
	EventType: "git.pullrequest.merged",
	Resource:  &ADPullCompleted,
}

var ADPull = azuredevops.GitPullRequest{
	CreatedBy: &azuredevops.IdentityRef{
		ID:          azuredevops.String("d6245f20-2af8-44f4-9451-8107cb2767db"),
		DisplayName: azuredevops.String("User"),
		UniqueName:  azuredevops.String("user@example.com"),
	},
	LastMergeSourceCommit: &azuredevops.GitCommitRef{
		CommitID: azuredevops.String("b60280bc6e62e2f880f1b63c1e24987664d3bda3"),
		URL:      azuredevops.String("https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/commits/b60280bc6e62e2f880f1b63c1e24987664d3bda3"),
	},
	PullRequestID: azuredevops.Int(1),
	Repository:    &ADRepo,
	SourceRefName: azuredevops.String("refs/heads/feature/sourceBranch"),
	Status:        azuredevops.String("active"),
	TargetRefName: azuredevops.String("refs/heads/targetBranch"),
	URL:           azuredevops.String("https://dev.azure.com/fabrikam/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/pullRequests/21"),
}

var ADPullCompleted = azuredevops.GitPullRequest{
	CreatedBy: &azuredevops.IdentityRef{
		ID:          azuredevops.String("d6245f20-2af8-44f4-9451-8107cb2767db"),
		DisplayName: azuredevops.String("User"),
		UniqueName:  azuredevops.String("user@example.com"),
	},
	LastMergeSourceCommit: &azuredevops.GitCommitRef{
		CommitID: azuredevops.String("b60280bc6e62e2f880f1b63c1e24987664d3bda3"),
		URL:      azuredevops.String("https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/commits/b60280bc6e62e2f880f1b63c1e24987664d3bda3"),
	},
	PullRequestID: azuredevops.Int(1),
	Repository:    &ADRepo,
	SourceRefName: azuredevops.String("refs/heads/owner/sourceBranch"),
	Status:        azuredevops.String("completed"),
	TargetRefName: azuredevops.String("refs/heads/targetBranch"),
	URL:           azuredevops.String("https://dev.azure.com/fabrikam/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/pullRequests/21"),
}

var ADRepo = azuredevops.GitRepository{
	DefaultBranch: azuredevops.String("refs/heads/master"),
	Name:          azuredevops.String("repo"),
	ParentRepository: &azuredevops.GitRepositoryRef{
		Name: azuredevops.String("owner"),
	},
	Project: &azuredevops.TeamProjectReference{
		ID:    azuredevops.String("a21f5f20-4a12-aaf4-ab12-9a0927cbbb90"),
		Name:  azuredevops.String("project"),
		State: azuredevops.String("unchanged"),
	},
	WebURL: azuredevops.String("https://dev.azure.com/owner/project/_git/repo"),
}

var ADPullJSON = `{
	"repository": {
		"id": "3411ebc1-d5aa-464f-9615-0b527bc66719",
		"name": "repo",
		"url": "https://dev.azure.com/owner/project/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719",
		"webUrl": "https://dev.azure.com/owner/project/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719",
		"project": {
			"id": "a7573007-bbb3-4341-b726-0c4148a07853",
			"name": "project",
			"description": "test project created on Halloween 2016",
			"url": "https://dev.azure.com/owner/_apis/projects/a7573007-bbb3-4341-b726-0c4148a07853",
			"state": "wellFormed",
			"revision": 7
		},
		"remoteUrl": "https://dev.azure.com/owner/project/_git/repo"
	},
	"pullRequestId": 22,
	"codeReviewId": 22,
	"status": "active",
	"createdBy": {
		"id": "d6245f20-2af8-44f4-9451-8107cb2767db",
		"displayName": "Normal Paulk",
		"uniqueName": "fabrikamfiber16@hotmail.com",
		"url": "https://dev.azure.com/owner/_apis/Identities/d6245f20-2af8-44f4-9451-8107cb2767db",
		"imageUrl": "https://dev.azure.com/owner/_api/_common/identityImage?id=d6245f20-2af8-44f4-9451-8107cb2767db"
	},
	"creationDate": "2016-11-01T16:30:31.6655471Z",
	"title": "A new feature",
	"description": "Adding a new feature",
	"sourceRefName": "refs/heads/npaulk/my_work",
	"targetRefName": "refs/heads/new_feature",
	"mergeStatus": "succeeded",
	"mergeId": "f5fc8381-3fb2-49fe-8a0d-27dcc2d6ef82",
	"lastMergeSourceCommit": {
		"commitId": "b60280bc6e62e2f880f1b63c1e24987664d3bda3",
		"url": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/commits/b60280bc6e62e2f880f1b63c1e24987664d3bda3"
	},
	"lastMergeTargetCommit": {
		"commitId": "f47bbc106853afe3c1b07a81754bce5f4b8dbf62",
		"url": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/commits/f47bbc106853afe3c1b07a81754bce5f4b8dbf62"
	},
	"lastMergeCommit": {
		"commitId": "39f52d24533cc712fc845ed9fd1b6c06b3942588",
		"author": {
			"name": "Normal Paulk",
			"email": "fabrikamfiber16@hotmail.com",
			"date": "2016-11-01T16:30:32Z"
		},
		"committer": {
			"name": "Normal Paulk",
			"email": "fabrikamfiber16@hotmail.com",
			"date": "2016-11-01T16:30:32Z"
		},
		"comment": "Merge pull request 22 from npaulk/my_work into new_feature",
		"url": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/commits/39f52d24533cc712fc845ed9fd1b6c06b3942588"
	},
	"reviewers": [
		{
			"reviewerUrl": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/pullRequests/22/reviewers/d6245f20-2af8-44f4-9451-8107cb2767db",
			"vote": 0,
			"id": "d6245f20-2af8-44f4-9451-8107cb2767db",
			"displayName": "Normal Paulk",
			"uniqueName": "fabrikamfiber16@hotmail.com",
			"url": "https://dev.azure.com/owner/_apis/Identities/d6245f20-2af8-44f4-9451-8107cb2767db",
			"imageUrl": "https://dev.azure.com/owner/_api/_common/identityImage?id=d6245f20-2af8-44f4-9451-8107cb2767db"
		}
	],
	"url": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/pullRequests/22",
	"_links": {
		"self": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/pullRequests/22"
		},
		"repository": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719"
		},
		"workItems": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/pullRequests/22/workitems"
		},
		"sourceBranch": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/refs"
		},
		"targetBranch": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/refs"
		},
		"sourceCommit": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/commits/b60280bc6e62e2f880f1b63c1e24987664d3bda3"
		},
		"targetCommit": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/commits/f47bbc106853afe3c1b07a81754bce5f4b8dbf62"
		},
		"createdBy": {
			"href": "https://dev.azure.com/owner/_apis/Identities/d6245f20-2af8-44f4-9451-8107cb2767db"
		},
		"iterations": {
			"href": "https://dev.azure.com/owner/_apis/git/repositories/3411ebc1-d5aa-464f-9615-0b527bc66719/pullRequests/22/iterations"
		}
	},
	"supportsIterations": true,
	"artifactId": "vstfs:///Git/PullRequestId/a7573007-bbb3-4341-b726-0c4148a07853%2f3411ebc1-d5aa-464f-9615-0b527bc66719%2f22"
}`
