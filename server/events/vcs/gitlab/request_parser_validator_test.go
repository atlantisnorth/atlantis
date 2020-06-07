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

package gitlab_test

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"

	. "github.com/petergtz/pegomock"
	vcsgitlab "github.com/runatlantis/atlantis/server/events/vcs/gitlab"
	. "github.com/runatlantis/atlantis/testing"
	gitlab "github.com/xanzy/go-gitlab"
)

var parser = vcsgitlab.DefaultRequestParserValidator{}

func TestValidate_InvalidSecret(t *testing.T) {
	t.Log("If the secret header is set and doesn't match expected an error is returned")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString("")
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Token", "does-not-match")
	_, err = parser.ParseAndValidate(req, []byte("secret"))
	Assert(t, err != nil, "should be an error")
	Equals(t, "header X-Gitlab-Token=does-not-match did not match expected secret", err.Error())
}

func TestValidate_ValidSecret(t *testing.T) {
	t.Log("If the secret header matches then the event is returned")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString(mergeEventJSON)
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Token", "secret")
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	b, err := parser.ParseAndValidate(req, []byte("secret"))
	Ok(t, err)
	Equals(t, "atlantis-example", b.(gitlab.MergeEvent).Project.Name)
}

func TestValidate_NoSecret(t *testing.T) {
	t.Log("If there is no secret then we ignore the secret header and return the event")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString(mergeEventJSON)
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Token", "random secret")
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	b, err := parser.ParseAndValidate(req, nil)
	Ok(t, err)
	Equals(t, "atlantis-example", b.(gitlab.MergeEvent).Project.Name)
}

func TestValidate_InvalidMergeEvent(t *testing.T) {
	t.Log("If the merge event is malformed there should be an error")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString("{")
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	_, err = parser.ParseAndValidate(req, nil)
	Assert(t, err != nil, "should be an error")
	Equals(t, "unexpected end of JSON input", err.Error())
}

func TestValidate_InvalidMergeCommentEvent(t *testing.T) {
	t.Log("If the merge comment event is malformed there should be an error")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString("{")
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Event", "Note Hook")
	_, err = parser.ParseAndValidate(req, nil)
	Assert(t, err != nil, "should be an error")
	Equals(t, "unexpected end of JSON input", err.Error())
}

func TestValidate_UnrecognizedEvent(t *testing.T) {
	t.Log("If the event is not one we care about we return nil")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString("")
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Event", "Random Event")
	event, err := parser.ParseAndValidate(req, nil)
	Ok(t, err)
	Equals(t, nil, event)
}

func TestValidate_ValidMergeEvent(t *testing.T) {
	t.Log("If the merge event is valid it should be returned")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString(mergeEventJSON)
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	b, err := parser.ParseAndValidate(req, nil)
	Ok(t, err)
	Equals(t, "atlantis-example", b.(gitlab.MergeEvent).Project.Name)
}

// If the comment was on a commit instead of a merge request, make sure we
// return the right object.
func TestValidate_CommitCommentEvent(t *testing.T) {
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString(commitCommentEventJSON)
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Event", "Note Hook")
	b, err := parser.ParseAndValidate(req, nil)
	Ok(t, err)
	Equals(t, "gitlab.CommitCommentEvent", reflect.TypeOf(b).String())
}

func TestValidate_ValidMergeCommentEvent(t *testing.T) {
	t.Log("If the merge comment event is valid it should be returned")
	RegisterMockTestingT(t)
	buf := bytes.NewBufferString(mergeCommentEventJSON)
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Gitlab-Event", "Note Hook")
	b, err := parser.ParseAndValidate(req, nil)
	Ok(t, err)
	Equals(t, "Gitlab Test", b.(gitlab.MergeCommentEvent).Project.Name)
}

var mergeEventJSON = `{
  "object_kind": "merge_request",
  "event_type": "merge_request",
  "user": {
    "name": "Luke Kysow",
    "username": "lkysow",
    "avatar_url": "https://secure.gravatar.com/avatar/25fd57e71590fe28736624ff24d41c5f?s=80&d=identicon"
  },
  "project": {
    "id": 4580910,
    "name": "atlantis-example",
    "description": "",
    "web_url": "https://gitlab.com/lkysow/atlantis-example",
    "avatar_url": null,
    "git_ssh_url": "git@gitlab.com:lkysow/atlantis-example.git",
    "git_http_url": "https://gitlab.com/lkysow/atlantis-example.git",
    "namespace": "lkysow",
    "visibility_level": 20,
    "path_with_namespace": "lkysow/atlantis-example",
    "default_branch": "master",
    "ci_config_path": null,
    "homepage": "https://gitlab.com/lkysow/atlantis-example",
    "url": "git@gitlab.com:lkysow/atlantis-example.git",
    "ssh_url": "git@gitlab.com:lkysow/atlantis-example.git",
    "http_url": "https://gitlab.com/lkysow/atlantis-example.git"
  },
  "object_attributes": {
    "assignee_id": null,
    "author_id": 1755902,
    "created_at": "2018-12-12 16:15:21 UTC",
    "description": "",
    "head_pipeline_id": null,
    "id": 20809239,
    "iid": 12,
    "last_edited_at": null,
    "last_edited_by_id": null,
    "merge_commit_sha": null,
    "merge_error": null,
    "merge_params": {
      "force_remove_source_branch": false
    },
    "merge_status": "unchecked",
    "merge_user_id": null,
    "merge_when_pipeline_succeeds": false,
    "milestone_id": null,
    "source_branch": "patch-1",
    "source_project_id": 4580910,
    "state": "opened",
    "target_branch": "master",
    "target_project_id": 4580910,
    "time_estimate": 0,
    "title": "Update main.tf",
    "updated_at": "2018-12-12 16:15:21 UTC",
    "updated_by_id": null,
    "url": "https://gitlab.com/lkysow/atlantis-example/merge_requests/12",
    "source": {
      "id": 4580910,
      "name": "atlantis-example",
      "description": "",
      "web_url": "https://gitlab.com/sourceorg/atlantis-example",
      "avatar_url": null,
      "git_ssh_url": "git@gitlab.com:sourceorg/atlantis-example.git",
      "git_http_url": "https://gitlab.com/sourceorg/atlantis-example.git",
      "namespace": "sourceorg",
      "visibility_level": 20,
      "path_with_namespace": "sourceorg/atlantis-example",
      "default_branch": "master",
      "ci_config_path": null,
      "homepage": "https://gitlab.com/sourceorg/atlantis-example",
      "url": "git@gitlab.com:sourceorg/atlantis-example.git",
      "ssh_url": "git@gitlab.com:sourceorg/atlantis-example.git",
      "http_url": "https://gitlab.com/sourceorg/atlantis-example.git"
    },
    "target": {
      "id": 4580910,
      "name": "atlantis-example",
      "description": "",
      "web_url": "https://gitlab.com/lkysow/atlantis-example",
      "avatar_url": null,
      "git_ssh_url": "git@gitlab.com:lkysow/atlantis-example.git",
      "git_http_url": "https://gitlab.com/lkysow/atlantis-example.git",
      "namespace": "lkysow",
      "visibility_level": 20,
      "path_with_namespace": "lkysow/atlantis-example",
      "default_branch": "master",
      "ci_config_path": null,
      "homepage": "https://gitlab.com/lkysow/atlantis-example",
      "url": "git@gitlab.com:lkysow/atlantis-example.git",
      "ssh_url": "git@gitlab.com:lkysow/atlantis-example.git",
      "http_url": "https://gitlab.com/lkysow/atlantis-example.git"
    },
    "last_commit": {
      "id": "d2eae324ca26242abca45d7b49d582cddb2a4f15",
      "message": "Update main.tf",
      "timestamp": "2018-12-12T16:15:10Z",
      "url": "https://gitlab.com/lkysow/atlantis-example/commit/d2eae324ca26242abca45d7b49d582cddb2a4f15",
      "author": {
        "name": "Luke Kysow",
        "email": "lkysow@gmail.com"
      }
    },
    "work_in_progress": false,
    "total_time_spent": 0,
    "human_total_time_spent": null,
    "human_time_estimate": null,
    "action": "open"
  },
  "labels": [

  ],
  "changes": {
    "author_id": {
      "previous": null,
      "current": 1755902
    },
    "created_at": {
      "previous": null,
      "current": "2018-12-12 16:15:21 UTC"
    },
    "description": {
      "previous": null,
      "current": ""
    },
    "id": {
      "previous": null,
      "current": 20809239
    },
    "iid": {
      "previous": null,
      "current": 12
    },
    "merge_params": {
      "previous": {
      },
      "current": {
        "force_remove_source_branch": false
      }
    },
    "source_branch": {
      "previous": null,
      "current": "patch-1"
    },
    "source_project_id": {
      "previous": null,
      "current": 4580910
    },
    "target_branch": {
      "previous": null,
      "current": "master"
    },
    "target_project_id": {
      "previous": null,
      "current": 4580910
    },
    "title": {
      "previous": null,
      "current": "Update main.tf"
    },
    "updated_at": {
      "previous": null,
      "current": "2018-12-12 16:15:21 UTC"
    },
    "total_time_spent": {
      "previous": null,
      "current": 0
    }
  },
  "repository": {
    "name": "atlantis-example",
    "url": "git@gitlab.com:lkysow/atlantis-example.git",
    "description": "",
    "homepage": "https://gitlab.com/lkysow/atlantis-example"
  }
}
`

var mergeCommentEventJSON = `{
  "object_kind": "note",
  "user": {
    "name": "Administrator",
    "username": "root",
    "avatar_url": "http://www.gravatar.com/avatar/e64c7d89f26bd1972efa854d13d7dd61?s=40\u0026d=identicon"
  },
  "project_id": 5,
  "project":{
    "id": 5,
    "name":"Gitlab Test",
    "description":"Aut reprehenderit ut est.",
    "web_url":"http://example.com/gitlabhq/gitlab-test",
    "avatar_url":null,
    "git_ssh_url":"git@example.com:gitlabhq/gitlab-test.git",
    "git_http_url":"https://example.com/gitlabhq/gitlab-test.git",
    "namespace":"Gitlab Org",
    "visibility_level":10,
    "path_with_namespace":"gitlabhq/gitlab-test",
    "default_branch":"master",
    "homepage":"http://example.com/gitlabhq/gitlab-test",
    "url":"https://example.com/gitlabhq/gitlab-test.git",
    "ssh_url":"git@example.com:gitlabhq/gitlab-test.git",
    "http_url":"https://example.com/gitlabhq/gitlab-test.git"
  },
  "repository":{
    "name": "Gitlab Test",
    "url": "http://localhost/gitlab-org/gitlab-test.git",
    "description": "Aut reprehenderit ut est.",
    "homepage": "http://example.com/gitlab-org/gitlab-test"
  },
  "object_attributes": {
    "id": 1244,
    "note": "This MR needs work.",
    "noteable_type": "MergeRequest",
    "author_id": 1,
    "created_at": "2015-05-17",
    "updated_at": "2015-05-17",
    "project_id": 5,
    "attachment": null,
    "line_code": null,
    "commit_id": "",
    "noteable_id": 7,
    "system": false,
    "st_diff": null,
    "url": "http://example.com/gitlab-org/gitlab-test/merge_requests/1#note_1244"
  },
  "merge_request": {
    "id": 7,
    "target_branch": "markdown",
    "source_branch": "master",
    "source_project_id": 5,
    "author_id": 8,
    "assignee_id": 28,
    "title": "Tempora et eos debitis quae laborum et.",
    "created_at": "2015-03-01 20:12:53 UTC",
    "updated_at": "2015-03-21 18:27:27 UTC",
    "milestone_id": 11,
    "state": "opened",
    "merge_status": "cannot_be_merged",
    "target_project_id": 5,
    "iid": 1,
    "description": "Et voluptas corrupti assumenda temporibus. Architecto cum animi eveniet amet asperiores. Vitae numquam voluptate est natus sit et ad id.",
    "position": 0,
    "source":{
      "name":"Gitlab Test",
      "description":"Aut reprehenderit ut est.",
      "web_url":"http://example.com/gitlab-org/gitlab-test",
      "avatar_url":null,
      "git_ssh_url":"git@example.com:gitlab-org/gitlab-test.git",
      "git_http_url":"https://example.com/gitlab-org/gitlab-test.git",
      "namespace":"Gitlab Org",
      "visibility_level":10,
      "path_with_namespace":"gitlab-org/gitlab-test",
      "default_branch":"master",
      "homepage":"http://example.com/gitlab-org/gitlab-test",
      "url":"https://example.com/gitlab-org/gitlab-test.git",
      "ssh_url":"git@example.com:gitlab-org/gitlab-test.git",
      "http_url":"https://example.com/gitlab-org/gitlab-test.git",
      "git_http_url":"https://example.com/gitlab-org/gitlab-test.git"
    },
    "target": {
      "name":"Gitlab Test",
      "description":"Aut reprehenderit ut est.",
      "web_url":"http://example.com/gitlabhq/gitlab-test",
      "avatar_url":null,
      "git_ssh_url":"git@example.com:gitlabhq/gitlab-test.git",
      "git_http_url":"https://example.com/gitlabhq/gitlab-test.git",
      "namespace":"Gitlab Org",
      "visibility_level":10,
      "path_with_namespace":"gitlabhq/gitlab-test",
      "default_branch":"master",
      "homepage":"http://example.com/gitlabhq/gitlab-test",
      "url":"https://example.com/gitlabhq/gitlab-test.git",
      "ssh_url":"git@example.com:gitlabhq/gitlab-test.git",
      "http_url":"https://example.com/gitlabhq/gitlab-test.git"
    },
    "last_commit": {
      "id": "562e173be03b8ff2efb05345d12df18815438a4b",
      "message": "Merge branch 'another-branch' into 'master'\n\nCheck in this test\n",
      "timestamp": "2002-10-02T10:00:00-05:00",
      "url": "http://example.com/gitlab-org/gitlab-test/commit/562e173be03b8ff2efb05345d12df18815438a4b",
      "author": {
        "name": "John Smith",
        "email": "john@example.com"
      }
    },
    "work_in_progress": false,
    "assignee": {
      "name": "User1",
      "username": "user1",
      "avatar_url": "http://www.gravatar.com/avatar/e64c7d89f26bd1972efa854d13d7dd61?s=40\u0026d=identicon"
    }
  }
}`

var commitCommentEventJSON = `{
  "object_kind": "note",
  "user": {
    "name": "Administrator",
    "username": "root",
    "avatar_url": "http://www.gravatar.com/avatar/e64c7d89f26bd1972efa854d13d7dd61?s=40\u0026d=identicon"
  },
  "project_id": 5,
  "project":{
    "id": 5,
    "name":"Gitlab Test",
    "description":"Aut reprehenderit ut est.",
    "web_url":"http://example.com/gitlabhq/gitlab-test",
    "avatar_url":null,
    "git_ssh_url":"git@example.com:gitlabhq/gitlab-test.git",
    "git_http_url":"http://example.com/gitlabhq/gitlab-test.git",
    "namespace":"GitlabHQ",
    "visibility_level":20,
    "path_with_namespace":"gitlabhq/gitlab-test",
    "default_branch":"master",
    "homepage":"http://example.com/gitlabhq/gitlab-test",
    "url":"http://example.com/gitlabhq/gitlab-test.git",
    "ssh_url":"git@example.com:gitlabhq/gitlab-test.git",
    "http_url":"http://example.com/gitlabhq/gitlab-test.git"
  },
  "repository":{
    "name": "Gitlab Test",
    "url": "http://example.com/gitlab-org/gitlab-test.git",
    "description": "Aut reprehenderit ut est.",
    "homepage": "http://example.com/gitlab-org/gitlab-test"
  },
  "object_attributes": {
    "id": 1243,
    "note": "This is a commit comment. How does this work?",
    "noteable_type": "Commit",
    "author_id": 1,
    "created_at": "2015-05-17 18:08:09 UTC",
    "updated_at": "2015-05-17 18:08:09 UTC",
    "project_id": 5,
    "attachment":null,
    "line_code": "bec9703f7a456cd2b4ab5fb3220ae016e3e394e3_0_1",
    "commit_id": "cfe32cf61b73a0d5e9f13e774abde7ff789b1660",
    "noteable_id": null,
    "system": false,
    "st_diff": {
      "diff": "--- /dev/null\n+++ b/six\n@@ -0,0 +1 @@\n+Subproject commit 409f37c4f05865e4fb208c771485f211a22c4c2d\n",
      "new_path": "six",
      "old_path": "six",
      "a_mode": "0",
      "b_mode": "160000",
      "new_file": true,
      "renamed_file": false,
      "deleted_file": false
    },
    "url": "http://example.com/gitlab-org/gitlab-test/commit/cfe32cf61b73a0d5e9f13e774abde7ff789b1660#note_1243"
  },
  "commit": {
    "id": "cfe32cf61b73a0d5e9f13e774abde7ff789b1660",
    "message": "Add submodule\n\nSigned-off-by: Dmitriy Zaporozhets \u003cdmitriy.zaporozhets@gmail.com\u003e\n",
    "timestamp": "2014-02-27T10:06:20+02:00",
    "url": "http://example.com/gitlab-org/gitlab-test/commit/cfe32cf61b73a0d5e9f13e774abde7ff789b1660",
    "author": {
      "name": "Dmitriy Zaporozhets",
      "email": "dmitriy.zaporozhets@gmail.com"
    }
  }
}`
