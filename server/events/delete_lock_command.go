package events

import (
	"github.com/runatlantis/atlantis/server/events/db"
	"github.com/runatlantis/atlantis/server/events/locking"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/logging"
)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_delete_lock_command.go DeleteLockCommand

// DeleteLockCommand is the first step after a command request has been parsed.
type DeleteLockCommand interface {
	DeleteLock(id string) (*models.ProjectLock, error)
	DeleteLocksByPull(repoFullName string, pullNum int) error
}

type JobURLGenerator interface {
	GenerateJobURL(pull models.PullRequest) string
}

// DefaultDeleteLockCommand deletes a specific lock after a request from the LocksController.
type DefaultDeleteLockCommand struct {
	Locker           locking.Locker
	Logger           *logging.SimpleLogger
	WorkingDir       WorkingDir
	WorkingDirLocker WorkingDirLocker
	DB               *db.BoltDB
}

// DeleteLock handles deleting the lock at id
func (l *DefaultDeleteLockCommand) DeleteLock(id string) (*models.ProjectLock, error) {
	lock, err := l.Locker.Unlock(id)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return nil, nil
	}

	l.deleteWorkingDir(*lock)
	return lock, nil
}

// DeleteLocksByPull handles deleting all locks for the pull request
func (l *DefaultDeleteLockCommand) DeleteLocksByPull(repoFullName string, pullNum int) error {
	locks, err := l.Locker.UnlockByPull(repoFullName, pullNum)
	if err != nil {
		return err
	}
	if len(locks) == 0 {
		return nil
	}

	for i := 0; i < len(locks); i++ {
		lock := locks[i]
		l.deleteWorkingDir(lock)
	}

	return nil
}

func (l *DefaultDeleteLockCommand) deleteWorkingDir(lock models.ProjectLock) {
	// NOTE: Because BaseRepo was added to the PullRequest model later, previous
	// installations of Atlantis will have locks in their DB that do not have
	// this field on PullRequest. We skip deleting the working dir in this case.
	if lock.Pull.BaseRepo == (models.Repo{}) {
		return
	}
	unlock, err := l.WorkingDirLocker.TryLock(lock.Pull.BaseRepo.FullName, lock.Pull.Num, lock.Workspace)
	if err != nil {
		l.Logger.Err("unable to obtain working dir lock when trying to delete old plans: %s", err)
	} else {
		defer unlock()
		// nolint: vetshadow
		if err := l.WorkingDir.DeleteForWorkspace(lock.Pull.BaseRepo, lock.Pull, lock.Workspace); err != nil {
			l.Logger.Err("unable to delete workspace: %s", err)
		}
	}
	if err := l.DB.UpdateProjectStatus(lock.Pull, lock.Workspace, lock.Project.Path, models.DiscardedPlanStatus); err != nil {
		l.Logger.Err("unable to delete project status: %s", err)
	}
}
