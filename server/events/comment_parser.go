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
	"io/ioutil"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flynn-archive/go-shlex"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/yaml"
	"github.com/spf13/pflag"
)

const (
	workspaceFlagLong  = "workspace"
	workspaceFlagShort = "w"
	dirFlagLong        = "dir"
	dirFlagShort       = "d"
	projectFlagLong    = "project"
	projectFlagShort   = "p"
	verboseFlagLong    = "verbose"
	verboseFlagShort   = ""
	atlantisExecutable = "atlantis"
	allFlag            = "all"
)

// multiLineRegex is used to ignore multi-line comments since those aren't valid
// Atlantis commands. If the second line just has newlines then we let it pass
// through because when you double click on a comment in GitHub and then you
// paste it again, GitHub adds two newlines and so we wanted to allow copying
// and pasting GitHub comments.
var multiLineRegex = regexp.MustCompile(`.*\r?\n[^\r\n]+`)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_comment_parsing.go CommentParsing

// CommentParsing handles parsing pull request comments.
type CommentParsing interface {
	// Parse attempts to parse a pull request comment to see if it's an Atlantis
	// command.
	Parse(comment string, vcsHost models.VCSHostType) CommentParseResult
}

// CommentBuilder builds comment commands that can be used on pull requests.
type CommentBuilder interface {
	// BuildPlanComment builds a plan comment for the specified args.
	BuildPlanComment(repoRelDir string, workspace string, project string, commentArgs []string) string
	// BuildApplyComment builds an apply comment for the specified args.
	BuildApplyComment(repoRelDir string, workspace string, project string) string
}

// CommentParser implements CommentParsing
type CommentParser struct {
	GithubUser      string
	GitlabUser      string
	BitbucketUser   string
	AzureDevopsUser string
}

// CommentParseResult describes the result of parsing a comment as a command.
type CommentParseResult struct {
	// Command is the successfully parsed command. Will be nil if
	// CommentResponse or Ignore is set.
	Command *CommentCommand
	// CommentResponse is set when we should respond immediately to the command
	// for example for atlantis help.
	CommentResponse string
	// Ignore is set to true when we should just ignore this comment.
	Ignore bool
}

// Parse parses the comment as an Atlantis command.
//
// Valid commands contain:
// - The initial "executable" name, 'run' or 'atlantis' or '@GithubUser'
//   where GithubUser is the API user Atlantis is running as.
// - Then a command, either 'plan', 'apply', or 'help'.
// - Then optional flags, then an optional separator '--' followed by optional
//   extra flags to be appended to the terraform plan/apply command.
//
// Examples:
// - atlantis help
// - run plan
// - @GithubUser plan -w staging
// - atlantis plan -w staging -d dir --verbose
// - atlantis plan --verbose -- -key=value -key2 value2
//
func (e *CommentParser) Parse(comment string, vcsHost models.VCSHostType) CommentParseResult {
	if multiLineRegex.MatchString(comment) {
		return CommentParseResult{Ignore: true}
	}

	// We first use strings.Fields to parse and do an initial evaluation.
	// Later we use a proper shell parser and re-parse.
	args := strings.Fields(comment)
	if len(args) < 1 {
		return CommentParseResult{Ignore: true}
	}

	// Helpfully warn the user if they're using "terraform" instead of "atlantis"
	if args[0] == "terraform" {
		return CommentParseResult{CommentResponse: DidYouMeanAtlantisComment}
	}

	// Atlantis can be invoked using the name of the VCS host user we're
	// running under. Need to be able to match against that user.
	var vcsUser string
	switch vcsHost {
	case models.Github:
		vcsUser = e.GithubUser
	case models.Gitlab:
		vcsUser = e.GitlabUser
	case models.BitbucketCloud, models.BitbucketServer:
		vcsUser = e.BitbucketUser
	case models.AzureDevops:
		vcsUser = e.AzureDevopsUser
	}
	executableNames := []string{"run", atlantisExecutable, "@" + vcsUser}
	if !e.stringInSlice(args[0], executableNames) {
		return CommentParseResult{Ignore: true}
	}

	// Now that we know Atlantis is being invoked, re-parse using a shell-style
	// parser.
	args, err := shlex.Split(comment)
	if err != nil {
		return CommentParseResult{CommentResponse: fmt.Sprintf("```\nError parsing command: %s\n```", err)}
	}
	if len(args) < 1 {
		return CommentParseResult{Ignore: true}
	}

	// If they've just typed the name of the executable then give them the help
	// output.
	if len(args) == 1 {
		return CommentParseResult{CommentResponse: HelpComment}
	}
	command := args[1]

	// Help output.
	if e.stringInSlice(command, []string{"help", "-h", "--help"}) {
		return CommentParseResult{CommentResponse: HelpComment}
	}

	// Need to have a plan or apply at this point.
	if !e.stringInSlice(command, []string{models.PlanCommand.String(), models.ApplyCommand.String()}) {
		return CommentParseResult{CommentResponse: fmt.Sprintf("```\nError: unknown command %q.\nRun 'atlantis --help' for usage.\n```", command)}
	}

	var workspace string
	var dir string
	var project string
	var verbose bool
	var all bool
	var flagSet *pflag.FlagSet
	var name models.CommandName

	// Set up the flag parsing depending on the command.
	switch command {
	case models.PlanCommand.String():
		name = models.PlanCommand
		flagSet = pflag.NewFlagSet(models.PlanCommand.String(), pflag.ContinueOnError)
		flagSet.SetOutput(ioutil.Discard)
		flagSet.StringVarP(&workspace, workspaceFlagLong, workspaceFlagShort, "", "Switch to this Terraform workspace before planning.")
		flagSet.StringVarP(&dir, dirFlagLong, dirFlagShort, "", "Which directory to run plan in relative to root of repo, ex. 'child/dir'.")
		flagSet.StringVarP(&project, projectFlagLong, projectFlagShort, "", fmt.Sprintf("Which project to run plan for. Refers to the name of the project configured in %s. Cannot be used at same time as workspace or dir flags.", yaml.AtlantisYAMLFilename))
		flagSet.BoolVarP(&verbose, verboseFlagLong, verboseFlagShort, false, "Append Atlantis log to comment.")
		flagSet.BoolVar(&all, allFlag, false, "Run plan on all projects.")
	case models.ApplyCommand.String():
		name = models.ApplyCommand
		flagSet = pflag.NewFlagSet(models.ApplyCommand.String(), pflag.ContinueOnError)
		flagSet.SetOutput(ioutil.Discard)
		flagSet.StringVarP(&workspace, workspaceFlagLong, workspaceFlagShort, "", "Apply the plan for this Terraform workspace.")
		flagSet.StringVarP(&dir, dirFlagLong, dirFlagShort, "", "Apply the plan for this directory, relative to root of repo, ex. 'child/dir'.")
		flagSet.StringVarP(&project, projectFlagLong, projectFlagShort, "", fmt.Sprintf("Apply the plan for this project. Refers to the name of the project configured in %s. Cannot be used at same time as workspace or dir flags.", yaml.AtlantisYAMLFilename))
		flagSet.BoolVarP(&verbose, verboseFlagLong, verboseFlagShort, false, "Append Atlantis log to comment.")
	default:
		return CommentParseResult{CommentResponse: fmt.Sprintf("Error: unknown command %q – this is a bug", command)}
	}

	// Now parse the flags.
	// It's safe to use [2:] because we know there's at least 2 elements in args.
	err = flagSet.Parse(args[2:])
	if err == pflag.ErrHelp {
		return CommentParseResult{CommentResponse: fmt.Sprintf("```\nUsage of %s:\n%s\n```", command, flagSet.FlagUsagesWrapped(usagesCols))}
	}
	if err != nil {
		return CommentParseResult{CommentResponse: e.errMarkdown(err.Error(), command, flagSet)}
	}

	var unusedArgs []string
	if flagSet.ArgsLenAtDash() == -1 {
		unusedArgs = flagSet.Args()
	} else {
		unusedArgs = flagSet.Args()[0:flagSet.ArgsLenAtDash()]
	}
	if len(unusedArgs) > 0 {
		return CommentParseResult{CommentResponse: e.errMarkdown(fmt.Sprintf("unknown argument(s) – %s", strings.Join(unusedArgs, " ")), command, flagSet)}
	}

	var extraArgs []string
	if flagSet.ArgsLenAtDash() != -1 {
		extraArgs = flagSet.Args()[flagSet.ArgsLenAtDash():]
	}

	dir, err = e.validateDir(dir)
	if err != nil {
		return CommentParseResult{CommentResponse: e.errMarkdown(err.Error(), command, flagSet)}
	}

	// Use the same validation that Terraform uses: https://git.io/vxGhU. Plus
	// we also don't allow '..'. We don't want the workspace to contain a path
	// since we create files based on the name.
	if workspace != url.PathEscape(workspace) || strings.Contains(workspace, "..") {
		return CommentParseResult{CommentResponse: e.errMarkdown(fmt.Sprintf("invalid workspace: %q", workspace), command, flagSet)}
	}

	if all && (project != "" || workspace != "" || dir != "") {
		err := fmt.Sprintf("cannot use --%s at same time as same time as -%s/--%s, -%s/--%s or -%s/--%s", allFlag, projectFlagShort, projectFlagLong, dirFlagShort, dirFlagLong, workspaceFlagShort, workspaceFlagLong)
		return CommentParseResult{CommentResponse: e.errMarkdown(err, command, flagSet)}
	}

	// If project is specified, dir or workspace should not be set. Since we
	// dir/workspace have defaults we can't detect if the user set the flag
	// to the default or didn't set the flag so there is an edge case here we
	// don't detect, ex. atlantis plan -p project -d . -w default won't cause
	// an error.
	if project != "" && (workspace != "" || dir != "") {
		err := fmt.Sprintf("cannot use -%s/--%s at same time as -%s/--%s or -%s/--%s", projectFlagShort, projectFlagLong, dirFlagShort, dirFlagLong, workspaceFlagShort, workspaceFlagLong)
		return CommentParseResult{CommentResponse: e.errMarkdown(err, command, flagSet)}
	}

	return CommentParseResult{
		Command: NewCommentCommand(dir, extraArgs, name, verbose, workspace, project, all),
	}
}

// BuildPlanComment builds a plan comment for the specified args.
func (e *CommentParser) BuildPlanComment(repoRelDir string, workspace string, project string, commentArgs []string) string {
	flags := e.buildFlags(repoRelDir, workspace, project)
	commentFlags := ""
	if len(commentArgs) > 0 {
		var flagsWithoutQuotes []string
		for _, f := range commentArgs {
			f = strings.TrimPrefix(f, "\"")
			f = strings.TrimSuffix(f, "\"")
			flagsWithoutQuotes = append(flagsWithoutQuotes, f)
		}
		commentFlags = fmt.Sprintf(" -- %s", strings.Join(flagsWithoutQuotes, " "))
	}
	return fmt.Sprintf("%s %s%s%s", atlantisExecutable, models.PlanCommand.String(), flags, commentFlags)
}

// BuildApplyComment builds an apply comment for the specified args.
func (e *CommentParser) BuildApplyComment(repoRelDir string, workspace string, project string) string {
	flags := e.buildFlags(repoRelDir, workspace, project)
	return fmt.Sprintf("%s %s%s", atlantisExecutable, models.ApplyCommand.String(), flags)
}

func (e *CommentParser) buildFlags(repoRelDir string, workspace string, project string) string {
	// Add quotes if dir has spaces.
	if strings.Contains(repoRelDir, " ") {
		repoRelDir = fmt.Sprintf("%q", repoRelDir)
	}

	switch {
	// If project is specified we can just use its name.
	case project != "":
		return fmt.Sprintf(" -%s %s", projectFlagShort, project)
	case repoRelDir == DefaultRepoRelDir && workspace == DefaultWorkspace:
		// If it's the root and default workspace then we just need to specify one
		// of the flags and the other will get defaulted.
		return fmt.Sprintf(" -%s %s", dirFlagShort, DefaultRepoRelDir)
	case repoRelDir == DefaultRepoRelDir:
		// If dir is the default then we just need to specify workspace.
		return fmt.Sprintf(" -%s %s", workspaceFlagShort, workspace)
	case workspace == DefaultWorkspace:
		// If workspace is the default then we just need to specify the dir.

		return fmt.Sprintf(" -%s %s", dirFlagShort, repoRelDir)
	default:
		// Otherwise we have to specify both flags.
		return fmt.Sprintf(" -%s %s -%s %s", dirFlagShort, repoRelDir, workspaceFlagShort, workspace)
	}
}

func (e *CommentParser) validateDir(dir string) (string, error) {
	if dir == "" {
		return dir, nil
	}
	validatedDir := filepath.Clean(dir)
	// Join with . so the path is relative. This helps us if they use '/',
	// and is safe to do if their path is relative since it's a no-op.
	validatedDir = filepath.Join(".", validatedDir)
	// Need to clean again to resolve relative validatedDirs.
	validatedDir = filepath.Clean(validatedDir)
	// Detect relative dirs since they're not allowed.
	if strings.HasPrefix(validatedDir, "..") {
		return "", fmt.Errorf("using a relative path %q with -%s/--%s is not allowed", dir, dirFlagShort, dirFlagLong)
	}

	return validatedDir, nil
}

func (e *CommentParser) stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func (e *CommentParser) errMarkdown(errMsg string, command string, flagSet *pflag.FlagSet) string {
	return fmt.Sprintf("```\nError: %s.\nUsage of %s:\n%s```", errMsg, command, flagSet.FlagUsagesWrapped(usagesCols))
}

// HelpComment is the comment we add to the pull request when someone runs
// `atlantis help`.
var HelpComment = "```cmake\n" +
	`atlantis
Terraform Pull Request Automation

Usage:
  atlantis <command> [options] -- [terraform options]

Examples:
  # run plan in the root directory passing the -target flag to terraform
  atlantis plan -d . -- -target=resource

  # apply all unapplied plans from this pull request
  atlantis apply

  # apply the plan for the root directory and staging workspace
  atlantis apply -d . -w staging

Commands:
  plan   Runs 'terraform plan' for the changes in this pull request.
         To plan a specific project, use the -d, -w and -p flags.
  apply  Runs 'terraform apply' on all unapplied plans from this pull request.
         To only apply a specific plan, use the -d, -w and -p flags.
  help   View help.

Flags:
  -h, --help   help for atlantis

Use "atlantis [command] --help" for more information about a command.` +
	"\n```"

// DidYouMeanAtlantisComment is the comment we add to the pull request when
// someone runs a command with terraform instead of atlantis.
var DidYouMeanAtlantisComment = "Did you mean to use `atlantis` instead of `terraform`?"
