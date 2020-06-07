package testing

import (
	"crypto/tls"
	"net/http"
)

// DisableSSLVerification disables ssl verification for the global http client
// and returns a function to be called in a defer that will re-enable it.
func DisableSSLVerification() func() {
	orig := http.DefaultTransport.(*http.Transport).TLSClientConfig
	// nolint: gosec
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return func() {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = orig
	}
}
