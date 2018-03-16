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
package models_test

import (
	"fmt"
	"testing"

	"github.com/runatlantis/atlantis/server/events/models"
	. "github.com/runatlantis/atlantis/testing"
)

func TestNewRepo_EmptyRepoFullName(t *testing.T) {
	_, err := models.NewRepo("", "https://github.com/notowner/repo.git", "u", "p")
	ErrEquals(t, "repoFullName can't be empty", err)
}

func TestNewRepo_EmptyCloneURL(t *testing.T) {
	_, err := models.NewRepo("owner/repo", "", "u", "p")
	ErrEquals(t, "cloneURL can't be empty", err)
}

func TestNewRepo_InvalidCloneURL(t *testing.T) {
	_, err := models.NewRepo("owner/repo", ":", "u", "p")
	ErrEquals(t, "invalid clone url: parse :: missing protocol scheme", err)
}

func TestNewRepo_CloneURLWrongRepo(t *testing.T) {
	_, err := models.NewRepo("owner/repo", "https://github.com/notowner/repo.git", "u", "p")
	ErrEquals(t, `expected clone url to have path "/owner/repo.git" but had "/notowner/repo.git"`, err)
}

func TestNewRepo_FullNameWrongFormat(t *testing.T) {
	cases := []string{
		"owner/repo/extra",
		"/",
		"//",
		"///",
		"a/",
		"/b",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			cloneURL := fmt.Sprintf("https://github.com/%s.git", c)
			_, err := models.NewRepo(c, cloneURL, "u", "p")
			ErrEquals(t, fmt.Sprintf(`invalid repo format "%s"`, c), err)
		})
	}
}

func TestNewRepo_HTTPAuth(t *testing.T) {
	// When the url has http the auth should be added.
	repo, err := models.NewRepo("owner/repo", "http://github.com/owner/repo.git", "u", "p")
	Ok(t, err)
	Equals(t, models.Repo{
		Hostname:          "github.com",
		SanitizedCloneURL: "http://github.com/owner/repo.git",
		CloneURL:          "http://u:p@github.com/owner/repo.git",
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
	}, repo)
}

func TestNewRepo_HTTPSAuth(t *testing.T) {
	// When the url has https the auth should be added.
	repo, err := models.NewRepo("owner/repo", "https://github.com/owner/repo.git", "u", "p")
	Ok(t, err)
	Equals(t, models.Repo{
		Hostname:          "github.com",
		SanitizedCloneURL: "https://github.com/owner/repo.git",
		CloneURL:          "https://u:p@github.com/owner/repo.git",
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
	}, repo)
}
