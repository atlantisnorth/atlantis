package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/runatlantis/atlantis/server/events/db"

	"github.com/gorilla/mux"
	. "github.com/petergtz/pegomock"
	"github.com/runatlantis/atlantis/server"
	"github.com/runatlantis/atlantis/server/events"

	"github.com/runatlantis/atlantis/server/events/locking/mocks"
	mocks2 "github.com/runatlantis/atlantis/server/events/mocks"
	"github.com/runatlantis/atlantis/server/events/models"
	vcsmocks "github.com/runatlantis/atlantis/server/events/vcs/mocks"
	"github.com/runatlantis/atlantis/server/logging"
	sMocks "github.com/runatlantis/atlantis/server/mocks"
	. "github.com/runatlantis/atlantis/testing"
)

func AnyRepo() models.Repo {
	RegisterMatcher(NewAnyMatcher(reflect.TypeOf(models.Repo{})))
	return models.Repo{}
}

func TestGetLocks_LockerErr(t *testing.T) {
	t.Log("If there is an error retrieving the locks, make sure it is propagated")
	RegisterMockTestingT(t)
	l := mocks.NewMockLocker()
	When(l.List()).ThenReturn(nil, errors.New("err"))
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
		Locker: l,
	}
	req, _ := http.NewRequest("GET", "/api/locks", bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	lc.GetLocks(w, req)
	responseContains(t, w, http.StatusInternalServerError, "Error listing locks")
}

func TestGetLocks_None(t *testing.T) {
	t.Log("If are no locks, we should return an empty list")
	RegisterMockTestingT(t)
	l := mocks.NewMockLocker()
	When(l.List()).ThenReturn(make(map[string]models.ProjectLock), nil)
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
		Locker: l,
	}
	req, _ := http.NewRequest("GET", "/api/locks", bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	lc.GetLocks(w, req)
	Assert(t, w.Code == 200, "expected successful status code from GetLocks but got %s", w.Code)
	body, err := ioutil.ReadAll(w.Result().Body)
	Ok(t, err)
	var response server.GetLocksResponse
	err = json.Unmarshal(body, &response)
	Ok(t, err)
	Assert(t, len(response.Result) == 0, "did not expect any locks to be returned")
}

func TestGetLocks_Success(t *testing.T) {
	t.Log("Should be able to return list of PRs to locks held by PR")
	RegisterMockTestingT(t)
	l := mocks.NewMockLocker()
	listResponse := make(map[string]models.ProjectLock)
	listResponse["test"] = models.ProjectLock{
		Pull: models.PullRequest{URL: "url", Author: "lkysow"},
	}
	When(l.List()).ThenReturn(listResponse, nil)
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
		Locker: l,
	}
	req, _ := http.NewRequest("GET", "/api/locks", bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	lc.GetLocks(w, req)
	Assert(t, w.Code == 200, "expected successful status code from GetLocks but got %s", w.Code)
	body, err := ioutil.ReadAll(w.Result().Body)
	Ok(t, err)
	var response server.GetLocksResponse
	err = json.Unmarshal(body, &response)
	Ok(t, err)
	Assert(t, len(response.Result) == 1, "expected single Result list GetLocks, was instead length %i", len(response.Result))
	testMap := response.Result[0]
	expectedVal := "url"
	Assert(t, testMap.PullRequestURL == expectedVal, "expected lock map ID to equal %s, was %s", expectedVal, testMap.PullRequestURL)
	Assert(t, testMap.LockID == "test", "expected lock map ID to equal %s, was %s", "test", testMap.LockID)
}

func TestGetLocks_MultipleSuccess(t *testing.T) {
	t.Log("Should be able to list multiple locks")
	RegisterMockTestingT(t)
	l := mocks.NewMockLocker()
	listResponse := make(map[string]models.ProjectLock)
	listResponse["test"] = models.ProjectLock{
		Pull: models.PullRequest{URL: "url", Author: "lkysow"},
	}
	listResponse["testTwo"] = models.ProjectLock{
		Pull: models.PullRequest{URL: "url", Author: "lkysow"},
	}
	listResponse["testThree"] = models.ProjectLock{
		Pull: models.PullRequest{URL: "urlTwo", Author: "lkysow"},
	}
	When(l.List()).ThenReturn(listResponse, nil)
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
		Locker: l,
	}
	req, _ := http.NewRequest("GET", "/api/locks", bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	lc.GetLocks(w, req)
	Assert(t, w.Code == 200, "expected successful status code from GetLocks but got %s", w.Code)
	body, err := ioutil.ReadAll(w.Result().Body)
	Ok(t, err)
	var response server.GetLocksResponse
	err = json.Unmarshal(body, &response)
	Ok(t, err)
	Assert(t, len(response.Result) == 3, "expected map with three entries from GetLocks, was instead length %i", len(response.Result))
	for _, lockMap := range response.Result {
		if lockMap.PullRequestURL == "url" {
			if lockMap.LockID != "testTwo" && lockMap.LockID != "test" {
				Assert(t, false, "unexpected lock ID - found lock with url %s and ID %s", lockMap.PullRequestURL, lockMap.LockID)
			}
		} else if lockMap.PullRequestURL == "urlTwo" {
			expectedLockID := "testThree"
			Assert(t, lockMap.LockID == expectedLockID, "expected PR at %s to have lock ID %s but found %s instead", lockMap.PullRequestURL, lockMap.LockID, expectedLockID)
		} else {
			Assert(t, false, "Found PR with unexpected URL - %s", lockMap.PullRequestURL)
		}
	}
}

func TestGetLockRoute_NoLockID(t *testing.T) {
	t.Log("If there is no lock ID in the request then we should get a 400")
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
	}
	lc.GetLock(w, req)
	responseContains(t, w, http.StatusBadRequest, "No lock id in request")
}

func TestGetLock_InvalidLockID(t *testing.T) {
	t.Log("If the lock ID is invalid then we should get a 400")
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "%A@"})
	w := httptest.NewRecorder()
	lc.GetLock(w, req)
	responseContains(t, w, http.StatusBadRequest, "Invalid lock id")
}

func TestGetLock_LockerErr(t *testing.T) {
	t.Log("If there is an error retrieving the lock, a 500 is returned")
	RegisterMockTestingT(t)
	l := mocks.NewMockLocker()
	When(l.GetLock("id")).ThenReturn(nil, errors.New("err"))
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
		Locker: l,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.GetLock(w, req)
	responseContains(t, w, http.StatusInternalServerError, "err")
}

func TestGetLock_None(t *testing.T) {
	t.Log("If there is no lock at that ID we get a 404")
	RegisterMockTestingT(t)
	l := mocks.NewMockLocker()
	When(l.GetLock("id")).ThenReturn(nil, nil)
	lc := server.LocksController{
		Logger: logging.NewNoopLogger(),
		Locker: l,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.GetLock(w, req)
	responseContains(t, w, http.StatusNotFound, "No lock found at id \"id\"")
}

func TestGetLock_Success(t *testing.T) {
	t.Log("Should be able to render a lock successfully")
	RegisterMockTestingT(t)
	l := mocks.NewMockLocker()
	When(l.GetLock("id")).ThenReturn(&models.ProjectLock{
		Project:   models.Project{RepoFullName: "owner/repo", Path: "path"},
		Pull:      models.PullRequest{URL: "url", Author: "lkysow"},
		Workspace: "workspace",
	}, nil)
	tmpl := sMocks.NewMockTemplateWriter()
	atlantisURL, err := url.Parse("https://example.com/basepath")
	Ok(t, err)
	lc := server.LocksController{
		Logger:             logging.NewNoopLogger(),
		Locker:             l,
		LockDetailTemplate: tmpl,
		AtlantisVersion:    "1300135",
		AtlantisURL:        atlantisURL,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.GetLock(w, req)
	tmpl.VerifyWasCalledOnce().Execute(w, server.LockDetailData{
		LockKeyEncoded:  "id",
		LockKey:         "id",
		RepoOwner:       "owner",
		RepoName:        "repo",
		PullRequestLink: "url",
		LockedBy:        "lkysow",
		Workspace:       "workspace",
		AtlantisVersion: "1300135",
		CleanedBasePath: "/basepath",
	})
	responseContains(t, w, http.StatusOK, "")
}

func TestDeleteLock_NoLockID(t *testing.T) {
	t.Log("If there is no lock ID in the request then we should get a 400")
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	lc := server.LocksController{Logger: logging.NewNoopLogger()}
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusBadRequest, "No lock id in request")
}

func TestDeleteLock_InvalidLockID(t *testing.T) {
	t.Log("If the lock ID is invalid then we should get a 400")
	lc := server.LocksController{Logger: logging.NewNoopLogger()}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "%A@"})
	w := httptest.NewRecorder()
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusBadRequest, "Invalid lock id \"%A@\"")
}

func TestDeleteLock_LockerErr(t *testing.T) {
	t.Log("If there is an error retrieving the lock, a 500 is returned")
	RegisterMockTestingT(t)
	dlc := mocks2.NewMockDeleteLockCommand()
	When(dlc.DeleteLock("id")).ThenReturn(nil, errors.New("err"))
	lc := server.LocksController{
		DeleteLockCommand: dlc,
		Logger:            logging.NewNoopLogger(),
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusInternalServerError, "err")
}

func TestDeleteLock_None(t *testing.T) {
	t.Log("If there is no lock at that ID we get a 404")
	RegisterMockTestingT(t)
	dlc := mocks2.NewMockDeleteLockCommand()
	When(dlc.DeleteLock("id")).ThenReturn(nil, nil)
	lc := server.LocksController{
		DeleteLockCommand: dlc,
		Logger:            logging.NewNoopLogger(),
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusNotFound, "No lock found at id \"id\"")
}

func TestDeleteLock_OldFormat(t *testing.T) {
	t.Log("If the lock doesn't have BaseRepo set it is deleted successfully")
	RegisterMockTestingT(t)
	cp := vcsmocks.NewMockClient()
	dlc := mocks2.NewMockDeleteLockCommand()
	When(dlc.DeleteLock("id")).ThenReturn(&models.ProjectLock{}, nil)
	lc := server.LocksController{
		DeleteLockCommand: dlc,
		Logger:            logging.NewNoopLogger(),
		VCSClient:         cp,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusOK, "Deleted lock id \"id\"")
	cp.VerifyWasCalled(Never()).CreateComment(AnyRepo(), AnyInt(), AnyString(), AnyString())
}

func TestDeleteLock_UpdateProjectStatus(t *testing.T) {
	t.Log("When deleting a lock, pull status has to be updated to reflect discarded plan")
	RegisterMockTestingT(t)

	repoName := "owner/repo"
	projectPath := "path"
	workspaceName := "workspace"

	cp := vcsmocks.NewMockClient()
	l := mocks2.NewMockDeleteLockCommand()
	workingDir := mocks2.NewMockWorkingDir()
	workingDirLocker := events.NewDefaultWorkingDirLocker()
	pull := models.PullRequest{
		BaseRepo: models.Repo{FullName: repoName},
	}
	When(l.DeleteLock("id")).ThenReturn(&models.ProjectLock{
		Pull:      pull,
		Workspace: workspaceName,
		Project: models.Project{
			Path:         projectPath,
			RepoFullName: repoName,
		},
	}, nil)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	db, err := db.New(tmp)
	Ok(t, err)
	// Seed the DB with a successful plan for that project (that is later discarded).
	_, err = db.UpdatePullWithResults(pull, []models.ProjectResult{
		{
			Command:    models.PlanCommand,
			RepoRelDir: projectPath,
			Workspace:  workspaceName,
			PlanSuccess: &models.PlanSuccess{
				TerraformOutput: "tf-output",
				LockURL:         "lock-url",
			},
		},
	})
	Ok(t, err)
	lc := server.LocksController{
		DeleteLockCommand: l,
		Logger:            logging.NewNoopLogger(),
		VCSClient:         cp,
		WorkingDirLocker:  workingDirLocker,
		WorkingDir:        workingDir,
		DB:                db,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusOK, "Deleted lock id \"id\"")
	status, err := db.GetPullStatus(pull)
	Ok(t, err)
	Assert(t, status != nil, "status was nil")
	Equals(t, []models.ProjectStatus{
		{
			Workspace:  workspaceName,
			RepoRelDir: projectPath,
			Status:     models.DiscardedPlanStatus,
		},
	}, status.Projects)
}

func TestDeleteLock_CommentFailed(t *testing.T) {
	t.Log("If the commenting fails we still return success")
	RegisterMockTestingT(t)
	dlc := mocks2.NewMockDeleteLockCommand()
	When(dlc.DeleteLock("id")).ThenReturn(&models.ProjectLock{
		Pull: models.PullRequest{
			BaseRepo: models.Repo{FullName: "owner/repo"},
		},
	}, nil)
	cp := vcsmocks.NewMockClient()
	workingDir := mocks2.NewMockWorkingDir()
	workingDirLocker := events.NewDefaultWorkingDirLocker()
	When(cp.CreateComment(AnyRepo(), AnyInt(), AnyString(), AnyString())).ThenReturn(errors.New("err"))
	tmp, cleanup := TempDir(t)
	defer cleanup()
	db, err := db.New(tmp)
	Ok(t, err)
	lc := server.LocksController{
		DeleteLockCommand: dlc,
		Logger:            logging.NewNoopLogger(),
		VCSClient:         cp,
		WorkingDir:        workingDir,
		WorkingDirLocker:  workingDirLocker,
		DB:                db,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusOK, "Deleted lock id \"id\"")
}

func TestDeleteLock_CommentSuccess(t *testing.T) {
	t.Log("We should comment back on the pull request if the lock is deleted")
	RegisterMockTestingT(t)
	cp := vcsmocks.NewMockClient()
	dlc := mocks2.NewMockDeleteLockCommand()
	workingDir := mocks2.NewMockWorkingDir()
	workingDirLocker := events.NewDefaultWorkingDirLocker()
	pull := models.PullRequest{
		BaseRepo: models.Repo{FullName: "owner/repo"},
	}
	When(dlc.DeleteLock("id")).ThenReturn(&models.ProjectLock{
		Pull:      pull,
		Workspace: "workspace",
		Project: models.Project{
			Path:         "path",
			RepoFullName: "owner/repo",
		},
	}, nil)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	db, err := db.New(tmp)
	Ok(t, err)
	lc := server.LocksController{
		DeleteLockCommand: dlc,
		Logger:            logging.NewNoopLogger(),
		VCSClient:         cp,
		DB:                db,
		WorkingDir:        workingDir,
		WorkingDirLocker:  workingDirLocker,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(nil))
	req = mux.SetURLVars(req, map[string]string{"id": "id"})
	w := httptest.NewRecorder()
	lc.DeleteLock(w, req)
	responseContains(t, w, http.StatusOK, "Deleted lock id \"id\"")
	cp.VerifyWasCalled(Once()).CreateComment(pull.BaseRepo, pull.Num,
		"**Warning**: The plan for dir: `path` workspace: `workspace` was **discarded** via the Atlantis UI.\n\n"+
			"To `apply` this plan you must run `plan` again.", "")
}

func TestUnlock_MalformedJSON(t *testing.T) {
	t.Log("If there is no UnlockRequest JSON in the request then we should get a 400")
	req, _ := http.NewRequest("POST", "", bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	lc := server.LocksController{Logger: logging.NewNoopLogger()}
	lc.Unlock(w, req)
	responseContains(t, w, http.StatusBadRequest, "Invalid unlock request. Failed with error")
}

func TestUnlock_EmptyLockID(t *testing.T) {
	t.Log("If no lock IDs are provided then we should get a 400")
	lc := server.LocksController{Logger: logging.NewNoopLogger()}
	requestJSON := []byte("{ \"LockIDs\": [] }")
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(requestJSON))
	w := httptest.NewRecorder()
	lc.Unlock(w, req)
	responseContains(t, w, http.StatusBadRequest, "No IDs specified")
}

func TestUnlock_LockerErr(t *testing.T) {
	t.Log("If the locker fails to unlock then we should see the failure propagated")
	requestJSON := []byte("{ \"LockIDs\": [\"id\"] }")
	l := mocks.NewMockLocker()
	pull := models.PullRequest{
		BaseRepo: models.Repo{FullName: "owner/repo"},
	}
	When(l.Unlock("id")).ThenReturn(&models.ProjectLock{
		Pull:      pull,
		Workspace: "workspace",
		Project: models.Project{
			Path:         "path",
			RepoFullName: "owner/repo",
		},
	}, errors.New("err"))
	lc := server.LocksController{
		Locker: l,
		Logger: logging.NewNoopLogger(),
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(requestJSON))
	w := httptest.NewRecorder()
	lc.Unlock(w, req)
	responseContains(t, w, http.StatusOK, "\"Success\":false")
}

func TestUnlock_NoSuchLock(t *testing.T) {
	t.Log("If there is no lock at that ID we get a successful response")
	requestJSON := []byte("{ \"LockIDs\": [\"id\"] }")
	l := mocks.NewMockLocker()
	When(l.Unlock("id")).ThenReturn(nil, nil)
	lc := server.LocksController{
		Locker: l,
		Logger: logging.NewNoopLogger(),
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(requestJSON))
	w := httptest.NewRecorder()
	lc.Unlock(w, req)
	responseContains(t, w, http.StatusOK, "\"Success\":true")
}

func TestUnlock_Success(t *testing.T) {
	t.Log("On successful unlock we get a successful response")
	requestJSON := []byte("{ \"LockIDs\": [\"id\"] }")
	l := mocks.NewMockLocker()
	workingDir := mocks2.NewMockWorkingDir()
	workingDirLocker := events.NewDefaultWorkingDirLocker()
	tmp, cleanup := TempDir(t)
	defer cleanup()
	db, err := db.New(tmp)
	Ok(t, err)
	pull := models.PullRequest{
		BaseRepo: models.Repo{FullName: "owner/repo"},
	}
	When(l.Unlock("id")).ThenReturn(&models.ProjectLock{
		Pull:      pull,
		Workspace: "workspace",
		Project: models.Project{
			Path:         "path",
			RepoFullName: "owner/repo",
		},
	}, nil)
	lc := server.LocksController{
		Locker:           l,
		Logger:           logging.NewNoopLogger(),
		WorkingDirLocker: workingDirLocker,
		WorkingDir:       workingDir,
		DB:               db,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(requestJSON))
	w := httptest.NewRecorder()
	lc.Unlock(w, req)
	responseContains(t, w, http.StatusOK, "\"Success\":true")
}

func TestUnlock_MultipleSuccess(t *testing.T) {
	t.Log("We can unlock multiple locks")
	requestJSON := []byte("{ \"LockIDs\": [\"id\", \"id2\"] }")
	l := mocks.NewMockLocker()
	workingDir := mocks2.NewMockWorkingDir()
	workingDirLocker := events.NewDefaultWorkingDirLocker()
	tmp, cleanup := TempDir(t)
	defer cleanup()
	db, err := db.New(tmp)
	Ok(t, err)
	pull := models.PullRequest{
		BaseRepo: models.Repo{FullName: "owner/repo"},
	}
	When(l.Unlock("id")).ThenReturn(&models.ProjectLock{
		Pull:      pull,
		Workspace: "workspace",
		Project: models.Project{
			Path:         "path",
			RepoFullName: "owner/repo",
		},
	}, nil)
	lc := server.LocksController{
		Locker:           l,
		Logger:           logging.NewNoopLogger(),
		WorkingDirLocker: workingDirLocker,
		WorkingDir:       workingDir,
		DB:               db,
	}
	req, _ := http.NewRequest("GET", "", bytes.NewBuffer(requestJSON))
	w := httptest.NewRecorder()
	lc.Unlock(w, req)
	body, err := ioutil.ReadAll(w.Result().Body)
	Ok(t, err)
	var response server.UnlockResponse
	err = json.Unmarshal(body, &response)
	Ok(t, err)
	expectedLen := 2
	Assert(t, len(response.Result) == expectedLen, "expected response to be of length %s, was %s instead", expectedLen, len(response.Result))
	for _, lockResult := range response.Result {
		Assert(t, lockResult.Success, "expected lock to be released successfully")
	}
}
