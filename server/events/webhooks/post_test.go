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
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/runatlantis/atlantis/server/events/webhooks"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

func TestSend_PostMessage(t *testing.T) {
	t.Log("Sending a hook should send a request")

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			Assert(t, req.URL.String() == "/", "wrong path")

			var bodyBytes []byte
			bodyBytes, _ = ioutil.ReadAll(req.Body)
			var result webhooks.ApplyResult
			err := json.Unmarshal(bodyBytes, &result)
			Ok(t, err)

			Assert(t, result.Workspace == "production", "wrong data")
		}))
	defer server.Close()

	hook := webhooks.PostWebhook{
		Client: server.Client(),
		URL:    server.URL,
	}
	result := webhooks.ApplyResult{
		Workspace: "production",
	}

	err := hook.Send(logging.NewNoopLogger(t), result)
	Ok(t, err)
}
