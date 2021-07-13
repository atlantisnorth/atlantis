package runtime

import (
	"path/filepath"

	version "github.com/hashicorp/go-version"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/runtime/common"
)

// InitStep runs `terraform init`.
type InitStepRunner struct {
	TerraformExecutor TerraformExec
	DefaultTFVersion  *version.Version
}

func (i *InitStepRunner) Run(ctx models.ProjectCommandContext, extraArgs []string, path string, envs map[string]string) (string, error) {
	tfVersion := i.DefaultTFVersion
	if ctx.TerraformVersion != nil {
		tfVersion = ctx.TerraformVersion
	}

	terraformInitVerb := []string{"init"}
	terraformInitArgs := []string{"-input=false"}

	// If we're running < 0.9 we have to use `terraform get` instead of `init`.
	if MustConstraint("< 0.9.0").Check(tfVersion) {
		ctx.Log.Info("running terraform version %s so will use `get` instead of `init`", tfVersion)
		terraformInitVerb = []string{"get"}
		terraformInitArgs = []string{}
	}

	terraformInitArgs = append(terraformInitArgs, "-no-color")

	lockfilePath := filepath.Join(path, ".terraform.lock.hcl")
	if MustConstraint("< 0.14.0").Check(tfVersion) || !common.FileExists(lockfilePath) {
		terraformInitArgs = append(terraformInitArgs, "-upgrade")
	}

	finalArgs := common.DeDuplicateExtraArgs(terraformInitArgs, extraArgs)

	terraformInitCmd := append(terraformInitVerb, finalArgs...)

	out, err := i.TerraformExecutor.RunCommandWithVersion(ctx.Log, path, terraformInitCmd, envs, tfVersion, ctx.Workspace)
	// Only include the init output if there was an error. Otherwise it's
	// unnecessary and lengthens the comment.
	if err != nil {
		return out, err
	}
	return "", nil
}
