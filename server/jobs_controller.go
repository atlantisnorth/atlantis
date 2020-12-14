package server

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/logging"
)

// Concurrency model in this controller:
//
// - We keep a buffer of all the output so far for each PR
// - We keep a set of clients for each PR
// - When a new client is added, the entire buffer is sent through so
//   it is "up to date"
// - When a new line of output is added, we send it to all current
//   clients and then add it to the buffer
// - Changes to the buffer or set of clients are wrapped in a mutex
//
// This maintains the invariant that whenever a function call returns,
// all of the clients have seen every message that is in the buffer.
//
// When we get a message that tells us to clear the buffer, we just do
// it, without telling any clients about it. This won't have any
// effect on the frontend, except that newly connected clients won't
// get the old messages sent to them, because the server has
// "forgotten" about them. We clear the buffer for a PR when starting
// and finishing processing that PR, because after the processing has
// finished the output is posted to GitHub anyway and it doesn't need
// to be persisted in the Atlantis web UI.

type JobsController struct {
	AtlantisVersion     string
	AtlantisURL         *url.URL
	Logger              *logging.SimpleLogger
	JobTemplate         TemplateWriter
	TerraformOutputChan <-chan *models.TerraformOutputLine

	logBuffers map[string][]string
	wsChans    map[string]map[chan string]bool
	chanLock   sync.Mutex
}

// Register client to get Terraform output for PR. Send all the output
// currently in the buffer.
func (j *JobsController) addChan(pull string) chan string {
	ch := make(chan string, 1000)
	j.chanLock.Lock()
	for _, line := range j.logBuffers[pull] {
		ch <- line
	}
	if j.wsChans == nil {
		j.wsChans = map[string]map[chan string]bool{}
	}
	if j.wsChans[pull] == nil {
		j.wsChans[pull] = map[chan string]bool{}
	}
	j.wsChans[pull][ch] = true
	j.chanLock.Unlock()
	return ch
}

// Deregister client to no longer get Terraform output for PR.
func (j *JobsController) removeChan(pull string, ch chan string) {
	j.chanLock.Lock()
	delete(j.wsChans[pull], ch)
	j.chanLock.Unlock()
}

// Clear out log lines that are in the buffer (which would be
// immediately received by a newly connecting client). This won't
// cause anything to be deleted from an existing client webpage, but
// it does mean the data won't reappear on a refresh.
func (j *JobsController) clearLogLines(pull string) {
	j.chanLock.Lock()
	delete(j.logBuffers, pull)
	j.chanLock.Unlock()
}

// Add a log line to the buffer and send it to all clients.
func (j *JobsController) writeLogLine(pull string, line string) {
	j.chanLock.Lock()
	if j.logBuffers == nil {
		j.logBuffers = map[string][]string{}
	}
	for ch, _ := range j.wsChans[pull] {
		select {
		case ch <- line:
		default:
			// If we get to this case then it means that
			// the channel is full, which probably means
			// that the client has disconnected, which
			// results in items not getting pulled off the
			// channel to send to the websocket, which
			// means we should stop stuffing things into
			// that channel.
			delete(j.wsChans[pull], ch)
		}
	}
	if j.logBuffers[pull] == nil {
		j.logBuffers[pull] = []string{}
	}
	j.logBuffers[pull] = append(j.logBuffers[pull], line)
	j.chanLock.Unlock()
}

// Listens to the Terraform output channel and distributes messages to
// all clients. This function blocks, please invoke it in a goroutine.
func (j *JobsController) Listen() {
	for msg := range j.TerraformOutputChan {
		if msg.ClearBefore {
			j.clearLogLines(msg.PullSlug)
		}
		j.writeLogLine(msg.PullSlug, msg.Line)
		if msg.ClearAfter {
			j.clearLogLines(msg.PullSlug)
		}
	}
}

// Gets the PR slug (e.g. "plaid/go/4423") from the HTTP request
// parameters.
func getPullSlug(r *http.Request) (string, error) {
	org, ok := mux.Vars(r)["org"]
	if !ok {
		return "", fmt.Errorf("Internal error: no org in route")
	}
	repo, ok := mux.Vars(r)["repo"]
	if !ok {
		return "", fmt.Errorf("Internal error: no repo in route")
	}
	pull, ok := mux.Vars(r)["pull"]
	if !ok {
		return "", fmt.Errorf("Internal error: no pull in route")
	}
	return fmt.Sprintf("%s/%s/%s", org, repo, pull), nil
}

func (j *JobsController) GetJob(w http.ResponseWriter, r *http.Request) {
	pullSlug, err := getPullSlug(r)
	if err != nil {
		j.respond(w, logging.Error, http.StatusInternalServerError, err.Error())
		return
	}

	viewData := JobData{
		AtlantisVersion: j.AtlantisVersion,
		PullSlug:        pullSlug,
		CleanedBasePath: j.AtlantisURL.Path,
	}

	if err := j.JobTemplate.Execute(w, viewData); err != nil {
		j.Logger.Err(err.Error())
	}
}

var upgrader = websocket.Upgrader{}

func (j *JobsController) GetJobWS(w http.ResponseWriter, r *http.Request) {
	pullSlug, err := getPullSlug(r)
	if err != nil {
		j.respond(w, logging.Error, http.StatusInternalServerError, err.Error())
		return
	}

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		j.Logger.Warn("Failed to upgrade websocket: %s", err)
		return
	}

	defer c.Close()

	ch := j.addChan(pullSlug)
	defer j.removeChan(pullSlug, ch)

	for msg := range ch {
		// Hopefully, the websocket library has the good sense
		// to return an error eventually from the WriteMessage
		// call when the connection hangs, instead of just
		// letting the connection sit open forever.
		if err := c.WriteMessage(websocket.BinaryMessage, []byte(msg+"\r\n")); err != nil {
			j.Logger.Warn("Failed to write ws message: %s", err)
			return
		}
	}
}

// respond is a helper function to respond and log the response. lvl is the log
// level to log at, code is the HTTP response code.
func (j *JobsController) respond(w http.ResponseWriter, lvl logging.LogLevel, responseCode int, format string, args ...interface{}) {
	response := fmt.Sprintf(format, args...)
	j.Logger.Log(lvl, response)
	w.WriteHeader(responseCode)
	fmt.Fprintln(w, response)
}
