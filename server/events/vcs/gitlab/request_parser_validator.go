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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	gitlab "github.com/xanzy/go-gitlab"
)

const gitlabHeader = "X-Gitlab-Event"
const secretHeader = "X-Gitlab-Token" // #nosec

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_gitlab_request_parser_validator.go GitlabRequestParserValidator

// RequestParserValidator parses and validates GitLab requests.
type RequestParserValidator interface {
	// ParseAndValidate validates that the request has a token header matching secret.
	// If the secret does not match it returns an error.
	// If secret is empty it does not check the token header.
	// It then parses the request as a GitLab object depending on the header
	// provided by GitLab identifying the webhook type. If the webhook type
	// is not recognized it will return nil but will not return an error.
	// Usage:
	//	event, err := GitlabRequestParserValidator.ParseAndValidate(r, secret)
	//	if err != nil {
	//		return
	//	}
	//	switch event := event.(type) {
	//	case gitlab.MergeCommentEvent:
	//		// handle
	//	case gitlab.MergeEvent:
	//		// handle
	//	default:
	//		// unsupported event
	//	}
	ParseAndValidate(r *http.Request, secret []byte) (interface{}, error)
}

// DefaultRequestParserValidator parses and validates GitLab requests.
type DefaultRequestParserValidator struct{}

// ParseAndValidate returns the JSON payload of the request.
// See GitlabRequestParserValidator.ParseAndValidate().
func (d *DefaultRequestParserValidator) ParseAndValidate(r *http.Request, secret []byte) (interface{}, error) {
	const mergeEventHeader = "Merge Request Hook"
	const noteEventHeader = "Note Hook"

	// Validate secret if specified.
	headerSecret := r.Header.Get(secretHeader)
	secretStr := string(secret)
	if len(secret) != 0 && headerSecret != secretStr {
		return nil, fmt.Errorf("header %s=%s did not match expected secret", secretHeader, headerSecret)
	}

	// Parse request into a gitlab object based on the object type specified
	// in the gitlabHeader.
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	switch r.Header.Get(gitlabHeader) {
	case mergeEventHeader:
		var m gitlab.MergeEvent
		if err := json.Unmarshal(bytes, &m); err != nil {
			return nil, err
		}
		return m, nil
	case noteEventHeader:
		// First, parse a small part of the json to determine if this is a
		// comment on a merge request or a commit.
		var subset struct {
			ObjectAttributes struct {
				NoteableType string `json:"noteable_type"`
			} `json:"object_attributes"`
		}
		if err := json.Unmarshal(bytes, &subset); err != nil {
			return nil, err
		}

		// We then parse into the correct comment event type.
		switch subset.ObjectAttributes.NoteableType {
		case "Commit":
			var e gitlab.CommitCommentEvent
			err := json.Unmarshal(bytes, &e)
			return e, err
		case "MergeRequest":
			var e gitlab.MergeCommentEvent
			err := json.Unmarshal(bytes, &e)
			return e, err
		}
	}
	return nil, nil
}
