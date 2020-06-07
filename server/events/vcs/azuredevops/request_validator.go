package azuredevops

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/mcdafydd/go-azuredevops/azuredevops"
)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_azuredevops_request_validator.go AzureDevopsRequestValidator

// RequestValidator handles checking if Azure DevOps requests
// contain a valid Basic authentication username and password.
type RequestValidator interface {
	// Validate returns the JSON payload of the request.
	// If both username and password values have a length greater than zero,
	// it checks that the credentials match those configured in Atlantis.
	// If either username or password have a length of zero, the payload is
	// returned without further checking.
	Validate(r *http.Request, user []byte, pass []byte) ([]byte, error)
}

// DefaultAzureDevopsRequestValidator handles checking if Azure DevOps
// requests contain the correct Basic auth username and password.
type DefaultAzureDevopsRequestValidator struct{}

// Validate returns the JSON payload of the request.
// If secret is not empty, it checks that the request was signed
// by secret and returns an error if it was not.
// If secret is empty, it does not check if the request was signed.
func (d *DefaultAzureDevopsRequestValidator) Validate(r *http.Request, user []byte, pass []byte) ([]byte, error) {
	if len(user) != 0 && len(pass) != 0 {
		return d.validateWithBasicAuth(r, user, pass)
	}
	return d.validateWithoutBasicAuth(r)
}

func (d *DefaultAzureDevopsRequestValidator) validateWithBasicAuth(r *http.Request, user []byte, pass []byte) ([]byte, error) {
	payload, err := azuredevops.ValidatePayload(r, user, pass)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (d *DefaultAzureDevopsRequestValidator) validateWithoutBasicAuth(r *http.Request) ([]byte, error) {
	ct := r.Header.Get("Content-Type")
	if ct == "application/json" || ct == "application/json; charset=utf-8" {
		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("could not read body: %s", err)
		}
		return payload, nil
	}
	return nil, fmt.Errorf("webhook request has unsupported Content-Type %q", ct)
}
