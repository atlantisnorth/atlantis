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
	"strings"
	"testing"

	. "github.com/petergtz/pegomock"
	"github.com/runatlantis/atlantis/server/events/webhooks"
	"github.com/runatlantis/atlantis/server/events/webhooks/mocks"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

const (
	validEvent     = webhooks.ApplyEvent
	validRegex     = ".*"
	validKindSlack = webhooks.SlackKind
	validKindPost  = webhooks.PostKind
	validChannel   = "validchannel"
	validURL       = "https://localhost"
)

var validConfigSlack = webhooks.Config{
	Event:          validEvent,
	WorkspaceRegex: validRegex,
	Kind:           validKindSlack,
	Channel:        validChannel,
}

var validConfigPost = webhooks.Config{
	Event: validEvent,
	Kind:  validKindPost,
	URL:   validURL,
}

func validConfigs() []webhooks.Config {
	return []webhooks.Config{validConfigSlack, validConfigPost}
}

func TestNewWebhooksManager_InvalidRegex(t *testing.T) {
	t.Log("When given an invalid regex in a config, an error is returned")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	When(client.ChannelExists(validChannel)).ThenReturn(true, nil)

	invalidRegex := "("
	configs := validConfigs()
	configs[0].WorkspaceRegex = invalidRegex
	_, err := webhooks.NewMultiWebhookSender(configs, client)
	Assert(t, err != nil, "expected error")
	Assert(t, strings.Contains(err.Error(), "error parsing regexp"), "expected regex error")
}

func TestNewWebhooksManager_NoEvent(t *testing.T) {
	t.Log("When the event key is not specified in a config, an error is returned")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	configs := validConfigs()
	configs[0].Event = ""
	_, err := webhooks.NewMultiWebhookSender(configs, client)
	Assert(t, err != nil, "expected error")
	Equals(t, "must specify \"kind\" and \"event\" keys for webhooks", err.Error())
}

func TestNewWebhooksManager_UnsupportedEvent(t *testing.T) {
	t.Log("When given an unsupported event in a config, an error is returned")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	When(client.ChannelExists(validChannel)).ThenReturn(true, nil)

	unsupportedEvent := "badevent"
	configs := validConfigs()
	configs[0].Event = unsupportedEvent
	_, err := webhooks.NewMultiWebhookSender(configs, client)
	Assert(t, err != nil, "expected error")
	Equals(t, "\"event: badevent\" not supported. Only \"event: apply\" is supported right now", err.Error())
}

func TestNewWebhooksManager_NoKind(t *testing.T) {
	t.Log("When the kind key is not specified in a config, an error is returned")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	configs := validConfigs()
	configs[0].Kind = ""
	_, err := webhooks.NewMultiWebhookSender(configs, client)
	Assert(t, err != nil, "expected error")
	Equals(t, "must specify \"kind\" and \"event\" keys for webhooks", err.Error())
}

func TestNewWebhooksManager_NoUrl(t *testing.T) {
	t.Log("When the url key is not specified in a config, an error is returned")
	confs := validConfigs()[1:]
	confs[0].URL = ""
	_, err := webhooks.NewMultiWebhookSender(confs, nil)
	Assert(t, err != nil, "expected error")
	Equals(t, "must specify \"url\" if using a webhook of \"kind: post\"", err.Error())
}

func TestNewWebhooksManager_UnsupportedKind(t *testing.T) {
	t.Log("When given an unsupported kind in a config, an error is returned")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	When(client.ChannelExists(validChannel)).ThenReturn(true, nil)

	unsupportedKind := "badkind"
	configs := validConfigs()
	configs[0].Kind = unsupportedKind
	_, err := webhooks.NewMultiWebhookSender(configs, client)
	Assert(t, err != nil, "expected error")
	Equals(t, "\"kind: badkind\" not supported. Only \"slack\" and \"post\" are supported right now", err.Error())
}

func TestNewWebhooksManager_NoConfigSuccess(t *testing.T) {
	t.Log("When there are no configs, function should succeed")
	t.Log("passing any client should succeed")
	var emptyConfigs []webhooks.Config
	emptyToken := ""
	m, err := webhooks.NewMultiWebhookSender(emptyConfigs, webhooks.NewSlackClient(emptyToken))
	Ok(t, err)
	Equals(t, 0, len(m.Webhooks)) // nolint: staticcheck

	t.Log("passing nil client should succeed")
	m, err = webhooks.NewMultiWebhookSender(emptyConfigs, nil)
	Ok(t, err)
	Equals(t, 0, len(m.Webhooks)) // nolint: staticcheck
}

func TestNewWebhooksManager_SingleConfigSuccessSlack(t *testing.T) {
	t.Log("When there is one valid config, function should succeed")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	When(client.TokenIsSet()).ThenReturn(true)
	When(client.ChannelExists(validChannel)).ThenReturn(true, nil)

	configs := validConfigs()[:1]
	m, err := webhooks.NewMultiWebhookSender(configs, client)
	Ok(t, err)
	Equals(t, 1, len(m.Webhooks)) // nolint: staticcheck
}

func TestNewWebhooksManager_SingleConfigSuccessPost(t *testing.T) {
	t.Log("When there is one valid config, function should succeed")

	configs := validConfigs()[1:]
	m, err := webhooks.NewMultiWebhookSender(configs, nil)
	Ok(t, err)
	Equals(t, 1, len(m.Webhooks)) // nolint: staticcheck
}

func TestNewWebhooksManager_MultipleConfigSuccess(t *testing.T) {
	t.Log("When there are multiple valid configs, function should succeed")
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	When(client.TokenIsSet()).ThenReturn(true)
	When(client.ChannelExists(validChannel)).ThenReturn(true, nil)

	var configs []webhooks.Config
	nConfigs := 5
	for i := 0; i < nConfigs; i++ {
		configs = append(configs, validConfigSlack)
	}
	for i := 0; i < nConfigs; i++ {
		configs = append(configs, validConfigPost)
	}
	m, err := webhooks.NewMultiWebhookSender(configs, client)
	Ok(t, err)
	Equals(t, nConfigs+nConfigs, len(m.Webhooks)) // nolint: staticcheck
}

func TestSend_SingleSuccess(t *testing.T) {
	t.Log("Sending one webhook should succeed")
	RegisterMockTestingT(t)
	sender := mocks.NewMockSender()
	manager := webhooks.MultiWebhookSender{
		Webhooks: []webhooks.Sender{sender},
	}
	logger := logging.NewNoopLogger(t)
	result := webhooks.ApplyResult{}
	manager.Send(logger, result) // nolint: errcheck
	sender.VerifyWasCalledOnce().Send(logger, result)
}

func TestSend_MultipleSuccess(t *testing.T) {
	t.Log("Sending multiple webhooks should succeed")
	RegisterMockTestingT(t)
	senders := []*mocks.MockSender{
		mocks.NewMockSender(),
		mocks.NewMockSender(),
		mocks.NewMockSender(),
	}
	manager := webhooks.MultiWebhookSender{
		Webhooks: []webhooks.Sender{senders[0], senders[1], senders[2]},
	}
	logger := logging.NewNoopLogger(t)
	result := webhooks.ApplyResult{}
	err := manager.Send(logger, result)
	Ok(t, err)
	for _, s := range senders {
		s.VerifyWasCalledOnce().Send(logger, result)
	}
}
