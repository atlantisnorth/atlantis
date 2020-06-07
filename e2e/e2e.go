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

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/google/go-github/v31/github"
)

type E2ETester struct {
	githubClient *GithubClient
	repoURL      string
	ownerName    string
	repoName     string
	hookID       int64
	cloneDirRoot string
	projectType  Project
}

type E2EResult struct {
	projectType          string
	githubPullRequestURL string
	testResult           string
}

var testFileData = `
resource "null_resource" "hello" {
}
`

// nolint: gosec
func (t *E2ETester) Start() (*E2EResult, error) {
	cloneDir := fmt.Sprintf("%s/%s-test", t.cloneDirRoot, t.projectType.Name)
	branchName := fmt.Sprintf("%s-%s", t.projectType.Name, time.Now().Format("20060102150405"))
	testFileName := fmt.Sprintf("%s.tf", t.projectType.Name)
	e2eResult := &E2EResult{}
	e2eResult.projectType = t.projectType.Name

	// create the directory and parents if necessary
	log.Printf("creating dir %q", cloneDir)
	if err := os.MkdirAll(cloneDir, 0700); err != nil {
		return e2eResult, fmt.Errorf("failed to create dir %q prior to cloning, attempting to continue: %v", cloneDir, err)
	}

	cloneCmd := exec.Command("git", "clone", t.repoURL, cloneDir)
	// git clone the repo
	log.Printf("git cloning into %q", cloneDir)
	if output, err := cloneCmd.CombinedOutput(); err != nil {
		return e2eResult, fmt.Errorf("failed to clone repository: %v: %s", err, string(output))
	}

	// checkout a new branch for the project
	log.Printf("checking out branch %q", branchName)
	checkoutCmd := exec.Command("git", "checkout", "-b", branchName)
	checkoutCmd.Dir = cloneDir
	if err := checkoutCmd.Run(); err != nil {
		return e2eResult, fmt.Errorf("failed to git checkout branch %q: %v", branchName, err)
	}

	// write a file for running the tests
	randomData := []byte(testFileData)
	filePath := fmt.Sprintf("%s/%s/%s", cloneDir, t.projectType.Name, testFileName)
	log.Printf("creating file to commit %q", filePath)
	err := ioutil.WriteFile(filePath, randomData, 0644)
	if err != nil {
		return e2eResult, fmt.Errorf("couldn't write file %s: %v", filePath, err)
	}

	// add the file
	log.Printf("git add file %q", filePath)
	addCmd := exec.Command("git", "add", filePath)
	addCmd.Dir = cloneDir
	if err = addCmd.Run(); err != nil {
		return e2eResult, fmt.Errorf("failed to git add file %q: %v", filePath, err)
	}

	// commit the file
	log.Printf("git commit file %q", filePath)
	commitCmd := exec.Command("git", "commit", "-am", "test commit")
	commitCmd.Dir = cloneDir
	var output []byte
	if output, err = commitCmd.CombinedOutput(); err != nil {
		return e2eResult, fmt.Errorf("failed to run git commit in %q: %v: %v", cloneDir, err, string(output))
	}

	// push the branch to remote
	log.Printf("git push branch %q", branchName)
	pushCmd := exec.Command("git", "push", "origin", branchName)
	pushCmd.Dir = cloneDir
	if err = pushCmd.Run(); err != nil {
		return e2eResult, fmt.Errorf("failed to git push branch %q: %v", branchName, err)
	}

	// create a new pr
	title := fmt.Sprintf("This is a test pull request for atlantis e2e test for %s project type", t.projectType.Name)
	head := fmt.Sprintf("%s:%s", t.ownerName, branchName)
	body := ""
	base := "master"
	newPullRequest := &github.NewPullRequest{Title: &title, Head: &head, Body: &body, Base: &base}

	pull, _, err := t.githubClient.client.PullRequests.Create(t.githubClient.ctx, t.ownerName, t.repoName, newPullRequest)
	if err != nil {
		return e2eResult, fmt.Errorf("error while creating new pull request: %v", err)
	}

	// set pull request url
	e2eResult.githubPullRequestURL = pull.GetHTMLURL()

	log.Printf("created pull request %s", pull.GetHTMLURL())

	// defer closing pull request and delete remote branch
	defer cleanUp(t, pull.GetNumber(), branchName) // nolint: errcheck

	// wait for atlantis to respond to webhook and autoplan.
	time.Sleep(2 * time.Second)

	state := "not started"
	// waiting for atlantis run and finish
	maxLoops := 20
	i := 0
	for ; i < maxLoops && checkStatus(state); i++ {
		time.Sleep(2 * time.Second)
		state, _ = getAtlantisStatus(t, branchName)
		if state == "" {
			log.Println("atlantis run hasn't started")
			continue
		}
		log.Printf("atlantis run is in %s state", state)
	}
	if i == maxLoops {
		state = "timed out"
	}

	log.Printf("atlantis run finished with status %q", state)
	e2eResult.testResult = state
	// check if atlantis run was a success
	if state != "success" {
		return e2eResult, fmt.Errorf("atlantis run project type %q failed with %q status", t.projectType.Name, state)
	}

	return e2eResult, nil
}

func getAtlantisStatus(t *E2ETester, branchName string) (string, error) {
	// check repo status
	combinedStatus, _, err := t.githubClient.client.Repositories.GetCombinedStatus(t.githubClient.ctx, t.ownerName, t.repoName, branchName, nil)
	if err != nil {
		return "", err
	}

	for _, status := range combinedStatus.Statuses {
		if status.GetContext() == "atlantis/plan" {
			return status.GetState(), nil
		}
	}

	return "", nil
}

func checkStatus(state string) bool {
	for _, s := range []string{"success", "error", "failure"} {
		if state == s {
			return false
		}
	}
	return true
}

func cleanUp(t *E2ETester, pullRequestNumber int, branchName string) error {
	// clean up
	pullClosed, _, err := t.githubClient.client.PullRequests.Edit(t.githubClient.ctx, t.ownerName, t.repoName, pullRequestNumber, &github.PullRequest{State: github.String("closed")})
	if err != nil {
		return fmt.Errorf("error while closing new pull request: %v", err)
	}
	log.Printf("closed pull request %d", pullClosed.GetNumber())

	deleteBranchName := fmt.Sprintf("%s/%s", "heads", branchName)
	_, err = t.githubClient.client.Git.DeleteRef(t.githubClient.ctx, t.ownerName, t.repoName, deleteBranchName)
	if err != nil {
		return fmt.Errorf("error while deleting branch %s: %v", deleteBranchName, err)
	}
	log.Printf("deleted branch %s", deleteBranchName)

	return nil
}
