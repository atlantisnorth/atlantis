package bitbucketcloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
	validator "gopkg.in/go-playground/validator.v9"
)

type Client struct {
	HTTPClient  *http.Client
	Username    string
	Password    string
	BaseURL     string
	AtlantisURL string
}

// NewClient builds a bitbucket cloud client. atlantisURL is the
// URL for Atlantis that will be linked to from the build status icons. This
// linking is annoying because we don't have anywhere good to link but a URL is
// required.
func NewClient(httpClient *http.Client, username string, password string, atlantisURL string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		HTTPClient:  httpClient,
		Username:    username,
		Password:    password,
		BaseURL:     BaseURL,
		AtlantisURL: atlantisURL,
	}
}

// GetModifiedFiles returns the names of files that were modified in the merge request
// relative to the repo root, e.g. parent/child/file.txt.
func (b *Client) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	var files []string

	nextPageURL := fmt.Sprintf("%s/2.0/repositories/%s/pullrequests/%d/diffstat", b.BaseURL, repo.FullName, pull.Num)
	// We'll only loop 1000 times as a safety measure.
	maxLoops := 1000
	for i := 0; i < maxLoops; i++ {
		resp, err := b.makeRequest("GET", nextPageURL, nil)
		if err != nil {
			return nil, err
		}
		var diffStat DiffStat
		if err := json.Unmarshal(resp, &diffStat); err != nil {
			return nil, errors.Wrapf(err, "Could not parse response %q", string(resp))
		}
		if err := validator.New().Struct(diffStat); err != nil {
			return nil, errors.Wrapf(err, "API response %q was missing fields", string(resp))
		}
		for _, v := range diffStat.Values {
			if v.Old != nil {
				files = append(files, *v.Old.Path)
			}
			if v.New != nil {
				files = append(files, *v.New.Path)
			}
		}
		if diffStat.Next == nil || *diffStat.Next == "" {
			break
		}
		nextPageURL = *diffStat.Next
	}

	// Now ensure all files are unique.
	hash := make(map[string]bool)
	var unique []string
	for _, f := range files {
		if !hash[f] {
			unique = append(unique, f)
			hash[f] = true
		}
	}
	return unique, nil
}

// CreateComment creates a comment on the merge request.
func (b *Client) CreateComment(repo models.Repo, pullNum int, comment string, command string) error {
	// NOTE: I tried to find the maximum size of a comment for bitbucket.org but
	// I got up to 200k chars without issue so for now I'm not going to bother
	// to detect this.
	bodyBytes, err := json.Marshal(map[string]map[string]string{"content": {
		"raw": comment,
	}})
	if err != nil {
		return errors.Wrap(err, "json encoding")
	}
	path := fmt.Sprintf("%s/2.0/repositories/%s/pullrequests/%d/comments", b.BaseURL, repo.FullName, pullNum)
	_, err = b.makeRequest("POST", path, bytes.NewBuffer(bodyBytes))
	return err
}

func (b *Client) HidePrevCommandComments(repo models.Repo, pullNum int, command string) error {
	return nil
}

// PullIsApproved returns true if the merge request was approved.
func (b *Client) PullIsApproved(repo models.Repo, pull models.PullRequest) (bool, error) {
	path := fmt.Sprintf("%s/2.0/repositories/%s/pullrequests/%d", b.BaseURL, repo.FullName, pull.Num)
	resp, err := b.makeRequest("GET", path, nil)
	if err != nil {
		return false, err
	}
	var pullResp PullRequest
	if err := json.Unmarshal(resp, &pullResp); err != nil {
		return false, errors.Wrapf(err, "Could not parse response %q", string(resp))
	}
	if err := validator.New().Struct(pullResp); err != nil {
		return false, errors.Wrapf(err, "API response %q was missing fields", string(resp))
	}
	authorUUID := *pullResp.Author.UUID
	for _, participant := range pullResp.Participants {
		// Bitbucket allows the author to approve their own pull request. This
		// defeats the purpose of approvals so we don't count that approval.
		if *participant.Approved && *participant.User.UUID != authorUUID {
			return true, nil
		}
	}
	return false, nil
}

// PullIsMergeable returns true if the merge request has no conflicts and can be merged.
func (b *Client) PullIsMergeable(repo models.Repo, pull models.PullRequest) (bool, error) {
	nextPageURL := fmt.Sprintf("%s/2.0/repositories/%s/pullrequests/%d/diffstat", b.BaseURL, repo.FullName, pull.Num)
	// We'll only loop 1000 times as a safety measure.
	maxLoops := 1000
	for i := 0; i < maxLoops; i++ {
		resp, err := b.makeRequest("GET", nextPageURL, nil)
		if err != nil {
			return false, err
		}
		var diffStat DiffStat
		if err := json.Unmarshal(resp, &diffStat); err != nil {
			return false, errors.Wrapf(err, "Could not parse response %q", string(resp))
		}
		if err := validator.New().Struct(diffStat); err != nil {
			return false, errors.Wrapf(err, "API response %q was missing fields", string(resp))
		}
		for _, v := range diffStat.Values {
			// These values are undocumented, found via manual testing.
			if *v.Status == "merge conflict" || *v.Status == "local deleted" {
				return false, nil
			}
		}
		if diffStat.Next == nil || *diffStat.Next == "" {
			break
		}
		nextPageURL = *diffStat.Next
	}
	return true, nil
}

// UpdateStatus updates the status of a commit.
func (b *Client) UpdateStatus(repo models.Repo, pull models.PullRequest, status models.CommitStatus, src string, description string, url string) error {
	bbState := "FAILED"
	switch status {
	case models.PendingCommitStatus:
		bbState = "INPROGRESS"
	case models.SuccessCommitStatus:
		bbState = "SUCCESSFUL"
	case models.FailedCommitStatus:
		bbState = "FAILED"
	}

	// URL is a required field for bitbucket statuses. We default to the
	// Atlantis server's URL.
	if url == "" {
		url = b.AtlantisURL
	}

	bodyBytes, err := json.Marshal(map[string]string{
		"key":         src,
		"url":         url,
		"state":       bbState,
		"description": description,
	})

	path := fmt.Sprintf("%s/2.0/repositories/%s/commit/%s/statuses/build", b.BaseURL, repo.FullName, pull.HeadCommit)
	if err != nil {
		return errors.Wrap(err, "json encoding")
	}
	_, err = b.makeRequest("POST", path, bytes.NewBuffer(bodyBytes))
	return err
}

// MergePull merges the pull request.
func (b *Client) MergePull(pull models.PullRequest, pullOptions models.PullRequestOptions) error {
	path := fmt.Sprintf("%s/2.0/repositories/%s/pullrequests/%d/merge", b.BaseURL, pull.BaseRepo.FullName, pull.Num)
	_, err := b.makeRequest("POST", path, nil)
	return err
}

// MarkdownPullLink specifies the character used in a pull request comment.
func (b *Client) MarkdownPullLink(pull models.PullRequest) (string, error) {
	return fmt.Sprintf("#%d", pull.Num), nil
}

// prepRequest adds auth and necessary headers.
func (b *Client) prepRequest(method string, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(b.Username, b.Password)
	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	// Add this header to disable CSRF checks.
	// See https://confluence.atlassian.com/cloudkb/xsrf-check-failed-when-calling-cloud-apis-826874382.html
	req.Header.Add("X-Atlassian-Token", "no-check")
	return req, nil
}

func (b *Client) makeRequest(method string, path string, reqBody io.Reader) ([]byte, error) {
	req, err := b.prepRequest(method, path, reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "constructing request")
	}
	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck
	requestStr := fmt.Sprintf("%s %s", method, path)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("making request %q unexpected status code: %d, body: %s", requestStr, resp.StatusCode, string(respBody))
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "reading response from request %q", requestStr)
	}
	return respBody, nil
}

// GetTeamNamesForUser returns the names of the teams or groups that the user belongs to (in the organization the repository belongs to).
func (b *Client) GetTeamNamesForUser(repo models.Repo, user models.User) ([]string, error) {
	return nil, nil
}

func (b *Client) SupportsSingleFileDownload(models.Repo) bool {
	return false
}

// DownloadRepoConfigFile return `atlantis.yaml` content from VCS (which support fetch a single file from repository)
// The first return value indicate that repo contain atlantis.yaml or not
// if BaseRepo had one repo config file, its content will placed on the second return value
func (b *Client) DownloadRepoConfigFile(pull models.PullRequest) (bool, []byte, error) {
	return false, []byte{}, fmt.Errorf("Not Implemented")
}
