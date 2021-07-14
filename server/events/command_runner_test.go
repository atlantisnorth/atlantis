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

package events_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/runatlantis/atlantis/server/core/db"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
	"github.com/runatlantis/atlantis/server/logging"

	"github.com/google/go-github/v31/github"
	. "github.com/petergtz/pegomock"
	lockingmocks "github.com/runatlantis/atlantis/server/core/locking/mocks"
	"github.com/runatlantis/atlantis/server/events"
	"github.com/runatlantis/atlantis/server/events/mocks"
	eventmocks "github.com/runatlantis/atlantis/server/events/mocks"
	"github.com/runatlantis/atlantis/server/events/mocks/matchers"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/models/fixtures"
	vcsmocks "github.com/runatlantis/atlantis/server/events/vcs/mocks"
	. "github.com/runatlantis/atlantis/testing"
)

var projectCommandBuilder *mocks.MockProjectCommandBuilder
var projectCommandRunner *mocks.MockProjectCommandRunner
var eventParsing *mocks.MockEventParsing
var azuredevopsGetter *mocks.MockAzureDevopsPullGetter
var githubGetter *mocks.MockGithubPullGetter
var gitlabGetter *mocks.MockGitlabMergeRequestGetter
var ch events.DefaultCommandRunner
var workingDir events.WorkingDir
var pendingPlanFinder *mocks.MockPendingPlanFinder
var drainer *events.Drainer
var deleteLockCommand *mocks.MockDeleteLockCommand
var commitUpdater *mocks.MockCommitStatusUpdater

// TODO: refactor these into their own unit tests.
// these were all split out from default command runner in an effort to improve
// readability however the tests were kept as is.
var dbUpdater *events.DBUpdater
var pullUpdater *events.PullUpdater
var autoMerger *events.AutoMerger
var policyCheckCommandRunner *events.PolicyCheckCommandRunner
var approvePoliciesCommandRunner *events.ApprovePoliciesCommandRunner
var planCommandRunner *events.PlanCommandRunner
var applyLockChecker *lockingmocks.MockApplyLockChecker
var applyCommandRunner *events.ApplyCommandRunner
var unlockCommandRunner *events.UnlockCommandRunner
var preWorkflowHooksCommandRunner events.PreWorkflowHooksCommandRunner

func setup(t *testing.T) *vcsmocks.MockClient {
	RegisterMockTestingT(t)
	projectCommandBuilder = mocks.NewMockProjectCommandBuilder()
	eventParsing = mocks.NewMockEventParsing()
	vcsClient := vcsmocks.NewMockClient()
	githubGetter = mocks.NewMockGithubPullGetter()
	gitlabGetter = mocks.NewMockGitlabMergeRequestGetter()
	azuredevopsGetter = mocks.NewMockAzureDevopsPullGetter()
	logger = logging.NewNoopLogger(t)
	projectCommandRunner = mocks.NewMockProjectCommandRunner()
	workingDir = mocks.NewMockWorkingDir()
	pendingPlanFinder = mocks.NewMockPendingPlanFinder()
	commitUpdater = mocks.NewMockCommitStatusUpdater()

	tmp, cleanup := TempDir(t)
	defer cleanup()
	defaultBoltDB, err := db.New(tmp)
	Ok(t, err)

	drainer = &events.Drainer{}
	deleteLockCommand = eventmocks.NewMockDeleteLockCommand()
	applyLockChecker = lockingmocks.NewMockApplyLockChecker()

	dbUpdater = &events.DBUpdater{
		DB: defaultBoltDB,
	}

	pullUpdater = &events.PullUpdater{
		HidePrevPlanComments: false,
		VCSClient:            vcsClient,
		MarkdownRenderer:     &events.MarkdownRenderer{},
	}

	autoMerger = &events.AutoMerger{
		VCSClient:       vcsClient,
		GlobalAutomerge: false,
	}

	parallelPoolSize := 1
	SilenceNoProjects := false
	policyCheckCommandRunner = events.NewPolicyCheckCommandRunner(
		dbUpdater,
		pullUpdater,
		commitUpdater,
		projectCommandRunner,
		parallelPoolSize,
		false,
	)

	planCommandRunner = events.NewPlanCommandRunner(
		false,
		false,
		vcsClient,
		pendingPlanFinder,
		workingDir,
		commitUpdater,
		projectCommandBuilder,
		projectCommandRunner,
		dbUpdater,
		pullUpdater,
		policyCheckCommandRunner,
		autoMerger,
		parallelPoolSize,
		SilenceNoProjects,
		defaultBoltDB,
		deleteLockCommand,
	)

	applyCommandRunner = events.NewApplyCommandRunner(
		vcsClient,
		false,
		applyLockChecker,
		commitUpdater,
		projectCommandBuilder,
		projectCommandRunner,
		autoMerger,
		pullUpdater,
		dbUpdater,
		defaultBoltDB,
		parallelPoolSize,
		SilenceNoProjects,
		false,
	)

	approvePoliciesCommandRunner = events.NewApprovePoliciesCommandRunner(
		commitUpdater,
		projectCommandBuilder,
		projectCommandRunner,
		pullUpdater,
		dbUpdater,
		SilenceNoProjects,
		false,
	)

	unlockCommandRunner = events.NewUnlockCommandRunner(
		deleteLockCommand,
		vcsClient,
		SilenceNoProjects,
	)

	versionCommandRunner := events.NewVersionCommandRunner(
		pullUpdater,
		projectCommandBuilder,
		projectCommandRunner,
		parallelPoolSize,
		SilenceNoProjects,
	)

	commentCommandRunnerByCmd := map[models.CommandName]events.CommentCommandRunner{
		models.PlanCommand:            planCommandRunner,
		models.ApplyCommand:           applyCommandRunner,
		models.ApprovePoliciesCommand: approvePoliciesCommandRunner,
		models.UnlockCommand:          unlockCommandRunner,
		models.VersionCommand:         versionCommandRunner,
	}

	preWorkflowHooksCommandRunner = mocks.NewMockPreWorkflowHooksCommandRunner()

	When(preWorkflowHooksCommandRunner.RunPreHooks(matchers.AnyPtrToEventsCommandContext())).ThenReturn(nil)

	ch = events.DefaultCommandRunner{
		VCSClient:                     vcsClient,
		CommentCommandRunnerByCmd:     commentCommandRunnerByCmd,
		EventParser:                   eventParsing,
		GithubPullGetter:              githubGetter,
		GitlabMergeRequestGetter:      gitlabGetter,
		AzureDevopsPullGetter:         azuredevopsGetter,
		Logger:                        logger,
		AllowForkPRs:                  false,
		AllowForkPRsFlag:              "allow-fork-prs-flag",
		Drainer:                       drainer,
		PreWorkflowHooksCommandRunner: preWorkflowHooksCommandRunner,
		PullStatusFetcher:             defaultBoltDB,
	}
	return vcsClient
}

func TestRunCommentCommand_LogPanics(t *testing.T) {
	t.Log("if there is a panic it is commented back on the pull request")
	vcsClient := setup(t)
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenPanic("panic test - if you're seeing this in a test failure this isn't the failing test")
	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, 1, &events.CommentCommand{Name: models.PlanCommand})
	_, _, comment, _ := vcsClient.VerifyWasCalledOnce().CreateComment(matchers.AnyModelsRepo(), AnyInt(), AnyString(), AnyString()).GetCapturedArguments()
	Assert(t, strings.Contains(comment, "Error: goroutine panic"), fmt.Sprintf("comment should be about a goroutine panic but was %q", comment))
}

func TestRunCommentCommand_GithubPullErr(t *testing.T) {
	t.Log("if getting the github pull request fails an error should be logged")
	vcsClient := setup(t)
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(nil, errors.New("err"))
	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, nil)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, fixtures.Pull.Num, "`Error: making pull request API call to GitHub: err`", "")
}

func TestRunCommentCommand_GitlabMergeRequestErr(t *testing.T) {
	t.Log("if getting the gitlab merge request fails an error should be logged")
	vcsClient := setup(t)
	When(gitlabGetter.GetMergeRequest(fixtures.GitlabRepo.FullName, fixtures.Pull.Num)).ThenReturn(nil, errors.New("err"))
	ch.RunCommentCommand(fixtures.GitlabRepo, &fixtures.GitlabRepo, nil, fixtures.User, fixtures.Pull.Num, nil)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GitlabRepo, fixtures.Pull.Num, "`Error: making merge request API call to GitLab: err`", "")
}

func TestRunCommentCommand_GithubPullParseErr(t *testing.T) {
	t.Log("if parsing the returned github pull request fails an error should be logged")
	vcsClient := setup(t)
	var pull github.PullRequest
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(&pull, nil)
	When(eventParsing.ParseGithubPull(&pull)).ThenReturn(fixtures.Pull, fixtures.GithubRepo, fixtures.GitlabRepo, errors.New("err"))

	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, nil)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, fixtures.Pull.Num, "`Error: extracting required fields from comment data: err`", "")
}

func TestRunCommentCommand_ForkPRDisabled(t *testing.T) {
	t.Log("if a command is run on a forked pull request and this is disabled atlantis should" +
		" comment saying that this is not allowed")
	vcsClient := setup(t)
	// by default these are false so don't need to reset
	ch.AllowForkPRs = false
	ch.SilenceForkPRErrors = false
	var pull github.PullRequest
	modelPull := models.PullRequest{
		BaseRepo: fixtures.GithubRepo,
		State:    models.OpenPullState,
	}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(&pull, nil)

	headRepo := fixtures.GithubRepo
	headRepo.FullName = "forkrepo/atlantis"
	headRepo.Owner = "forkrepo"
	When(eventParsing.ParseGithubPull(&pull)).ThenReturn(modelPull, modelPull.BaseRepo, headRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, nil)
	commentMessage := fmt.Sprintf("Atlantis commands can't be run on fork pull requests. To enable, set --%s  or, to disable this message, set --%s", ch.AllowForkPRsFlag, ch.SilenceForkPRErrorsFlag)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, modelPull.Num, commentMessage, "")
}

func TestRunCommentCommand_ForkPRDisabled_SilenceEnabled(t *testing.T) {
	t.Log("if a command is run on a forked pull request and forks are disabled and we are silencing errors do not comment with error")
	vcsClient := setup(t)
	ch.AllowForkPRs = false // by default it's false so don't need to reset
	ch.SilenceForkPRErrors = true
	var pull github.PullRequest
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(&pull, nil)

	headRepo := fixtures.GithubRepo
	headRepo.FullName = "forkrepo/atlantis"
	headRepo.Owner = "forkrepo"
	When(eventParsing.ParseGithubPull(&pull)).ThenReturn(modelPull, modelPull.BaseRepo, headRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, nil)
	vcsClient.VerifyWasCalled(Never()).CreateComment(matchers.AnyModelsRepo(), AnyInt(), AnyString(), AnyString())
}

func TestRunCommentCommandPlan_NoProjects_SilenceEnabled(t *testing.T) {
	t.Log("if a plan command is run on a pull request and SilenceNoProjects is enabled and we are silencing all comments if the modified files don't have a matching project")
	vcsClient := setup(t)
	planCommandRunner.SilenceNoProjects = true
	var pull github.PullRequest
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(&pull, nil)
	When(eventParsing.ParseGithubPull(&pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.PlanCommand})
	vcsClient.VerifyWasCalled(Never()).CreateComment(matchers.AnyModelsRepo(), AnyInt(), AnyString(), AnyString())
	commitUpdater.VerifyWasCalledOnce().UpdateCombinedCount(
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest(),
		matchers.EqModelsCommitStatus(models.SuccessCommitStatus),
		matchers.EqModelsCommandName(models.PlanCommand),
		EqInt(0),
		EqInt(0),
	)
}

func TestRunCommentCommandApply_NoProjects_SilenceEnabled(t *testing.T) {
	t.Log("if an apply command is run on a pull request and SilenceNoProjects is enabled and we are silencing all comments if the modified files don't have a matching project")
	vcsClient := setup(t)
	applyCommandRunner.SilenceNoProjects = true
	var pull github.PullRequest
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(&pull, nil)
	When(eventParsing.ParseGithubPull(&pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.ApplyCommand})
	vcsClient.VerifyWasCalled(Never()).CreateComment(matchers.AnyModelsRepo(), AnyInt(), AnyString(), AnyString())
	commitUpdater.VerifyWasCalledOnce().UpdateCombinedCount(
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest(),
		matchers.EqModelsCommitStatus(models.SuccessCommitStatus),
		matchers.EqModelsCommandName(models.ApplyCommand),
		EqInt(0),
		EqInt(0),
	)
}

func TestRunCommentCommandApprovePolicy_NoProjects_SilenceEnabled(t *testing.T) {
	t.Log("if an approve_policy command is run on a pull request and SilenceNoProjects is enabled and we are silencing all comments if the modified files don't have a matching project")
	vcsClient := setup(t)
	approvePoliciesCommandRunner.SilenceNoProjects = true
	var pull github.PullRequest
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(&pull, nil)
	When(eventParsing.ParseGithubPull(&pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.ApprovePoliciesCommand})
	vcsClient.VerifyWasCalled(Never()).CreateComment(matchers.AnyModelsRepo(), AnyInt(), AnyString(), AnyString())
	commitUpdater.VerifyWasCalledOnce().UpdateCombinedCount(
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest(),
		matchers.EqModelsCommitStatus(models.SuccessCommitStatus),
		matchers.EqModelsCommandName(models.PolicyCheckCommand),
		EqInt(0),
		EqInt(0),
	)
}

func TestRunCommentCommandUnlock_NoProjects_SilenceEnabled(t *testing.T) {
	t.Log("if an unlock command is run on a pull request and SilenceNoProjects is enabled and we are silencing all comments if the modified files don't have a matching project")
	vcsClient := setup(t)
	unlockCommandRunner.SilenceNoProjects = true
	var pull github.PullRequest
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(&pull, nil)
	When(eventParsing.ParseGithubPull(&pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.UnlockCommand})
	vcsClient.VerifyWasCalled(Never()).CreateComment(matchers.AnyModelsRepo(), AnyInt(), AnyString(), AnyString())
}

func TestRunCommentCommand_DisableApplyAllDisabled(t *testing.T) {
	t.Log("if \"atlantis apply\" is run and this is disabled atlantis should" +
		" comment saying that this is not allowed")
	vcsClient := setup(t)
	applyCommandRunner.DisableApplyAll = true
	pull := &github.PullRequest{
		State: github.String("open"),
	}
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState, Num: fixtures.Pull.Num}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, modelPull.Num, &events.CommentCommand{Name: models.ApplyCommand})
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, modelPull.Num, "**Error:** Running `atlantis apply` without flags is disabled. You must specify which project to apply via the `-d <dir>`, `-w <workspace>` or `-p <project name>` flags.", "apply")
}

func TestRunCommentCommand_DisableDisableAutoplan(t *testing.T) {
	t.Log("if \"DisableAutoplan is true\" are disabled and we are silencing return and do not comment with error")
	setup(t)
	ch.DisableAutoplan = true
	defer func() { ch.DisableAutoplan = false }()

	When(projectCommandBuilder.BuildAutoplanCommands(matchers.AnyPtrToEventsCommandContext())).
		ThenReturn([]models.ProjectCommandContext{
			{
				CommandName: models.PlanCommand,
			},
			{
				CommandName: models.PlanCommand,
			},
		}, nil)

	ch.RunAutoplanCommand(fixtures.GithubRepo, fixtures.GithubRepo, fixtures.Pull, fixtures.User)
	projectCommandBuilder.VerifyWasCalled(Never()).BuildAutoplanCommands(matchers.AnyPtrToEventsCommandContext())
}

func TestRunCommentCommand_ClosedPull(t *testing.T) {
	t.Log("if a command is run on a closed pull request atlantis should" +
		" comment saying that this is not allowed")
	vcsClient := setup(t)
	pull := &github.PullRequest{
		State: github.String("closed"),
	}
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.ClosedPullState, Num: fixtures.Pull.Num}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, nil)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, modelPull.Num, "Atlantis commands can't be run on closed pull requests", "")
}

func TestRunUnlockCommand_VCSComment(t *testing.T) {
	t.Log("if unlock PR command is run, atlantis should" +
		" invoke the delete command and comment on PR accordingly")

	vcsClient := setup(t)
	pull := &github.PullRequest{
		State: github.String("open"),
	}
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState, Num: fixtures.Pull.Num}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.UnlockCommand})

	deleteLockCommand.VerifyWasCalledOnce().DeleteLocksByPull(fixtures.GithubRepo.FullName, fixtures.Pull.Num)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, fixtures.Pull.Num, "All Atlantis locks for this PR have been unlocked and plans discarded", "unlock")
}

func TestRunUnlockCommandFail_VCSComment(t *testing.T) {
	t.Log("if unlock PR command is run and delete fails, atlantis should" +
		" invoke comment on PR with error message")

	vcsClient := setup(t)
	pull := &github.PullRequest{
		State: github.String("open"),
	}
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState, Num: fixtures.Pull.Num}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)
	When(deleteLockCommand.DeleteLocksByPull(fixtures.GithubRepo.FullName, fixtures.Pull.Num)).ThenReturn(0, errors.New("err"))

	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.UnlockCommand})

	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, fixtures.Pull.Num, "Failed to delete PR locks", "unlock")
}

func TestRunAutoplanCommand_DeleteLocksByPull(t *testing.T) {
	setup(t)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	When(projectCommandBuilder.BuildAutoplanCommands(matchers.AnyPtrToEventsCommandContext())).
		ThenReturn([]models.ProjectCommandContext{
			{
				CommandName: models.PlanCommand,
			},
			{
				CommandName: models.PlanCommand,
			},
		}, nil)
	When(projectCommandRunner.Plan(matchers.AnyModelsProjectCommandContext())).ThenReturn(models.ProjectResult{PlanSuccess: &models.PlanSuccess{}})
	When(workingDir.GetPullDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn(tmp, nil)
	fixtures.Pull.BaseRepo = fixtures.GithubRepo
	ch.RunAutoplanCommand(fixtures.GithubRepo, fixtures.GithubRepo, fixtures.Pull, fixtures.User)
	deleteLockCommand.VerifyWasCalledOnce().DeleteLocksByPull(fixtures.Pull.BaseRepo.FullName, fixtures.Pull.Num)
}

func TestRunGenericPlanCommand_DeleteLocksByPull(t *testing.T) {
	setup(t)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	When(projectCommandRunner.Plan(matchers.AnyModelsProjectCommandContext())).ThenReturn(models.ProjectResult{PlanSuccess: &models.PlanSuccess{}})
	When(workingDir.GetPullDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn(tmp, nil)
	pull := &github.PullRequest{State: github.String("open")}
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState, Num: fixtures.Pull.Num}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	fixtures.Pull.BaseRepo = fixtures.GithubRepo
	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.PlanCommand})
	deleteLockCommand.VerifyWasCalledOnce().DeleteLocksByPull(fixtures.Pull.BaseRepo.FullName, fixtures.Pull.Num)
}

func TestRunSpecificPlanCommandDoesnt_DeleteLocksByPull(t *testing.T) {
	setup(t)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	When(projectCommandRunner.Plan(matchers.AnyModelsProjectCommandContext())).ThenReturn(models.ProjectResult{PlanSuccess: &models.PlanSuccess{}})
	When(workingDir.GetPullDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn(tmp, nil)
	fixtures.Pull.BaseRepo = fixtures.GithubRepo
	ch.RunCommentCommand(fixtures.GithubRepo, nil, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.PlanCommand, ProjectName: "default"})
	deleteLockCommand.VerifyWasCalled(Never()).DeleteLocksByPull(fixtures.Pull.BaseRepo.FullName, fixtures.Pull.Num)
}

// Test that if one plan fails and we are using automerge, that
// we delete the plans.
func TestRunAutoplanCommandWithError_DeletePlans(t *testing.T) {
	setup(t)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	When(projectCommandBuilder.BuildAutoplanCommands(matchers.AnyPtrToEventsCommandContext())).
		ThenReturn([]models.ProjectCommandContext{
			{
				CommandName: models.PlanCommand,
			},
			{
				CommandName: models.PlanCommand,
			},
		}, nil)
	callCount := 0
	When(projectCommandRunner.Plan(matchers.AnyModelsProjectCommandContext())).Then(func(_ []Param) ReturnValues {
		if callCount == 0 {
			// The first call, we return a successful result.
			callCount++
			return ReturnValues{
				models.ProjectResult{
					PlanSuccess: &models.PlanSuccess{},
				},
			}
		}
		// The second call, we return a failed result.
		return ReturnValues{
			models.ProjectResult{
				Error: errors.New("err"),
			},
		}
	})

	When(workingDir.GetPullDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).
		ThenReturn(tmp, nil)
	fixtures.Pull.BaseRepo = fixtures.GithubRepo
	ch.RunAutoplanCommand(fixtures.GithubRepo, fixtures.GithubRepo, fixtures.Pull, fixtures.User)
	// gets called twice: the first time before the plan starts, the second time after the plan errors
	pendingPlanFinder.VerifyWasCalledOnce().DeletePlans(tmp)
}

func TestFailedApprovalCreatesFailedStatusUpdate(t *testing.T) {
	t.Log("if \"atlantis approve_policies\" is run by non policy owner policy check status fails.")
	setup(t)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	pull := &github.PullRequest{
		State: github.String("open"),
	}

	modelPull := models.PullRequest{
		BaseRepo: fixtures.GithubRepo,
		State:    models.OpenPullState,
		Num:      fixtures.Pull.Num,
	}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	When(projectCommandBuilder.BuildApprovePoliciesCommands(matchers.AnyPtrToEventsCommandContext(), matchers.AnyPtrToEventsCommentCommand())).ThenReturn([]models.ProjectCommandContext{
		{
			CommandName: models.ApprovePoliciesCommand,
		},
		{
			CommandName: models.ApprovePoliciesCommand,
		},
	}, nil)

	When(workingDir.GetPullDir(fixtures.GithubRepo, fixtures.Pull)).ThenReturn(tmp, nil)

	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, &fixtures.Pull, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.ApprovePoliciesCommand})
	commitUpdater.VerifyWasCalledOnce().UpdateCombinedCount(
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest(),
		matchers.EqModelsCommitStatus(models.SuccessCommitStatus),
		matchers.EqModelsCommandName(models.PolicyCheckCommand),
		EqInt(0),
		EqInt(0),
	)
}

func TestApprovedPoliciesUpdateFailedPolicyStatus(t *testing.T) {
	t.Log("if \"atlantis approve_policies\" is run by policy owner all policy checks are approved.")
	setup(t)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	pull := &github.PullRequest{
		State: github.String("open"),
	}

	modelPull := models.PullRequest{
		BaseRepo: fixtures.GithubRepo,
		State:    models.OpenPullState,
		Num:      fixtures.Pull.Num,
	}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	When(projectCommandBuilder.BuildApprovePoliciesCommands(matchers.AnyPtrToEventsCommandContext(), matchers.AnyPtrToEventsCommentCommand())).ThenReturn([]models.ProjectCommandContext{
		{
			CommandName: models.ApprovePoliciesCommand,
			PolicySets: valid.PolicySets{
				Owners: valid.PolicyOwners{
					Users: []string{fixtures.User.Username},
				},
			},
		},
	}, nil)

	When(workingDir.GetPullDir(fixtures.GithubRepo, fixtures.Pull)).ThenReturn(tmp, nil)
	When(projectCommandRunner.ApprovePolicies(matchers.AnyModelsProjectCommandContext())).Then(func(_ []Param) ReturnValues {
		return ReturnValues{
			models.ProjectResult{
				Command:            models.PolicyCheckCommand,
				PolicyCheckSuccess: &models.PolicyCheckSuccess{},
			},
		}
	})

	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, &fixtures.Pull, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.ApprovePoliciesCommand})
	commitUpdater.VerifyWasCalledOnce().UpdateCombinedCount(
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest(),
		matchers.EqModelsCommitStatus(models.SuccessCommitStatus),
		matchers.EqModelsCommandName(models.PolicyCheckCommand),
		EqInt(1),
		EqInt(1),
	)
}

func TestApplyMergeablityWhenPolicyCheckFails(t *testing.T) {
	t.Log("if \"atlantis apply\" is run with failing policy check then apply is not performed")
	setup(t)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	pull := &github.PullRequest{
		State: github.String("open"),
	}

	modelPull := models.PullRequest{
		BaseRepo: fixtures.GithubRepo,
		State:    models.OpenPullState,
		Num:      fixtures.Pull.Num,
	}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)

	_, _ = boltDB.UpdatePullWithResults(modelPull, []models.ProjectResult{
		{
			Command:     models.PolicyCheckCommand,
			Error:       fmt.Errorf("failing policy"),
			ProjectName: "default",
			Workspace:   "default",
			RepoRelDir:  ".",
		},
	})

	When(ch.VCSClient.PullIsMergeable(fixtures.GithubRepo, modelPull)).ThenReturn(true, nil)

	When(projectCommandBuilder.BuildApplyCommands(matchers.AnyPtrToEventsCommandContext(), matchers.AnyPtrToEventsCommentCommand())).Then(func(args []Param) ReturnValues {
		return ReturnValues{
			[]models.ProjectCommandContext{
				{
					CommandName:       models.ApplyCommand,
					ProjectName:       "default",
					Workspace:         "default",
					RepoRelDir:        ".",
					ProjectPlanStatus: models.ErroredPolicyCheckStatus,
				},
			},
			nil,
		}
	})

	When(workingDir.GetPullDir(fixtures.GithubRepo, modelPull)).ThenReturn(tmp, nil)
	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, &modelPull, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.ApplyCommand})
}

func TestApplyWithAutoMerge_VSCMerge(t *testing.T) {
	t.Log("if \"atlantis apply\" is run with automerge then a VCS merge is performed")

	vcsClient := setup(t)
	pull := &github.PullRequest{
		State: github.String("open"),
	}
	modelPull := models.PullRequest{BaseRepo: fixtures.GithubRepo, State: models.OpenPullState}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(pull, nil)
	When(eventParsing.ParseGithubPull(pull)).ThenReturn(modelPull, modelPull.BaseRepo, fixtures.GithubRepo, nil)
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()

	pullOptions := models.PullRequestOptions{
		DeleteSourceBranchOnMerge: false,
	}

	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.ApplyCommand})
	vcsClient.VerifyWasCalledOnce().MergePull(modelPull, pullOptions)
}

func TestRunApply_DiscardedProjects(t *testing.T) {
	t.Log("if \"atlantis apply\" is run with automerge and at least one project" +
		" has a discarded plan, automerge should not take place")
	vcsClient := setup(t)
	autoMerger.GlobalAutomerge = true
	defer func() { autoMerger.GlobalAutomerge = false }()
	tmp, cleanup := TempDir(t)
	defer cleanup()
	boltDB, err := db.New(tmp)
	Ok(t, err)
	dbUpdater.DB = boltDB
	applyCommandRunner.DB = boltDB
	pull := fixtures.Pull
	pull.BaseRepo = fixtures.GithubRepo
	_, err = boltDB.UpdatePullWithResults(pull, []models.ProjectResult{
		{
			Command:    models.PlanCommand,
			RepoRelDir: ".",
			Workspace:  "default",
			PlanSuccess: &models.PlanSuccess{
				TerraformOutput: "tf-output",
				LockURL:         "lock-url",
			},
		},
	})
	Ok(t, err)
	Ok(t, boltDB.UpdateProjectStatus(pull, "default", ".", models.DiscardedPlanStatus))
	ghPull := &github.PullRequest{
		State: github.String("open"),
	}
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenReturn(ghPull, nil)
	When(eventParsing.ParseGithubPull(ghPull)).ThenReturn(pull, pull.BaseRepo, fixtures.GithubRepo, nil)
	When(workingDir.GetPullDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).
		ThenReturn(tmp, nil)
	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, &pull, fixtures.User, fixtures.Pull.Num, &events.CommentCommand{Name: models.ApplyCommand})

	vcsClient.VerifyWasCalled(Never()).MergePull(matchers.AnyModelsPullRequest(), matchers.AnyModelsPullRequestOptions())
}

func TestRunCommentCommand_DrainOngoing(t *testing.T) {
	t.Log("if drain is ongoing then a message should be displayed")
	vcsClient := setup(t)
	drainer.ShutdownBlocking()
	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, nil)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, fixtures.Pull.Num, "Atlantis server is shutting down, please try again later.", "")
}

func TestRunCommentCommand_DrainNotOngoing(t *testing.T) {
	t.Log("if drain is not ongoing then remove ongoing operation must be called even if panic occurred")
	setup(t)
	When(githubGetter.GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)).ThenPanic("panic test - if you're seeing this in a test failure this isn't the failing test")
	ch.RunCommentCommand(fixtures.GithubRepo, &fixtures.GithubRepo, nil, fixtures.User, fixtures.Pull.Num, nil)
	githubGetter.VerifyWasCalledOnce().GetPullRequest(fixtures.GithubRepo, fixtures.Pull.Num)
	Equals(t, 0, drainer.GetStatus().InProgressOps)
}

func TestRunAutoplanCommand_DrainOngoing(t *testing.T) {
	t.Log("if drain is ongoing then a message should be displayed")
	vcsClient := setup(t)
	drainer.ShutdownBlocking()
	ch.RunAutoplanCommand(fixtures.GithubRepo, fixtures.GithubRepo, fixtures.Pull, fixtures.User)
	vcsClient.VerifyWasCalledOnce().CreateComment(fixtures.GithubRepo, fixtures.Pull.Num, "Atlantis server is shutting down, please try again later.", "plan")
}

func TestRunAutoplanCommand_DrainNotOngoing(t *testing.T) {
	t.Log("if drain is not ongoing then remove ongoing operation must be called even if panic occurred")
	setup(t)
	fixtures.Pull.BaseRepo = fixtures.GithubRepo
	When(projectCommandBuilder.BuildAutoplanCommands(matchers.AnyPtrToEventsCommandContext())).ThenPanic("panic test - if you're seeing this in a test failure this isn't the failing test")
	ch.RunAutoplanCommand(fixtures.GithubRepo, fixtures.GithubRepo, fixtures.Pull, fixtures.User)
	projectCommandBuilder.VerifyWasCalledOnce().BuildAutoplanCommands(matchers.AnyPtrToEventsCommandContext())
	Equals(t, 0, drainer.GetStatus().InProgressOps)
}
