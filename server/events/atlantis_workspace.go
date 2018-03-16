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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/logging"
)

const workspacePrefix = "repos"

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_atlantis_workspace.go AtlantisWorkspace

// AtlantisWorkspace handles the workspace on disk for running commands.
type AtlantisWorkspace interface {
	// Clone git clones headRepo, checks out the branch and then returns the
	// absolute path to the root of the cloned repo.
	Clone(log *logging.SimpleLogger, baseRepo models.Repo, headRepo models.Repo, p models.PullRequest, workspace string) (string, error)
	// GetWorkspace returns the path to the workspace for this repo and pull.
	GetWorkspace(r models.Repo, p models.PullRequest, workspace string) (string, error)
	// Delete deletes the workspace for this repo and pull.
	Delete(r models.Repo, p models.PullRequest) error
}

// FileWorkspace implements AtlantisWorkspace with the file system.
type FileWorkspace struct {
	DataDir string
}

// Clone git clones headRepo, checks out the branch and then returns the absolute
// path to the root of the cloned repo.
func (w *FileWorkspace) Clone(
	log *logging.SimpleLogger,
	baseRepo models.Repo,
	headRepo models.Repo,
	p models.PullRequest,
	workspace string) (string, error) {
	cloneDir := w.cloneDir(baseRepo, p, workspace)

	// This is safe to do because we lock runs on repo/pull/workspace so no one else
	// is using this workspace.
	log.Info("cleaning clone directory %q", cloneDir)
	if err := os.RemoveAll(cloneDir); err != nil {
		return "", errors.Wrap(err, "deleting old workspace")
	}

	// Create the directory and parents if necessary.
	log.Info("creating dir %q", cloneDir)
	if err := os.MkdirAll(cloneDir, 0700); err != nil {
		return "", errors.Wrap(err, "creating new workspace")
	}

	log.Info("git cloning %q into %q", headRepo.SanitizedCloneURL, cloneDir)
	cloneCmd := exec.Command("git", "clone", headRepo.CloneURL, cloneDir) // #nosec
	if output, err := cloneCmd.CombinedOutput(); err != nil {
		return "", errors.Wrapf(err, "cloning %s: %s", headRepo.SanitizedCloneURL, string(output))
	}

	// Check out the branch for this PR.
	log.Info("checking out branch %q", p.Branch)
	checkoutCmd := exec.Command("git", "checkout", p.Branch) // #nosec
	checkoutCmd.Dir = cloneDir
	if err := checkoutCmd.Run(); err != nil {
		return "", errors.Wrapf(err, "checking out branch %s", p.Branch)
	}
	return cloneDir, nil
}

// GetWorkspace returns the path to the workspace for this repo and pull.
func (w *FileWorkspace) GetWorkspace(r models.Repo, p models.PullRequest, workspace string) (string, error) {
	repoDir := w.cloneDir(r, p, workspace)
	if _, err := os.Stat(repoDir); err != nil {
		return "", errors.Wrap(err, "checking if workspace exists")
	}
	return repoDir, nil
}

// Delete deletes the workspace for this repo and pull.
func (w *FileWorkspace) Delete(r models.Repo, p models.PullRequest) error {
	return os.RemoveAll(w.repoPullDir(r, p))
}

func (w *FileWorkspace) repoPullDir(r models.Repo, p models.PullRequest) string {
	return filepath.Join(w.DataDir, workspacePrefix, r.FullName, strconv.Itoa(p.Num))
}

func (w *FileWorkspace) cloneDir(r models.Repo, p models.PullRequest, workspace string) string {
	return filepath.Join(w.repoPullDir(r, p), workspace)
}
