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

var preWh events.DefaultPreWorkflowHooksCommandRunner
var preWhWorkingDir *mocks.MockWorkingDir
var preWhWorkingDirLocker *mocks.MockWorkingDirLocker
var whPreWorkflowHookRunner *runtime_mocks.MockPreWorkflowHookRunner

func preWorkflowHooksSetup(t *testing.T) {
	RegisterMockTestingT(t)
	vcsClient := vcsmocks.NewMockClient()
	preWhWorkingDir = mocks.NewMockWorkingDir()
	preWhWorkingDirLocker = mocks.NewMockWorkingDirLocker()
	whPreWorkflowHookRunner = runtime_mocks.NewMockPreWorkflowHookRunner()

	preWh = events.DefaultPreWorkflowHooksCommandRunner{
		VCSClient:             vcsClient,
		WorkingDirLocker:      preWhWorkingDirLocker,
		WorkingDir:            preWhWorkingDir,
		PreWorkflowHookRunner: whPreWorkflowHookRunner,
	}
}

func newBool(b bool) *bool {
	return &b
}

func TestRunPreHooks_Clone(t *testing.T) {

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
		preWorkflowHooksSetup(t)

		var unlockCalled *bool = newBool(false)
		unlockFn := func() {
			unlockCalled = newBool(true)
		}

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PreWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		preWh.GlobalCfg = globalCfg

		When(preWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(unlockFn, nil)
		When(preWhWorkingDir.Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)).ThenReturn(repoDir, false, nil)
		When(whPreWorkflowHookRunner.Run(pCtx, testHook.RunCommand, repoDir)).ThenReturn(result, nil)

		err := preWh.RunPreHooks(ctx)

		Ok(t, err)
		whPreWorkflowHookRunner.VerifyWasCalledOnce().Run(pCtx, testHook.RunCommand, repoDir)
		Assert(t, *unlockCalled == true, "unlock function called")
	})
	t.Run("success hooks not in cfg", func(t *testing.T) {
		preWorkflowHooksSetup(t)
		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				// one with hooks but mismatched id
				{
					ID: "id1",
					PreWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
				// one with the correct id but no hooks
				{
					ID:               fixtures.GithubRepo.ID(),
					PreWorkflowHooks: []*valid.WorkflowHook{},
				},
			},
		}

		preWh.GlobalCfg = globalCfg

		err := preWh.RunPreHooks(ctx)

		Ok(t, err)

		whPreWorkflowHookRunner.VerifyWasCalled(Never()).Run(pCtx, testHook.RunCommand, repoDir)
		preWhWorkingDirLocker.VerifyWasCalled(Never()).TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)
		preWhWorkingDir.VerifyWasCalled(Never()).Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)
	})
	t.Run("error locking work dir", func(t *testing.T) {
		preWorkflowHooksSetup(t)

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PreWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		preWh.GlobalCfg = globalCfg

		When(preWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(func() {}, errors.New("some error"))

		err := preWh.RunPreHooks(ctx)

		Assert(t, err != nil, "error not nil")
		preWhWorkingDir.VerifyWasCalled(Never()).Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)
		whPreWorkflowHookRunner.VerifyWasCalled(Never()).Run(pCtx, testHook.RunCommand, repoDir)
	})

	t.Run("error cloning", func(t *testing.T) {
		preWorkflowHooksSetup(t)

		var unlockCalled *bool = newBool(false)
		unlockFn := func() {
			unlockCalled = newBool(true)
		}

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PreWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		preWh.GlobalCfg = globalCfg

		When(preWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(unlockFn, nil)
		When(preWhWorkingDir.Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)).ThenReturn(repoDir, false, errors.New("some error"))

		err := preWh.RunPreHooks(ctx)

		Assert(t, err != nil, "error not nil")

		whPreWorkflowHookRunner.VerifyWasCalled(Never()).Run(pCtx, testHook.RunCommand, repoDir)
		Assert(t, *unlockCalled == true, "unlock function called")
	})

	t.Run("error running pre hook", func(t *testing.T) {
		preWorkflowHooksSetup(t)

		var unlockCalled *bool = newBool(false)
		unlockFn := func() {
			unlockCalled = newBool(true)
		}

		globalCfg := valid.GlobalCfg{
			Repos: []valid.Repo{
				{
					ID: fixtures.GithubRepo.ID(),
					PreWorkflowHooks: []*valid.WorkflowHook{
						&testHook,
					},
				},
			},
		}

		preWh.GlobalCfg = globalCfg

		When(preWhWorkingDirLocker.TryLock(fixtures.GithubRepo.FullName, newPull.Num, events.DefaultWorkspace)).ThenReturn(unlockFn, nil)
		When(preWhWorkingDir.Clone(log, fixtures.GithubRepo, newPull, events.DefaultWorkspace)).ThenReturn(repoDir, false, nil)
		When(whPreWorkflowHookRunner.Run(pCtx, testHook.RunCommand, repoDir)).ThenReturn(result, errors.New("some error"))

		err := preWh.RunPreHooks(ctx)

		Assert(t, err != nil, "error not nil")
		Assert(t, *unlockCalled == true, "unlock function called")
	})
}
