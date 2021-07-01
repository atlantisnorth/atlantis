package events_test

import (
	"errors"
	"testing"

	. "github.com/petergtz/pegomock"
	"github.com/runatlantis/atlantis/server/events"
	"github.com/runatlantis/atlantis/server/events/mocks"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/models/fixtures"
	runtime_mocks "github.com/runatlantis/atlantis/server/events/runtime/mocks"
	vcsmocks "github.com/runatlantis/atlantis/server/events/vcs/mocks"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

var postWh events.DefaultPostWorkflowHooksCommandRunner
var postWhWorkingDir *mocks.MockWorkingDir
var postWhWorkingDirLocker *mocks.MockWorkingDirLocker
var whPostWorkflowHookRunner *runtime_mocks.MockPostWorkflowHookRunner

func postWorkflowHooksSetup(t *testing.T) {
	RegisterMockTestingT(t)
	vcsClient := vcsmocks.NewMockClient()
	postWhWorkingDir = mocks.NewMockWorkingDir()
	postWhWorkingDirLocker = mocks.NewMockWorkingDirLocker()
	whPostWorkflowHookRunner = runtime_mocks.NewMockPostWorkflowHookRunner()

	postWh = events.DefaultPostWorkflowHooksCommandRunner{
		VCSClient:              vcsClient,
		WorkingDirLocker:       postWhWorkingDirLocker,
		WorkingDir:             postWhWorkingDir,
		PostWorkflowHookRunner: whPostWorkflowHookRunner,
	}
}

func TestRunPostHooks_Clone(t *testing.T) {

	log := logging.NewNoopLogger(t)

	var newPull = fixtures.Pull
	newPull.BaseRepo = fixtures.GithubRepo

	ctx := &events.CommandContext{
		Pull:     newPull,
		HeadRepo: fixtures.GithubRepo,
		User:     fixtures.User,
		Log:      log,
	}

	testHook := valid.WorkflowHook{
		StepName:   "test",
		RunCommand: "some command",
	}

	pCtx := models.WorkflowHookCommandContext{
		BaseRepo: fixtures.GithubRepo,
		HeadRepo: fixtures.GithubRepo,
		Pull:     newPull,
		Log:      log,
		User:     fixtures.User,
		Verbose:  false,
	}

	repoDir := "path/to/repo"
	result := "some result"

	t.Run("success hooks in cfg", func(t *testing.T) {
		postWorkflowHooksSetup(t)

		var unlockCalled *bool = newBool(false)
		unlockFn := func() {
			unlockCalled = newBool(true)
		}

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PostWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		postWh.GlobalCfg = globalCfg

		When(postWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(unlockFn, nil)
		When(postWhWorkingDir.Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)).ThenReturn(repoDir, false, nil)
		When(whPostWorkflowHookRunner.Run(pCtx, testHook.RunCommand, repoDir)).ThenReturn(result, nil)

		err := postWh.RunPostHooks(ctx)

		Ok(t, err)
		whPostWorkflowHookRunner.VerifyWasCalledOnce().Run(pCtx, testHook.RunCommand, repoDir)
		Assert(t, *unlockCalled == true, "unlock function called")
	})
	t.Run("success hooks not in cfg", func(t *testing.T) {
		postWorkflowHooksSetup(t)
		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				// one with hooks but mismatched id
				{
					ID: "id1",
					PostWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
				// one with the correct id but no hooks
				{
					ID:                fixtures.GithubRepo.ID(),
					PostWorkflowHooks: []*valid.WorkflowHook{},
				},
			},
		}

		postWh.GlobalCfg = globalCfg

		err := postWh.RunPostHooks(ctx)

		Ok(t, err)

		whPostWorkflowHookRunner.VerifyWasCalled(Never()).Run(pCtx, testHook.RunCommand, repoDir)
		postWhWorkingDirLocker.VerifyWasCalled(Never()).TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)
		postWhWorkingDir.VerifyWasCalled(Never()).Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)
	})
	t.Run("error locking work dir", func(t *testing.T) {
		postWorkflowHooksSetup(t)

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PostWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		postWh.GlobalCfg = globalCfg

		When(postWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(func() {}, errors.New("some error"))

		err := postWh.RunPostHooks(ctx)

		Assert(t, err != nil, "error not nil")
		postWhWorkingDir.VerifyWasCalled(Never()).Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)
		whPostWorkflowHookRunner.VerifyWasCalled(Never()).Run(pCtx, testHook.RunCommand, repoDir)
	})

	t.Run("error cloning", func(t *testing.T) {
		postWorkflowHooksSetup(t)

		var unlockCalled *bool = newBool(false)
		unlockFn := func() {
			unlockCalled = newBool(true)
		}

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PostWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		postWh.GlobalCfg = globalCfg

		When(postWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(unlockFn, nil)
		When(postWhWorkingDir.Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)).ThenReturn(repoDir, false, errors.New("some error"))

		err := postWh.RunPostHooks(ctx)

		Assert(t, err != nil, "error not nil")

		whPostWorkflowHookRunner.VerifyWasCalled(Never()).Run(pCtx, testHook.RunCommand, repoDir)
		Assert(t, *unlockCalled == true, "unlock function called")
	})

	t.Run("error running post hook", func(t *testing.T) {
		postWorkflowHooksSetup(t)

		var unlockCalled *bool = newBool(false)
		unlockFn := func() {
			unlockCalled = newBool(true)
		}

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PostWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		postWh.GlobalCfg = globalCfg

		When(postWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(unlockFn, nil)
		When(postWhWorkingDir.Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)).ThenReturn(repoDir, false, nil)
		When(whPostWorkflowHookRunner.Run(pCtx, testHook.RunCommand, repoDir)).ThenReturn(result, errors.New("some error"))

		err := postWh.RunPostHooks(ctx)

		Assert(t, err != nil, "error not nil")
		Assert(t, *unlockCalled == true, "unlock function called")
	})
}
