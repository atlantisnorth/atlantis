package events_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/petergtz/pegomock"
	"github.com/runatlantis/atlantis/server/events"
	"github.com/runatlantis/atlantis/server/events/matchers"
	"github.com/runatlantis/atlantis/server/events/mocks"
	"github.com/runatlantis/atlantis/server/events/models"
	vcsmocks "github.com/runatlantis/atlantis/server/events/vcs/mocks"
	"github.com/runatlantis/atlantis/server/events/yaml"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

func TestDefaultProjectCommandBuilder_BuildAutoplanCommands(t *testing.T) {
	// expCtxFields define the ctx fields we're going to assert on.
	// Since we're focused on autoplanning here, we don't validate all the
	// fields so the tests are more obvious and targeted.
	type expCtxFields struct {
		ProjectName string
		RepoRelDir  string
		Workspace   string
	}
	cases := []struct {
		Description    string
		AtlantisYAML   string
		ServerSideYAML string
		exp            []expCtxFields
	}{
		{
			Description: "simple atlantis.yaml",
			AtlantisYAML: `
version: 3
projects:
- dir: .
`,
			exp: []expCtxFields{
				{
					ProjectName: "",
					RepoRelDir:  ".",
					Workspace:   "default",
				},
			},
		},
		{
			Description: "some projects disabled",
			AtlantisYAML: `
version: 3
projects:
- dir: .
  autoplan:
    enabled: false
- dir: .
  workspace: myworkspace
  autoplan:
    when_modified: ["main.tf"]
- dir: .
  name: myname
  workspace: myworkspace2
`,
			exp: []expCtxFields{
				{
					ProjectName: "",
					RepoRelDir:  ".",
					Workspace:   "myworkspace",
				},
				{
					ProjectName: "myname",
					RepoRelDir:  ".",
					Workspace:   "myworkspace2",
				},
			},
		},
		{
			Description: "some projects disabled",
			AtlantisYAML: `
version: 3
projects:
- dir: .
  autoplan:
    enabled: false
- dir: .
  workspace: myworkspace
  autoplan:
    when_modified: ["main.tf"]
- dir: .
  workspace: myworkspace2
`,
			exp: []expCtxFields{
				{
					ProjectName: "",
					RepoRelDir:  ".",
					Workspace:   "myworkspace",
				},
				{
					ProjectName: "",
					RepoRelDir:  ".",
					Workspace:   "myworkspace2",
				},
			},
		},
		{
			Description: "no projects modified",
			AtlantisYAML: `
version: 3
projects:
- dir: mydir
`,
			exp: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			RegisterMockTestingT(t)
			tmpDir, cleanup := DirStructure(t, map[string]interface{}{
				"main.tf": nil,
			})
			defer cleanup()

			workingDir := mocks.NewMockWorkingDir()
			When(workingDir.Clone(matchers.AnyPtrToLoggingSimpleLogger(), matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())).ThenReturn(tmpDir, false, nil)
			vcsClient := vcsmocks.NewMockClient()
			When(vcsClient.GetModifiedFiles(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn([]string{"main.tf"}, nil)
			if c.AtlantisYAML != "" {
				err := ioutil.WriteFile(filepath.Join(tmpDir, yaml.AtlantisYAMLFilename), []byte(c.AtlantisYAML), 0600)
				Ok(t, err)
			}

			builder := &events.DefaultProjectCommandBuilder{
				WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
				WorkingDir:         workingDir,
				ParserValidator:    &yaml.ParserValidator{},
				VCSClient:          vcsClient,
				ProjectFinder:      &events.DefaultProjectFinder{},
				PendingPlanFinder:  &events.DefaultPendingPlanFinder{},
				CommentBuilder:     &events.CommentParser{},
				GlobalCfg:          valid.NewGlobalCfg(false, false, false),
				SkipCloneNoChanges: false,
			}

			ctxs, err := builder.BuildAutoplanCommands(&events.CommandContext{
				PullMergeable: true,
			})
			Ok(t, err)
			Equals(t, len(c.exp), len(ctxs))
			for i, actCtx := range ctxs {
				expCtx := c.exp[i]
				Equals(t, expCtx.ProjectName, actCtx.ProjectName)
				Equals(t, expCtx.RepoRelDir, actCtx.RepoRelDir)
				Equals(t, expCtx.Workspace, actCtx.Workspace)
			}
		})
	}
}

// Test building a plan and apply command for one project.
func TestDefaultProjectCommandBuilder_BuildSinglePlanApplyCommand(t *testing.T) {
	cases := []struct {
		Description    string
		AtlantisYAML   string
		Cmd            events.CommentCommand
		ExpCommentArgs []string
		ExpWorkspace   string
		ExpDir         string
		ExpProjectName string
		ExpErr         string
		ExpApplyReqs   []string
	}{
		{
			Description: "no atlantis.yaml",
			Cmd: events.CommentCommand{
				RepoRelDir: ".",
				Flags:      []string{"commentarg"},
				Name:       models.PlanCommand,
				Workspace:  "myworkspace",
			},
			AtlantisYAML:   "",
			ExpCommentArgs: []string{`\c\o\m\m\e\n\t\a\r\g`},
			ExpWorkspace:   "myworkspace",
			ExpDir:         ".",
			ExpApplyReqs:   []string{},
		},
		{
			Description: "no atlantis.yaml with project flag",
			Cmd: events.CommentCommand{
				RepoRelDir:  ".",
				Name:        models.PlanCommand,
				ProjectName: "myproject",
			},
			AtlantisYAML: "",
			ExpErr:       "cannot specify a project name unless an atlantis.yaml file exists to configure projects",
		},
		{
			Description: "simple atlantis.yaml",
			Cmd: events.CommentCommand{
				RepoRelDir: ".",
				Name:       models.PlanCommand,
				Workspace:  "myworkspace",
			},
			AtlantisYAML: `
version: 3
projects:
- dir: .
  workspace: myworkspace
  apply_requirements: [approved]`,
			ExpApplyReqs: []string{"approved"},
			ExpWorkspace: "myworkspace",
			ExpDir:       ".",
		},
		{
			Description: "atlantis.yaml wrong dir",
			Cmd: events.CommentCommand{
				RepoRelDir: ".",
				Name:       models.PlanCommand,
				Workspace:  "myworkspace",
			},
			AtlantisYAML: `
version: 3
projects:
- dir: notroot
  workspace: myworkspace
  apply_requirements: [approved]`,
			ExpWorkspace: "myworkspace",
			ExpDir:       ".",
			ExpApplyReqs: []string{},
		},
		{
			Description: "atlantis.yaml wrong workspace",
			Cmd: events.CommentCommand{
				RepoRelDir: ".",
				Name:       models.PlanCommand,
				Workspace:  "myworkspace",
			},
			AtlantisYAML: `
version: 3
projects:
- dir: .
  workspace: notmyworkspace
  apply_requirements: [approved]`,
			ExpErr: "running commands in workspace \"myworkspace\" is not allowed because this directory is only configured for the following workspaces: notmyworkspace",
		},
		{
			Description: "atlantis.yaml with projectname",
			Cmd: events.CommentCommand{
				Name:        models.PlanCommand,
				ProjectName: "myproject",
			},
			AtlantisYAML: `
version: 3
projects:
- name: myproject
  dir: .
  workspace: myworkspace
  apply_requirements: [approved]`,
			ExpApplyReqs:   []string{"approved"},
			ExpProjectName: "myproject",
			ExpWorkspace:   "myworkspace",
			ExpDir:         ".",
		},
		{
			Description: "atlantis.yaml with mergeable apply requirement",
			Cmd: events.CommentCommand{
				Name:        models.PlanCommand,
				ProjectName: "myproject",
			},
			AtlantisYAML: `
version: 3
projects:
- name: myproject
  dir: .
  workspace: myworkspace
  apply_requirements: [mergeable]`,
			ExpApplyReqs:   []string{"mergeable"},
			ExpProjectName: "myproject",
			ExpWorkspace:   "myworkspace",
			ExpDir:         ".",
		},
		{
			Description: "atlantis.yaml with mergeable and approved apply requirements",
			Cmd: events.CommentCommand{
				Name:        models.PlanCommand,
				ProjectName: "myproject",
			},
			AtlantisYAML: `
version: 3
projects:
- name: myproject
  dir: .
  workspace: myworkspace
  apply_requirements: [mergeable, approved]`,
			ExpApplyReqs:   []string{"mergeable", "approved"},
			ExpProjectName: "myproject",
			ExpWorkspace:   "myworkspace",
			ExpDir:         ".",
		},
		{
			Description: "atlantis.yaml with multiple dir/workspaces matching",
			Cmd: events.CommentCommand{
				Name:       models.PlanCommand,
				RepoRelDir: ".",
				Workspace:  "myworkspace",
			},
			AtlantisYAML: `
version: 3
projects:
- name: myproject
  dir: .
  workspace: myworkspace
  apply_requirements: [approved]
- name: myproject2
  dir: .
  workspace: myworkspace
`,
			ExpErr: "must specify project name: more than one project defined in atlantis.yaml matched dir: \".\" workspace: \"myworkspace\"",
		},
		{
			Description: "atlantis.yaml with project flag not matching",
			Cmd: events.CommentCommand{
				Name:        models.PlanCommand,
				RepoRelDir:  ".",
				Workspace:   "default",
				ProjectName: "notconfigured",
			},
			AtlantisYAML: `
version: 3
projects:
- dir: .
`,
			ExpErr: "no project with name \"notconfigured\" is defined in atlantis.yaml",
		},
	}

	for _, c := range cases {
		// NOTE: we're testing both plan and apply here.
		for _, cmdName := range []models.CommandName{models.PlanCommand, models.ApplyCommand} {
			t.Run(c.Description+"_"+cmdName.String(), func(t *testing.T) {
				RegisterMockTestingT(t)
				tmpDir, cleanup := DirStructure(t, map[string]interface{}{
					"main.tf": nil,
				})
				defer cleanup()

				workingDir := mocks.NewMockWorkingDir()
				When(workingDir.Clone(matchers.AnyPtrToLoggingSimpleLogger(), matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())).ThenReturn(tmpDir, false, nil)
				When(workingDir.GetWorkingDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())).ThenReturn(tmpDir, nil)
				vcsClient := vcsmocks.NewMockClient()
				When(vcsClient.GetModifiedFiles(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn([]string{"main.tf"}, nil)
				if c.AtlantisYAML != "" {
					err := ioutil.WriteFile(filepath.Join(tmpDir, yaml.AtlantisYAMLFilename), []byte(c.AtlantisYAML), 0600)
					Ok(t, err)
				}

				builder := &events.DefaultProjectCommandBuilder{
					WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
					WorkingDir:         workingDir,
					ParserValidator:    &yaml.ParserValidator{},
					VCSClient:          vcsClient,
					ProjectFinder:      &events.DefaultProjectFinder{},
					CommentBuilder:     &events.CommentParser{},
					GlobalCfg:          valid.NewGlobalCfg(true, false, false),
					SkipCloneNoChanges: false,
				}

				var actCtxs []models.ProjectCommandContext
				var err error
				if cmdName == models.PlanCommand {
					actCtxs, err = builder.BuildPlanCommands(&events.CommandContext{}, &c.Cmd)
				} else {
					actCtxs, err = builder.BuildApplyCommands(&events.CommandContext{}, &c.Cmd)
				}

				if c.ExpErr != "" {
					ErrEquals(t, c.ExpErr, err)
					return
				}
				Ok(t, err)
				Equals(t, 1, len(actCtxs))
				actCtx := actCtxs[0]
				Equals(t, c.ExpDir, actCtx.RepoRelDir)
				Equals(t, c.ExpWorkspace, actCtx.Workspace)
				Equals(t, c.ExpCommentArgs, actCtx.EscapedCommentArgs)
				Equals(t, c.ExpProjectName, actCtx.ProjectName)
				Equals(t, c.ExpApplyReqs, actCtx.ApplyRequirements)
			})
		}
	}
}

func TestDefaultProjectCommandBuilder_BuildPlanCommands(t *testing.T) {
	// expCtxFields define the ctx fields we're going to assert on.
	// Since we're focused on autoplanning here, we don't validate all the
	// fields so the tests are more obvious and targeted.
	type expCtxFields struct {
		ProjectName string
		RepoRelDir  string
		Workspace   string
	}
	cases := map[string]struct {
		DirStructure  map[string]interface{}
		AtlantisYAML  string
		ModifiedFiles []string
		Exp           []expCtxFields
	}{
		"no atlantis.yaml": {
			DirStructure: map[string]interface{}{
				"project1": map[string]interface{}{
					"main.tf": nil,
				},
				"project2": map[string]interface{}{
					"main.tf": nil,
				},
			},
			ModifiedFiles: []string{"project1/main.tf", "project2/main.tf"},
			Exp: []expCtxFields{
				{
					ProjectName: "",
					RepoRelDir:  "project1",
					Workspace:   "default",
				},
				{
					ProjectName: "",
					RepoRelDir:  "project2",
					Workspace:   "default",
				},
			},
		},
		"no modified files": {
			DirStructure: map[string]interface{}{
				"main.tf": nil,
			},
			ModifiedFiles: []string{},
			Exp:           []expCtxFields{},
		},
		"follow when_modified config": {
			DirStructure: map[string]interface{}{
				"project1": map[string]interface{}{
					"main.tf": nil,
				},
				"project2": map[string]interface{}{
					"main.tf": nil,
				},
				"project3": map[string]interface{}{
					"main.tf": nil,
				},
			},
			AtlantisYAML: `version: 3
projects:
- dir: project1 # project1 uses the defaults
- dir: project2 # project2 has autoplan disabled but should use default when_modified
  autoplan:
    enabled: false
- dir: project3 # project3 has an empty when_modified
  autoplan:
    enabled: false
    when_modified: []`,
			ModifiedFiles: []string{"project1/main.tf", "project2/main.tf", "project3/main.tf"},
			Exp: []expCtxFields{
				{
					ProjectName: "",
					RepoRelDir:  "project1",
					Workspace:   "default",
				},
				{
					ProjectName: "",
					RepoRelDir:  "project2",
					Workspace:   "default",
				},
			},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			RegisterMockTestingT(t)
			tmpDir, cleanup := DirStructure(t, c.DirStructure)
			defer cleanup()

			workingDir := mocks.NewMockWorkingDir()
			When(workingDir.Clone(matchers.AnyPtrToLoggingSimpleLogger(), matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())).ThenReturn(tmpDir, false, nil)
			When(workingDir.GetWorkingDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())).ThenReturn(tmpDir, nil)
			vcsClient := vcsmocks.NewMockClient()
			When(vcsClient.GetModifiedFiles(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn(c.ModifiedFiles, nil)
			if c.AtlantisYAML != "" {
				err := ioutil.WriteFile(filepath.Join(tmpDir, yaml.AtlantisYAMLFilename), []byte(c.AtlantisYAML), 0600)
				Ok(t, err)
			}

			builder := &events.DefaultProjectCommandBuilder{
				WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
				WorkingDir:         workingDir,
				ParserValidator:    &yaml.ParserValidator{},
				VCSClient:          vcsClient,
				ProjectFinder:      &events.DefaultProjectFinder{},
				CommentBuilder:     &events.CommentParser{},
				GlobalCfg:          valid.NewGlobalCfg(true, false, false),
				SkipCloneNoChanges: false,
			}

			ctxs, err := builder.BuildPlanCommands(
				&events.CommandContext{},
				&events.CommentCommand{
					RepoRelDir:  "",
					Flags:       nil,
					Name:        models.PlanCommand,
					Verbose:     false,
					Workspace:   "",
					ProjectName: "",
				})
			Ok(t, err)
			Equals(t, len(c.Exp), len(ctxs))
			for i, actCtx := range ctxs {
				expCtx := c.Exp[i]
				Equals(t, expCtx.ProjectName, actCtx.ProjectName)
				Equals(t, expCtx.RepoRelDir, actCtx.RepoRelDir)
				Equals(t, expCtx.Workspace, actCtx.Workspace)
			}
		})
	}
}

// Test building apply command for multiple projects when the comment
// isn't for a specific project, i.e. atlantis apply.
// In this case we should apply all outstanding plans.
func TestDefaultProjectCommandBuilder_BuildMultiApply(t *testing.T) {
	RegisterMockTestingT(t)
	tmpDir, cleanup := DirStructure(t, map[string]interface{}{
		"workspace1": map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf":          nil,
				"workspace.tfplan": nil,
			},
			"project2": map[string]interface{}{
				"main.tf":          nil,
				"workspace.tfplan": nil,
			},
		},
		"workspace2": map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf":          nil,
				"workspace.tfplan": nil,
			},
			"project2": map[string]interface{}{
				"main.tf":          nil,
				"workspace.tfplan": nil,
			},
		},
	})
	defer cleanup()
	// Initialize git repos in each workspace so that the .tfplan files get
	// picked up.
	runCmd(t, filepath.Join(tmpDir, "workspace1"), "git", "init")
	runCmd(t, filepath.Join(tmpDir, "workspace2"), "git", "init")

	workingDir := mocks.NewMockWorkingDir()
	When(workingDir.GetPullDir(
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest())).
		ThenReturn(tmpDir, nil)

	builder := &events.DefaultProjectCommandBuilder{
		WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
		WorkingDir:         workingDir,
		ParserValidator:    &yaml.ParserValidator{},
		VCSClient:          nil,
		ProjectFinder:      &events.DefaultProjectFinder{},
		PendingPlanFinder:  &events.DefaultPendingPlanFinder{},
		CommentBuilder:     &events.CommentParser{},
		GlobalCfg:          valid.NewGlobalCfg(false, false, false),
		SkipCloneNoChanges: false,
	}

	ctxs, err := builder.BuildApplyCommands(
		&events.CommandContext{},
		&events.CommentCommand{
			RepoRelDir:  "",
			Flags:       nil,
			Name:        models.ApplyCommand,
			Verbose:     false,
			Workspace:   "",
			ProjectName: "",
		})
	Ok(t, err)
	Equals(t, 4, len(ctxs))
	Equals(t, "project1", ctxs[0].RepoRelDir)
	Equals(t, "workspace1", ctxs[0].Workspace)
	Equals(t, "project2", ctxs[1].RepoRelDir)
	Equals(t, "workspace1", ctxs[1].Workspace)
	Equals(t, "project1", ctxs[2].RepoRelDir)
	Equals(t, "workspace2", ctxs[2].Workspace)
	Equals(t, "project2", ctxs[3].RepoRelDir)
	Equals(t, "workspace2", ctxs[3].Workspace)
}

// Test that if a directory has a list of workspaces configured then we don't
// allow plans for other workspace names.
func TestDefaultProjectCommandBuilder_WrongWorkspaceName(t *testing.T) {
	RegisterMockTestingT(t)
	workingDir := mocks.NewMockWorkingDir()

	tmpDir, cleanup := DirStructure(t, map[string]interface{}{
		"pulldir": map[string]interface{}{
			"notconfigured": map[string]interface{}{},
		},
	})
	defer cleanup()
	repoDir := filepath.Join(tmpDir, "pulldir/notconfigured")

	yamlCfg := `version: 3
projects:
- dir: .
  workspace: default
- dir: .
  workspace: staging
`
	err := ioutil.WriteFile(filepath.Join(repoDir, yaml.AtlantisYAMLFilename), []byte(yamlCfg), 0600)
	Ok(t, err)

	When(workingDir.Clone(
		matchers.AnyPtrToLoggingSimpleLogger(),
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest(),
		AnyString())).ThenReturn(repoDir, false, nil)
	When(workingDir.GetWorkingDir(
		matchers.AnyModelsRepo(),
		matchers.AnyModelsPullRequest(),
		AnyString())).ThenReturn(repoDir, nil)

	builder := &events.DefaultProjectCommandBuilder{
		WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
		WorkingDir:         workingDir,
		ParserValidator:    &yaml.ParserValidator{},
		VCSClient:          nil,
		ProjectFinder:      &events.DefaultProjectFinder{},
		CommentBuilder:     &events.CommentParser{},
		GlobalCfg:          valid.NewGlobalCfg(true, false, false),
		SkipCloneNoChanges: false,
	}

	ctx := &events.CommandContext{
		HeadRepo: models.Repo{},
		Pull:     models.PullRequest{},
		User:     models.User{},
		Log:      logging.NewNoopLogger(),
	}
	_, err = builder.BuildPlanCommands(ctx, &events.CommentCommand{
		RepoRelDir:  ".",
		Flags:       nil,
		Name:        models.PlanCommand,
		Verbose:     false,
		Workspace:   "notconfigured",
		ProjectName: "",
	})
	ErrEquals(t, "running commands in workspace \"notconfigured\" is not allowed because this directory is only configured for the following workspaces: default, staging", err)
}

// Test that extra comment args are escaped.
func TestDefaultProjectCommandBuilder_EscapeArgs(t *testing.T) {
	cases := []struct {
		ExtraArgs      []string
		ExpEscapedArgs []string
	}{
		{
			ExtraArgs:      []string{"arg1", "arg2"},
			ExpEscapedArgs: []string{`\a\r\g\1`, `\a\r\g\2`},
		},
		{
			ExtraArgs:      []string{"-var=$(touch bad)"},
			ExpEscapedArgs: []string{`\-\v\a\r\=\$\(\t\o\u\c\h\ \b\a\d\)`},
		},
		{
			ExtraArgs:      []string{"-- ;echo bad"},
			ExpEscapedArgs: []string{`\-\-\ \;\e\c\h\o\ \b\a\d`},
		},
	}

	for _, c := range cases {
		t.Run(strings.Join(c.ExtraArgs, " "), func(t *testing.T) {
			RegisterMockTestingT(t)
			tmpDir, cleanup := DirStructure(t, map[string]interface{}{
				"main.tf": nil,
			})
			defer cleanup()

			workingDir := mocks.NewMockWorkingDir()
			When(workingDir.Clone(matchers.AnyPtrToLoggingSimpleLogger(), matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())).ThenReturn(tmpDir, false, nil)
			When(workingDir.GetWorkingDir(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())).ThenReturn(tmpDir, nil)
			vcsClient := vcsmocks.NewMockClient()
			When(vcsClient.GetModifiedFiles(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn([]string{"main.tf"}, nil)

			builder := &events.DefaultProjectCommandBuilder{
				WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
				WorkingDir:         workingDir,
				ParserValidator:    &yaml.ParserValidator{},
				VCSClient:          vcsClient,
				ProjectFinder:      &events.DefaultProjectFinder{},
				CommentBuilder:     &events.CommentParser{},
				GlobalCfg:          valid.NewGlobalCfg(true, false, false),
				SkipCloneNoChanges: false,
			}

			var actCtxs []models.ProjectCommandContext
			var err error
			actCtxs, err = builder.BuildPlanCommands(&events.CommandContext{}, &events.CommentCommand{
				RepoRelDir: ".",
				Flags:      c.ExtraArgs,
				Name:       models.PlanCommand,
				Verbose:    false,
				Workspace:  "default",
			})
			Ok(t, err)
			Equals(t, 1, len(actCtxs))
			actCtx := actCtxs[0]
			Equals(t, c.ExpEscapedArgs, actCtx.EscapedCommentArgs)
		})
	}
}

// Test that terraform version is used when specified in terraform configuration
func TestDefaultProjectCommandBuilder_TerraformVersionSpecified(t *testing.T) {
	// For the following tests:
	// If terraform configuration is used, result should be `0.12.8`.
	// If project configuration is used, result should be `0.12.6`.
	// If default is to be used, result should be `nil`.

	tfReleases := []string{"0.12.9", "0.12.8", "0.12.6"}

	baseVersionConfig := `
terraform {
  required_version = "%s0.12.8"
}
`

	atlantisYamlContent := `
version: 3
projects:
- dir: project1 # project1 uses the defaults
  terraform_version: v0.12.6
`

	exactSymbols := []string{"", "="}

	type testCase struct {
		DirStructure  map[string]interface{}
		ModifiedFiles []string
		Exp           map[string][]int
	}

	testCases := make(map[string]testCase)

	for _, exactSymbol := range exactSymbols {
		testCases[fmt.Sprintf("exact version in terraform config using \"%s\"", exactSymbol)] = testCase{
			DirStructure: map[string]interface{}{
				"project1": map[string]interface{}{
					"main.tf": fmt.Sprintf(baseVersionConfig, exactSymbol),
				},
			},
			ModifiedFiles: []string{"project1/main.tf"},
			Exp: map[string][]int{
				"project1": {0, 12, 8},
			},
		}
	}

	// atlantis.yaml should take precedence over terraform config
	testCases["with project config and terraform config"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": fmt.Sprintf(baseVersionConfig, exactSymbols[0]),
			},
			yaml.AtlantisYAMLFilename: atlantisYamlContent,
		},
		ModifiedFiles: []string{"project1/main.tf", "project2/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 12, 6},
		},
	}

	testCases["with project config only"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": nil,
			},
			yaml.AtlantisYAMLFilename: atlantisYamlContent,
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 12, 6},
		},
	}

	testCases["neither project config or terraform config"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": nil,
			},
		},
		ModifiedFiles: []string{"project1/main.tf", "project2/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
	}

	testCases["project with different terraform config"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": fmt.Sprintf(baseVersionConfig, exactSymbols[0]),
			},
			"project2": map[string]interface{}{
				"main.tf": strings.Replace(fmt.Sprintf(baseVersionConfig, exactSymbols[0]), "0.12.8", "0.12.9", -1),
			},
		},
		ModifiedFiles: []string{"project1/main.tf", "project2/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 12, 8},
			"project2": {0, 12, 9},
		},
	}

	testCases["project with invalid terraform config"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = asdflkj;
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			RegisterMockTestingT(t)

			tmpDir, cleanup := DirStructure(t, testCase.DirStructure)

			defer cleanup()
			vcsClient := vcsmocks.NewMockClient()
			When(vcsClient.GetModifiedFiles(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn(testCase.ModifiedFiles, nil)

			workingDir := mocks.NewMockWorkingDir()
			When(workingDir.Clone(
				matchers.AnyPtrToLoggingSimpleLogger(),
				matchers.AnyModelsRepo(),
				matchers.AnyModelsPullRequest(),
				AnyString())).ThenReturn(tmpDir, false, nil)

			When(workingDir.GetWorkingDir(
				matchers.AnyModelsRepo(),
				matchers.AnyModelsPullRequest(),
				AnyString())).ThenReturn(tmpDir, nil)

			rl := mocks.NewMockReleasesLister()
			When(rl.ListReleases()).ThenReturn(tfReleases, nil)

			builder := &events.DefaultProjectCommandBuilder{
				WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
				WorkingDir:         workingDir,
				VCSClient:          vcsClient,
				ParserValidator:    &yaml.ParserValidator{},
				ProjectFinder:      &events.DefaultProjectFinder{},
				CommentBuilder:     &events.CommentParser{},
				GlobalCfg:          valid.NewGlobalCfg(true, false, false),
				SkipCloneNoChanges: false,
				ReleasesLister:     rl,
			}

			actCtxs, err := builder.BuildPlanCommands(
				&events.CommandContext{},
				&events.CommentCommand{
					RepoRelDir: "",
					Flags:      nil,
					Name:       models.PlanCommand,
					Verbose:    false,
				})

			Ok(t, err)
			Equals(t, len(testCase.Exp), len(actCtxs))
			for _, actCtx := range actCtxs {
				if testCase.Exp[actCtx.RepoRelDir] != nil {
					Assert(t, actCtx.TerraformVersion != nil, "TerraformVersion is nil.")
					Equals(t, testCase.Exp[actCtx.RepoRelDir], actCtx.TerraformVersion.Segments())
				} else {
					Assert(t, actCtx.TerraformVersion == nil, "TerraformVersion is supposed to be nil, got .", actCtx.TerraformVersion)
				}
			}
		})
	}
}

// Test that a correct terraform version is used when a constraint is specified in the terraform configuration
func TestDefaultProjectCommandBuilder_TerraformVersionConstraints(t *testing.T) {

	tfReleases := []string{"0.14.0-rc1", "0.14.0-beta2", "0.14.0-beta1", "0.14.0-alpha20201007", "0.14.0-alpha20200923", "0.14.0-alpha20200910", "0.13.5", "0.13.4", "0.13.3", "0.13.2", "0.13.1", "0.13.0", "0.13.0-rc1", "0.13.0-beta3", "0.13.0-beta2", "0.13.0-beta1", "0.12.29", "0.12.28", "0.12.27", "0.12.26", "0.12.25", "0.12.24", "0.12.23", "0.12.22", "0.12.21", "0.12.20", "0.12.19", "0.12.18", "0.12.17", "0.12.16", "0.12.15", "0.12.14", "0.12.13", "0.12.12", "0.12.11", "0.12.10", "0.12.9", "0.12.8", "0.12.7", "0.12.6", "0.12.5", "0.12.4", "0.12.3", "0.12.2", "0.12.1", "0.12.0", "0.12.0-rc1", "0.12.0-beta2", "0.12.0-beta1", "0.12.0-alpha4", "0.12.0-alpha3", "0.12.0-alpha2", "0.12.0-alpha1", "0.11.15-oci", "0.11.14", "0.11.13", "0.11.12", "0.11.12-beta1", "0.11.11", "0.11.10", "0.11.9", "0.11.9-beta1", "0.11.8", "0.11.7", "0.11.6", "0.11.5", "0.11.4", "0.11.3", "0.11.2", "0.11.1", "0.11.0", "0.11.0-rc1", "0.11.0-beta1", "0.10.8", "0.10.7", "0.10.6", "0.10.5", "0.10.4", "0.10.3", "0.10.2", "0.10.1", "0.10.0", "0.10.0-rc1", "0.10.0-beta2", "0.10.0-beta1", "0.9.11", "0.9.10", "0.9.9", "0.9.8", "0.9.7", "0.9.6", "0.9.5", "0.9.4", "0.9.3", "0.9.2", "0.9.1", "0.9.0", "0.8.8", "0.8.7", "0.8.6", "0.8.5", "0.8.4", "0.8.3", "0.8.2", "0.8.1", "0.8.0", "0.7.13", "0.7.12", "0.7.11", "0.7.10", "0.7.9", "0.7.8", "0.7.7", "0.7.6", "0.7.5", "0.7.4", "0.7.3", "0.7.2", "0.7.1", "0.7.0", "0.6.16", "0.6.15", "0.6.14", "0.6.13", "0.6.12", "0.6.11", "0.6.10", "0.6.9", "0.6.8", "0.6.7", "0.6.6", "0.6.5", "0.6.4", "0.6.3", "0.6.2", "0.6.1", "0.6.0", "0.5.3", "0.5.1", "0.5.0", "0.4.2", "0.4.1", "0.4.0", "0.3.7", "0.3.6", "0.3.5", "0.3.1", "0.3.0", "0.2.2", "0.2.1", "0.2.0", "0.1.1", "0.1.0"}

	defaultRL := mocks.NewMockReleasesLister()
	When(defaultRL.ListReleases()).ThenReturn(tfReleases, nil)

	listingFailRL := mocks.NewMockReleasesLister()
	When(listingFailRL.ListReleases()).ThenReturn(nil, fmt.Errorf("mock error"))

	badListRL := mocks.NewMockReleasesLister()
	When(badListRL.ListReleases()).ThenReturn([]string{"0.14.0-rc1", "0.14.0-beta2", "0.14.0-beta1", "0.14.0-alpha20201007", "asdf"}, nil)

	type testCase struct {
		DirStructure  map[string]interface{}
		ModifiedFiles []string
		Exp           map[string][]int
		ReleaseLister *mocks.MockReleasesLister
	}

	testCases := make(map[string]testCase)

	testCases["version constraint in terraform config using <"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<0.13.0"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 12, 29},
		},
		ReleaseLister: defaultRL,
	}

	testCases["unsatisfiable version constraint in terraform config using <"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<0.1.0"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: defaultRL,
	}

	testCases["version constraint in terraform config using <="] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<=0.14.0"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 13, 5},
		},
		ReleaseLister: defaultRL,
	}

	testCases["match exact version constraint in terraform config using <="] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<=0.13.0"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 13, 0},
		},
		ReleaseLister: defaultRL,
	}

	testCases["unsatisfiable version constraint in terraform config using <="] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<=0.0.9"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: defaultRL,
	}

	testCases["version constraint in terraform config using >"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = ">0.11.0"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 13, 5},
		},
		ReleaseLister: defaultRL,
	}

	testCases["unsatisfiable version constraint in terraform config using >"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = ">0.13.5"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: defaultRL,
	}

	testCases["version constraint in terraform config using >="] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = ">=0.13.0"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 13, 5},
		},
		ReleaseLister: defaultRL,
	}

	testCases["match exact version constraint in terraform config using >="] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = ">=0.13.5"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 13, 5},
		},
		ReleaseLister: defaultRL,
	}

	testCases["unsatisfiable version constraint in terraform config using >="] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = ">=0.13.6"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: defaultRL,
	}

	testCases["pessimistic version constraint in terraform config for patches"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "~>0.12.5"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 12, 29},
		},
		ReleaseLister: defaultRL,
	}

	testCases["pessimistic version constraint in terraform config for minors"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "~>0.12"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 13, 5},
		},
		ReleaseLister: defaultRL,
	}

	testCases["invalid constraint"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "><0.13"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: defaultRL,
	}

	testCases["multiple constraints"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = ">0.12, <0.13"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": {0, 12, 29},
		},
		ReleaseLister: defaultRL,
	}

	testCases["unsatisfiable multiple constraints"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<0.12, >0.13"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: defaultRL,
	}

	testCases["fail to list releases"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<0.12, >0.13"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: listingFailRL,
	}

	testCases["invalid list of releases"] = testCase{
		DirStructure: map[string]interface{}{
			"project1": map[string]interface{}{
				"main.tf": `
				terraform {
				  required_version = "<0.12, >0.13"
				}`,
			},
		},
		ModifiedFiles: []string{"project1/main.tf"},
		Exp: map[string][]int{
			"project1": nil,
		},
		ReleaseLister: badListRL,
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			RegisterMockTestingT(t)

			tmpDir, cleanup := DirStructure(t, testCase.DirStructure)

			defer cleanup()
			vcsClient := vcsmocks.NewMockClient()
			When(vcsClient.GetModifiedFiles(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn(testCase.ModifiedFiles, nil)

			workingDir := mocks.NewMockWorkingDir()
			When(workingDir.Clone(
				matchers.AnyPtrToLoggingSimpleLogger(),
				matchers.AnyModelsRepo(),
				matchers.AnyModelsPullRequest(),
				AnyString())).ThenReturn(tmpDir, false, nil)

			When(workingDir.GetWorkingDir(
				matchers.AnyModelsRepo(),
				matchers.AnyModelsPullRequest(),
				AnyString())).ThenReturn(tmpDir, nil)

			builder := &events.DefaultProjectCommandBuilder{
				WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
				WorkingDir:         workingDir,
				VCSClient:          vcsClient,
				ParserValidator:    &yaml.ParserValidator{},
				ProjectFinder:      &events.DefaultProjectFinder{},
				CommentBuilder:     &events.CommentParser{},
				GlobalCfg:          valid.NewGlobalCfg(true, false, false),
				SkipCloneNoChanges: false,
				ReleasesLister:     testCase.ReleaseLister,
			}

			actCtxs, err := builder.BuildPlanCommands(
				&events.CommandContext{},
				&events.CommentCommand{
					RepoRelDir: "",
					Flags:      nil,
					Name:       models.PlanCommand,
					Verbose:    false,
				})

			Ok(t, err)
			Equals(t, len(testCase.Exp), len(actCtxs))
			for _, actCtx := range actCtxs {
				if testCase.Exp[actCtx.RepoRelDir] != nil {
					Assert(t, actCtx.TerraformVersion != nil, "TerraformVersion is nil.")
					Equals(t, testCase.Exp[actCtx.RepoRelDir], actCtx.TerraformVersion.Segments())
				} else {
					Assert(t, actCtx.TerraformVersion == nil, "TerraformVersion is supposed to be nil, got %s.", actCtx.TerraformVersion)
				}
			}
		})
	}
}

// Test that we don't clone the repo if there were no changes based on the atlantis.yaml file.
func TestDefaultProjectCommandBuilder_SkipCloneNoChanges(t *testing.T) {
	atlantisYAML := `
version: 3
projects:
- dir: dir1`

	RegisterMockTestingT(t)
	vcsClient := vcsmocks.NewMockClient()
	When(vcsClient.GetModifiedFiles(matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest())).ThenReturn([]string{"main.tf"}, nil)
	When(vcsClient.SupportsSingleFileDownload(matchers.AnyModelsRepo())).ThenReturn(true)
	When(vcsClient.DownloadRepoConfigFile(matchers.AnyModelsPullRequest())).ThenReturn(true, []byte(atlantisYAML), nil)
	workingDir := mocks.NewMockWorkingDir()

	builder := &events.DefaultProjectCommandBuilder{
		WorkingDirLocker:   events.NewDefaultWorkingDirLocker(),
		WorkingDir:         workingDir,
		ParserValidator:    &yaml.ParserValidator{},
		VCSClient:          vcsClient,
		ProjectFinder:      &events.DefaultProjectFinder{},
		CommentBuilder:     &events.CommentParser{},
		GlobalCfg:          valid.NewGlobalCfg(true, false, false),
		SkipCloneNoChanges: true,
	}

	var actCtxs []models.ProjectCommandContext
	var err error
	actCtxs, err = builder.BuildAutoplanCommands(&events.CommandContext{
		HeadRepo:      models.Repo{},
		Pull:          models.PullRequest{},
		User:          models.User{},
		Log:           nil,
		PullMergeable: true,
	})
	Ok(t, err)
	Equals(t, 0, len(actCtxs))
	workingDir.VerifyWasCalled(Never()).Clone(matchers.AnyPtrToLoggingSimpleLogger(), matchers.AnyModelsRepo(), matchers.AnyModelsPullRequest(), AnyString())
}
