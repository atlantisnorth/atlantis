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
	"fmt"
	"strings"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs"
)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_commit_status_updater.go CommitStatusUpdater

// CommitStatusUpdater updates the status of a commit with the VCS host. We set
// the status to signify whether the plan/apply succeeds.
type CommitStatusUpdater interface {
	// Update updates the status of the head commit of pull.
	Update(repo models.Repo, pull models.PullRequest, status models.CommitStatus, command models.CommandName) error
	// UpdateProjectResult updates the status of the head commit given the
	// state of response.
	// todo: rename this so it doesn't conflict with UpdateProject
	UpdateProjectResult(ctx *CommandContext, commandName models.CommandName, res CommandResult) error
	// UpdateProject sets the commit status for the project represented by
	// ctx.
	UpdateProject(ctx models.ProjectCommandContext, cmdName models.CommandName, status models.CommitStatus, url string) error
}

// DefaultCommitStatusUpdater implements CommitStatusUpdater.
type DefaultCommitStatusUpdater struct {
	Client vcs.Client
}

// Update updates the commit status.
func (d *DefaultCommitStatusUpdater) Update(repo models.Repo, pull models.PullRequest, status models.CommitStatus, command models.CommandName) error {
	description := fmt.Sprintf("%s %s", strings.Title(command.String()), strings.Title(status.String()))
	return d.Client.UpdateStatus(repo, pull, status, "Atlantis", description, "")
}

// UpdateProjectResult updates the commit status based on the status of res.
func (d *DefaultCommitStatusUpdater) UpdateProjectResult(ctx *CommandContext, commandName models.CommandName, res CommandResult) error {
	var status models.CommitStatus
	if res.Error != nil || res.Failure != "" {
		status = models.FailedCommitStatus
	} else {
		var statuses []models.CommitStatus
		for _, p := range res.ProjectResults {
			statuses = append(statuses, p.Status())
		}
		status = d.worstStatus(statuses)
	}
	return d.Update(ctx.BaseRepo, ctx.Pull, status, commandName)
}

func (d *DefaultCommitStatusUpdater) UpdateProject(ctx models.ProjectCommandContext, cmdName models.CommandName, status models.CommitStatus, url string) error {
	projectID := ctx.GetProjectName()
	if projectID == "" {
		projectID = fmt.Sprintf("%s/%s", ctx.RepoRelDir, ctx.Workspace)
	}
	src := fmt.Sprintf("%s/atlantis: %s", cmdName.String(), projectID)
	var descripWords string
	switch status {
	case models.PendingCommitStatus:
		descripWords = "in progress..."
	case models.FailedCommitStatus:
		descripWords = "failed."
	case models.SuccessCommitStatus:
		descripWords = "succeeded."
	}
	descrip := fmt.Sprintf("%s %s", strings.Title(cmdName.String()), descripWords)
	return d.Client.UpdateStatus(ctx.BaseRepo, ctx.Pull, status, src, descrip, url)
}

func (d *DefaultCommitStatusUpdater) worstStatus(ss []models.CommitStatus) models.CommitStatus {
	for _, s := range ss {
		if s == models.FailedCommitStatus {
			return models.FailedCommitStatus
		}
	}
	return models.SuccessCommitStatus
}
