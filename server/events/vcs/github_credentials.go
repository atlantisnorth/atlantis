package vcs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v31/github"
	"github.com/pkg/errors"
)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_github_credentials.go GithubCredentials

// GithubCredentials handles creating http.Clients that authenticate.
type GithubCredentials interface {
	Client() (*http.Client, error)
	GetToken() (string, error)
	GetUser() (string, error)
}

// GithubAnonymousCredentials expose no credentials.
type GithubAnonymousCredentials struct{}

// Client returns a client with no credentials.
func (c *GithubAnonymousCredentials) Client() (*http.Client, error) {
	tr := http.DefaultTransport
	return &http.Client{Transport: tr}, nil
}

// GetUser returns the username for these credentials.
func (c *GithubAnonymousCredentials) GetUser() (string, error) {
	return "anonymous", nil
}

// GetToken returns an empty token.
func (c *GithubAnonymousCredentials) GetToken() (string, error) {
	return "", nil
}

// GithubUserCredentials implements GithubCredentials for the personal auth token flow.
type GithubUserCredentials struct {
	User  string
	Token string
}

// Client returns a client for basic auth user credentials.
func (c *GithubUserCredentials) Client() (*http.Client, error) {
	tr := &github.BasicAuthTransport{
		Username: strings.TrimSpace(c.User),
		Password: strings.TrimSpace(c.Token),
	}
	return tr.Client(), nil
}

// GetUser returns the username for these credentials.
func (c *GithubUserCredentials) GetUser() (string, error) {
	return c.User, nil
}

// GetToken returns the user token.
func (c *GithubUserCredentials) GetToken() (string, error) {
	return c.Token, nil
}

// GithubAppCredentials implements GithubCredentials for github app installation token flow.
type GithubAppCredentials struct {
	AppID          int64
	Key            []byte
	Hostname       string
	apiURL         *url.URL
	installationID int64
	tr             *ghinstallation.Transport
	AppSlug        string
}

// Client returns a github app installation client.
func (c *GithubAppCredentials) Client() (*http.Client, error) {
	itr, err := c.transport()
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: itr}, nil
}

// GetUser returns the username for these credentials.
func (c *GithubAppCredentials) GetUser() (string, error) {
	// Keeping backwards compatibility since this flag is optional
	if c.AppSlug == "" {
		return "", nil
	}
	client, err := c.Client()

	if err != nil {
		return "", errors.Wrap(err, "initializing client")
	}

	ghClient := github.NewClient(client)
	ghClient.BaseURL = c.getAPIURL()
	ctx := context.Background()

	app, _, err := ghClient.Apps.Get(ctx, c.AppSlug)

	if err != nil {
		return "", errors.Wrap(err, "getting app details")
	}
	// Currently there is no way to get the bot's login info, so this is a
	// hack until Github exposes that.
	return fmt.Sprintf("%s[bot]", app.GetName()), nil
}

// GetToken returns a fresh installation token.
func (c *GithubAppCredentials) GetToken() (string, error) {
	tr, err := c.transport()
	if err != nil {
		return "", errors.Wrap(err, "transport failed")
	}

	return tr.Token(context.Background())
}

func (c *GithubAppCredentials) getInstallationID() (int64, error) {
	if c.installationID != 0 {
		return c.installationID, nil
	}

	tr := http.DefaultTransport
	// A non-installation transport
	t, err := ghinstallation.NewAppsTransport(tr, c.AppID, c.Key)
	if err != nil {
		return 0, err
	}
	t.BaseURL = c.getAPIURL().String()

	// Query github with the app's JWT
	client := github.NewClient(&http.Client{Transport: t})
	client.BaseURL = c.getAPIURL()
	ctx := context.Background()

	installations, _, err := client.Apps.ListInstallations(ctx, nil)
	if err != nil {
		return 0, err
	}

	if len(installations) != 1 {
		return 0, fmt.Errorf("wrong number of installations, expected 1, found %d", len(installations))
	}

	c.installationID = installations[0].GetID()
	return c.installationID, nil
}

func (c *GithubAppCredentials) transport() (*ghinstallation.Transport, error) {
	if c.tr != nil {
		return c.tr, nil
	}

	installationID, err := c.getInstallationID()
	if err != nil {
		return nil, err
	}

	tr := http.DefaultTransport
	itr, err := ghinstallation.New(tr, c.AppID, installationID, c.Key)
	if err == nil {
		apiURL := c.getAPIURL()
		itr.BaseURL = strings.TrimSuffix(apiURL.String(), "/")
		c.tr = itr
	}
	return itr, err
}

func (c *GithubAppCredentials) getAPIURL() *url.URL {
	if c.apiURL != nil {
		return c.apiURL
	}

	c.apiURL = resolveGithubAPIURL(c.Hostname)
	return c.apiURL
}

func resolveGithubAPIURL(hostname string) *url.URL {
	// If we're using github.com then we don't need to do any additional configuration
	// for the client. It we're using Github Enterprise, then we need to manually
	// set the base url for the API.
	baseURL := &url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path:   "/",
	}

	if hostname != "github.com" {
		baseURL.Host = hostname
		baseURL.Path = "/api/v3/"
	}

	return baseURL
}
