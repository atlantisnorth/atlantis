package runtime

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/runatlantis/atlantis/server/events/models"
)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_post_workflows_hook_runner.go PostWorkflowHookRunner
type PostWorkflowHookRunner interface {
	Run(ctx models.WorkflowHookCommandContext, command string, path string) (string, error)
}

type DefaultPostWorkflowHookRunner struct{}

func (wh DefaultPostWorkflowHookRunner) Run(ctx models.WorkflowHookCommandContext, command string, path string) (string, error) {
	cmd := exec.Command("sh", "-c", command) // #nosec
	cmd.Dir = path

	baseEnvVars := os.Environ()
	customEnvVars := map[string]string{
		"BASE_BRANCH_NAME": ctx.Pull.BaseBranch,
		"BASE_REPO_NAME":   ctx.BaseRepo.Name,
		"BASE_REPO_OWNER":  ctx.BaseRepo.Owner,
		"DIR":              path,
		"HEAD_BRANCH_NAME": ctx.Pull.HeadBranch,
		"HEAD_COMMIT":      ctx.Pull.HeadCommit,
		"HEAD_REPO_NAME":   ctx.HeadRepo.Name,
		"HEAD_REPO_OWNER":  ctx.HeadRepo.Owner,
		"PULL_AUTHOR":      ctx.Pull.Author,
		"PULL_NUM":         fmt.Sprintf("%d", ctx.Pull.Num),
		"USER_NAME":        ctx.User.Username,
	}

	finalEnvVars := baseEnvVars
	for key, val := range customEnvVars {
		finalEnvVars = append(finalEnvVars, fmt.Sprintf("%s=%s", key, val))
	}

	cmd.Env = finalEnvVars
	out, err := cmd.CombinedOutput()

	if err != nil {
		err = fmt.Errorf("%s: running %q in %q: \n%s", err, command, path, out)
		ctx.Log.Debug("error: %s", err)
		return "", err
	}
	ctx.Log.Info("successfully ran %q in %q", command, path)
	return string(out), nil
}
