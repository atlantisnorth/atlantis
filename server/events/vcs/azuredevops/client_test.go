package azuredevops_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	devops "github.com/mcdafydd/go-azuredevops/azuredevops"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs/azuredevops"
	"github.com/runatlantis/atlantis/server/events/vcs/azuredevops/fixtures"
	. "github.com/runatlantis/atlantis/testing"
)

func TestClient_MergePull(t *testing.T) {
	cases := []struct {
		description string
		response    string
		code        int
		expErr      string
	}{
		{
			"success",
			adMergeSuccess,
			200,
			"",
		},
		{
			"405",
			`{"message":"405 Method Not Allowed"}`,
			405,
			"405 {message: 405 Method Not Allowed}",
		},
		{
			"406",
			`{"message":"406 Branch cannot be merged"}`,
			406,
			"406 {message: 406 Branch cannot be merged}",
		},
	}

	// Set default pull request completion options
	mcm := devops.NoFastForward.String()
	twi := new(bool)
	*twi = true
	completionOptions := devops.GitPullRequestCompletionOptions{
		BypassPolicy:            new(bool),
		BypassReason:            devops.String(""),
		DeleteSourceBranch:      new(bool),
		MergeCommitMessage:      devops.String("commit message"),
		MergeStrategy:           &mcm,
		SquashMerge:             new(bool),
		TransitionWorkItems:     twi,
		TriggeredByAutoComplete: new(bool),
	}

	id := devops.IdentityRef{}
	pull := devops.GitPullRequest{
		PullRequestID: devops.Int(22),
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					// The first request should hit this URL.
					case "/owner/project/_apis/git/repositories/repo/pullrequests/22?api-version=5.1-preview.1":
						w.WriteHeader(c.code)
						w.Write([]byte(c.response)) // nolint: errcheck
					default:
						t.Errorf("got unexpected request at %q", r.RequestURI)
						http.Error(w, "not found", http.StatusNotFound)
					}
				}))

			testServerURL, err := url.Parse(testServer.URL)
			Ok(t, err)
			client, err := azuredevops.NewClient(testServerURL.Host, "token")
			Ok(t, err)
			defer DisableSSLVerification()()

			merge, _, err := client.Client.PullRequests.Merge(context.Background(),
				"owner",
				"project",
				"repo",
				pull.GetPullRequestID(),
				&pull,
				completionOptions,
				id,
			)

			if err != nil {
				fmt.Printf("Merge failed: %+v\n", err)
				return
			}
			fmt.Printf("Successfully merged pull request: %+v\n", merge)

			err = client.MergePull(models.PullRequest{
				Num: 22,
				BaseRepo: models.Repo{
					FullName: "owner/project/repo",
					Owner:    "owner",
					Name:     "repo",
				},
			})
			if c.expErr == "" {
				Ok(t, err)
			} else {
				ErrContains(t, c.expErr, err)
				ErrContains(t, "unable to merge merge request, it may not be in a mergeable state", err)
			}
		})
	}
}

func TestClient_UpdateStatus(t *testing.T) {
	cases := []struct {
		status             models.CommitStatus
		expState           string
		supportsIterations bool
	}{
		{
			models.PendingCommitStatus,
			"pending",
			true,
		},
		{
			models.SuccessCommitStatus,
			"succeeded",
			true,
		},
		{
			models.FailedCommitStatus,
			"failed",
			true,
		},
		{
			models.PendingCommitStatus,
			"pending",
			false,
		},
		{
			models.SuccessCommitStatus,
			"succeeded",
			false,
		},
		{
			models.FailedCommitStatus,
			"failed",
			false,
		},
	}
	iterResponse := `{"count": 2, "value": [{"id": 1, "sourceRefCommit": { "commitId": "oldsha"}}, {"id": 2, "sourceRefCommit": { "commitId": "sha"}}]}`
	prResponse := `{"supportsIterations": %t}`
	partResponse := `{"context":{"genre":"Atlantis Bot","name":"src"},"description":"description","state":"%s","targetUrl":"https://google.com"`

	for _, c := range cases {
		prResponse := fmt.Sprintf(prResponse, c.supportsIterations)
		t.Run(c.expState, func(t *testing.T) {
			gotRequest := false
			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					case "/owner/project/_apis/git/repositories/repo/pullrequests/22/statuses?api-version=5.1-preview.1":
						gotRequest = true
						defer r.Body.Close() // nolint: errcheck
						body, err := ioutil.ReadAll(r.Body)
						Ok(t, err)
						exp := fmt.Sprintf(partResponse, c.expState)
						if c.supportsIterations == true {
							exp = fmt.Sprintf("%s%s}\n", exp, `,"iterationId":2`)
						} else {
							exp = fmt.Sprintf("%s}\n", exp)
						}
						Equals(t, exp, string(body))
						w.Write([]byte(exp)) // nolint: errcheck
					case "/owner/project/_apis/git/repositories/repo/pullrequests/22/iterations?api-version=5.1":
						w.Write([]byte(iterResponse)) // nolint: errcheck
					case "/owner/project/_apis/git/pullrequests/22?api-version=5.1-preview.1":
						w.Write([]byte(prResponse)) // nolint: errcheck
					default:
						t.Errorf("got unexpected request at %q", r.RequestURI)
						http.Error(w, "not found", http.StatusNotFound)
					}
				}))

			testServerURL, err := url.Parse(testServer.URL)
			Ok(t, err)
			client, err := azuredevops.NewClient(testServerURL.Host, "token")
			Ok(t, err)
			defer DisableSSLVerification()()

			repo := models.Repo{
				FullName: "owner/project/repo",
				Owner:    "owner",
				Name:     "repo",
			}
			err = client.UpdateStatus(repo, models.PullRequest{
				Num:        22,
				BaseRepo:   repo,
				HeadCommit: "sha",
			}, c.status, "src", "description", "https://google.com")
			Ok(t, err)
			Assert(t, gotRequest, "expected to get the request")
		})
	}
}

// GetModifiedFiles should make multiple requests if more than one page
// and concat results.
func TestClient_GetModifiedFiles(t *testing.T) {
	itemRespTemplate := `{
		"changes": [
	{
		"item": {
			"gitObjectType": "blob",
			"path": "%s",
			"url": "https://dev.azure.com/fabrikam/_apis/git/repositories/278d5cd2-584d-4b63-824a-2ba458937249/items/MyWebSite/MyWebSite/%s?versionType=Commit"
		},
		"changeType": "add"
	},
	{
		"item": {
			"gitObjectType": "blob",
			"path": "%s",
			"url": "https://dev.azure.com/fabrikam/_apis/git/repositories/278d5cd2-584d-4b63-824a-2ba458937249/items/MyWebSite/MyWebSite/%s?versionType=Commit"
		},
		"changeType": "add"
	}
]}`
	resp := fmt.Sprintf(itemRespTemplate, "/file1.txt", "/file1.txt", "/file2.txt", "/file2.txt")
	testServer := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.RequestURI {
			// The first request should hit this URL.
			case "/owner/project/_apis/git/repositories/repo/pullrequests/1?api-version=5.1-preview.1&includeWorkItemRefs=true":
				w.Write([]byte(fixtures.ADPullJSON)) // nolint: errcheck
			// The second should hit this URL.
			case "/owner/project/_apis/git/repositories/repo/commits/b60280bc6e62e2f880f1b63c1e24987664d3bda3/changes?api-version=5.1-preview.1":
				// We write a header that means there's an additional page.
				w.Write([]byte(resp)) // nolint: errcheck
				return
			default:
				t.Errorf("got unexpected request at %q", r.RequestURI)
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}))

	testServerURL, err := url.Parse(testServer.URL)
	Ok(t, err)
	client, err := azuredevops.NewClient(testServerURL.Host, "token")
	Ok(t, err)
	defer DisableSSLVerification()()

	files, err := client.GetModifiedFiles(models.Repo{
		FullName:          "owner/project/repo",
		Owner:             "owner",
		Name:              "repo",
		CloneURL:          "",
		SanitizedCloneURL: "",
		VCSHost: models.VCSHost{
			Type:     models.AzureDevops,
			Hostname: "dev.azure.com",
		},
	}, models.PullRequest{
		Num: 1,
	})
	Ok(t, err)
	Equals(t, []string{"file1.txt", "file2.txt"}, files)
}

func TestClient_PullIsMergeable(t *testing.T) {
	cases := []struct {
		testName     string
		mergeStatus  string
		policyStatus string
		expMergeable bool
	}{
		{
			"merge conflicts",
			devops.MergeConflicts.String(),
			"approved",
			false,
		},
		{
			"rejected policy status",
			devops.MergeSucceeded.String(),
			"rejected",
			false,
		},
		{
			"merge succeeded",
			devops.MergeSucceeded.String(),
			"approved",
			true,
		},
	}

	jsonPullRequestBytes, err := ioutil.ReadFile("fixtures/azuredevops-pr.json")
	Ok(t, err)

	jsonPolicyEvaluationBytes, err := ioutil.ReadFile("fixtures/azuredevops-policyevaluations.json")
	Ok(t, err)

	pullRequestBody := string(jsonPullRequestBytes)
	policyEvaluationsBody := string(jsonPolicyEvaluationBytes)

	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			pullRequestResponse := strings.Replace(pullRequestBody, `"mergeStatus": "notSet"`, fmt.Sprintf(`"mergeStatus": "%s"`, c.mergeStatus), 1)
			policyEvaluationsResponse := strings.Replace(policyEvaluationsBody, `"status": "approved"`, fmt.Sprintf(`"status": "%s"`, c.policyStatus), 1)

			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					case "/owner/project/_apis/git/repositories/repo/pullrequests/1?api-version=5.1-preview.1&includeWorkItemRefs=true":
						w.Write([]byte(pullRequestResponse)) // nolint: errcheck
						return
					case "/owner/project/_apis/policy/evaluations?api-version=5.1-preview&artifactId=vstfs%3A%2F%2F%2FCodeReview%2FCodeReviewId%2F33333333-3333-3333-333333333333%2F1":
						w.Write([]byte(policyEvaluationsResponse)) // nolint: errcheck
						return
					default:
						t.Errorf("got unexpected request at %q", r.RequestURI)
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
				}))

			testServerURL, err := url.Parse(testServer.URL)
			Ok(t, err)

			client, err := azuredevops.NewClient(testServerURL.Host, "token")
			Ok(t, err)

			defer DisableSSLVerification()()

			actMergeable, err := client.PullIsMergeable(models.Repo{
				FullName:          "owner/project/repo",
				Owner:             "owner",
				Name:              "repo",
				CloneURL:          "",
				SanitizedCloneURL: "",
				VCSHost: models.VCSHost{
					Type:     models.AzureDevops,
					Hostname: "dev.azure.com",
				},
			}, models.PullRequest{
				Num: 1,
			})
			Ok(t, err)
			Equals(t, c.expMergeable, actMergeable)
		})
	}
}

func TestClient_PullIsApproved(t *testing.T) {
	cases := []struct {
		testName           string
		reviewerUniqueName string
		reviewerVote       int
		expApproved        bool
	}{
		{
			"approved",
			"atlantis.reviewer@example.com",
			devops.VoteApproved,
			true,
		},
		{
			"approved with suggestions",
			"atlantis.reviewer@example.com",
			devops.VoteApprovedWithSuggestions,
			true,
		},
		{
			"no vote",
			"atlantis.reviewer@example.com",
			devops.VoteNone,
			false,
		},
		{
			"vote waiting for author",
			"atlantis.reviewer@example.com",
			devops.VoteWaitingForAuthor,
			false,
		},
		{
			"vote rejected",
			"atlantis.reviewer@example.com",
			devops.VoteRejected,
			false,
		},
		{
			"approved only by author",
			"atlantis.author@example.com",
			devops.VoteApproved,
			false,
		},
	}

	jsBytes, err := ioutil.ReadFile("fixtures/azuredevops-pr.json")
	Ok(t, err)

	json := string(jsBytes)
	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			response := strings.Replace(json, `"vote": 0,`, fmt.Sprintf(`"vote": %d,`, c.reviewerVote), 1)
			response = strings.Replace(response, "atlantis.reviewer@example.com", c.reviewerUniqueName, 1)

			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					case "/owner/project/_apis/git/repositories/repo/pullrequests/1?api-version=5.1-preview.1&includeWorkItemRefs=true":
						w.Write([]byte(response)) // nolint: errcheck
						return
					default:
						t.Errorf("got unexpected request at %q", r.RequestURI)
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
				}))

			testServerURL, err := url.Parse(testServer.URL)
			Ok(t, err)

			client, err := azuredevops.NewClient(testServerURL.Host, "token")
			Ok(t, err)

			defer DisableSSLVerification()()

			actApproved, err := client.PullIsApproved(models.Repo{
				FullName:          "owner/project/repo",
				Owner:             "owner",
				Name:              "repo",
				CloneURL:          "",
				SanitizedCloneURL: "",
				VCSHost: models.VCSHost{
					Type:     models.AzureDevops,
					Hostname: "dev.azure.com",
				},
			}, models.PullRequest{
				Num: 1,
			})
			Ok(t, err)
			Equals(t, c.expApproved, actApproved)
		})
	}
}

func TestClient_GetPullRequest(t *testing.T) {
	// Use a real Azure DevOps json response and edit the mergeable_state field.
	jsBytes, err := ioutil.ReadFile("fixtures/azuredevops-pr.json")
	Ok(t, err)
	response := string(jsBytes)

	t.Run("get pull request", func(t *testing.T) {
		testServer := httptest.NewTLSServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.RequestURI {
				case "/owner/project/_apis/git/repositories/repo/pullrequests/1?api-version=5.1-preview.1&includeWorkItemRefs=true":
					w.Write([]byte(response)) // nolint: errcheck
					return
				default:
					t.Errorf("got unexpected request at %q", r.RequestURI)
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
			}))
		testServerURL, err := url.Parse(testServer.URL)
		Ok(t, err)
		client, err := azuredevops.NewClient(testServerURL.Host, "token")
		Ok(t, err)
		defer DisableSSLVerification()()

		_, err = client.GetPullRequest(models.Repo{
			FullName:          "owner/project/repo",
			Owner:             "owner",
			Name:              "repo",
			CloneURL:          "",
			SanitizedCloneURL: "",
			VCSHost: models.VCSHost{
				Type:     models.AzureDevops,
				Hostname: "dev.azure.com",
			},
		}, 1)
		Ok(t, err)
	})
}

func TestClient_MarkdownPullLink(t *testing.T) {
	client, err := azuredevops.NewClient("hostname", "token")
	Ok(t, err)
	pull := models.PullRequest{Num: 1}
	s, _ := client.MarkdownPullLink(pull)
	exp := "!1"
	Equals(t, exp, s)
}

var adMergeSuccess = `{
	"status": "completed",
	"mergeStatus": "succeeded",
	"autoCompleteSetBy": {
					"id": "54d125f7-69f7-4191-904f-c5b96b6261c8",
					"displayName": "Jamal Hartnett",
					"uniqueName": "fabrikamfiber4@hotmail.com",
					"url": "https://vssps.dev.azure.com/fabrikam/_apis/Identities/54d125f7-69f7-4191-904f-c5b96b6261c8",
					"imageUrl": "https://dev.azure.com/fabrikam/DefaultCollection/_api/_common/identityImage?id=54d125f7-69f7-4191-904f-c5b96b6261c8"
	},
	"pullRequestId": 22,
	"completionOptions": {
					"bypassPolicy":false,
					"bypassReason":"",
					"deleteSourceBranch":false,
					"mergeCommitMessage":"TEST MERGE COMMIT MESSAGE",
					"mergeStrategy":"noFastForward",
					"squashMerge":false,
					"transitionWorkItems":true,
					"triggeredByAutoComplete":false
	}
}`
