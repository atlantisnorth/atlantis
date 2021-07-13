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

package events

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/core/runtime"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/runtime/common"
	"github.com/runatlantis/atlantis/server/events/webhooks"
	"github.com/runatlantis/atlantis/server/events/yaml/raw"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
	"github.com/runatlantis/atlantis/server/logging"
)

// DirNotExistErr is an error caused by the directory not existing.
type DirNotExistErr struct {
	RepoRelDir string
}

// Error implements the error interface.
func (d DirNotExistErr) Error() string {
	return fmt.Sprintf("dir %q does not exist", d.RepoRelDir)
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_lock_url_generator.go LockURLGenerator

// LockURLGenerator generates urls to locks.
type LockURLGenerator interface {
	// GenerateLockURL returns the full URL to the lock at lockID.
	GenerateLockURL(lockID string) string
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_step_runner.go StepRunner

// StepRunner runs steps. Steps are individual pieces of execution like
// `terraform plan`.
type StepRunner interface {
	// Run runs the step.
	Run(ctx models.ProjectCommandContext, extraArgs []string, path string, envs map[string]string) (string, error)
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_custom_step_runner.go CustomStepRunner

// CustomStepRunner runs custom run steps.
type CustomStepRunner interface {
	// Run cmd in path.
	Run(ctx models.ProjectCommandContext, cmd string, path string, envs map[string]string) (string, error)
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_env_step_runner.go EnvStepRunner

// EnvStepRunner runs env steps.
type EnvStepRunner interface {
	Run(ctx models.ProjectCommandContext, cmd string, value string, path string, envs map[string]string) (string, error)
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_webhooks_sender.go WebhooksSender

// WebhooksSender sends webhook.
type WebhooksSender interface {
	// Send sends the webhook.
	Send(log logging.SimpleLogging, res webhooks.ApplyResult) error
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_project_command_runner.go ProjectCommandRunner

type ProjectPlanCommandRunner interface {
	// Plan runs terraform plan for the project described by ctx.
	Plan(ctx models.ProjectCommandContext) models.ProjectResult
}

type ProjectApplyCommandRunner interface {
	// Apply runs terraform apply for the project described by ctx.
	Apply(ctx models.ProjectCommandContext) models.ProjectResult
}

type ProjectPolicyCheckCommandRunner interface {
	// PolicyCheck runs OPA defined policies for the project desribed by ctx.
	PolicyCheck(ctx models.ProjectCommandContext) models.ProjectResult
}

type ProjectApprovePoliciesCommandRunner interface {
	// Approves any failing OPA policies.
	ApprovePolicies(ctx models.ProjectCommandContext) models.ProjectResult
}

type ProjectVersionCommandRunner interface {
	// Version runs terraform version for the project described by ctx.
	Version(ctx models.ProjectCommandContext) models.ProjectResult
}

// ProjectCommandRunner runs project commands. A project command is a command
// for a specific TF project.
type ProjectCommandRunner interface {
	ProjectPlanCommandRunner
	ProjectApplyCommandRunner
	ProjectPolicyCheckCommandRunner
	ProjectApprovePoliciesCommandRunner
	ProjectVersionCommandRunner
}

// DefaultProjectCommandRunner implements ProjectCommandRunner.
type DefaultProjectCommandRunner struct {
	Locker                ProjectLocker
	LockURLGenerator      LockURLGenerator
	InitStepRunner        StepRunner
	PlanStepRunner        StepRunner
	ShowStepRunner        StepRunner
	ApplyStepRunner       StepRunner
	PolicyCheckStepRunner StepRunner
	VersionStepRunner     StepRunner
	RunStepRunner         CustomStepRunner
	EnvStepRunner         EnvStepRunner
	PullApprovedChecker   runtime.PullApprovedChecker
	WorkingDir            WorkingDir
	Webhooks              WebhooksSender
	WorkingDirLocker      WorkingDirLocker
}

// Plan runs terraform plan for the project described by ctx.
func (p *DefaultProjectCommandRunner) Plan(ctx models.ProjectCommandContext) models.ProjectResult {
	planSuccess, failure, err := p.doPlan(ctx)
	return models.ProjectResult{
		Command:     models.PlanCommand,
		PlanSuccess: planSuccess,
		Error:       err,
		Failure:     failure,
		RepoRelDir:  ctx.RepoRelDir,
		Workspace:   ctx.Workspace,
		ProjectName: ctx.ProjectName,
	}
}

// PolicyCheck evaluates policies defined with Rego for the project described by ctx.
func (p *DefaultProjectCommandRunner) PolicyCheck(ctx models.ProjectCommandContext) models.ProjectResult {
	policySuccess, failure, err := p.doPolicyCheck(ctx)
	return models.ProjectResult{
		Command:            models.PolicyCheckCommand,
		PolicyCheckSuccess: policySuccess,
		Error:              err,
		Failure:            failure,
		RepoRelDir:         ctx.RepoRelDir,
		Workspace:          ctx.Workspace,
		ProjectName:        ctx.ProjectName,
	}
}

// Apply runs terraform apply for the project described by ctx.
func (p *DefaultProjectCommandRunner) Apply(ctx models.ProjectCommandContext) models.ProjectResult {
	applyOut, failure, err := p.doApply(ctx)
	return models.ProjectResult{
		Command:      models.ApplyCommand,
		Failure:      failure,
		Error:        err,
		ApplySuccess: applyOut,
		RepoRelDir:   ctx.RepoRelDir,
		Workspace:    ctx.Workspace,
		ProjectName:  ctx.ProjectName,
	}
}

func (p *DefaultProjectCommandRunner) ApprovePolicies(ctx models.ProjectCommandContext) models.ProjectResult {
	approvedOut, failure, err := p.doApprovePolicies(ctx)
	return models.ProjectResult{
		Command:            models.PolicyCheckCommand,
		Failure:            failure,
		Error:              err,
		PolicyCheckSuccess: approvedOut,
		RepoRelDir:         ctx.RepoRelDir,
		Workspace:          ctx.Workspace,
		ProjectName:        ctx.ProjectName,
	}
}

func (p *DefaultProjectCommandRunner) Version(ctx models.ProjectCommandContext) models.ProjectResult {
	versionOut, failure, err := p.doVersion(ctx)
	return models.ProjectResult{
		Command:        models.VersionCommand,
		Failure:        failure,
		Error:          err,
		VersionSuccess: versionOut,
		RepoRelDir:     ctx.RepoRelDir,
		Workspace:      ctx.Workspace,
		ProjectName:    ctx.ProjectName,
	}
}

func (p *DefaultProjectCommandRunner) doApprovePolicies(ctx models.ProjectCommandContext) (*models.PolicyCheckSuccess, string, error) {

	// TODO: Make this a bit smarter
	// without checking some sort of state that the policy check has indeed passed this is likely to cause issues

	return &models.PolicyCheckSuccess{
		PolicyCheckOutput: "Policies approved",
	}, "", nil
}

func (p *DefaultProjectCommandRunner) doPolicyCheck(ctx models.ProjectCommandContext) (*models.PolicyCheckSuccess, string, error) {
	// Acquire Atlantis lock for this repo/dir/workspace.
	// This should already be acquired from the prior plan operation.
	// if for some reason an unlock happens between the plan and policy check step
	// we will attempt to capture the lock here but fail to get the working directory
	// at which point we will unlock again to preserve functionality
	// If we fail to capture the lock here (super unlikely) then we error out and the user is forced to replan
	lockAttempt, err := p.Locker.TryLock(ctx.Log, ctx.Pull, ctx.User, ctx.Workspace, models.NewProject(ctx.Pull.BaseRepo.FullName, ctx.RepoRelDir))

	if err != nil {
		return nil, "", errors.Wrap(err, "acquiring lock")
	}
	if !lockAttempt.LockAcquired {
		return nil, lockAttempt.LockFailureReason, nil
	}
	ctx.Log.Debug("acquired lock for project")

	// Acquire internal lock for the directory we're going to operate in.
	// We should refactor this to keep the lock for the duration of plan and policy check since as of now
	// there is a small gap where we don't have the lock and if we can't get this here, we should just unlock the PR.
	unlockFn, err := p.WorkingDirLocker.TryLock(ctx.Pull.BaseRepo.FullName, ctx.Pull.Num, ctx.Workspace)
	if err != nil {
		return nil, "", err
	}
	defer unlockFn()

	// we shouldn't attempt to clone this again. If changes occur to the pull request while the plan is happening
	// that shouldn't affect this particular operation.
	repoDir, err := p.WorkingDir.GetWorkingDir(ctx.Pull.BaseRepo, ctx.Pull, ctx.Workspace)
	if err != nil {

		// let's unlock here since something probably nuked our directory between the plan and policy check phase
		if unlockErr := lockAttempt.UnlockFn(); unlockErr != nil {
			ctx.Log.Err("error unlocking state after plan error: %v", unlockErr)
		}

		if os.IsNotExist(err) {
			return nil, "", errors.New("project has not been cloned–did you run plan?")
		}
		return nil, "", err
	}
	absPath := filepath.Join(repoDir, ctx.RepoRelDir)
	if _, err = os.Stat(absPath); os.IsNotExist(err) {

		// let's unlock here since something probably nuked our directory between the plan and policy check phase
		if unlockErr := lockAttempt.UnlockFn(); unlockErr != nil {
			ctx.Log.Err("error unlocking state after plan error: %v", unlockErr)
		}

		return nil, "", DirNotExistErr{RepoRelDir: ctx.RepoRelDir}
	}

	outputs, err := p.runSteps(ctx.Steps, ctx, absPath)
	if err != nil {
		// Note: we are explicitly not unlocking the pr here since a failing policy check will require
		// approval
		return nil, "", fmt.Errorf("%s\n%s", err, strings.Join(outputs, "\n"))
	}

	return &models.PolicyCheckSuccess{
		LockURL:           p.LockURLGenerator.GenerateLockURL(lockAttempt.LockKey),
		PolicyCheckOutput: strings.Join(outputs, "\n"),
		RePlanCmd:         ctx.RePlanCmd,
		ApplyCmd:          ctx.ApplyCmd,

		// set this to false right now because we don't have this information
		// TODO: refactor the templates in a sane way so we don't need this
		HasDiverged: false,
	}, "", nil
}

func (p *DefaultProjectCommandRunner) doPlan(ctx models.ProjectCommandContext) (*models.PlanSuccess, string, error) {
	// Acquire Atlantis lock for this repo/dir/workspace.
	lockAttempt, err := p.Locker.TryLock(ctx.Log, ctx.Pull, ctx.User, ctx.Workspace, models.NewProject(ctx.Pull.BaseRepo.FullName, ctx.RepoRelDir))
	if err != nil {
		return nil, "", errors.Wrap(err, "acquiring lock")
	}
	if !lockAttempt.LockAcquired {
		return nil, lockAttempt.LockFailureReason, nil
	}
	ctx.Log.Debug("acquired lock for project")

	// Acquire internal lock for the directory we're going to operate in.
	unlockFn, err := p.WorkingDirLocker.TryLock(ctx.Pull.BaseRepo.FullName, ctx.Pull.Num, ctx.Workspace)
	if err != nil {
		return nil, "", err
	}
	defer unlockFn()

	// Clone is idempotent so okay to run even if the repo was already cloned.
	repoDir, hasDiverged, cloneErr := p.WorkingDir.Clone(ctx.Log, ctx.HeadRepo, ctx.Pull, ctx.Workspace)
	if cloneErr != nil {
		if unlockErr := lockAttempt.UnlockFn(); unlockErr != nil {
			ctx.Log.Err("error unlocking state after plan error: %v", unlockErr)
		}
		return nil, "", cloneErr
	}
	projAbsPath := filepath.Join(repoDir, ctx.RepoRelDir)
	if _, err = os.Stat(projAbsPath); os.IsNotExist(err) {
		return nil, "", DirNotExistErr{RepoRelDir: ctx.RepoRelDir}
	}

	lockFileName := ".terraform.lock.hcl"
	lockfilePath := filepath.Join(projAbsPath, lockFileName)
	lockFileExistsProirToPlan := common.FileExists(lockfilePath)

	outputs, err := p.runSteps(ctx.Steps, ctx, projAbsPath)
	if err != nil {
		if unlockErr := lockAttempt.UnlockFn(); unlockErr != nil {
			ctx.Log.Err("error unlocking state after plan error: %v", unlockErr)
		}
		if !lockFileExistsProirToPlan && common.FileExists(lockfilePath) {
			// if the lock file did not exist before running the plan delete the lock file that was generated
			// by terraform init so that on subsequent actions modules and providers are not locked to the versions
			// detected as a part of this plan
			ctx.Log.Debug("Deleting `%s` that was generated by terraform init", lockFileName)
			delErr := os.Remove(lockfilePath)
			if delErr != nil {
				ctx.Log.Info("Error Deleting `%s`", lockFileName)
			}
		}
		return nil, "", fmt.Errorf("%s\n%s", err, strings.Join(outputs, "\n"))
	}

	return &models.PlanSuccess{
		LockURL:         p.LockURLGenerator.GenerateLockURL(lockAttempt.LockKey),
		TerraformOutput: strings.Join(outputs, "\n"),
		RePlanCmd:       ctx.RePlanCmd,
		ApplyCmd:        ctx.ApplyCmd,
		HasDiverged:     hasDiverged,
	}, "", nil
}

func (p *DefaultProjectCommandRunner) doApply(ctx models.ProjectCommandContext) (applyOut string, failure string, err error) {
	repoDir, err := p.WorkingDir.GetWorkingDir(ctx.Pull.BaseRepo, ctx.Pull, ctx.Workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", errors.New("project has not been cloned–did you run plan?")
		}
		return "", "", err
	}
	absPath := filepath.Join(repoDir, ctx.RepoRelDir)
	if _, err = os.Stat(absPath); os.IsNotExist(err) {
		return "", "", DirNotExistErr{RepoRelDir: ctx.RepoRelDir}
	}

	for _, req := range ctx.ApplyRequirements {
		switch req {
		case raw.ApprovedApplyRequirement:
			approved, err := p.PullApprovedChecker.PullIsApproved(ctx.Pull.BaseRepo, ctx.Pull) // nolint: vetshadow
			if err != nil {
				return "", "", errors.Wrap(err, "checking if pull request was approved")
			}
			if !approved {
				return "", "Pull request must be approved by at least one person other than the author before running apply.", nil
			}
		// this should come before mergeability check since mergeability is a superset of this check.
		case valid.PoliciesPassedApplyReq:
			if ctx.ProjectPlanStatus == models.ErroredPolicyCheckStatus {
				return "", "All policies must pass for project before running apply", nil
			}
		case raw.MergeableApplyRequirement:
			if !ctx.PullMergeable {
				return "", "Pull request must be mergeable before running apply.", nil
			}
		case raw.UnDivergedApplyRequirement:
			if p.WorkingDir.HasDiverged(ctx.Log, repoDir) {
				return "", "Default branch must be rebased onto pull request before running apply.", nil
			}
		}
	}
	// Acquire internal lock for the directory we're going to operate in.
	unlockFn, err := p.WorkingDirLocker.TryLock(ctx.Pull.BaseRepo.FullName, ctx.Pull.Num, ctx.Workspace)
	if err != nil {
		return "", "", err
	}
	defer unlockFn()

	outputs, err := p.runSteps(ctx.Steps, ctx, absPath)
	p.Webhooks.Send(ctx.Log, webhooks.ApplyResult{ // nolint: errcheck
		Workspace: ctx.Workspace,
		User:      ctx.User,
		Repo:      ctx.Pull.BaseRepo,
		Pull:      ctx.Pull,
		Success:   err == nil,
		Directory: ctx.RepoRelDir,
	})
	if err != nil {
		return "", "", fmt.Errorf("%s\n%s", err, strings.Join(outputs, "\n"))
	}
	return strings.Join(outputs, "\n"), "", nil
}

func (p *DefaultProjectCommandRunner) doVersion(ctx models.ProjectCommandContext) (versionOut string, failure string, err error) {
	repoDir, err := p.WorkingDir.GetWorkingDir(ctx.Pull.BaseRepo, ctx.Pull, ctx.Workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", errors.New("project has not been cloned–did you run plan?")
		}
		return "", "", err
	}
	absPath := filepath.Join(repoDir, ctx.RepoRelDir)
	if _, err = os.Stat(absPath); os.IsNotExist(err) {
		return "", "", DirNotExistErr{RepoRelDir: ctx.RepoRelDir}
	}

	// Acquire internal lock for the directory we're going to operate in.
	unlockFn, err := p.WorkingDirLocker.TryLock(ctx.Pull.BaseRepo.FullName, ctx.Pull.Num, ctx.Workspace)
	if err != nil {
		return "", "", err
	}
	defer unlockFn()

	outputs, err := p.runSteps(ctx.Steps, ctx, absPath)
	if err != nil {
		return "", "", fmt.Errorf("%s\n%s", err, strings.Join(outputs, "\n"))
	}

	return strings.Join(outputs, "\n"), "", nil
}

func (p *DefaultProjectCommandRunner) runSteps(steps []valid.Step, ctx models.ProjectCommandContext, absPath string) ([]string, error) {
	var outputs []string
	envs := make(map[string]string)
	for _, step := range steps {
		var out string
		var err error
		switch step.StepName {
		case "init":
			out, err = p.InitStepRunner.Run(ctx, step.ExtraArgs, absPath, envs)
		case "plan":
			out, err = p.PlanStepRunner.Run(ctx, step.ExtraArgs, absPath, envs)
		case "show":
			_, err = p.ShowStepRunner.Run(ctx, step.ExtraArgs, absPath, envs)
		case "policy_check":
			out, err = p.PolicyCheckStepRunner.Run(ctx, step.ExtraArgs, absPath, envs)
		case "apply":
			out, err = p.ApplyStepRunner.Run(ctx, step.ExtraArgs, absPath, envs)
		case "version":
			out, err = p.VersionStepRunner.Run(ctx, step.ExtraArgs, absPath, envs)
		case "run":
			out, err = p.RunStepRunner.Run(ctx, step.RunCommand, absPath, envs)
		case "env":
			out, err = p.EnvStepRunner.Run(ctx, step.RunCommand, step.EnvVarValue, absPath, envs)
			envs[step.EnvVarName] = out
			// We reset out to the empty string because we don't want it to
			// be printed to the PR, it's solely to set the environment variable.
			out = ""
		}

		if out != "" {
			outputs = append(outputs, out)
		}
		if err != nil {
			return outputs, err
		}
	}
	return outputs, nil
}
