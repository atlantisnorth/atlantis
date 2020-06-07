package azuredevops

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcdafydd/go-azuredevops/azuredevops"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs/common"
)

// Client represents an Azure DevOps VCS client
type Client struct {
	Client *azuredevops.Client
	ctx    context.Context
}

// NewClient returns a valid Azure DevOps client.
func NewClient(hostname string, token string) (*Client, error) {
	tp := azuredevops.BasicAuthTransport{
		Username: "",
		Password: strings.TrimSpace(token),
	}
	httpClient := tp.Client()
	httpClient.Timeout = time.Second * 10
	var adClient, err = azuredevops.NewClient(httpClient)
	if err != nil {
		return nil, err
	}

	if hostname != "dev.azure.com" {
		baseURL := fmt.Sprintf("https://%s/", hostname)
		base, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid azure devops hostname trying to parse %s", baseURL)
		}
		adClient.BaseURL = *base
	}

	client := &Client{
		Client: adClient,
		ctx:    context.Background(),
	}

	return client, nil
}

// GetModifiedFiles returns the names of files that were modified in the merge request
// relative to the repo root, e.g. parent/child/file.txt.
func (g *Client) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	var files []string

	opts := azuredevops.PullRequestGetOptions{
		IncludeWorkItemRefs: true,
	}
	owner, project, repoName := SplitAzureDevopsRepoFullName(repo.FullName)
	commitIDResponse, _, _ := g.Client.PullRequests.GetWithRepo(g.ctx, owner, project, repoName, pull.Num, &opts)

	commitID := commitIDResponse.GetLastMergeSourceCommit().GetCommitID()

	r, _, _ := g.Client.Git.GetChanges(g.ctx, owner, project, repoName, commitID)

	for _, change := range r.Changes {
		item := change.GetItem()
		// Convert the path to a relative path from the repo's root.
		relativePath := filepath.Clean("./" + item.GetPath())
		files = append(files, relativePath)

		// If the file was renamed, we'll want to run plan in the directory
		// it was moved from as well.
		changeType := azuredevops.Rename.String()
		if change.ChangeType == &changeType {
			// Convert the path to a relative path from the repo's root.
			relativePath = filepath.Clean("./" + change.GetSourceServerItem())
			files = append(files, relativePath)
		}
	}

	return files, nil
}

// CreateComment creates a comment on a pull request.
//
// If comment length is greater than the max comment length we split into
// multiple comments.
func (g *Client) CreateComment(repo models.Repo, pullNum int, comment string) error {
	sepEnd := "\n```\n</details>" +
		"\n<br>\n\n**Warning**: Output length greater than max comment size. Continued in next comment."
	sepStart := "Continued from previous comment.\n<details><summary>Show Output</summary>\n\n" +
		"```diff\n"

	// maxCommentLength is the maximum number of chars allowed in a single comment
	// This length was copied from the Github client - haven't found documentation
	// or tested limit in Azure DevOps.
	const maxCommentLength = 65536

	comments := common.SplitComment(comment, maxCommentLength, sepEnd, sepStart)
	owner, project, repoName := SplitAzureDevopsRepoFullName(repo.FullName)

	for _, c := range comments {
		commentType := "text"
		parentCommentID := 0

		prComment := azuredevops.Comment{
			CommentType:     &commentType,
			Content:         &c,
			ParentCommentID: &parentCommentID,
		}
		prComments := []*azuredevops.Comment{&prComment}
		body := azuredevops.GitPullRequestCommentThread{
			Comments: prComments,
		}
		_, _, err := g.Client.PullRequests.CreateComments(g.ctx, owner, project, repoName, pullNum, &body)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *Client) HidePrevPlanComments(repo models.Repo, pullNum int) error {
	return nil
}

// PullIsApproved returns true if the merge request was approved by another reviewer.
// https://docs.microsoft.com/en-us/azure/devops/repos/git/branch-policies?view=azure-devops#require-a-minimum-number-of-reviewers
func (g *Client) PullIsApproved(repo models.Repo, pull models.PullRequest) (bool, error) {
	owner, project, repoName := SplitAzureDevopsRepoFullName(repo.FullName)

	opts := azuredevops.PullRequestGetOptions{
		IncludeWorkItemRefs: true,
	}
	adPull, _, err := g.Client.PullRequests.GetWithRepo(g.ctx, owner, project, repoName, pull.Num, &opts)
	if err != nil {
		return false, errors.Wrap(err, "getting pull request")
	}

	for _, review := range adPull.Reviewers {
		if review == nil {
			continue
		}

		if review.IdentityRef.GetUniqueName() == adPull.GetCreatedBy().GetUniqueName() {
			continue
		}

		if review.GetVote() == azuredevops.VoteApproved || review.GetVote() == azuredevops.VoteApprovedWithSuggestions {
			return true, nil
		}
	}

	return false, nil
}

// PullIsMergeable returns true if the merge request can be merged.
func (g *Client) PullIsMergeable(repo models.Repo, pull models.PullRequest) (bool, error) {
	owner, project, repoName := SplitAzureDevopsRepoFullName(repo.FullName)

	opts := azuredevops.PullRequestGetOptions{IncludeWorkItemRefs: true}
	adPull, _, err := g.Client.PullRequests.GetWithRepo(g.ctx, owner, project, repoName, pull.Num, &opts)
	if err != nil {
		return false, errors.Wrap(err, "getting pull request")
	}

	if *adPull.MergeStatus != azuredevops.MergeSucceeded.String() {
		return false, nil
	}

	if *adPull.IsDraft {
		return false, nil
	}

	if *adPull.Status != azuredevops.PullActive.String() {
		return false, nil
	}

	projectID := *adPull.Repository.Project.ID
	artifactID := g.Client.PolicyEvaluations.GetPullRequestArtifactID(projectID, pull.Num)
	policyEvaluations, _, err := g.Client.PolicyEvaluations.List(g.ctx, owner, project, artifactID, &azuredevops.PolicyEvaluationsListOptions{})
	if err != nil {
		return false, errors.Wrap(err, "getting policy evaluations")
	}

	for _, policyEvaluation := range policyEvaluations {
		if !*policyEvaluation.Configuration.IsEnabled || *policyEvaluation.Configuration.IsDeleted {
			continue
		}

		// Ignore the Atlantis status, even if its set as a blocker.
		// This status should not be considered when evaluating if the pull request can be applied.
		settings := (policyEvaluation.Configuration.Settings).(map[string]interface{})
		if status, ok := settings["statusName"]; ok && status == "atlantis/apply" {
			continue
		}

		if *policyEvaluation.Configuration.IsBlocking && *policyEvaluation.Status != azuredevops.PolicyEvaluationApproved {
			return false, nil
		}
	}

	return true, nil
}

// GetPullRequest returns the pull request.
func (g *Client) GetPullRequest(repo models.Repo, num int) (*azuredevops.GitPullRequest, error) {
	opts := azuredevops.PullRequestGetOptions{
		IncludeWorkItemRefs: true,
	}
	owner, project, repoName := SplitAzureDevopsRepoFullName(repo.FullName)
	pull, _, err := g.Client.PullRequests.GetWithRepo(g.ctx, owner, project, repoName, num, &opts)
	return pull, err
}

// UpdateStatus updates the build status of a commit.
func (g *Client) UpdateStatus(repo models.Repo, pull models.PullRequest, state models.CommitStatus, src string, description string, url string) error {
	adState := azuredevops.GitError.String()
	switch state {
	case models.PendingCommitStatus:
		adState = azuredevops.GitPending.String()
	case models.SuccessCommitStatus:
		adState = azuredevops.GitSucceeded.String()
	case models.FailedCommitStatus:
		adState = azuredevops.GitFailed.String()
	}

	genreStr := "Atlantis Bot"
	status := azuredevops.GitPullRequestStatus{}
	status.Context = &azuredevops.GitStatusContext{
		Name:  &src,
		Genre: &genreStr,
	}
	status.Description = &description
	status.State = &adState
	if url != "" {
		status.TargetURL = &url
	}

	owner, project, repoName := SplitAzureDevopsRepoFullName(repo.FullName)

	opts := azuredevops.PullRequestListOptions{}
	source, resp, err := g.Client.PullRequests.Get(g.ctx, owner, project, pull.Num, &opts)
	if err != nil {
		return errors.Wrap(err, "getting pull request")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Wrapf(err, "http response code %d getting pull request", resp.StatusCode)
	}
	if source.GetSupportsIterations() {
		opts := azuredevops.PullRequestIterationsListOptions{}
		iterations, resp, err := g.Client.PullRequests.ListIterations(g.ctx, owner, project, repoName, pull.Num, &opts)
		if err != nil {
			return errors.Wrap(err, "listing pull request iterations")
		}
		if resp.StatusCode != http.StatusOK {
			return errors.Wrapf(err, "http response code %d listing pull request iterations", resp.StatusCode)
		}
		for _, iteration := range iterations {
			if sourceRef := iteration.GetSourceRefCommit(); sourceRef != nil {
				if *sourceRef.CommitID == pull.HeadCommit {
					status.IterationID = iteration.ID
					break
				}
			}
		}
		if iterationID := status.IterationID; iterationID != nil {
			if !(*iterationID >= 1) {
				return errors.New("supportsIterations was true but got invalid iteration ID or no matching iteration commit SHA was found")
			}
		}
	}
	_, resp, err = g.Client.PullRequests.CreateStatus(g.ctx, owner, project, repoName, pull.Num, &status)
	if err != nil {
		return errors.Wrap(err, "creating pull request status")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Wrapf(err, "http response code %d creating pull request status", resp.StatusCode)
	}
	return err
}

// MergePull merges the merge request using the default no fast-forward strategy
// If the user has set a branch policy that disallows no fast-forward, the merge will fail
// until we handle branch policies
// https://docs.microsoft.com/en-us/azure/devops/repos/git/branch-policies?view=azure-devops
func (g *Client) MergePull(pull models.PullRequest) error {
	descriptor := "Atlantis Terraform Pull Request Automation"
	i := "atlantis"
	imageURL := "https://github.com/runatlantis/atlantis/raw/master/runatlantis.io/.vuepress/public/hero.png"
	id := azuredevops.IdentityRef{
		Descriptor: &descriptor,
		ID:         &i,
		ImageURL:   &imageURL,
	}
	// Set default pull request completion options
	mcm := azuredevops.NoFastForward.String()
	twi := new(bool)
	*twi = true
	completionOpts := azuredevops.GitPullRequestCompletionOptions{
		BypassPolicy:            new(bool),
		BypassReason:            azuredevops.String(""),
		DeleteSourceBranch:      new(bool),
		MergeCommitMessage:      azuredevops.String(common.AutomergeCommitMsg),
		MergeStrategy:           &mcm,
		SquashMerge:             new(bool),
		TransitionWorkItems:     twi,
		TriggeredByAutoComplete: new(bool),
	}

	// Construct request body from supplied parameters
	mergePull := new(azuredevops.GitPullRequest)
	mergePull.AutoCompleteSetBy = &id
	mergePull.CompletionOptions = &completionOpts

	owner, project, repoName := SplitAzureDevopsRepoFullName(pull.BaseRepo.FullName)
	mergeResult, _, err := g.Client.PullRequests.Merge(
		g.ctx,
		owner,
		project,
		repoName,
		pull.Num,
		mergePull,
		completionOpts,
		id,
	)
	if err != nil {
		return errors.Wrap(err, "merging pull request")
	}
	if *mergeResult.MergeStatus != azuredevops.MergeSucceeded.String() {
		return fmt.Errorf("could not merge pull request: %s", mergeResult.GetMergeFailureMessage())
	}
	return nil
}

// MarkdownPullLink specifies the string used in a pull request comment to reference another pull request.
func (g *Client) MarkdownPullLink(pull models.PullRequest) (string, error) {
	return fmt.Sprintf("!%d", pull.Num), nil
}

// SplitAzureDevopsRepoFullName splits a repo full name up into its owner,
// repo and project name segments. If the repoFullName is malformed, may
// return empty strings for owner, repo, or project.  Azure DevOps uses
// repoFullName format owner/project/repo.
//
// Ex. runatlantis/atlantis => (runatlantis, atlantis)
//     gitlab/subgroup/runatlantis/atlantis => (gitlab/subgroup/runatlantis, atlantis)
//     azuredevops/project/atlantis => (azuredevops, project, atlantis)
func SplitAzureDevopsRepoFullName(repoFullName string) (owner string, project string, repo string) {
	firstSlashIdx := strings.Index(repoFullName, "/")
	lastSlashIdx := strings.LastIndex(repoFullName, "/")
	slashCount := strings.Count(repoFullName, "/")
	if lastSlashIdx == -1 || lastSlashIdx == len(repoFullName)-1 {
		return "", "", ""
	}
	if firstSlashIdx != lastSlashIdx && slashCount == 2 {
		return repoFullName[:firstSlashIdx],
			repoFullName[firstSlashIdx+1 : lastSlashIdx],
			repoFullName[lastSlashIdx+1:]
	}
	return repoFullName[:lastSlashIdx], "", repoFullName[lastSlashIdx+1:]
}
