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

package webhooks_test

import (
	"regexp"
	"testing"

	. "github.com/petergtz/pegomock"
	"github.com/runatlantis/atlantis/server/events/webhooks"
	"github.com/runatlantis/atlantis/server/events/webhooks/mocks"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

func TestSend_SlackMessage(t *testing.T) {
	t.Log("Sending a hook with a matching regex should call PostMessage")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	regex, err := regexp.Compile(".*")
	Ok(t, err)

	channel := "somechannel"
	hook := webhooks.SlackWebhook{
		Client:         client,
		WorkspaceRegex: regex,
		Channel:        channel,
	}
	result := webhooks.ApplyResult{
		Workspace: "production",
	}

	t.Log("PostMessage should be called, doesn't matter if it errors or not")
	_ = hook.Send(logging.NewNoopLogger(t), result)
	client.VerifyWasCalledOnce().PostMessage(channel, result)
}

func TestSend_SlackNoopSuccess(t *testing.T) {
	t.Log("Sending a hook with a non-matching regex should succeed")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	regex, err := regexp.Compile("weirdemv")
	Ok(t, err)

	channel := "somechannel"
	hook := webhooks.SlackWebhook{
		Client:         client,
		WorkspaceRegex: regex,
		Channel:        channel,
	}
	result := webhooks.ApplyResult{
		Workspace: "production",
	}
	err = hook.Send(logging.NewNoopLogger(t), result)
	Ok(t, err)
	client.VerifyWasCalled(Never()).PostMessage(channel, result)
}
