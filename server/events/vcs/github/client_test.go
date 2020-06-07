package github_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs/github"
	. "github.com/runatlantis/atlantis/testing"

	"github.com/shurcooL/githubv4"
)

// GetModifiedFiles should make multiple requests if more than one page
// and concat results.
func TestClient_GetModifiedFiles(t *testing.T) {
	respTemplate := `[
  {
    "sha": "bbcd538c8e72b8c175046e27cc8f907076331401",
    "filename": "%s",
    "status": "added",
    "additions": 103,
    "deletions": 21,
    "changes": 124,
    "blob_url": "https://github.com/octocat/Hello-World/blob/6dcb09b5b57875f334f61aebed695e2e4193db5e/file1.txt",
    "raw_url": "https://github.com/octocat/Hello-World/raw/6dcb09b5b57875f334f61aebed695e2e4193db5e/file1.txt",
    "contents_url": "https://api.github.com/repos/octocat/Hello-World/contents/file1.txt?ref=6dcb09b5b57875f334f61aebed695e2e4193db5e",
    "patch": "@@ -132,7 +132,7 @@ module Test @@ -1000,7 +1000,7 @@ module Test"
  }
]`
	firstResp := fmt.Sprintf(respTemplate, "file1.txt")
	secondResp := fmt.Sprintf(respTemplate, "file2.txt")
	testServer := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.RequestURI {
			// The first request should hit this URL.
			case "/api/v3/repos/owner/repo/pulls/1/files?per_page=300":
				// We write a header that means there's an additional page.
				w.Header().Add("Link", `<https://api.github.com/resource?page=2>; rel="next",
      <https://api.github.com/resource?page=2>; rel="last"`)
				w.Write([]byte(firstResp)) // nolint: errcheck
				return
				// The second should hit this URL.
			case "/api/v3/repos/owner/repo/pulls/1/files?page=2&per_page=300":
				w.Write([]byte(secondResp)) // nolint: errcheck
			default:
				t.Errorf("got unexpected request at %q", r.RequestURI)
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}))

	testServerURL, err := url.Parse(testServer.URL)
	Ok(t, err)
	client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
	Ok(t, err)
	defer DisableSSLVerification()()

	files, err := client.GetModifiedFiles(models.Repo{
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
		CloneURL:          "",
		SanitizedCloneURL: "",
		VCSHost: models.VCSHost{
			Type:     models.Github,
			Hostname: "github.com",
		},
	}, models.PullRequest{
		Num: 1,
	})
	Ok(t, err)
	Equals(t, []string{"file1.txt", "file2.txt"}, files)
}

// GetModifiedFiles should include the source and destination of a moved
// file.
func TestClient_GetModifiedFilesMovedFile(t *testing.T) {
	resp := `[
  {
    "sha": "bbcd538c8e72b8c175046e27cc8f907076331401",
    "filename": "new/filename.txt",
    "previous_filename": "previous/filename.txt",
    "status": "renamed",
    "additions": 103,
    "deletions": 21,
    "changes": 124,
    "blob_url": "https://github.com/octocat/Hello-World/blob/6dcb09b5b57875f334f61aebed695e2e4193db5e/file1.txt",
    "raw_url": "https://github.com/octocat/Hello-World/raw/6dcb09b5b57875f334f61aebed695e2e4193db5e/file1.txt",
    "contents_url": "https://api.github.com/repos/octocat/Hello-World/contents/file1.txt?ref=6dcb09b5b57875f334f61aebed695e2e4193db5e",
    "patch": "@@ -132,7 +132,7 @@ module Test @@ -1000,7 +1000,7 @@ module Test"
  }
]`
	testServer := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.RequestURI {
			// The first request should hit this URL.
			case "/api/v3/repos/owner/repo/pulls/1/files?per_page=300":
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
	client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
	Ok(t, err)
	defer DisableSSLVerification()()

	files, err := client.GetModifiedFiles(models.Repo{
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
		CloneURL:          "",
		SanitizedCloneURL: "",
		VCSHost: models.VCSHost{
			Type:     models.Github,
			Hostname: "github.com",
		},
	}, models.PullRequest{
		Num: 1,
	})
	Ok(t, err)
	Equals(t, []string{"new/filename.txt", "previous/filename.txt"}, files)
}

func TestClient_PaginatesComments(t *testing.T) {
	calls := 0
	issueResps := []string{
		`[
	{"node_id": "1", "body": "asd\nplan\nasd", "user": {"login": "someone-else"}},
	{"node_id": "2", "body": "asd plan\nasd", "user": {"login": "user"}}
]`,
		`[
	{"node_id": "3", "body": "asd", "user": {"login": "someone-else"}},
	{"node_id": "4", "body": "asdasd", "user": {"login": "someone-else"}}
]`,
		`[
	{"node_id": "5", "body": "asd plan", "user": {"login": "someone-else"}},
	{"node_id": "6", "body": "asd\nplan", "user": {"login": "user"}}
]`,
		`[
	{"node_id": "7", "body": "asd", "user": {"login": "user"}},
	{"node_id": "8", "body": "asd plan \n asd", "user": {"login": "user"}}
]`,
	}
	minimizeResp := "{}"
	type graphQLCall struct {
		Variables struct {
			Input githubv4.MinimizeCommentInput `json:"input"`
		} `json:"variables"`
	}
	gotMinimizeCalls := make([]graphQLCall, 0, 2)
	testServer := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method + " " + r.RequestURI {
			case "POST /api/graphql":
				defer r.Body.Close() // nolint: errcheck
				body, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body error: %v", err)
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}
				call := graphQLCall{}
				err = json.Unmarshal(body, &call)
				if err != nil {
					t.Errorf("parse body error: %v", err)
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}
				gotMinimizeCalls = append(gotMinimizeCalls, call)
				w.Write([]byte(minimizeResp)) // nolint: errcheck
				return
			default:
				if r.Method != "GET" || !strings.HasPrefix(r.RequestURI, "/api/v3/repos/owner/repo/issues/123/comments") {
					t.Errorf("got unexpected request at %q", r.RequestURI)
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				if (calls + 1) < len(issueResps) {
					w.Header().Add(
						"Link",
						fmt.Sprintf(
							`<http://%s/api/v3/repos/owner/repo/issues/123/comments?page=%d&per_page=100>; rel="next"`,
							r.Host,
							calls+1,
						),
					)
				}
				w.Write([]byte(issueResps[calls])) // nolint: errcheck
				calls++
			}
		}),
	)

	testServerURL, err := url.Parse(testServer.URL)
	Ok(t, err)

	client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
	Ok(t, err)
	defer DisableSSLVerification()()

	err = client.HidePrevPlanComments(
		models.Repo{
			FullName:          "owner/repo",
			Owner:             "owner",
			Name:              "repo",
			CloneURL:          "",
			SanitizedCloneURL: "",
			VCSHost: models.VCSHost{
				Hostname: "github.com",
				Type:     models.Github,
			},
		},
		123,
	)
	Ok(t, err)
	Equals(t, 2, len(gotMinimizeCalls))
	Equals(t, "2", gotMinimizeCalls[0].Variables.Input.SubjectID)
	Equals(t, "8", gotMinimizeCalls[1].Variables.Input.SubjectID)
	Equals(t, githubv4.ReportedContentClassifiersOutdated, gotMinimizeCalls[0].Variables.Input.Classifier)
	Equals(t, githubv4.ReportedContentClassifiersOutdated, gotMinimizeCalls[1].Variables.Input.Classifier)
}

func TestClient_HideOldComments(t *testing.T) {
	// Only comment 6 should be minimized, because it's by the same Atlantis bot user
	// and it has "plan" in the first line of the comment body.
	issueResp := `[
	{"node_id": "1", "body": "asd\nplan\nasd", "user": {"login": "someone-else"}},
	{"node_id": "2", "body": "asd plan\nasd", "user": {"login": "someone-else"}},
	{"node_id": "3", "body": "asdasdasd\nasdasdasd", "user": {"login": "someone-else"}},
	{"node_id": "4", "body": "asdasdasd\nasdasdasd", "user": {"login": "user"}},
	{"node_id": "5", "body": "asd\nplan\nasd", "user": {"login": "user"}},
	{"node_id": "6", "body": "asd plan\nasd", "user": {"login": "user"}},
	{"node_id": "7", "body": "asdasdasd", "user": {"login": "user"}}
]`
	minimizeResp := "{}"
	type graphQLCall struct {
		Variables struct {
			Input githubv4.MinimizeCommentInput `json:"input"`
		} `json:"variables"`
	}
	gotMinimizeCalls := make([]graphQLCall, 0, 1)
	testServer := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method + " " + r.RequestURI {
			// This gets the pull request's comments.
			case "GET /api/v3/repos/owner/repo/issues/123/comments?direction=asc&sort=created":
				w.Write([]byte(issueResp)) // nolint: errcheck
				return
			case "POST /api/graphql":
				if accept, has := r.Header["Accept"]; !has || accept[0] != "application/vnd.github.queen-beryl-preview+json" {
					t.Error("missing preview header")
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				defer r.Body.Close() // nolint: errcheck
				body, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body error: %v", err)
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}
				call := graphQLCall{}
				err = json.Unmarshal(body, &call)
				if err != nil {
					t.Errorf("parse body error: %v", err)
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}
				gotMinimizeCalls = append(gotMinimizeCalls, call)
				w.Write([]byte(minimizeResp)) // nolint: errcheck
				return
			default:
				t.Errorf("got unexpected request at %q", r.RequestURI)
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}),
	)

	testServerURL, err := url.Parse(testServer.URL)
	Ok(t, err)

	client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
	Ok(t, err)
	defer DisableSSLVerification()()

	err = client.HidePrevPlanComments(
		models.Repo{
			FullName:          "owner/repo",
			Owner:             "owner",
			Name:              "repo",
			CloneURL:          "",
			SanitizedCloneURL: "",
			VCSHost: models.VCSHost{
				Hostname: "github.com",
				Type:     models.Github,
			},
		},
		123,
	)
	Ok(t, err)
	Equals(t, 1, len(gotMinimizeCalls))
	Equals(t, "6", gotMinimizeCalls[0].Variables.Input.SubjectID)
	Equals(t, githubv4.ReportedContentClassifiersOutdated, gotMinimizeCalls[0].Variables.Input.Classifier)
}

func TestClient_UpdateStatus(t *testing.T) {
	cases := []struct {
		status   models.CommitStatus
		expState string
	}{
		{
			models.PendingCommitStatus,
			"pending",
		},
		{
			models.SuccessCommitStatus,
			"success",
		},
		{
			models.FailedCommitStatus,
			"failure",
		},
	}

	for _, c := range cases {
		t.Run(c.status.String(), func(t *testing.T) {
			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					case "/api/v3/repos/owner/repo/statuses/":
						body, err := ioutil.ReadAll(r.Body)
						Ok(t, err)
						exp := fmt.Sprintf(`{"state":"%s","target_url":"https://google.com","description":"description","context":"src"}%s`, c.expState, "\n")
						Equals(t, exp, string(body))
						defer r.Body.Close() // nolint: errcheck
						w.WriteHeader(http.StatusOK)
					default:
						t.Errorf("got unexpected request at %q", r.RequestURI)
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
				}))

			testServerURL, err := url.Parse(testServer.URL)
			Ok(t, err)
			client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
			Ok(t, err)
			defer DisableSSLVerification()()

			err = client.UpdateStatus(models.Repo{
				FullName:          "owner/repo",
				Owner:             "owner",
				Name:              "repo",
				CloneURL:          "",
				SanitizedCloneURL: "",
				VCSHost: models.VCSHost{
					Type:     models.Github,
					Hostname: "github.com",
				},
			}, models.PullRequest{
				Num: 1,
			}, c.status, "src", "description", "https://google.com")
			Ok(t, err)
		})
	}
}

func TestClient_PullIsApproved(t *testing.T) {
	respTemplate := `[
		{
			"id": %d,
			"node_id": "MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA=",
			"user": {
			  "login": "octocat",
			  "id": 1,
			  "node_id": "MDQ6VXNlcjE=",
			  "avatar_url": "https://github.com/images/error/octocat_happy.gif",
			  "gravatar_id": "",
			  "url": "https://api.github.com/users/octocat",
			  "html_url": "https://github.com/octocat",
			  "followers_url": "https://api.github.com/users/octocat/followers",
			  "following_url": "https://api.github.com/users/octocat/following{/other_user}",
			  "gists_url": "https://api.github.com/users/octocat/gists{/gist_id}",
			  "starred_url": "https://api.github.com/users/octocat/starred{/owner}{/repo}",
			  "subscriptions_url": "https://api.github.com/users/octocat/subscriptions",
			  "organizations_url": "https://api.github.com/users/octocat/orgs",
			  "repos_url": "https://api.github.com/users/octocat/repos",
			  "events_url": "https://api.github.com/users/octocat/events{/privacy}",
			  "received_events_url": "https://api.github.com/users/octocat/received_events",
			  "type": "User",
			  "site_admin": false
			},
			"body": "Here is the body for the review.",
			"commit_id": "ecdd80bb57125d7ba9641ffaa4d7d2c19d3f3091",
			"state": "CHANGES_REQUESTED",
			"html_url": "https://github.com/octocat/Hello-World/pull/12#pullrequestreview-%d",
			"pull_request_url": "https://api.github.com/repos/octocat/Hello-World/pulls/12",
			"_links": {
			  "html": {
				"href": "https://github.com/octocat/Hello-World/pull/12#pullrequestreview-%d"
			  },
			  "pull_request": {
				"href": "https://api.github.com/repos/octocat/Hello-World/pulls/12"
			  }
			}
		  }
]`
	firstResp := fmt.Sprintf(respTemplate, 80, 80, 80)
	secondResp := fmt.Sprintf(respTemplate, 81, 81, 81)
	testServer := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.RequestURI {
			// The first request should hit this URL.
			case "/api/v3/repos/owner/repo/pulls/1/reviews?per_page=300":
				// We write a header that means there's an additional page.
				w.Header().Add("Link", `<https://api.github.com/resource?page=2>; rel="next",
      <https://api.github.com/resource?page=2>; rel="last"`)
				w.Write([]byte(firstResp)) // nolint: errcheck
				return
				// The second should hit this URL.
			case "/api/v3/repos/owner/repo/pulls/1/reviews?page=2&per_page=300":
				w.Write([]byte(secondResp)) // nolint: errcheck
			default:
				t.Errorf("got unexpected request at %q", r.RequestURI)
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}))

	testServerURL, err := url.Parse(testServer.URL)
	Ok(t, err)
	client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
	Ok(t, err)
	defer DisableSSLVerification()()

	approved, err := client.PullIsApproved(models.Repo{
		FullName:          "owner/repo",
		Owner:             "owner",
		Name:              "repo",
		CloneURL:          "",
		SanitizedCloneURL: "",
		VCSHost: models.VCSHost{
			Type:     models.Github,
			Hostname: "github.com",
		},
	}, models.PullRequest{
		Num: 1,
	})
	Ok(t, err)
	Equals(t, false, approved)
}

func TestClient_PullIsMergeable(t *testing.T) {
	cases := []struct {
		state        string
		expMergeable bool
	}{
		{
			"dirty",
			false,
		},
		{
			"unknown",
			false,
		},
		{
			"blocked",
			false,
		},
		{
			"behind",
			false,
		},
		{
			"random",
			false,
		},
		{
			"unstable",
			true,
		},
		{
			"has_hooks",
			true,
		},
		{
			"clean",
			true,
		},
		{
			"",
			false,
		},
	}

	// Use a real GitHub json response and edit the mergeable_state field.
	jsBytes, err := ioutil.ReadFile("fixtures/github-pull-request.json")
	Ok(t, err)
	json := string(jsBytes)

	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			response := strings.Replace(json,
				`"mergeable_state": "clean"`,
				fmt.Sprintf(`"mergeable_state": "%s"`, c.state),
				1,
			)

			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					case "/api/v3/repos/owner/repo/pulls/1":
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
			client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
			Ok(t, err)
			defer DisableSSLVerification()()

			actMergeable, err := client.PullIsMergeable(models.Repo{
				FullName:          "owner/repo",
				Owner:             "owner",
				Name:              "repo",
				CloneURL:          "",
				SanitizedCloneURL: "",
				VCSHost: models.VCSHost{
					Type:     models.Github,
					Hostname: "github.com",
				},
			}, models.PullRequest{
				Num: 1,
			})
			Ok(t, err)
			Equals(t, c.expMergeable, actMergeable)
		})
	}
}

func TestClient_MergePullHandlesError(t *testing.T) {
	cases := []struct {
		code    int
		message string
		merged  string
		expErr  string
	}{
		{
			code:    200,
			message: "Pull Request successfully merged",
			merged:  "true",
			expErr:  "",
		},
		{
			code:    405,
			message: "Pull Request is not mergeable",
			expErr:  "405 Pull Request is not mergeable []",
		},
		{
			code:    409,
			message: "Head branch was modified. Review and try the merge again.",
			expErr:  "409 Head branch was modified. Review and try the merge again. []",
		},
	}

	jsBytes, err := ioutil.ReadFile("fixtures/github-repo.json")
	Ok(t, err)

	for _, c := range cases {
		t.Run(c.message, func(t *testing.T) {
			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					case "/api/v3/repos/owner/repo":
						w.Write(jsBytes) // nolint: errcheck
						return
					case "/api/v3/repos/owner/repo/pulls/1/merge":
						body, err := ioutil.ReadAll(r.Body)
						Ok(t, err)
						exp := "{\"merge_method\":\"merge\"}\n"
						Equals(t, exp, string(body))
						var resp string
						if c.code == 200 {
							resp = fmt.Sprintf(`{"message":"%s","merged":%s}%s`, c.message, c.merged, "\n")
						} else {
							resp = fmt.Sprintf(`{"message":"%s"}%s`, c.message, "\n")
						}
						defer r.Body.Close() // nolint: errcheck
						w.WriteHeader(c.code)
						w.Write([]byte(resp)) // nolint: errcheck
					default:
						t.Errorf("got unexpected request at %q", r.RequestURI)
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
				}))

			testServerURL, err := url.Parse(testServer.URL)
			Ok(t, err)
			client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
			Ok(t, err)
			defer DisableSSLVerification()()

			err = client.MergePull(
				models.PullRequest{
					BaseRepo: models.Repo{
						FullName:          "owner/repo",
						Owner:             "owner",
						Name:              "repo",
						CloneURL:          "",
						SanitizedCloneURL: "",
						VCSHost: models.VCSHost{
							Type:     models.Github,
							Hostname: "github.com",
						},
					},
					Num: 1,
				})

			if c.expErr == "" {
				Ok(t, err)
			} else {
				ErrContains(t, c.expErr, err)
			}
		})
	}
}

// Test that if the pull request only allows a certain merge method that we
// use that method
func TestClient_MergePullCorrectMethod(t *testing.T) {
	cases := map[string]struct {
		allowMerge  bool
		allowRebase bool
		allowSquash bool
		expMethod   string
	}{
		"all true": {
			allowMerge:  true,
			allowRebase: true,
			allowSquash: true,
			expMethod:   "merge",
		},
		"all false (edge case)": {
			allowMerge:  false,
			allowRebase: false,
			allowSquash: false,
			expMethod:   "merge",
		},
		"merge: false rebase: true squash: true": {
			allowMerge:  false,
			allowRebase: true,
			allowSquash: true,
			expMethod:   "rebase",
		},
		"merge: false rebase: false squash: true": {
			allowMerge:  false,
			allowRebase: false,
			allowSquash: true,
			expMethod:   "squash",
		},
		"merge: false rebase: true squash: false": {
			allowMerge:  false,
			allowRebase: true,
			allowSquash: false,
			expMethod:   "rebase",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {

			// Modify response.
			jsBytes, err := ioutil.ReadFile("fixtures/github-repo.json")
			Ok(t, err)
			resp := string(jsBytes)
			resp = strings.Replace(resp,
				`"allow_squash_merge": true`,
				fmt.Sprintf(`"allow_squash_merge": %t`, c.allowSquash),
				-1)
			resp = strings.Replace(resp,
				`"allow_merge_commit": true`,
				fmt.Sprintf(`"allow_merge_commit": %t`, c.allowMerge),
				-1)
			resp = strings.Replace(resp,
				`"allow_rebase_merge": true`,
				fmt.Sprintf(`"allow_rebase_merge": %t`, c.allowRebase),
				-1)

			testServer := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.RequestURI {
					case "/api/v3/repos/runatlantis/atlantis":
						w.Write([]byte(resp)) // nolint: errcheck
						return
					case "/api/v3/repos/runatlantis/atlantis/pulls/1/merge":
						body, err := ioutil.ReadAll(r.Body)
						Ok(t, err)
						defer r.Body.Close() // nolint: errcheck
						type bodyJSON struct {
							CommitMessage *string `json:"commit_message,omitempty"`
							MergeMethod   string  `json:"merge_method"`
						}
						expBody := bodyJSON{
							MergeMethod: c.expMethod,
						}
						expBytes, err := json.Marshal(expBody)
						Ok(t, err)
						Equals(t, string(expBytes)+"\n", string(body))

						resp := `{"sha":"6dcb09b5b57875f334f61aebed695e2e4193db5e","merged":true,"message":"Pull Request successfully merged"}`
						w.Write([]byte(resp)) // nolint: errcheck
					default:
						t.Errorf("got unexpected request at %q", r.RequestURI)
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
				}))

			testServerURL, err := url.Parse(testServer.URL)
			Ok(t, err)
			client, err := github.NewClient(testServerURL.Host, "user", "pass", nil)
			Ok(t, err)
			defer DisableSSLVerification()()

			err = client.MergePull(
				models.PullRequest{
					BaseRepo: models.Repo{
						FullName:          "runatlantis/atlantis",
						Owner:             "runatlantis",
						Name:              "atlantis",
						CloneURL:          "",
						SanitizedCloneURL: "",
						VCSHost: models.VCSHost{
							Type:     models.Github,
							Hostname: "github.com",
						},
					},
					Num: 1,
				})
			Ok(t, err)
		})
	}
}

func TestClient_MarkdownPullLink(t *testing.T) {
	client, err := github.NewClient("hostname", "user", "pass", nil)
	Ok(t, err)
	pull := models.PullRequest{Num: 1}
	s, _ := client.MarkdownPullLink(pull)
	exp := "#1"
	Equals(t, exp, s)
}
