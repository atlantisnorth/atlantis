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
	"fmt"
	"strings"
	"testing"

	"github.com/runatlantis/atlantis/server/events"
	"github.com/runatlantis/atlantis/server/events/models"
	. "github.com/runatlantis/atlantis/testing"
)

var commentParser = events.CommentParser{
	GithubUser: "github-user",
	GitlabUser: "gitlab-user",
}

func TestParse_Ignored(t *testing.T) {
	ignoreComments := []string{
		"",
		"a",
		"abc",
		"atlantis plan\nbut with newlines",
		"terraform plan\nbut with newlines",
		"This shouldn't error, but it does.",
	}
	for _, c := range ignoreComments {
		r := commentParser.Parse(c, models.Github)
		Assert(t, r.Ignore, "expected Ignore to be true for comment %q", c)
	}
}

func TestParse_HelpResponse(t *testing.T) {
	helpComments := []string{
		"run",
		"atlantis",
		"@github-user",
		"atlantis help",
		"atlantis --help",
		"atlantis -h",
		"atlantis help something else",
		"atlantis help plan",
	}
	for _, c := range helpComments {
		r := commentParser.Parse(c, models.Github)
		Equals(t, events.HelpComment, r.CommentResponse)
	}
}

func TestParse_UnusedArguments(t *testing.T) {
	t.Log("if there are unused flags we return an error")
	cases := []struct {
		Command models.CommandName
		Args    string
		Unused  string
	}{
		{
			models.PlanCommand,
			"-d . arg",
			"arg",
		},
		{
			models.PlanCommand,
			"arg -d .",
			"arg",
		},
		{
			models.PlanCommand,
			"arg",
			"arg",
		},
		{
			models.PlanCommand,
			"arg arg2",
			"arg arg2",
		},
		{
			models.PlanCommand,
			"-d . arg -w kjj arg2",
			"arg arg2",
		},
		{
			models.ApplyCommand,
			"-d . arg",
			"arg",
		},
		{
			models.ApplyCommand,
			"arg arg2",
			"arg arg2",
		},
		{
			models.ApplyCommand,
			"arg arg2 -- useful",
			"arg arg2",
		},
		{
			models.ApplyCommand,
			"arg arg2 --",
			"arg arg2",
		},
	}
	for _, c := range cases {
		comment := fmt.Sprintf("atlantis %s %s", c.Command.String(), c.Args)
		t.Run(comment, func(t *testing.T) {
			r := commentParser.Parse(comment, models.Github)
			usage := PlanUsage
			if c.Command == models.ApplyCommand {
				usage = ApplyUsage
			}
			Equals(t, fmt.Sprintf("```\nError: unknown argument(s) – %s.\n%s```", c.Unused, usage), r.CommentResponse)
		})
	}
}

func TestParse_DidYouMeanAtlantis(t *testing.T) {
	t.Log("given a comment that should result in a 'did you mean atlantis'" +
		"response, should set CommentParseResult.CommentResult")
	comments := []string{
		"terraform",
		"terraform help",
		"terraform --help",
		"terraform -h",
		"terraform plan",
		"terraform apply",
		"terraform plan -w workspace -d . -- test",
	}
	for _, c := range comments {
		r := commentParser.Parse(c, models.Github)
		Assert(t, r.CommentResponse == events.DidYouMeanAtlantisComment,
			"For comment %q expected CommentResponse==%q but got %q", c, events.DidYouMeanAtlantisComment, r.CommentResponse)
	}
}

func TestParse_InvalidCommand(t *testing.T) {
	t.Log("given a comment with an invalid atlantis command, should return " +
		"a warning.")
	comments := []string{
		"atlantis paln",
		"atlantis Plan",
		"atlantis appely apply",
	}
	for _, c := range comments {
		r := commentParser.Parse(c, models.Github)
		exp := fmt.Sprintf("```\nError: unknown command %q.\nRun 'atlantis --help' for usage.\n```", strings.Fields(c)[1])
		Assert(t, r.CommentResponse == exp,
			"For comment %q expected CommentResponse==%q but got %q", c, exp, r.CommentResponse)
	}
}

func TestParse_SubcommandUsage(t *testing.T) {
	t.Log("given a comment asking for the usage of a subcommand should " +
		"return help")
	comments := []string{
		"atlantis plan -h",
		"atlantis plan --help",
		"atlantis apply -h",
		"atlantis apply --help",
	}
	for _, c := range comments {
		r := commentParser.Parse(c, models.Github)
		exp := "Usage of " + strings.Fields(c)[1]
		Assert(t, strings.Contains(r.CommentResponse, exp),
			"For comment %q expected CommentResponse %q to contain %q", c, r.CommentResponse, exp)
		Assert(t, !strings.Contains(r.CommentResponse, "Error:"),
			"For comment %q expected CommentResponse %q to not contain %q", c, r.CommentResponse, "Error: ")
	}
}

func TestParse_InvalidFlags(t *testing.T) {
	t.Log("given a comment with a valid atlantis command but invalid" +
		" flags, should return a warning and the proper usage")
	cases := []struct {
		comment string
		exp     string
	}{
		{
			"atlantis plan -e",
			"Error: unknown shorthand flag: 'e' in -e",
		},
		{
			"atlantis plan --abc",
			"Error: unknown flag: --abc",
		},
		{
			"atlantis apply -e",
			"Error: unknown shorthand flag: 'e' in -e",
		},
		{
			"atlantis apply --abc",
			"Error: unknown flag: --abc",
		},
	}
	for _, c := range cases {
		r := commentParser.Parse(c.comment, models.Github)
		Assert(t, strings.Contains(r.CommentResponse, c.exp),
			"For comment %q expected CommentResponse %q to contain %q", c.comment, r.CommentResponse, c.exp)
		Assert(t, strings.Contains(r.CommentResponse, "Usage of "),
			"For comment %q expected CommentResponse %q to contain %q", c.comment, r.CommentResponse, "Usage of ")
	}
}

func TestParse_RelativeDirPath(t *testing.T) {
	t.Log("if -d is used with a relative path, should return an error")
	comments := []string{
		"atlantis plan -d ..",
		"atlantis apply -d ..",
		// These won't return an error because we prepend with . when parsing.
		//"atlantis plan -d /..",
		//"atlantis apply -d /..",
		"atlantis plan -d ./..",
		"atlantis apply -d ./..",
		"atlantis plan -d a/b/../../..",
		"atlantis apply -d a/../..",
	}
	for _, c := range comments {
		r := commentParser.Parse(c, models.Github)
		exp := "Error: using a relative path"
		Assert(t, strings.Contains(r.CommentResponse, exp),
			"For comment %q expected CommentResponse %q to contain %q", c, r.CommentResponse, exp)
	}
}

// If there's multiple lines but it's whitespace, allow the command. This
// occurs when you copy and paste via GitHub.
func TestParse_Multiline(t *testing.T) {
	comments := []string{
		"atlantis plan\n",
		"atlantis plan\n\n",
		"atlantis plan\r\n",
		"atlantis plan\r\n\r\n",
	}
	for _, comment := range comments {
		t.Run(comment, func(t *testing.T) {
			r := commentParser.Parse(comment, models.Github)
			Equals(t, "", r.CommentResponse)
			Equals(t, &events.CommentCommand{
				RepoRelDir:  "",
				Flags:       nil,
				Name:        models.PlanCommand,
				Verbose:     false,
				Workspace:   "",
				ProjectName: "",
				All:         false,
			}, r.Command)
		})
	}
}

func TestParse_InvalidWorkspace(t *testing.T) {
	t.Log("if -w is used with '..' or '/', should return an error")
	comments := []string{
		"atlantis plan -w ..",
		"atlantis apply -w ..",
		"atlantis plan -w /",
		"atlantis apply -w /",
		"atlantis plan -w ..abc",
		"atlantis apply -w abc..",
		"atlantis plan -w abc..abc",
		"atlantis apply -w ../../../etc/passwd",
	}
	for _, c := range comments {
		r := commentParser.Parse(c, models.Github)
		exp := "Error: invalid workspace"
		Assert(t, strings.Contains(r.CommentResponse, exp),
			"For comment %q expected CommentResponse %q to contain %q", c, r.CommentResponse, exp)
	}
}

func TestParse_UsingProjectAtSameTimeAsWorkspaceOrDir(t *testing.T) {
	cases := []string{
		"atlantis plan -w workspace -p project",
		"atlantis plan -d dir -p project",
		"atlantis plan -d dir -w workspace -p project",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			r := commentParser.Parse(c, models.Github)
			exp := "Error: cannot use -p/--project at same time as -d/--dir or -w/--workspace"
			Assert(t, strings.Contains(r.CommentResponse, exp),
				"For comment %q expected CommentResponse %q to contain %q", c, r.CommentResponse, exp)
		})
	}
}

func TestParse_UsingAllAtSameTimeAsProjectOrWorkspaceOrDir(t *testing.T) {
	cases := []string{
		"atlantis plan --all -d dir",
		"atlantis plan --all -p project",
		"atlantis plan --all -w workspace",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			r := commentParser.Parse(c, models.Github)
			exp := "Error: cannot use --all at same time as same time as -p/--project, -d/--dir or -w/--workspace"
			Assert(t, strings.Contains(r.CommentResponse, exp),
				"For comment %q expected CommentResponse %q to contain %q", c, r.CommentResponse, exp)
		})
	}
}

func TestParse_Parsing(t *testing.T) {
	cases := []struct {
		flags        string
		expWorkspace string
		expDir       string
		expVerbose   bool
		expExtraArgs string
		expProject   string
		expAll       bool
	}{
		// Test defaults.
		{
			"",
			"",
			"",
			false,
			"",
			"",
			false,
		},
		// Test each short flag individually.
		{
			"-w workspace",
			"workspace",
			"",
			false,
			"",
			"",
			false,
		},
		{
			"-d dir",
			"",
			"dir",
			false,
			"",
			"",
			false,
		},
		{
			"-p project",
			"",
			"",
			false,
			"",
			"project",
			false,
		},
		{
			"--verbose",
			"",
			"",
			true,
			"",
			"",
			false,
		},
		// Test each long flag individually.
		{
			"--workspace workspace",
			"workspace",
			"",
			false,
			"",
			"",
			false,
		},
		{
			"--dir dir",
			"",
			"dir",
			false,
			"",
			"",
			false,
		},
		{
			"--project project",
			"",
			"",
			false,
			"",
			"project",
			false,
		},
		// Test all of them with different permutations.
		{
			"-w workspace -d dir --verbose",
			"workspace",
			"dir",
			true,
			"",
			"",
			false,
		},
		{
			"-d dir -w workspace --verbose",
			"workspace",
			"dir",
			true,
			"",
			"",
			false,
		},
		{
			"--verbose -w workspace -d dir",
			"workspace",
			"dir",
			true,
			"",
			"",
			false,
		},
		{
			"-p project --verbose",
			"",
			"",
			true,
			"",
			"project",
			false,
		},
		{
			"--verbose -p project",
			"",
			"",
			true,
			"",
			"project",
			false,
		},
		// Test that flags after -- are ignored
		{
			"-w workspace -d dir -- --verbose",
			"workspace",
			"dir",
			false,
			"--verbose",
			"",
			false,
		},
		{
			"-w workspace -- -d dir --verbose",
			"workspace",
			"",
			false,
			"-d dir --verbose",
			"",
			false,
		},
		// Test the extra args parsing.
		{
			"--",
			"",
			"",
			false,
			"",
			"",
			false,
		},
		{
			"-w workspace -d dir --verbose -- arg one -two --three &&",
			"workspace",
			"dir",
			true,
			"arg one -two --three &&",
			"",
			false,
		},
		// Test whitespace.
		{
			"\t-w\tworkspace\t-d\tdir\t--verbose\t--\targ\tone\t-two\t--three\t&&",
			"workspace",
			"dir",
			true,
			"arg one -two --three &&",
			"",
			false,
		},
		{
			"   -w   workspace   -d   dir   --verbose   --   arg   one   -two   --three   &&",
			"workspace",
			"dir",
			true,
			"arg one -two --three &&",
			"",
			false,
		},
		// Test that the dir string is normalized.
		{
			"-d /",
			"",
			".",
			false,
			"",
			"",
			false,
		},
		{
			"-d /adir",
			"",
			"adir",
			false,
			"",
			"",
			false,
		},
		{
			"-d .",
			"",
			".",
			false,
			"",
			"",
			false,
		},
		{
			"-d ./",
			"",
			".",
			false,
			"",
			"",
			false,
		},
		{
			"-d ./adir",
			"",
			"adir",
			false,
			"",
			"",
			false,
		},
		{
			"-d \"dir with space\"",
			"",
			"dir with space",
			false,
			"",
			"",
			false,
		},
	}
	for _, test := range cases {
		for _, cmdName := range []string{"plan", "apply"} {
			comment := fmt.Sprintf("atlantis %s %s", cmdName, test.flags)
			t.Run(comment, func(t *testing.T) {
				r := commentParser.Parse(comment, models.Github)
				Assert(t, r.CommentResponse == "", "CommentResponse should have been empty but was %q for comment %q", r.CommentResponse, comment)
				Assert(t, test.expDir == r.Command.RepoRelDir, "exp dir to equal %q but was %q for comment %q", test.expDir, r.Command.RepoRelDir, comment)
				Assert(t, test.expWorkspace == r.Command.Workspace, "exp workspace to equal %q but was %q for comment %q", test.expWorkspace, r.Command.Workspace, comment)
				Assert(t, test.expVerbose == r.Command.Verbose, "exp verbose to equal %v but was %v for comment %q", test.expVerbose, r.Command.Verbose, comment)
				actExtraArgs := strings.Join(r.Command.Flags, " ")
				Assert(t, test.expExtraArgs == actExtraArgs, "exp extra args to equal %v but got %v for comment %q", test.expExtraArgs, actExtraArgs, comment)
				if cmdName == "plan" {
					Assert(t, r.Command.Name == models.PlanCommand, "did not parse comment %q as plan command", comment)
				}
				if cmdName == "apply" {
					Assert(t, r.Command.Name == models.ApplyCommand, "did not parse comment %q as apply command", comment)
				}
				Assert(t, test.expAll == r.Command.All, "exp all to equal %v but was %v for comment %q", test.expAll, r.Command.All, comment)
			})
		}
	}
}

func TestBuildPlanApplyComment(t *testing.T) {
	cases := []struct {
		repoRelDir    string
		workspace     string
		project       string
		commentArgs   []string
		expPlanFlags  string
		expApplyFlags string
	}{
		{
			repoRelDir:    ".",
			workspace:     "default",
			project:       "",
			commentArgs:   nil,
			expPlanFlags:  "-d .",
			expApplyFlags: "-d .",
		},
		{
			repoRelDir:    "dir",
			workspace:     "default",
			project:       "",
			commentArgs:   nil,
			expPlanFlags:  "-d dir",
			expApplyFlags: "-d dir",
		},
		{
			repoRelDir:    ".",
			workspace:     "workspace",
			project:       "",
			commentArgs:   nil,
			expPlanFlags:  "-w workspace",
			expApplyFlags: "-w workspace",
		},
		{
			repoRelDir:    "dir",
			workspace:     "workspace",
			project:       "",
			commentArgs:   nil,
			expPlanFlags:  "-d dir -w workspace",
			expApplyFlags: "-d dir -w workspace",
		},
		{
			repoRelDir:    ".",
			workspace:     "default",
			project:       "project",
			commentArgs:   nil,
			expPlanFlags:  "-p project",
			expApplyFlags: "-p project",
		},
		{
			repoRelDir:    "dir",
			workspace:     "workspace",
			project:       "project",
			commentArgs:   nil,
			expPlanFlags:  "-p project",
			expApplyFlags: "-p project",
		},
		{
			repoRelDir:    ".",
			workspace:     "default",
			project:       "",
			commentArgs:   []string{`"arg1"`, `"arg2"`},
			expPlanFlags:  "-d . -- arg1 arg2",
			expApplyFlags: "-d .",
		},
		{
			repoRelDir:    "dir",
			workspace:     "workspace",
			project:       "",
			commentArgs:   []string{`"arg1"`, `"arg2"`, `arg3`},
			expPlanFlags:  "-d dir -w workspace -- arg1 arg2 arg3",
			expApplyFlags: "-d dir -w workspace",
		},
		{
			repoRelDir:    "dir with spaces",
			workspace:     "default",
			project:       "",
			expPlanFlags:  "-d \"dir with spaces\"",
			expApplyFlags: "-d \"dir with spaces\"",
		},
	}

	for _, c := range cases {
		t.Run(c.expPlanFlags, func(t *testing.T) {
			for _, cmd := range []models.CommandName{models.PlanCommand, models.ApplyCommand} {
				switch cmd {
				case models.PlanCommand:
					actComment := commentParser.BuildPlanComment(c.repoRelDir, c.workspace, c.project, c.commentArgs)
					Equals(t, fmt.Sprintf("atlantis plan %s", c.expPlanFlags), actComment)
				case models.ApplyCommand:
					actComment := commentParser.BuildApplyComment(c.repoRelDir, c.workspace, c.project)
					Equals(t, fmt.Sprintf("atlantis apply %s", c.expApplyFlags), actComment)
				}
			}
		})
	}
}

func TestParse_VCSUsername(t *testing.T) {
	cp := events.CommentParser{
		GithubUser:      "gh",
		GitlabUser:      "gl",
		BitbucketUser:   "bb",
		AzureDevopsUser: "ad",
	}
	cases := []struct {
		vcs  models.VCSHostType
		user string
	}{
		{
			vcs:  models.Github,
			user: "gh",
		},
		{
			vcs:  models.Gitlab,
			user: "gl",
		},
		{
			vcs:  models.BitbucketServer,
			user: "bb",
		},
		{
			vcs:  models.BitbucketCloud,
			user: "bb",
		},
		{
			vcs:  models.AzureDevops,
			user: "ad",
		},
	}

	for _, c := range cases {
		t.Run(c.vcs.String(), func(t *testing.T) {
			r := cp.Parse(fmt.Sprintf("@%s %s", c.user, "help"), c.vcs)
			Equals(t, events.HelpComment, r.CommentResponse)
		})
	}
}

var PlanUsage = `Usage of plan:
      --all                Run plan on all projects.
  -d, --dir string         Which directory to run plan in relative to root of repo,
                           ex. 'child/dir'.
  -p, --project string     Which project to run plan for. Refers to the name of the
                           project configured in atlantis.yaml. Cannot be used at
                           same time as workspace or dir flags.
      --verbose            Append Atlantis log to comment.
  -w, --workspace string   Switch to this Terraform workspace before planning.
`

var ApplyUsage = `Usage of apply:
  -d, --dir string         Apply the plan for this directory, relative to root of
                           repo, ex. 'child/dir'.
  -p, --project string     Apply the plan for this project. Refers to the name of
                           the project configured in atlantis.yaml. Cannot be used
                           at same time as workspace or dir flags.
      --verbose            Append Atlantis log to comment.
  -w, --workspace string   Apply the plan for this Terraform workspace.
`
