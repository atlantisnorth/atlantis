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
	"context"
	"log"
	"os"
	"strings"

	"fmt"

	multierror "github.com/hashicorp/go-multierror"
)

var defaultAtlantisURL = "http://localhost:4141"
var projectTypes = []Project{
	{"standalone", "atlantis apply -d standalone"},
	{"standalone-with-workspace", "atlantis apply -d standalone-with-workspace -w staging"},
}

type Project struct {
	Name         string
	ApplyCommand string
}

func main() {

	githubUsername := os.Getenv("GITHUB_USERNAME")
	if githubUsername == "" {
		log.Fatalf("GITHUB_USERNAME cannot be empty")
	}
	githubToken := os.Getenv("GITHUB_PASSWORD")
	if githubToken == "" {
		log.Fatalf("GITHUB_PASSWORD cannot be empty")
	}
	atlantisURL := os.Getenv("ATLANTIS_URL")
	if atlantisURL == "" {
		atlantisURL = defaultAtlantisURL
	}
	// add /events to the url
	atlantisURL = fmt.Sprintf("%s/events", atlantisURL)
	ownerName := os.Getenv("GITHUB_REPO_OWNER_NAME")
	if ownerName == "" {
		ownerName = "runatlantis"
	}
	repoName := os.Getenv("GITHUB_REPO_NAME")
	if repoName == "" {
		repoName = "atlantis-tests"
	}
	// using https to clone the repo
	repoURL := fmt.Sprintf("https://%s:%s@github.com/%s/%s.git", githubUsername, githubToken, ownerName, repoName)
	cloneDirRoot := os.Getenv("CLONE_DIR")
	if cloneDirRoot == "" {
		cloneDirRoot = "/tmp/atlantis-tests"
	}

	// clean workspace
	log.Printf("cleaning workspace %s", cloneDirRoot)
	err := cleanDir(cloneDirRoot)
	if err != nil {
		log.Fatalf("failed to clean dir %q before cloning, attempting to continue: %v", cloneDirRoot, err)
	}

	// create github client
	tp := github.BasicAuthTransport{
		Username: strings.TrimSpace(githubUsername),
		Password: strings.TrimSpace(githubToken),
	}
	ghClient := github.NewClient(tp.Client())

	githubClient := &GithubClient{client: ghClient, ctx: context.Background(), username: githubUsername}

	// we create atlantis hook once for the repo, since the atlantis server can handle multiple requests
	log.Printf("creating atlantis webhook with %s url", atlantisURL)
	hookID, err := createAtlantisWebhook(githubClient, ownerName, repoName, atlantisURL)
	if err != nil {
		log.Fatalf("error creating atlantis webhook: %v", err)
	}

	// create e2e test
	e2e := E2ETester{
		githubClient: githubClient,
		repoURL:      repoURL,
		ownerName:    ownerName,
		repoName:     repoName,
		hookID:       hookID,
		cloneDirRoot: cloneDirRoot,
	}

	// start e2e tests
	results, err := startTests(e2e)
	log.Printf("Test Results\n---------------------------\n")
	for _, result := range results {
		fmt.Printf("Project Type: %s \n", result.projectType)
		fmt.Printf("Pull Request Link: %s \n", result.githubPullRequestURL)
		fmt.Printf("Atlantis Run Status: %s \n", result.testResult)
		fmt.Println("---------------------------")
	}
	if err != nil {
		log.Fatalf(fmt.Sprintf("%s", err))
	}

}

func createAtlantisWebhook(g *GithubClient, ownerName string, repoName string, hookURL string) (int64, error) {
	// create atlantis hook
	atlantisHook := &github.Hook{
		Events: []string{"issue_comment", "pull_request", "push"},
		Config: map[string]interface{}{
			"url":          hookURL,
			"content_type": "json",
		},
		Active: github.Bool(true),
	}

	// moved to github.go
	hook, _, err := g.client.Repositories.CreateHook(g.ctx, ownerName, repoName, atlantisHook)
	if err != nil {
		return 0, err
	}
	log.Println(hook.GetURL())

	return hook.GetID(), nil
}

func deleteAtlantisHook(g *GithubClient, ownerName string, repoName string, hookID int64) error {
	_, err := g.client.Repositories.DeleteHook(g.ctx, ownerName, repoName, hookID)
	if err != nil {
		return err
	}
	log.Printf("deleted webhook id %d", hookID)

	return nil
}

func cleanDir(path string) error {
	return os.RemoveAll(path)
}

func startTests(e2e E2ETester) ([]*E2EResult, error) {
	var testResults []*E2EResult
	var testErrors *multierror.Error
	// delete webhook when we are done running tests
	defer deleteAtlantisHook(e2e.githubClient, e2e.ownerName, e2e.repoName, e2e.hookID) // nolint: errcheck

	for _, projectType := range projectTypes {
		log.Printf("starting e2e test for project type %q", projectType.Name)
		e2e.projectType = projectType
		// start e2e test
		result, err := e2e.Start()
		testResults = append(testResults, result)
		testErrors = multierror.Append(testErrors, err)
	}

	return testResults, testErrors.ErrorOrNil()
}
