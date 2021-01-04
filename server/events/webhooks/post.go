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

package webhooks

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/runatlantis/atlantis/server/logging"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Sends POST requests webhooks to url.
type PostWebhook struct {
	URL    string
	Client HTTPClient
}

func NewPost(url string) *PostWebhook {
	return &PostWebhook{URL: url, Client: &http.Client{}}
}

// Send sends the webhook to the specified url
func (p *PostWebhook) Send(log logging.SimpleLogging, applyResult ApplyResult) error {
	requestJSON, err := json.Marshal(applyResult)
	if err != nil {
		return err
	}

	req, _ := http.NewRequest("POST", p.URL, bytes.NewBuffer(requestJSON))
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(req)

	if err == nil {
		resp.Body.Close()
	}
	return err
}
