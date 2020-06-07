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

package github

import (
	"testing"

	. "github.com/runatlantis/atlantis/testing"
)

// If the hostname is github.com, should use normal BaseURL.
func TestNewClient_GithubCom(t *testing.T) {
	client, err := NewClient("github.com", "user", "pass", nil)
	Ok(t, err)
	Equals(t, "https://api.github.com/", client.client.BaseURL.String())
}

// If the hostname is a non-github hostname should use the right BaseURL.
func TestNewClient_NonGithub(t *testing.T) {
	client, err := NewClient("example.com", "user", "pass", nil)
	Ok(t, err)
	Equals(t, "https://example.com/api/v3/", client.client.BaseURL.String())
	// If possible in the future, test the GraphQL client's URL as well. But at the
	// moment the shurcooL library doesn't expose it.
}
