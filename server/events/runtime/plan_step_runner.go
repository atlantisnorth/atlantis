package runtime

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	version "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
)

const (
	defaultWorkspace = "default"
	// refreshSeparator is what separates the refresh stage from the calculated
	// plan during a terraform plan.
	refreshSeparator = "------------------------------------------------------------------------\n"
)

var (
	plusDiffRegex  = regexp.MustCompile(`(?m)^ {2}\+`)
	tildeDiffRegex = regexp.MustCompile(`(?m)^ {2}~`)
	minusDiffRegex = regexp.MustCompile(`(?m)^ {2}-`)
)

type PlanStepRunner struct {
	TerraformExecutor   TerraformExec
	DefaultTFVersion    *version.Version
	CommitStatusUpdater StatusUpdater
	AsyncTFExec         AsyncTFExec
}

func (p *PlanStepRunner) Run(ctx models.ProjectCommandContext, extraArgs []string, path string, envs map[string]string) (string, error) {
	tfVersion := p.DefaultTFVersion
	if ctx.TerraformVersion != nil {
		tfVersion = ctx.TerraformVersion
	}

	// We only need to switch workspaces in version 0.9.*. In older versions,
	// there is no such thing as a workspace so we don't need to do anything.
	if err := p.switchWorkspace(ctx, path, tfVersion, envs); err != nil {
		return "", err
	}

	planFile := filepath.Join(path, GetPlanFilename(ctx.Workspace, ctx.ProjectName))
	planCmd := p.buildPlanCmd(ctx, extraArgs, path, tfVersion, planFile)
	output, err := p.TerraformExecutor.RunCommandWithVersion(ctx, filepath.Clean(path), planCmd, envs, tfVersion)
	if p.isRemoteOpsErr(output, err) {
		ctx.Log.Debug("detected that this project is using TFE remote ops")
		return p.remotePlan(ctx, extraArgs, path, tfVersion, planFile, envs)
	}
	if err != nil {
		return output, err
	}
	return p.fmtPlanOutput(output), nil
}

// isRemoteOpsErr returns true if there was an error caused due to this
// project using TFE remote operations.
func (p *PlanStepRunner) isRemoteOpsErr(output string, err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(output, remoteOpsErr01114) || strings.Contains(output, remoteOpsErr012)
}

// remotePlan runs a terraform plan command compatible with TFE remote
// operations.
func (p *PlanStepRunner) remotePlan(ctx models.ProjectCommandContext, extraArgs []string, path string, tfVersion *version.Version, planFile string, envs map[string]string) (string, error) {
	argList := [][]string{
		{"plan", "-input=false", "-refresh", "-no-color"},
		extraArgs,
		ctx.EscapedCommentArgs,
	}
	args := p.flatten(argList)
	output, err := p.runRemotePlan(ctx, args, path, tfVersion, envs)
	if err != nil {
		return output, err
	}

	// If using remote ops, we create our own "fake" planfile with the
	// text output of the plan. We do this for two reasons:
	// 1) Atlantis relies on there being a planfile on disk to detect which
	// projects have outstanding plans.
	// 2) Remote ops don't support the -out parameter so we can't save the
	// plan. To ensure that what gets applied is the plan we printed to the PR,
	// during the apply phase, we diff the output we stored in the fake
	// planfile with the pending apply output.
	planOutput := output
	sepIdx := strings.Index(planOutput, refreshSeparator)
	if sepIdx > -1 {
		planOutput = planOutput[sepIdx+len(refreshSeparator):]
	}

	// We also prepend our own remote ops header to the file so during apply we
	// know this is a remote apply.
	err = ioutil.WriteFile(planFile, []byte(remoteOpsHeader+planOutput), 0644)
	if err != nil {
		return output, errors.Wrap(err, "unable to create planfile for remote ops")
	}

	return p.fmtPlanOutput(output), nil
}

// switchWorkspace changes the terraform workspace if necessary and will create
// it if it doesn't exist. It handles differences between versions.
func (p *PlanStepRunner) switchWorkspace(ctx models.ProjectCommandContext, path string, tfVersion *version.Version, envs map[string]string) error {
	// In versions less than 0.9 there is no support for workspaces.
	noWorkspaceSupport := MustConstraint("<0.9").Check(tfVersion)
	// If the user tried to set a specific workspace in the comment but their
	// version of TF doesn't support workspaces then error out.
	if noWorkspaceSupport && ctx.Workspace != defaultWorkspace {
		return fmt.Errorf("terraform version %s does not support workspaces", tfVersion)
	}
	if noWorkspaceSupport {
		return nil
	}

	// In version 0.9.* the workspace command was called env.
	workspaceCmd := "workspace"
	runningZeroPointNine := MustConstraint(">=0.9,<0.10").Check(tfVersion)
	if runningZeroPointNine {
		workspaceCmd = "env"
	}

	// Use `workspace show` to find out what workspace we're in now. If we're
	// already in the right workspace then no need to switch. This will save us
	// about ten seconds. This command is only available in > 0.10.
	if !runningZeroPointNine {
		workspaceShowOutput, err := p.TerraformExecutor.RunCommandWithVersion(ctx, path, []string{workspaceCmd, "show"}, envs, tfVersion)
		if err != nil {
			return err
		}
		// If `show` says we're already on this workspace then we're done.
		if strings.TrimSpace(workspaceShowOutput) == ctx.Workspace {
			return nil
		}
	}

	// Finally we'll have to select the workspace. We need to figure out if this
	// workspace exists so we can create it if it doesn't.
	// To do this we can either select and catch the error or use list and then
	// look for the workspace. Both commands take the same amount of time so
	// that's why we're running select here.
	_, err := p.TerraformExecutor.RunCommandWithVersion(ctx, path, []string{workspaceCmd, "select", "-no-color", ctx.Workspace}, envs, tfVersion)
	if err != nil {
		// If terraform workspace select fails we run terraform workspace
		// new to create a new workspace automatically.
		out, err := p.TerraformExecutor.RunCommandWithVersion(ctx, path, []string{workspaceCmd, "new", "-no-color", ctx.Workspace}, envs, tfVersion)
		if err != nil {
			return fmt.Errorf("%s: %s", err, out)
		}
	}
	return nil
}

func (p *PlanStepRunner) buildPlanCmd(ctx models.ProjectCommandContext, extraArgs []string, path string, tfVersion *version.Version, planFile string) []string {
	tfVars := p.tfVars(ctx, tfVersion)

	// Check if env/{workspace}.tfvars exist and include it. This is a use-case
	// from Hootsuite where Atlantis was first created so we're keeping this as
	// an homage and a favor so they don't need to refactor all their repos.
	// It's also a nice way to structure your repos to reduce duplication.
	var envFileArgs []string
	envFile := filepath.Join(path, "env", ctx.Workspace+".tfvars")
	if _, err := os.Stat(envFile); err == nil {
		envFileArgs = []string{"-var-file", envFile}
	}

	argList := [][]string{
		// NOTE: we need to quote the plan filename because Bitbucket Server can
		// have spaces in its repo owner names.
		{"plan", "-input=false", "-refresh", "-no-color", "-out", fmt.Sprintf("%q", planFile)},
		tfVars,
		extraArgs,
		ctx.EscapedCommentArgs,
		envFileArgs,
	}

	return p.flatten(argList)
}

// tfVars returns a list of "-var", "key=value" pairs that identify who and which
// repo this command is running for. This can be used for naming the
// session name in AWS which will identify in CloudTrail the source of
// Atlantis API calls.
// If using Terraform >= 0.12 we don't set any of these variables because
// those versions don't allow setting -var flags for any variables that aren't
// actually used in the configuration. Since there's no way for us to detect
// if the configuration is using those variables, we don't set them.
func (p *PlanStepRunner) tfVars(ctx models.ProjectCommandContext, tfVersion *version.Version) []string {
	if tfVersion.GreaterThanOrEqual(version.Must(version.NewVersion("0.12.0"))) {
		return nil
	}

	// NOTE: not using maps and looping here because we need to keep the
	// ordering for testing purposes.
	// NOTE: quoting the values because in Bitbucket the owner can have
	// spaces, ex -var atlantis_repo_owner="bitbucket owner".
	return []string{
		"-var",
		fmt.Sprintf("%s=%q", "atlantis_user", ctx.User.Username),
		"-var",
		fmt.Sprintf("%s=%q", "atlantis_repo", ctx.BaseRepo.FullName),
		"-var",
		fmt.Sprintf("%s=%q", "atlantis_repo_name", ctx.BaseRepo.Name),
		"-var",
		fmt.Sprintf("%s=%q", "atlantis_repo_owner", ctx.BaseRepo.Owner),
		"-var",
		fmt.Sprintf("%s=%d", "atlantis_pull_num", ctx.Pull.Num),
	}
}

func (p *PlanStepRunner) flatten(slices [][]string) []string {
	var flattened []string
	for _, v := range slices {
		flattened = append(flattened, v...)
	}
	return flattened
}

// fmtPlanOutput uses regex's to remove any leading whitespace in front of the
// terraform output so that the diff syntax highlighting works. Example:
// "  - aws_security_group_rule.allow_all" =>
// "- aws_security_group_rule.allow_all"
// We do it for +, ~ and -.
// It also removes the "Refreshing..." preamble.
func (p *PlanStepRunner) fmtPlanOutput(output string) string {
	// Plan output contains a lot of "Refreshing..." lines followed by a
	// separator. We want to remove everything before that separator.
	sepIdx := strings.Index(output, refreshSeparator)
	if sepIdx > -1 {
		output = output[sepIdx+len(refreshSeparator):]
	}

	output = plusDiffRegex.ReplaceAllString(output, "+")
	output = tildeDiffRegex.ReplaceAllString(output, "~")
	return minusDiffRegex.ReplaceAllString(output, "-")
}

// runRemotePlan runs a terraform command that utilizes the remote operations
// backend. It watches the command output for the run url to be printed, and
// then updates the commit status with a link to the run url.
// The run url is a link to the Terraform Enterprise UI where the output
// from the in-progress command can be viewed.
// cmdArgs is the args to terraform to execute.
// path is the path to where we need to execute.
func (p *PlanStepRunner) runRemotePlan(
	ctx models.ProjectCommandContext,
	cmdArgs []string,
	path string,
	tfVersion *version.Version,
	envs map[string]string) (string, error) {

	// updateStatusF will update the commit status and log any error.
	updateStatusF := func(status models.CommitStatus, url string) {
		if err := p.CommitStatusUpdater.UpdateProject(ctx, models.PlanCommand, status, url); err != nil {
			ctx.Log.Err("unable to update status: %s", err)
		}
	}

	// Start the async command execution.
	ctx.Log.Debug("starting async tf remote operation")
	_, outCh := p.AsyncTFExec.RunCommandAsync(ctx, filepath.Clean(path), cmdArgs, envs, tfVersion)
	var lines []string
	nextLineIsRunURL := false
	var runURL string
	var err error

	for line := range outCh {
		if line.Err != nil {
			err = line.Err
			break
		}
		lines = append(lines, line.Line)

		// Here we're checking for the run url and updating the status
		// if found.
		if line.Line == lineBeforeRunURL {
			nextLineIsRunURL = true
		} else if nextLineIsRunURL {
			runURL = strings.TrimSpace(line.Line)
			ctx.Log.Debug("remote run url found, updating commit status")
			updateStatusF(models.PendingCommitStatus, runURL)
			nextLineIsRunURL = false
		}
	}

	ctx.Log.Debug("async tf remote operation complete")
	output := strings.Join(lines, "\n")
	if err != nil {
		updateStatusF(models.FailedCommitStatus, runURL)
	} else {
		updateStatusF(models.SuccessCommitStatus, runURL)
	}
	return output, err
}

// remoteOpsErr01114 is the error terraform plan will return if this project is
// using TFE remote operations in TF 0.11.14.
var remoteOpsErr01114 = `Error: Saving a generated plan is currently not supported!

The "remote" backend does not support saving the generated execution
plan locally at this time.

`

// remoteOpsErr012 is the error terraform plan will return if this project is
// using TFE remote operations in TF 0.12.{0-4}. Later versions haven't been
// released yet at this time.
var remoteOpsErr012 = `Error: Saving a generated plan is currently not supported

The "remote" backend does not support saving the generated execution plan
locally at this time.

`

// remoteOpsHeader is the header we add to the planfile if this plan was
// generated using TFE remote operations.
var remoteOpsHeader = "Atlantis: this plan was created by remote ops\n"
