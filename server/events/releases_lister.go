package events

import "github.com/warrensbox/terraform-switcher/lib"

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_releases_lister.go ReleasesLister

// ReleasesLister is an interface to get and list releases

const (
	tfReleasesURL = "https://releases.hashicorp.com/terraform/"
)

type ReleasesLister interface {
	ListReleases() ([]string, error)
}

type DefaultReleasesLister struct{}

func (d DefaultReleasesLister) ListReleases() ([]string, error) {
	includePrerelease := true
	return lib.GetTFList(tfReleasesURL, includePrerelease)
}
