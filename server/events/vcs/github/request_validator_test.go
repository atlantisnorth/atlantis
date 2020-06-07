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

package github_test

import (
	"bytes"
	"net/http"
	"net/url"
	"testing"

	. "github.com/petergtz/pegomock"
	"github.com/runatlantis/atlantis/server/events/vcs/github"
	. "github.com/runatlantis/atlantis/testing"
)

func TestValidate_WithSecretErr(t *testing.T) {
	t.Log("if the request is not valid against the secret there is an error")
	RegisterMockTestingT(t)
	g := github.DefaultRequestValidator{}
	buf := bytes.NewBufferString("")
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Hub-Signature", "sha1=126f2c800419c60137ce748d7672e77b65cf16d6")
	req.Header.Set("Content-Type", "application/json")

	_, err = g.Validate(req, []byte("secret"))
	Assert(t, err != nil, "error should not be nil")
	Equals(t, "payload signature check failed", err.Error())
}

func TestValidate_WithSecret(t *testing.T) {
	t.Log("if the request is valid against the secret the payload is returned")
	RegisterMockTestingT(t)
	g := github.DefaultRequestValidator{}
	buf := bytes.NewBufferString(`{"yo":true}`)
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("X-Hub-Signature", "sha1=126f2c800419c60137ce748d7672e77b65cf16d6")
	req.Header.Set("Content-Type", "application/json")

	bs, err := g.Validate(req, []byte("0123456789abcdef"))
	Ok(t, err)
	Equals(t, `{"yo":true}`, string(bs))
}

func TestValidate_WithoutSecretInvalidContentType(t *testing.T) {
	t.Log("if the request has an invalid content type an error is returned")
	RegisterMockTestingT(t)
	g := github.DefaultRequestValidator{}
	buf := bytes.NewBufferString("")
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("Content-Type", "invalid")

	_, err = g.Validate(req, nil)
	Assert(t, err != nil, "error should not be nil")
	Equals(t, "webhook request has unsupported Content-Type \"invalid\"", err.Error())
}

func TestValidate_WithoutSecretJSON(t *testing.T) {
	t.Log("if the request is JSON the body is returned")
	RegisterMockTestingT(t)
	g := github.DefaultRequestValidator{}
	buf := bytes.NewBufferString(`{"yo":true}`)
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("Content-Type", "application/json")

	bs, err := g.Validate(req, nil)
	Ok(t, err)
	Equals(t, `{"yo":true}`, string(bs))
}

func TestValidate_WithoutSecretFormNoPayload(t *testing.T) {
	t.Log("if the request is form encoded and does not contain a payload param an error is returned")
	RegisterMockTestingT(t)
	g := github.DefaultRequestValidator{}
	buf := bytes.NewBufferString("")
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err = g.Validate(req, nil)
	Assert(t, err != nil, "error should not be nil")
	Equals(t, "webhook request did not contain expected 'payload' form value", err.Error())
}

func TestValidate_WithoutSecretForm(t *testing.T) {
	t.Log("if the request is form encoded and does not contain a payload param an error is returned")
	RegisterMockTestingT(t)
	g := github.DefaultRequestValidator{}
	form := url.Values{}
	form.Set("payload", `{"yo":true}`)
	buf := bytes.NewBufferString(form.Encode())
	req, err := http.NewRequest("POST", "http://localhost/event", buf)
	Ok(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	bs, err := g.Validate(req, nil)
	Ok(t, err)
	Equals(t, `{"yo":true}`, string(bs))
}
