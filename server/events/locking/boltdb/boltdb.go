// Copyright 2017 HootSuite Media Inc.
//
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
//
// Package boltdb provides a locking implementation using Bolt.
// Bolt is a key/value store that writes all data to a file.
// See https://github.com/boltdb/bolt for more information.
package boltdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
)

// BoltLocker is a locking backend using BoltDB
type BoltLocker struct {
	db              *bolt.DB
	locksBucketName []byte
	pullsBucketName []byte
}

const (
	locksBucketName  = "runLocks"
	pullsBucketName  = "pulls"
	pullKeySeparator = "::"
)

// New returns a valid locker. We need to be able to write to dataDir
// since bolt stores its data as a file
func New(dataDir string) (*BoltLocker, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, errors.Wrap(err, "creating data dir")
	}
	db, err := bolt.Open(path.Join(dataDir, "atlantis.db"), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		if err.Error() == "timeout" {
			return nil, errors.New("starting BoltDB: timeout (a possible cause is another Atlantis instance already running)")
		}
		return nil, errors.Wrap(err, "starting BoltDB")
	}

	// Create the buckets.
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err = tx.CreateBucketIfNotExists([]byte(locksBucketName)); err != nil {
			return errors.Wrapf(err, "creating bucket %q", locksBucketName)
		}
		if _, err = tx.CreateBucketIfNotExists([]byte(pullsBucketName)); err != nil {
			return errors.Wrapf(err, "creating bucket %q", pullsBucketName)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "starting BoltDB")
	}
	// todo: close BoltDB when server is sigtermed
	return &BoltLocker{db: db, locksBucketName: []byte(locksBucketName), pullsBucketName: []byte(pullsBucketName)}, nil
}

// NewWithDB is used for testing.
func NewWithDB(db *bolt.DB, bucket string) (*BoltLocker, error) {
	return &BoltLocker{db: db, locksBucketName: []byte(bucket), pullsBucketName: []byte(pullsBucketName)}, nil
}

// TryLock attempts to create a new lock. If the lock is
// acquired, it will return true and the lock returned will be newLock.
// If the lock is not acquired, it will return false and the current
// lock that is preventing this lock from being acquired.
func (b *BoltLocker) TryLock(newLock models.ProjectLock) (bool, models.ProjectLock, error) {
	var lockAcquired bool
	var currLock models.ProjectLock
	key := b.lockKey(newLock.Project, newLock.Workspace)
	newLockSerialized, _ := json.Marshal(newLock)
	transactionErr := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.locksBucketName)

		// if there is no run at that key then we're free to create the lock
		currLockSerialized := bucket.Get([]byte(key))
		if currLockSerialized == nil {
			// This will only error on readonly buckets, it's okay to ignore.
			bucket.Put([]byte(key), newLockSerialized) // nolint: errcheck
			lockAcquired = true
			currLock = newLock
			return nil
		}

		// otherwise the lock fails, return to caller the run that's holding the lock
		if err := json.Unmarshal(currLockSerialized, &currLock); err != nil {
			return errors.Wrap(err, "failed to deserialize current lock")
		}
		lockAcquired = false
		return nil
	})

	if transactionErr != nil {
		return false, currLock, errors.Wrap(transactionErr, "DB transaction failed")
	}

	return lockAcquired, currLock, nil
}

// Unlock attempts to unlock the project and workspace.
// If there is no lock, then it will return a nil pointer.
// If there is a lock, then it will delete it, and then return a pointer
// to the deleted lock.
func (b *BoltLocker) Unlock(p models.Project, workspace string) (*models.ProjectLock, error) {
	var lock models.ProjectLock
	foundLock := false
	key := b.lockKey(p, workspace)
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.locksBucketName)
		serialized := bucket.Get([]byte(key))
		if serialized != nil {
			if err := json.Unmarshal(serialized, &lock); err != nil {
				return errors.Wrap(err, "failed to deserialize lock")
			}
			foundLock = true
		}
		return bucket.Delete([]byte(key))
	})
	err = errors.Wrap(err, "DB transaction failed")
	if foundLock {
		return &lock, err
	}
	return nil, err
}

// List lists all current locks.
func (b *BoltLocker) List() ([]models.ProjectLock, error) {
	var locks []models.ProjectLock
	var locksBytes [][]byte
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.locksBucketName)
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			locksBytes = append(locksBytes, v)
		}
		return nil
	})
	if err != nil {
		return locks, errors.Wrap(err, "DB transaction failed")
	}

	// deserialize bytes into the proper objects
	for k, v := range locksBytes {
		var lock models.ProjectLock
		if err := json.Unmarshal(v, &lock); err != nil {
			return locks, errors.Wrap(err, fmt.Sprintf("failed to deserialize lock at key %q", string(k)))
		}
		locks = append(locks, lock)
	}

	return locks, nil
}

// UnlockByPull deletes all locks associated with that pull request and returns them.
func (b *BoltLocker) UnlockByPull(repoFullName string, pullNum int) ([]models.ProjectLock, error) {
	var locks []models.ProjectLock
	err := b.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(b.locksBucketName).Cursor()

		// we can use the repoFullName as a prefix search since that's the first part of the key
		for k, v := c.Seek([]byte(repoFullName)); k != nil && bytes.HasPrefix(k, []byte(repoFullName)); k, v = c.Next() {
			var lock models.ProjectLock
			if err := json.Unmarshal(v, &lock); err != nil {
				return errors.Wrapf(err, "deserializing lock at key %q", string(k))
			}
			if lock.Pull.Num == pullNum {
				locks = append(locks, lock)
			}
		}
		return nil
	})
	if err != nil {
		return locks, err
	}

	// delete the locks
	for _, lock := range locks {
		if _, err = b.Unlock(lock.Project, lock.Workspace); err != nil {
			return locks, errors.Wrapf(err, "unlocking repo %s, path %s, workspace %s", lock.Project.RepoFullName, lock.Project.Path, lock.Workspace)
		}
	}
	return locks, nil
}

// GetLock returns a pointer to the lock for that project and workspace.
// If there is no lock, it returns a nil pointer.
func (b *BoltLocker) GetLock(p models.Project, workspace string) (*models.ProjectLock, error) {
	key := b.lockKey(p, workspace)
	var lockBytes []byte
	err := b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(b.locksBucketName)
		lockBytes = b.Get([]byte(key))
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting lock data")
	}
	// lockBytes will be nil if there was no data at that key
	if lockBytes == nil {
		return nil, nil
	}

	var lock models.ProjectLock
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		return nil, errors.Wrapf(err, "deserializing lock at key %q", key)
	}

	// need to set it to Local after deserialization due to https://github.com/golang/go/issues/19486
	lock.Time = lock.Time.Local()
	return &lock, nil
}

func (b *BoltLocker) UpdatePullWithResults(pull models.PullRequest, newResults []models.ProjectResult) (*PullStatus, error) {
	key, err := b.pullKey(pull)
	if err != nil {
		return nil, err
	}

	var newStatus *PullStatus
	err = b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.pullsBucketName)
		currStatus, err := b.getPullFromBucket(bucket, key)
		if err != nil {
			return err
		}

		// If there is no pull OR if the pull we have is out of date, we
		// just write a new pull.
		if currStatus == nil || currStatus.Pull.HeadCommit != pull.HeadCommit {
			var statuses []ProjectStatus
			for _, r := range newResults {
				statuses = append(statuses, b.projectResultToProject(r))
			}
			newStatus = &PullStatus{
				Pull:     pull,
				Projects: statuses,
			}
		} else {
			// If there's an existing pull at the right commit then we have to
			// merge our project results with the existing ones. We do a merge
			// because it's possible a user is just applying a single project
			// in this command and so we don't want to delete our data about
			// other projects that aren't affected by this command.
			newStatus = currStatus
			for _, res := range newResults {
				// First, check if we should update any existing projects.
				updatedExisting := false
				for i := range newStatus.Projects {
					// NOTE: We're using a reference here because we are
					// in-place updating its Status field.
					proj := &newStatus.Projects[i]
					if res.Workspace == proj.Workspace &&
						res.RepoRelDir == proj.RepoRelDir &&
						res.ProjectName == proj.ProjectName {

						proj.Status = b.getPlanStatus(res)
						updatedExisting = true
						break
					}
				}

				if !updatedExisting {
					// If we didn't update an existing project, then we need to
					// add this because it's a new one.
					newStatus.Projects = append(newStatus.Projects, b.projectResultToProject(res))
				}
			}
		}

		// Now, we overwrite the key with our new status.
		return b.writePullToBucket(bucket, key, newStatus)
	})
	return newStatus, errors.Wrap(err, "DB transaction failed")
}

func (b *BoltLocker) GetPullStatus(pull models.PullRequest) (*PullStatus, error) {
	key, err := b.pullKey(pull)
	if err != nil {
		return nil, err
	}

	var pullStatus *PullStatus
	err = b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.pullsBucketName)
		var txErr error
		pullStatus, txErr = b.getPullFromBucket(bucket, key)
		return txErr
	})
	return pullStatus, errors.Wrap(err, "DB transaction failed")
}

func (b *BoltLocker) DeletePullStatus(pull models.PullRequest) error {
	key, err := b.pullKey(pull)
	if err != nil {
		return err
	}
	err = b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.pullsBucketName)
		return bucket.Delete(key)
	})
	return errors.Wrap(err, "DB transaction failed")
}

func (b *BoltLocker) DeleteProjectStatus(pull models.PullRequest, workspace string, repoRelDir string) error {
	key, err := b.pullKey(pull)
	if err != nil {
		return err
	}
	err = b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.pullsBucketName)
		currStatus, err := b.getPullFromBucket(bucket, key)
		if err != nil {
			return err
		}
		if currStatus == nil {
			return nil
		}

		// Create a new projectStatuses array without the ones we want to
		// delete.
		var newProjects []ProjectStatus
		for _, p := range currStatus.Projects {
			if p.Workspace == workspace && p.RepoRelDir == repoRelDir {
				continue
			}
			newProjects = append(newProjects, p)
		}

		// Overwrite the old pull status.
		currStatus.Projects = newProjects
		return b.writePullToBucket(bucket, key, currStatus)
	})
	return errors.Wrap(err, "DB transaction failed")
}

func (b *BoltLocker) pullKey(pull models.PullRequest) ([]byte, error) {
	hostname := pull.BaseRepo.VCSHost.Hostname
	if strings.Contains(hostname, pullKeySeparator) {
		return nil, fmt.Errorf("vcs hostname %q contains illegal string %q", hostname, pullKeySeparator)
	}
	repo := pull.BaseRepo.FullName
	if strings.Contains(repo, pullKeySeparator) {
		return nil, fmt.Errorf("repo name %q contains illegal string %q", hostname, pullKeySeparator)
	}

	return []byte(fmt.Sprintf("%s::%s::%d", hostname, repo, pull.Num)),
		nil
}

func (b *BoltLocker) lockKey(p models.Project, workspace string) string {
	return fmt.Sprintf("%s/%s/%s", p.RepoFullName, p.Path, workspace)
}

func (b *BoltLocker) getPullFromBucket(bucket *bolt.Bucket, key []byte) (*PullStatus, error) {
	serialized := bucket.Get(key)
	if serialized == nil {
		return nil, nil
	}

	var p PullStatus
	if err := json.Unmarshal(serialized, &p); err != nil {
		return nil, errors.Wrapf(err, "deserializing pull at %q with contents %q", key, serialized)
	}
	return &p, nil
}

func (b *BoltLocker) writePullToBucket(bucket *bolt.Bucket, key []byte, pull *PullStatus) error {
	serialized, err := json.Marshal(pull)
	if err != nil {
		return errors.Wrap(err, "serializing")
	}
	return bucket.Put(key, serialized)
}

func (b *BoltLocker) getPlanStatus(p models.ProjectResult) ProjectPlanStatus {
	if p.Error != nil {
		return ErroredPlanStatus
	}
	if p.Failure != "" {
		return ErroredPlanStatus
	}
	if p.PlanSuccess != nil {
		return PlannedPlanStatus
	}
	if p.ApplySuccess != "" {
		return AppliedPlanStatus
	}
	return ErroredPlanStatus
}

func (b *BoltLocker) projectResultToProject(p models.ProjectResult) ProjectStatus {
	return ProjectStatus{
		Workspace:   p.Workspace,
		RepoRelDir:  p.RepoRelDir,
		ProjectName: p.ProjectName,
		Status:      b.getPlanStatus(p),
	}
}

type PullStatus struct {
	Projects []ProjectStatus
	Pull     models.PullRequest
}

type ProjectStatus struct {
	Workspace   string
	RepoRelDir  string
	ProjectName string
	Status      ProjectPlanStatus
}

type ProjectPlanStatus int

const (
	ErroredPlanStatus ProjectPlanStatus = iota
	PlannedPlanStatus
	AppliedPlanStatus
)

func (p ProjectPlanStatus) String() string {
	switch p {
	case ErroredPlanStatus:
		return "errored"
	case PlannedPlanStatus:
		return "planned"
	case AppliedPlanStatus:
		return "applied"
	default:
		return "errored"
	}
}
