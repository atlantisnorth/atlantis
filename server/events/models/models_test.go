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

package models_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs/azuredevops"
	. "github.com/runatlantis/atlantis/testing"
)

func TestNewRepo_EmptyRepoFullName(t *testing.T) {
	_, err := models.NewRepo(models.Github, "", "https://github.com/notowner/repo.git", "u", "p")
	ErrEquals(t, "repoFullName can't be empty", err)
}

func TestNewRepo_EmptyCloneURL(t *testing.T) {
	_, err := models.NewRepo(models.Github, "owner/repo", "", "u", "p")
	ErrEquals(t, "cloneURL can't be empty", err)
}

func TestNewRepo_InvalidCloneURL(t *testing.T) {
	_, err := models.NewRepo(models.Github, "owner/repo", ":", "u", "p")
	ErrEquals(t, "invalid clone url: parse \":.git\": missing protocol scheme", err)
}

func TestNewRepo_CloneURLWrongRepo(t *testing.T) {
	_, err := models.NewRepo(models.Github, "owner/repo", "https://github.com/notowner/repo.git", "u", "p")
	ErrEquals(t, `expected clone url to have path "/owner/repo.git" but had "/notowner/repo.git"`, err)
}

func TestNewRepo_EmptyAzureDevopsProject(t *testing.T) {
	_, err := models.NewRepo(models.AzureDevops, "", "https://dev.azure.com/notowner/project/_git/repo", "u", "p")
	ErrEquals(t, "repoFullName can't be empty", err)
}

// For bitbucket server we don't validate the clone URL because the callers
// are actually constructing it.
func TestNewRepo_CloneURLBitbucketServer(t *testing.T) {
	repo, err := models.NewRepo(models.BitbucketServer, "owner/repo", "http://mycorp.com:7990/scm/at/atlantis-example.git", "u", "p")
	Ok(t, err)
	Equals(t, models.Repo{
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
		CloneURL:          "http://u:p@mycorp.com:7990/scm/at/atlantis-example.git",
		SanitizedCloneURL: "http://u:<redacted>@mycorp.com:7990/scm/at/atlantis-example.git",
		VCSHost: models.VCSHost{
			Hostname: "mycorp.com",
			Type:     models.BitbucketServer,
		},
	}, repo)
}

func TestNewRepo_FullNameWrongFormat(t *testing.T) {
	cases := []struct {
		repoFullName string
		expErr       string
	}{
		{
			"owner/repo/extra",
			`invalid repo format "owner/repo/extra", owner "owner/repo" should not contain any /'s`,
		},
		{
			"/",
			`invalid repo format "/", owner "" or repo "" was empty`,
		},
		{
			"//",
			`invalid repo format "//", owner "" or repo "" was empty`,
		},
		{
			"///",
			`invalid repo format "///", owner "" or repo "" was empty`,
		},
		{
			"a/",
			`invalid repo format "a/", owner "" or repo "" was empty`,
		},
		{
			"/b",
			`invalid repo format "/b", owner "" or repo "b" was empty`,
		},
	}
	for _, c := range cases {
		t.Run(c.repoFullName, func(t *testing.T) {
			cloneURL := fmt.Sprintf("https://github.com/%s.git", c.repoFullName)
			_, err := models.NewRepo(models.Github, c.repoFullName, cloneURL, "u", "p")
			ErrEquals(t, c.expErr, err)
		})
	}
}

// If the clone url doesn't end with .git, and VCS is not Azure DevOps, it is appended
func TestNewRepo_MissingDotGit(t *testing.T) {
	repo, err := models.NewRepo(models.BitbucketCloud, "owner/repo", "https://bitbucket.org/owner/repo", "u", "p")
	Ok(t, err)
	Equals(t, repo.CloneURL, "https://u:p@bitbucket.org/owner/repo.git")
	Equals(t, repo.SanitizedCloneURL, "https://u:<redacted>@bitbucket.org/owner/repo.git")
}

func TestNewRepo_HTTPAuth(t *testing.T) {
	// When the url has http the auth should be added.
	repo, err := models.NewRepo(models.Github, "owner/repo", "http://github.com/owner/repo.git", "u", "p")
	Ok(t, err)
	Equals(t, models.Repo{
		VCSHost: models.VCSHost{
			Hostname: "github.com",
			Type:     models.Github,
		},
		SanitizedCloneURL: "http://u:<redacted>@github.com/owner/repo.git",
		CloneURL:          "http://u:p@github.com/owner/repo.git",
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
	}, repo)
}

func TestNewRepo_HTTPSAuth(t *testing.T) {
	// When the url has https the auth should be added.
	repo, err := models.NewRepo(models.Github, "owner/repo", "https://github.com/owner/repo.git", "u", "p")
	Ok(t, err)
	Equals(t, models.Repo{
		VCSHost: models.VCSHost{
			Hostname: "github.com",
			Type:     models.Github,
		},
		SanitizedCloneURL: "https://u:<redacted>@github.com/owner/repo.git",
		CloneURL:          "https://u:p@github.com/owner/repo.git",
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
	}, repo)
}

func TestProject_String(t *testing.T) {
	Equals(t, "repofullname=owner/repo path=my/path", (models.Project{
		RepoFullName: "owner/repo",
		Path:         "my/path",
	}).String())
}

func TestNewProject(t *testing.T) {
	cases := []struct {
		path    string
		expPath string
	}{
		{
			"/",
			".",
		},
		{
			"./another/path",
			"another/path",
		},
		{
			".",
			".",
		},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			p := models.NewProject("repo/owner", c.path)
			Equals(t, c.expPath, p.Path)
		})
	}
}

func TestVCSHostType_ToString(t *testing.T) {
	cases := []struct {
		vcsType models.VCSHostType
		exp     string
	}{
		{
			models.Github,
			"Github",
		},
		{
			models.Gitlab,
			"Gitlab",
		},
		{
			models.BitbucketCloud,
			"BitbucketCloud",
		},
		{
			models.BitbucketServer,
			"BitbucketServer",
		},
		{
			models.AzureDevops,
			"AzureDevops",
		},
	}

	for _, c := range cases {
		t.Run(c.exp, func(t *testing.T) {
			Equals(t, c.exp, c.vcsType.String())
		})
	}
}

func TestSplitRepoFullName(t *testing.T) {
	cases := []struct {
		input    string
		expOwner string
		expRepo  string
	}{
		{
			"owner/repo",
			"owner",
			"repo",
		},
		{
			"group/subgroup/owner/repo",
			"group/subgroup/owner",
			"repo",
		},
		{
			"",
			"",
			"",
		},
		{
			"/",
			"",
			"",
		},
		{
			"owner/",
			"",
			"",
		},
		{
			"/repo",
			"",
			"repo",
		},
		{
			"group/subgroup/",
			"",
			"",
		},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			owner, repo := models.SplitRepoFullName(c.input)
			Equals(t, c.expOwner, owner)
			Equals(t, c.expRepo, repo)
		})
	}
}

// These test cases should cover the same behavior as TestSplitRepoFullName
// and only produce different output in the AzureDevops case of
// owner/project/repo.
func TestAzureDevopsSplitRepoFullName(t *testing.T) {
	cases := []struct {
		input      string
		expOwner   string
		expRepo    string
		expProject string
	}{
		{
			"owner/repo",
			"owner",
			"repo",
			"",
		},
		{
			"group/subgroup/owner/repo",
			"group/subgroup/owner",
			"repo",
			"",
		},
		{
			"group/subgroup/owner/project/repo",
			"group/subgroup/owner/project",
			"repo",
			"",
		},
		{
			"",
			"",
			"",
			"",
		},
		{
			"/",
			"",
			"",
			"",
		},
		{
			"owner/",
			"",
			"",
			"",
		},
		{
			"/repo",
			"",
			"repo",
			"",
		},
		{
			"group/subgroup/",
			"",
			"",
			"",
		},
		{
			"owner/project/repo",
			"owner",
			"repo",
			"project",
		},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			owner, project, repo := azuredevops.SplitAzureDevopsRepoFullName(c.input)
			Equals(t, c.expOwner, owner)
			Equals(t, c.expProject, project)
			Equals(t, c.expRepo, repo)
		})
	}
}

func TestProjectResult_IsSuccessful(t *testing.T) {
	cases := map[string]struct {
		pr  models.ProjectResult
		exp bool
	}{
		"plan success": {
			models.ProjectResult{
				PlanSuccess: &models.PlanSuccess{},
			},
			true,
		},
		"apply success": {
			models.ProjectResult{
				ApplySuccess: "success",
			},
			true,
		},
		"failure": {
			models.ProjectResult{
				Failure: "failure",
			},
			false,
		},
		"error": {
			models.ProjectResult{
				Error: errors.New("error"),
			},
			false,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			Equals(t, c.exp, c.pr.IsSuccessful())
		})
	}
}

func TestProjectResult_PlanStatus(t *testing.T) {
	cases := []struct {
		p         models.ProjectResult
		expStatus models.ProjectPlanStatus
	}{
		{
			p: models.ProjectResult{
				Command: models.PlanCommand,
				Error:   errors.New("err"),
			},
			expStatus: models.ErroredPlanStatus,
		},
		{
			p: models.ProjectResult{
				Command: models.PlanCommand,
				Failure: "failure",
			},
			expStatus: models.ErroredPlanStatus,
		},
		{
			p: models.ProjectResult{
				Command:     models.PlanCommand,
				PlanSuccess: &models.PlanSuccess{},
			},
			expStatus: models.PlannedPlanStatus,
		},
		{
			p: models.ProjectResult{
				Command: models.ApplyCommand,
				Error:   errors.New("err"),
			},
			expStatus: models.ErroredApplyStatus,
		},
		{
			p: models.ProjectResult{
				Command: models.ApplyCommand,
				Failure: "failure",
			},
			expStatus: models.ErroredApplyStatus,
		},
		{
			p: models.ProjectResult{
				Command:      models.ApplyCommand,
				ApplySuccess: "success",
			},
			expStatus: models.AppliedPlanStatus,
		},
	}

	for _, c := range cases {
		t.Run(c.expStatus.String(), func(t *testing.T) {
			Equals(t, c.expStatus, c.p.PlanStatus())
		})
	}
}

func TestPullStatus_StatusCount(t *testing.T) {
	ps := models.PullStatus{
		Projects: []models.ProjectStatus{
			{
				Status: models.PlannedPlanStatus,
			},
			{
				Status: models.PlannedPlanStatus,
			},
			{
				Status: models.AppliedPlanStatus,
			},
			{
				Status: models.ErroredApplyStatus,
			},
		},
	}

	Equals(t, 2, ps.StatusCount(models.PlannedPlanStatus))
	Equals(t, 1, ps.StatusCount(models.AppliedPlanStatus))
	Equals(t, 1, ps.StatusCount(models.ErroredApplyStatus))
	Equals(t, 0, ps.StatusCount(models.ErroredPlanStatus))
}
