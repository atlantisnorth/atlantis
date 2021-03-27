package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/runatlantis/atlantis/server/events/db"

	"github.com/gorilla/mux"
	"github.com/runatlantis/atlantis/server/events"
	"github.com/runatlantis/atlantis/server/events/locking"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs"
	"github.com/runatlantis/atlantis/server/logging"
)

// UnlockRequest is the format of requests made against /api/locks with the DELETE method to release atlantis locks
type UnlockRequest struct {
	LockIDs []string
}

// UnlockResponse is the format of responses made against /api/locks with the DELETE method
type UnlockResponse struct {
	Result []UnlockData
}

// UnlockData indicates whether the given Lock ID was able to be released
type UnlockData struct {
	LockID  string
	Success bool
}

// LocksController handles all requests relating to Atlantis locks.
type LocksController struct {
	AtlantisVersion    string
	AtlantisURL        *url.URL
	Locker             locking.Locker
	Logger             *logging.SimpleLogger
	VCSClient          vcs.Client
	LockDetailTemplate TemplateWriter
	WorkingDir         events.WorkingDir
	WorkingDirLocker   events.WorkingDirLocker
	DB                 *db.BoltDB
	DeleteLockCommand  events.DeleteLockCommand
}

// GetLocksResponse is returned to requests against GetLocks at /api/locks with the GET method. It contains a lock data object for each lock held by atlantis
type GetLocksResponse struct {
	Result []LockData
}

// LockData contains information about the lock, including the lock ID and which PR is holding the lock
type LockData struct {
	PullRequestURL string
	LockID         string
}

// GetLocks response to requests against /api/locks with a marshaled GetLocksResponse object that contains information about all open locks
func (l *LocksController) GetLocks(w http.ResponseWriter, _ *http.Request) {
	var result []LockData
	locks, err := l.Locker.List()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error listing locks: %s", err)
		return
	}
	for key, lock := range locks {
		result = append(result, LockData{PullRequestURL: lock.Pull.URL, LockID: key})
	}
	data, err := json.Marshal(GetLocksResponse{Result: result})
	if err != nil {
		l.respond(w, logging.Error, http.StatusInternalServerError, "Error creating list response: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		l.respond(w, logging.Error, http.StatusInternalServerError, "Error writing list response: %s", err)
	}
}

// GetLock is the GET /locks/{id} route. It renders the lock detail view.
func (l *LocksController) GetLock(w http.ResponseWriter, r *http.Request) {
	id, ok := mux.Vars(r)["id"]
	if !ok {
		l.respond(w, logging.Warn, http.StatusBadRequest, "No lock id in request")
		return
	}

	idUnencoded, err := url.QueryUnescape(id)
	if err != nil {
		l.respond(w, logging.Warn, http.StatusBadRequest, "Invalid lock id: %s", err)
		return
	}
	lock, err := l.Locker.GetLock(idUnencoded)
	if err != nil {
		l.respond(w, logging.Error, http.StatusInternalServerError, "Failed getting lock: %s", err)
		return
	}
	if lock == nil {
		l.respond(w, logging.Info, http.StatusNotFound, "No lock found at id %q", idUnencoded)
		return
	}

	owner, repo := models.SplitRepoFullName(lock.Project.RepoFullName)
	viewData := LockDetailData{
		LockKeyEncoded:  id,
		LockKey:         idUnencoded,
		PullRequestLink: lock.Pull.URL,
		LockedBy:        lock.Pull.Author,
		Workspace:       lock.Workspace,
		AtlantisVersion: l.AtlantisVersion,
		CleanedBasePath: l.AtlantisURL.Path,
		RepoOwner:       owner,
		RepoName:        repo,
	}

	err = l.LockDetailTemplate.Execute(w, viewData)
	if err != nil {
		l.Logger.Err(err.Error())
	}
}

// DeleteLock handles deleting the lock at id and commenting back on the
// pull request that the lock has been deleted.
func (l *LocksController) DeleteLock(w http.ResponseWriter, r *http.Request) {
	id, ok := mux.Vars(r)["id"]
	if !ok || id == "" {
		l.respond(w, logging.Warn, http.StatusBadRequest, "No lock id in request")
		return
	}

	idUnencoded, err := url.PathUnescape(id)
	if err != nil {
		l.respond(w, logging.Warn, http.StatusBadRequest, "Invalid lock id %q. Failed with error: %s", id, err)
		return
	}

	lock, err := l.DeleteLockCommand.DeleteLock(idUnencoded)
	if err != nil {
		l.respond(w, logging.Error, http.StatusInternalServerError, "deleting lock failed with: %s", err)
		return
	}

	if lock == nil {
		l.respond(w, logging.Info, http.StatusNotFound, "No lock found at id %q", idUnencoded)
		return
	}
	err = l.clearLock(lock)
	if err != nil {
		l.respond(w, logging.Error, http.StatusInternalServerError, "Failed unlocking the pull request: %s", err)
	}
	if lock.Pull.BaseRepo != (models.Repo{}) {
		// Once the lock has been deleted, comment back on the pull request.
		comment := fmt.Sprintf("**Warning**: The plan for dir: `%s` workspace: `%s` was **discarded** via the Atlantis UI.\n\n"+
			"To `apply` this plan you must run `plan` again.", lock.Project.Path, lock.Workspace)
		if err = l.VCSClient.CreateComment(lock.Pull.BaseRepo, lock.Pull.Num, comment, ""); err != nil {
			l.Logger.Warn("failed commenting on pull request: %s", err)
		}
	}
	l.respond(w, logging.Info, http.StatusOK, "Deleted lock id %q", id)
}

// Unlock accepts json list of lock IDs
func (l *LocksController) Unlock(w http.ResponseWriter, r *http.Request) {
	// Parse the JSON payload
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		l.respond(w, logging.Error, http.StatusBadRequest, "Invalid unlock request. Failed to read request: %s", err)
		return
	}
	var request UnlockRequest
	if err = json.Unmarshal(bytes, &request); err != nil {
		l.respond(w, logging.Error, http.StatusBadRequest, "Invalid unlock request. Failed with error: %s", err)
		return
	}
	if len(request.LockIDs) == 0 {
		l.respond(w, logging.Error, http.StatusBadRequest, "Invalid unlock request. No IDs specified in request")
		return
	}
	var result []UnlockData
	for _, id := range request.LockIDs {
		lock, err := l.Locker.Unlock(id)
		if err != nil {
			result = append(result, UnlockData{LockID: id, Success: false})
			continue
		}
		if lock == nil {
			// if there is no lock, we will consider that a success to make this idempotent
			result = append(result, UnlockData{LockID: id, Success: true})
			continue
		}
		err = l.clearLock(lock)
		if err != nil {
			result = append(result, UnlockData{LockID: id, Success: false})
		} else {
			result = append(result, UnlockData{LockID: id, Success: true})
		}
	}
	data, err := json.Marshal(UnlockResponse{Result: result})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error creating unlock response: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error writing unlock response: %s", err)
		return
	}
}

func (l *LocksController) clearLock(lock *models.ProjectLock) error {
	if lock.Pull.BaseRepo != (models.Repo{}) {
		unlock, err := l.WorkingDirLocker.TryLock(lock.Pull.BaseRepo.FullName, lock.Pull.Num, lock.Workspace)
		if err != nil {
			l.Logger.Err("unable to obtain working dir lock when trying to delete old plans: %s", err)
		} else {
			defer unlock()
			// nolint: vetshadow
			if err := l.WorkingDir.DeleteForWorkspace(lock.Pull.BaseRepo, lock.Pull, lock.Workspace); err != nil {
				l.Logger.Err("unable to delete workspace: %s", err)
				return err
			}
		}
		if err := l.DB.UpdateProjectStatus(lock.Pull, lock.Workspace, lock.Project.Path, models.DiscardedPlanStatus); err != nil {
			l.Logger.Err("unable to update project status: %s", err)
			return err
		}
	} else {
		l.Logger.Debug("skipping deleting workspace because BaseRepo field is empty")
	}
	return nil
}

// respond is a helper function to respond and log the response. lvl is the log
// level to log at, code is the HTTP response code.
func (l *LocksController) respond(w http.ResponseWriter, lvl logging.LogLevel, responseCode int, format string, args ...interface{}) {
	response := fmt.Sprintf(format, args...)
	l.Logger.Log(lvl, response)
	w.WriteHeader(responseCode)
	fmt.Fprintln(w, response)
}
