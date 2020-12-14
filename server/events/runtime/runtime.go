// Package runtime holds code for actually running commands vs. preparing
// and constructing.
package runtime

import (
	"fmt"
	"regexp"
	"strings"

	version "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/terraform"
	"github.com/runatlantis/atlantis/server/logging"
)

const (
	// lineBeforeRunURL is the line output during a remote operation right before
	// a link to the run url will be output.
	lineBeforeRunURL     = "To view this run in a browser, visit:"
	planfileSlashReplace = "::"
)

// TerraformExec brings the interface from TerraformClient into this package
// without causing circular imports.
type TerraformExec interface {
	RunCommandWithVersion(ctx models.ProjectCommandContext, path string, args []string, envs map[string]string, v *version.Version) (string, error)
	EnsureVersion(log *logging.SimpleLogger, v *version.Version) error
}

// AsyncTFExec brings the interface from TerraformClient into this package
// without causing circular imports.
// It's split from TerraformExec because due to a bug in pegomock with channels,
// we can't generate a mock for it so we hand-write it for this specific method.
type AsyncTFExec interface {
	// RunCommandAsync runs terraform with args. It immediately returns an
	// input and output channel. Callers can use the output channel to
	// get the realtime output from the command.
	// Callers can use the input channel to pass stdin input to the command.
	// If any error is passed on the out channel, there will be no
	// further output (so callers are free to exit).
	RunCommandAsync(ctx models.ProjectCommandContext, path string, args []string, envs map[string]string, v *version.Version) (chan<- string, <-chan terraform.Line)
}

// StatusUpdater brings the interface from CommitStatusUpdater into this package
// without causing circular imports.
type StatusUpdater interface {
	UpdateProject(ctx models.ProjectCommandContext, cmdName models.CommandName, status models.CommitStatus, url string) error
}

// MustConstraint returns a constraint. It panics on error.
func MustConstraint(constraint string) version.Constraints {
	c, err := version.NewConstraint(constraint)
	if err != nil {
		panic(err)
	}
	return c
}

// GetPlanFilename returns the filename (not the path) of the generated tf plan
// given a workspace and project name.
func GetPlanFilename(workspace string, projName string) string {
	if projName == "" {
		return fmt.Sprintf("%s.tfplan", workspace)
	}
	projName = strings.Replace(projName, "/", planfileSlashReplace, -1)
	return fmt.Sprintf("%s-%s.tfplan", projName, workspace)
}

// ProjectNameFromPlanfile returns the project name that a planfile with name
// filename is for. If filename is for a project without a name then it will
// return an empty string. workspace is the workspace this project is in.
func ProjectNameFromPlanfile(workspace string, filename string) (string, error) {
	r, err := regexp.Compile(fmt.Sprintf(`(.*?)-%s\.tfplan`, workspace))
	if err != nil {
		return "", errors.Wrap(err, "compiling project name regex, this is a bug")
	}
	projMatch := r.FindAllStringSubmatch(filename, 1)
	if projMatch == nil {
		return "", nil
	}
	rawProjName := projMatch[0][1]
	return strings.Replace(rawProjName, planfileSlashReplace, "/", -1), nil
}
