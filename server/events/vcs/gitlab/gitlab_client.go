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

package gitlab

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/runatlantis/atlantis/server/events/vcs/common"

	version "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/logging"

	"github.com/runatlantis/atlantis/server/events/models"
	gitlab "github.com/xanzy/go-gitlab"
)

type Client struct {
	Client *gitlab.Client
	// Version is set to the server version.
	Version *version.Version
}

// commonMarkSupported is a version constraint that is true when this version of
// GitLab supports CommonMark, a markdown specification.
// See https://about.gitlab.com/2018/07/22/gitlab-11-1-released/
var commonMarkSupported = MustConstraint(">=11.1")

// gitlabClientUnderTest is true if we're running under go test.
var gitlabClientUnderTest = false

// NewClient returns a valid GitLab client.
func NewClient(hostname string, token string, logger *logging.SimpleLogger) (*Client, error) {
	client := &Client{
		Client: gitlab.NewClient(nil, token),
	}

	// If not using gitlab.com we need to set the URL to the API.
	if hostname != "gitlab.com" {
		// We assume the url will be over HTTPS if the user doesn't specify a scheme.
		absoluteURL := hostname
		if !strings.HasPrefix(hostname, "http://") && !strings.HasPrefix(hostname, "https://") {
			absoluteURL = "https://" + absoluteURL
		}

		url, err := url.Parse(absoluteURL)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing URL %q", absoluteURL)
		}

		// Warn if this hostname isn't resolvable. The GitLab client
		// doesn't give good error messages in this case.
		ips, err := net.LookupIP(url.Hostname())
		if err != nil {
			logger.Warn("unable to resolve %q: %s", url.Hostname(), err)
		} else if len(ips) == 0 {
			logger.Warn("found no IPs while resolving %q", url.Hostname())
		}

		// Now we're ready to construct the client.
		absoluteURL = strings.TrimSuffix(absoluteURL, "/")
		apiURL := fmt.Sprintf("%s/api/v4/", absoluteURL)
		if err := client.Client.SetBaseURL(apiURL); err != nil {
			return nil, errors.Wrapf(err, "setting GitLab API URL: %s", apiURL)
		}
	}

	// Determine which version of GitLab is running.
	if !gitlabClientUnderTest {
		var err error
		client.Version, err = client.GetVersion()
		if err != nil {
			return nil, err
		}
		logger.Info("determined GitLab is running version %s", client.Version.String())
	}

	return client, nil
}

// GetModifiedFiles returns the names of files that were modified in the merge request
// relative to the repo root, e.g. parent/child/file.txt.
func (g *Client) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	const maxPerPage = 100
	var files []string
	nextPage := 1
	// Constructing the api url by hand so we can do pagination.
	apiURL := fmt.Sprintf("projects/%s/merge_requests/%d/changes", url.QueryEscape(repo.FullName), pull.Num)
	for {
		opts := gitlab.ListOptions{
			Page:    nextPage,
			PerPage: maxPerPage,
		}
		req, err := g.Client.NewRequest("GET", apiURL, opts, nil)
		if err != nil {
			return nil, err
		}
		mr := new(gitlab.MergeRequest)
		resp, err := g.Client.Do(req, mr)
		if err != nil {
			return nil, err
		}

		for _, f := range mr.Changes {
			files = append(files, f.NewPath)

			// If the file was renamed, we'll want to run plan in the directory
			// it was moved from as well.
			if f.RenamedFile {
				files = append(files, f.OldPath)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		nextPage = resp.NextPage
	}

	return files, nil
}

// CreateComment creates a comment on the merge request.
func (g *Client) CreateComment(repo models.Repo, pullNum int, comment string) error {
	_, _, err := g.Client.Notes.CreateMergeRequestNote(repo.FullName, pullNum, &gitlab.CreateMergeRequestNoteOptions{Body: gitlab.String(comment)})
	return err
}

func (g *Client) HidePrevPlanComments(repo models.Repo, pullNum int) error {
	return nil
}

// PullIsApproved returns true if the merge request was approved.
func (g *Client) PullIsApproved(repo models.Repo, pull models.PullRequest) (bool, error) {
	approvals, _, err := g.Client.MergeRequests.GetMergeRequestApprovals(repo.FullName, pull.Num)
	if err != nil {
		return false, err
	}
	if approvals.ApprovalsLeft > 0 {
		return false, nil
	}
	return true, nil
}

// PullIsMergeable returns true if the merge request can be merged.
// In GitLab, there isn't a single field that tells us if the pull request is
// mergeable so for now we check the merge_status and approvals_before_merge
// fields. We aren't checking if there are unresolved discussions or failing
// pipelines because those only block merges if the repo is set to require that.
// In order to check if the repo required these, we'd need to make another API
// call to get the repo settings. For now I'm going to leave this as is and if
// some users require checking this as well then we can revisit.
// It's also possible that GitLab implements their own "mergeable" field in
// their API in the future.
// See:
// - https://gitlab.com/gitlab-org/gitlab-ee/issues/3169
// - https://gitlab.com/gitlab-org/gitlab-ce/issues/42344
func (g *Client) PullIsMergeable(repo models.Repo, pull models.PullRequest) (bool, error) {
	mr, _, err := g.Client.MergeRequests.GetMergeRequest(repo.FullName, pull.Num, nil)
	if err != nil {
		return false, err
	}
	if mr.MergeStatus == "can_be_merged" && mr.ApprovalsBeforeMerge <= 0 {
		return true, nil
	}
	return false, nil
}

// UpdateStatus updates the build status of a commit.
func (g *Client) UpdateStatus(repo models.Repo, pull models.PullRequest, state models.CommitStatus, src string, description string, url string) error {
	gitlabState := gitlab.Failed
	switch state {
	case models.PendingCommitStatus:
		gitlabState = gitlab.Pending
	case models.FailedCommitStatus:
		gitlabState = gitlab.Failed
	case models.SuccessCommitStatus:
		gitlabState = gitlab.Success
	}
	_, _, err := g.Client.Commits.SetCommitStatus(repo.FullName, pull.HeadCommit, &gitlab.SetCommitStatusOptions{
		State:       gitlabState,
		Context:     gitlab.String(src),
		Description: gitlab.String(description),
		TargetURL:   &url,
	})
	return err
}

func (g *Client) GetMergeRequest(repoFullName string, pullNum int) (*gitlab.MergeRequest, error) {
	mr, _, err := g.Client.MergeRequests.GetMergeRequest(repoFullName, pullNum, nil)
	return mr, err
}

// MergePull merges the merge request.
func (g *Client) MergePull(pull models.PullRequest) error {
	commitMsg := common.AutomergeCommitMsg
	_, _, err := g.Client.MergeRequests.AcceptMergeRequest(
		pull.BaseRepo.FullName,
		pull.Num,
		&gitlab.AcceptMergeRequestOptions{
			MergeCommitMessage: &commitMsg,
		})
	return errors.Wrap(err, "unable to merge merge request, it may not be in a mergeable state")
}

// MarkdownPullLink specifies the string used in a pull request comment to reference another pull request.
func (g *Client) MarkdownPullLink(pull models.PullRequest) (string, error) {
	return fmt.Sprintf("!%d", pull.Num), nil
}

// GetVersion returns the version of the Gitlab server this client is using.
func (g *Client) GetVersion() (*version.Version, error) {
	req, err := g.Client.NewRequest("GET", "/version", nil, nil)
	if err != nil {
		return nil, err
	}
	versionResp := new(gitlab.Version)
	_, err = g.Client.Do(req, versionResp)
	if err != nil {
		return nil, err
	}
	// We need to strip any "-ee" or similar from the resulting version because go-version
	// uses that in its constraints and it breaks the comparison we're trying
	// to do for Common Mark.
	split := strings.Split(versionResp.Version, "-")
	parsedVersion, err := version.NewVersion(split[0])
	if err != nil {
		return nil, errors.Wrapf(err, "parsing response to /version: %q", versionResp.Version)
	}
	return parsedVersion, nil
}

// SupportsCommonMark returns true if the version of Gitlab this client is
// using supports the CommonMark markdown format.
func (g *Client) SupportsCommonMark() bool {
	// This function is called even if we didn't construct a gitlab client
	// so we need to handle that case.
	if g == nil {
		return false
	}

	return commonMarkSupported.Check(g.Version)
}

// MustConstraint returns a constraint. It panics on error.
func MustConstraint(constraint string) version.Constraints {
	c, err := version.NewConstraint(constraint)
	if err != nil {
		panic(err)
	}
	return c
}
