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

	"github.com/runatlantis/atlantis/server/events"
	"github.com/runatlantis/atlantis/server/events/models"
	. "github.com/runatlantis/atlantis/testing"
)

func TestRenderErr(t *testing.T) {
	err := errors.New("err")
	cases := []struct {
		Description string
		Command     models.CommandName
		Error       error
		Expected    string
	}{
		{
			"apply error",
			models.ApplyCommand,
			err,
			"**Apply Error**\n```\nerr\n```\n",
		},
		{
			"plan error",
			models.PlanCommand,
			err,
			"**Plan Error**\n```\nerr\n```\n",
		},
		{
			"policy check error",
			models.PolicyCheckCommand,
			err,
			"**Policy Check Error**\n```\nerr\n```" +
				"\n* :heavy_check_mark: To **approve** failing policies either request an approval from approvers or address the failure by modifying the codebase.\n\n",
		},
	}

	r := events.MarkdownRenderer{}
	for _, c := range cases {
		res := events.CommandResult{
			Error: c.Error,
		}
		for _, verbose := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s_%t", c.Description, verbose), func(t *testing.T) {
				s := r.Render(res, c.Command, "log", verbose, models.Github)
				if !verbose {
					Equals(t, c.Expected, s)
				} else {
					Equals(t, c.Expected+"<details><summary>Log</summary>\n  <p>\n\n```\nlog```\n</p></details>\n", s)
				}
			})
		}
	}
}

func TestRenderFailure(t *testing.T) {
	cases := []struct {
		Description string
		Command     models.CommandName
		Failure     string
		Expected    string
	}{
		{
			"apply failure",
			models.ApplyCommand,
			"failure",
			"**Apply Failed**: failure\n",
		},
		{
			"plan failure",
			models.PlanCommand,
			"failure",
			"**Plan Failed**: failure\n",
		},
		{
			"policy check failure",
			models.PolicyCheckCommand,
			"failure",
			"**Policy Check Failed**: failure\n",
		},
	}

	r := events.MarkdownRenderer{}
	for _, c := range cases {
		res := events.CommandResult{
			Failure: c.Failure,
		}
		for _, verbose := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s_%t", c.Description, verbose), func(t *testing.T) {
				s := r.Render(res, c.Command, "log", verbose, models.Github)
				if !verbose {
					Equals(t, c.Expected, s)
				} else {
					Equals(t, c.Expected+"<details><summary>Log</summary>\n  <p>\n\n```\nlog```\n</p></details>\n", s)
				}
			})
		}
	}
}

func TestRenderErrAndFailure(t *testing.T) {
	r := events.MarkdownRenderer{}
	res := events.CommandResult{
		Error:   errors.New("error"),
		Failure: "failure",
	}
	s := r.Render(res, models.PlanCommand, "", false, models.Github)
	Equals(t, "**Plan Error**\n```\nerror\n```\n", s)
}

func TestRenderProjectResults(t *testing.T) {
	cases := []struct {
		Description    string
		Command        models.CommandName
		ProjectResults []models.ProjectResult
		VCSHost        models.VCSHostType
		Expected       string
	}{
		{
			"no projects",
			models.PlanCommand,
			[]models.ProjectResult{},
			models.Github,
			"Ran Plan for 0 projects:\n\n\n\n",
		},
		{
			"single successful plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:  "workspace",
					RepoRelDir: "path",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"single successful plan with master ahead",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						HasDiverged:     true,
					},
					Workspace:  "workspace",
					RepoRelDir: "path",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

:warning: The branch we're merging into is ahead, it is recommended to pull new commits first.

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"single successful plan with project name",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:   "workspace",
					RepoRelDir:  "path",
					ProjectName: "projectname",
				},
			},
			models.Github,
			`Ran Plan for project: $projectname$ dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"single successful policy check with project name",
			models.PolicyCheckCommand,
			[]models.ProjectResult{
				{
					PolicyCheckSuccess: &models.PolicyCheckSuccess{
						PolicyCheckOutput: "2 tests, 1 passed, 0 warnings, 0 failure, 0 exceptions",
						LockURL:           "lock-url",
						RePlanCmd:         "atlantis plan -d path -w workspace",
						ApplyCmd:          "atlantis apply -d path -w workspace",
					},
					Workspace:   "workspace",
					RepoRelDir:  "path",
					ProjectName: "projectname",
				},
			},
			models.Github,
			`Ran Policy Check for project: $projectname$ dir: $path$ workspace: $workspace$

$$$diff
2 tests, 1 passed, 0 warnings, 0 failure, 0 exceptions
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To re-run policies **plan** this project again by commenting:
    * $atlantis plan -d path -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"single successful apply",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					ApplySuccess: "success",
					Workspace:    "workspace",
					RepoRelDir:   "path",
				},
			},
			models.Github,
			`Ran Apply for dir: $path$ workspace: $workspace$

$$$diff
success
$$$

`,
		},
		{
			"single successful apply with project name",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					ApplySuccess: "success",
					Workspace:    "workspace",
					RepoRelDir:   "path",
					ProjectName:  "projectname",
				},
			},
			models.Github,
			`Ran Apply for project: $projectname$ dir: $path$ workspace: $workspace$

$$$diff
success
$$$

`,
		},
		{
			"multiple successful plans",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						RePlanCmd:       "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path2",
					ProjectName: "projectname",
					PlanSuccess: &models.PlanSuccess{
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						TerraformOutput: "terraform-output2",
						LockURL:         "lock-url2",
						ApplyCmd:        "atlantis apply -d path2 -w workspace",
						RePlanCmd:       "atlantis plan -d path2 -w workspace",
					},
				},
			},
			models.Github,
			`Ran Plan for 2 projects:

1. dir: $path$ workspace: $workspace$
1. project: $projectname$ dir: $path2$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
### 2. project: $projectname$ dir: $path2$ workspace: $workspace$
$$$diff
terraform-output2
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path2 -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url2)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path2 -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"multiple successful policy checks",
			models.PolicyCheckCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PolicyCheckSuccess: &models.PolicyCheckSuccess{
						PolicyCheckOutput: "4 tests, 4 passed, 0 warnings, 0 failures, 0 exceptions",
						LockURL:           "lock-url",
						ApplyCmd:          "atlantis apply -d path -w workspace",
						RePlanCmd:         "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path2",
					ProjectName: "projectname",
					PolicyCheckSuccess: &models.PolicyCheckSuccess{
						PolicyCheckOutput: "4 tests, 4 passed, 0 warnings, 0 failures, 0 exceptions",
						LockURL:           "lock-url2",
						ApplyCmd:          "atlantis apply -d path2 -w workspace",
						RePlanCmd:         "atlantis plan -d path2 -w workspace",
					},
				},
			},
			models.Github,
			`Ran Policy Check for 2 projects:

1. dir: $path$ workspace: $workspace$
1. project: $projectname$ dir: $path2$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
4 tests, 4 passed, 0 warnings, 0 failures, 0 exceptions
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To re-run policies **plan** this project again by commenting:
    * $atlantis plan -d path -w workspace$

---
### 2. project: $projectname$ dir: $path2$ workspace: $workspace$
$$$diff
4 tests, 4 passed, 0 warnings, 0 failures, 0 exceptions
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path2 -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url2)
* :repeat: To re-run policies **plan** this project again by commenting:
    * $atlantis plan -d path2 -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"multiple successful applies",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					RepoRelDir:   "path",
					Workspace:    "workspace",
					ProjectName:  "projectname",
					ApplySuccess: "success",
				},
				{
					RepoRelDir:   "path2",
					Workspace:    "workspace",
					ApplySuccess: "success2",
				},
			},
			models.Github,
			`Ran Apply for 2 projects:

1. project: $projectname$ dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$

### 1. project: $projectname$ dir: $path$ workspace: $workspace$
$$$diff
success
$$$

---
### 2. dir: $path2$ workspace: $workspace$
$$$diff
success2
$$$

---

`,
		},
		{
			"single errored plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Error:      errors.New("error"),
					RepoRelDir: "path",
					Workspace:  "workspace",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

**Plan Error**
$$$
error
$$$

`,
		},
		{
			"single failed plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					RepoRelDir: "path",
					Workspace:  "workspace",
					Failure:    "failure",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

**Plan Failed**: failure

`,
		},
		{
			"successful, failed, and errored plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						RePlanCmd:       "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path2",
					Failure:    "failure",
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path3",
					ProjectName: "projectname",
					Error:       errors.New("error"),
				},
			},
			models.Github,
			`Ran Plan for 3 projects:

1. dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$
1. project: $projectname$ dir: $path3$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
### 2. dir: $path2$ workspace: $workspace$
**Plan Failed**: failure

---
### 3. project: $projectname$ dir: $path3$ workspace: $workspace$
**Plan Error**
$$$
error
$$$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"successful, failed, and errored policy check",
			models.PolicyCheckCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PolicyCheckSuccess: &models.PolicyCheckSuccess{
						PolicyCheckOutput: "4 tests, 4 passed, 0 warnings, 0 failures, 0 exceptions",
						LockURL:           "lock-url",
						ApplyCmd:          "atlantis apply -d path -w workspace",
						RePlanCmd:         "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path2",
					Failure:    "failure",
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path3",
					ProjectName: "projectname",
					Error:       errors.New("error"),
				},
			},
			models.Github,
			`Ran Policy Check for 3 projects:

1. dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$
1. project: $projectname$ dir: $path3$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
4 tests, 4 passed, 0 warnings, 0 failures, 0 exceptions
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To re-run policies **plan** this project again by commenting:
    * $atlantis plan -d path -w workspace$

---
### 2. dir: $path2$ workspace: $workspace$
**Policy Check Failed**: failure

---
### 3. project: $projectname$ dir: $path3$ workspace: $workspace$
**Policy Check Error**
$$$
error
$$$
* :heavy_check_mark: To **approve** failing policies either request an approval from approvers or address the failure by modifying the codebase.


---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"successful, failed, and errored apply",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					Workspace:    "workspace",
					RepoRelDir:   "path",
					ApplySuccess: "success",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path2",
					Failure:    "failure",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path3",
					Error:      errors.New("error"),
				},
			},
			models.Github,
			`Ran Apply for 3 projects:

1. dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$
1. dir: $path3$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
success
$$$

---
### 2. dir: $path2$ workspace: $workspace$
**Apply Failed**: failure

---
### 3. dir: $path3$ workspace: $workspace$
**Apply Error**
$$$
error
$$$

---

`,
		},
		{
			"successful, failed, and errored apply",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					Workspace:    "workspace",
					RepoRelDir:   "path",
					ApplySuccess: "success",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path2",
					Failure:    "failure",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path3",
					Error:      errors.New("error"),
				},
			},
			models.Github,
			`Ran Apply for 3 projects:

1. dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$
1. dir: $path3$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
success
$$$

---
### 2. dir: $path2$ workspace: $workspace$
**Apply Failed**: failure

---
### 3. dir: $path3$ workspace: $workspace$
**Apply Error**
$$$
error
$$$

---

`,
		},
	}

	r := events.MarkdownRenderer{}
	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			res := events.CommandResult{
				ProjectResults: c.ProjectResults,
			}
			for _, verbose := range []bool{true, false} {
				t.Run(c.Description, func(t *testing.T) {
					s := r.Render(res, c.Command, "log", verbose, c.VCSHost)
					expWithBackticks := strings.Replace(c.Expected, "$", "`", -1)
					if !verbose {
						Equals(t, expWithBackticks, s)
					} else {
						Equals(t, expWithBackticks+"<details><summary>Log</summary>\n  <p>\n\n```\nlog```\n</p></details>\n", s)
					}
				})
			}
		})
	}
}

// Test that if disable apply all is set then the apply all footer is not added
func TestRenderProjectResultsDisableApplyAll(t *testing.T) {
	cases := []struct {
		Description    string
		Command        models.CommandName
		ProjectResults []models.ProjectResult
		VCSHost        models.VCSHostType
		Expected       string
	}{
		{
			"single successful plan with disable apply all set",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:  "workspace",
					RepoRelDir: "path",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$


`,
		},
		{
			"single successful plan with project name with disable apply all set",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:   "workspace",
					RepoRelDir:  "path",
					ProjectName: "projectname",
				},
			},
			models.Github,
			`Ran Plan for project: $projectname$ dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$


`,
		},
		{
			"multiple successful plans, disable apply all set",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						RePlanCmd:       "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path2",
					ProjectName: "projectname",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output2",
						ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
						LockURL:         "lock-url2",
						ApplyCmd:        "atlantis apply -d path2 -w workspace",
						RePlanCmd:       "atlantis plan -d path2 -w workspace",
					},
				},
			},
			models.Github,
			`Ran Plan for 2 projects:

1. dir: $path$ workspace: $workspace$
1. project: $projectname$ dir: $path2$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

### 2. project: $projectname$ dir: $path2$ workspace: $workspace$
$$$diff
terraform-output2
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path2 -w workspace$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url2)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path2 -w workspace$


`,
		},
	}
	r := events.MarkdownRenderer{
		DisableApplyAll: true,
	}
	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			res := events.CommandResult{
				ProjectResults: c.ProjectResults,
			}
			for _, verbose := range []bool{true, false} {
				t.Run(c.Description, func(t *testing.T) {
					s := r.Render(res, c.Command, "log", verbose, c.VCSHost)
					expWithBackticks := strings.Replace(c.Expected, "$", "`", -1)
					if !verbose {
						Equals(t, expWithBackticks, s)
					} else {
						Equals(t, expWithBackticks+"<details><summary>Log</summary>\n  <p>\n\n```\nlog```\n</p></details>\n", s)
					}
				})
			}
		})
	}
}

// Test that if disable apply is set then the apply  footer is not added
func TestRenderProjectResultsDisableApply(t *testing.T) {
	cases := []struct {
		Description    string
		Command        models.CommandName
		ProjectResults []models.ProjectResult
		VCSHost        models.VCSHostType
		Expected       string
	}{
		{
			"single successful plan with disable apply set",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:  "workspace",
					RepoRelDir: "path",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$


`,
		},
		{
			"single successful plan with project name with disable apply set",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:   "workspace",
					RepoRelDir:  "path",
					ProjectName: "projectname",
				},
			},
			models.Github,
			`Ran Plan for project: $projectname$ dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$


`,
		},
		{
			"multiple successful plans, disable apply set",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						RePlanCmd:       "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path2",
					ProjectName: "projectname",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output2",
						LockURL:         "lock-url2",
						ApplyCmd:        "atlantis apply -d path2 -w workspace",
						RePlanCmd:       "atlantis plan -d path2 -w workspace",
					},
				},
			},
			models.Github,
			`Ran Plan for 2 projects:

1. dir: $path$ workspace: $workspace$
1. project: $projectname$ dir: $path2$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
terraform-output
$$$

* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

### 2. project: $projectname$ dir: $path2$ workspace: $workspace$
$$$diff
terraform-output2
$$$

* :put_litter_in_its_place: To **delete** this plan click [here](lock-url2)
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path2 -w workspace$


`,
		},
	}
	r := events.MarkdownRenderer{
		DisableApplyAll: true,
		DisableApply:    true,
	}
	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			res := events.CommandResult{
				ProjectResults: c.ProjectResults,
			}
			for _, verbose := range []bool{true, false} {
				t.Run(c.Description, func(t *testing.T) {
					s := r.Render(res, c.Command, "log", verbose, c.VCSHost)
					expWithBackticks := strings.Replace(c.Expected, "$", "`", -1)
					if !verbose {
						Equals(t, expWithBackticks, s)
					} else {
						Equals(t, expWithBackticks+"<details><summary>Log</summary>\n  <p>\n\n```\nlog```\n</p></details>\n", s)
					}
				})
			}
		})
	}
}

// Test that if folding is disabled that it's not used.
func TestRenderProjectResults_DisableFolding(t *testing.T) {
	mr := events.MarkdownRenderer{
		DisableMarkdownFolding: true,
	}

	rendered := mr.Render(events.CommandResult{
		ProjectResults: []models.ProjectResult{
			{
				RepoRelDir: ".",
				Workspace:  "default",
				Error:      errors.New(strings.Repeat("line\n", 13)),
			},
		},
	}, models.PlanCommand, "log", false, models.Github)
	Equals(t, false, strings.Contains(rendered, "<details>"))
}

// Test that if the output is longer than 12 lines, it gets wrapped on the right
// VCS hosts during an error.
func TestRenderProjectResults_WrappedErr(t *testing.T) {
	cases := []struct {
		VCSHost                 models.VCSHostType
		GitlabCommonMarkSupport bool
		Output                  string
		ShouldWrap              bool
	}{
		{
			VCSHost:    models.Github,
			Output:     strings.Repeat("line\n", 1),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.Github,
			Output:     strings.Repeat("line\n", 13),
			ShouldWrap: true,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: false,
			Output:                  strings.Repeat("line\n", 1),
			ShouldWrap:              false,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: false,
			Output:                  strings.Repeat("line\n", 13),
			ShouldWrap:              false,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: true,
			Output:                  strings.Repeat("line\n", 1),
			ShouldWrap:              false,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: true,
			Output:                  strings.Repeat("line\n", 13),
			ShouldWrap:              true,
		},
		{
			VCSHost:    models.BitbucketCloud,
			Output:     strings.Repeat("line\n", 1),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.BitbucketCloud,
			Output:     strings.Repeat("line\n", 13),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.BitbucketServer,
			Output:     strings.Repeat("line\n", 1),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.BitbucketServer,
			Output:     strings.Repeat("line\n", 13),
			ShouldWrap: false,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s_%v", c.VCSHost.String(), c.ShouldWrap),
			func(t *testing.T) {
				mr := events.MarkdownRenderer{
					GitlabSupportsCommonMark: c.GitlabCommonMarkSupport,
				}

				rendered := mr.Render(events.CommandResult{
					ProjectResults: []models.ProjectResult{
						{
							RepoRelDir: ".",
							Workspace:  "default",
							Error:      errors.New(c.Output),
						},
					},
				}, models.PlanCommand, "log", false, c.VCSHost)
				var exp string
				if c.ShouldWrap {
					exp = `Ran Plan for dir: $.$ workspace: $default$

**Plan Error**
<details><summary>Show Output</summary>

$$$
` + c.Output + `
$$$
</details>

`
				} else {
					exp = `Ran Plan for dir: $.$ workspace: $default$

**Plan Error**
$$$
` + c.Output + `
$$$

`
				}

				expWithBackticks := strings.Replace(exp, "$", "`", -1)
				Equals(t, expWithBackticks, rendered)
			})
	}
}

// Test that if the output is longer than 12 lines, it gets wrapped on the right
// VCS hosts for a single project.
func TestRenderProjectResults_WrapSingleProject(t *testing.T) {
	cases := []struct {
		VCSHost                 models.VCSHostType
		GitlabCommonMarkSupport bool
		Output                  string
		ShouldWrap              bool
	}{
		{
			VCSHost:    models.Github,
			Output:     strings.Repeat("line\n", 1),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.Github,
			Output:     strings.Repeat("line\n", 13) + "No changes. Infrastructure is up-to-date.",
			ShouldWrap: true,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: false,
			Output:                  strings.Repeat("line\n", 1),
			ShouldWrap:              false,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: false,
			Output:                  strings.Repeat("line\n", 13),
			ShouldWrap:              false,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: true,
			Output:                  strings.Repeat("line\n", 1),
			ShouldWrap:              false,
		},
		{
			VCSHost:                 models.Gitlab,
			GitlabCommonMarkSupport: true,
			Output:                  strings.Repeat("line\n", 13) + "No changes. Infrastructure is up-to-date.",
			ShouldWrap:              true,
		},
		{
			VCSHost:    models.BitbucketCloud,
			Output:     strings.Repeat("line\n", 1),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.BitbucketCloud,
			Output:     strings.Repeat("line\n", 13),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.BitbucketServer,
			Output:     strings.Repeat("line\n", 1),
			ShouldWrap: false,
		},
		{
			VCSHost:    models.BitbucketServer,
			Output:     strings.Repeat("line\n", 13),
			ShouldWrap: false,
		},
	}

	for _, c := range cases {
		for _, cmd := range []models.CommandName{models.PlanCommand, models.ApplyCommand} {
			t.Run(fmt.Sprintf("%s_%s_%v", c.VCSHost.String(), cmd.String(), c.ShouldWrap),
				func(t *testing.T) {
					mr := events.MarkdownRenderer{
						GitlabSupportsCommonMark: c.GitlabCommonMarkSupport,
					}
					var pr models.ProjectResult
					switch cmd {
					case models.PlanCommand:
						pr = models.ProjectResult{
							RepoRelDir: ".",
							Workspace:  "default",
							PlanSuccess: &models.PlanSuccess{
								TerraformOutput: c.Output,
								ChangeSummary: "Plan: 1 to add, 2 to change, 3 to destroy.",
								LockURL:         "lock-url",
								RePlanCmd:       "replancmd",
								ApplyCmd:        "applycmd",
							},
						}
					case models.ApplyCommand:
						pr = models.ProjectResult{
							RepoRelDir:   ".",
							Workspace:    "default",
							ApplySuccess: c.Output,
						}
					}
					rendered := mr.Render(events.CommandResult{
						ProjectResults: []models.ProjectResult{pr},
					}, cmd, "log", false, c.VCSHost)

					// Check result.
					var exp string
					switch cmd {
					case models.PlanCommand:
						if c.ShouldWrap {
							exp = `Ran Plan for dir: $.$ workspace: $default$

` + "Change Summary: `Plan: 1 to add, 2 to change, 3 to destroy.`" + `
<details><summary>Show Output</summary>

$$$diff
` + c.Output + `
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $applycmd$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $replancmd$
</details>
No changes. Infrastructure is up-to-date.

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`
						} else {
							exp = `Ran Plan for dir: $.$ workspace: $default$

$$$diff
` + c.Output + `
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $applycmd$
* :put_litter_in_its_place: To **delete** this plan click [here](lock-url)
* :repeat: To **plan** this project again, comment:
    * $replancmd$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`
						}
					case models.ApplyCommand:
						if c.ShouldWrap {
							exp = `Ran Apply for dir: $.$ workspace: $default$

<details><summary>Show Output</summary>

$$$diff
` + c.Output + `
$$$
</details>

`
						} else {
							exp = `Ran Apply for dir: $.$ workspace: $default$

$$$diff
` + c.Output + `
$$$

`
						}
					}

					expWithBackticks := strings.Replace(exp, "$", "`", -1)
					Equals(t, expWithBackticks, rendered)
				})
		}
	}
}

func TestRenderProjectResults_MultiProjectApplyWrapped(t *testing.T) {
	mr := events.MarkdownRenderer{}
	tfOut := strings.Repeat("line\n", 13)
	rendered := mr.Render(events.CommandResult{
		ProjectResults: []models.ProjectResult{
			{
				RepoRelDir:   ".",
				Workspace:    "staging",
				ApplySuccess: tfOut,
			},
			{
				RepoRelDir:   ".",
				Workspace:    "production",
				ApplySuccess: tfOut,
			},
		},
	}, models.ApplyCommand, "log", false, models.Github)
	exp := `Ran Apply for 2 projects:

1. dir: $.$ workspace: $staging$
1. dir: $.$ workspace: $production$

### 1. dir: $.$ workspace: $staging$
<details><summary>Show Output</summary>

$$$diff
` + tfOut + `
$$$
</details>

---
### 2. dir: $.$ workspace: $production$
<details><summary>Show Output</summary>

$$$diff
` + tfOut + `
$$$
</details>

---

`
	expWithBackticks := strings.Replace(exp, "$", "`", -1)
	Equals(t, expWithBackticks, rendered)
}

func TestRenderProjectResults_MultiProjectPlanWrapped(t *testing.T) {
	mr := events.MarkdownRenderer{}
	tfOut := strings.Repeat("line\n", 13) + "Plan: 1 to add, 0 to change, 0 to destroy."
	rendered := mr.Render(events.CommandResult{
		ProjectResults: []models.ProjectResult{
			{
				RepoRelDir: ".",
				Workspace:  "staging",
				PlanSuccess: &models.PlanSuccess{
					TerraformOutput: tfOut,
					ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
					LockURL:         "staging-lock-url",
					ApplyCmd:        "staging-apply-cmd",
					RePlanCmd:       "staging-replan-cmd",
				},
			},
			{
				RepoRelDir: ".",
				Workspace:  "production",
				PlanSuccess: &models.PlanSuccess{
					TerraformOutput: tfOut,
					ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
					LockURL:         "production-lock-url",
					ApplyCmd:        "production-apply-cmd",
					RePlanCmd:       "production-replan-cmd",
				},
			},
		},
	}, models.PlanCommand, "log", false, models.Github)
	exp := `Ran Plan for 2 projects:

1. dir: $.$ workspace: $staging$
1. dir: $.$ workspace: $production$

### 1. dir: $.$ workspace: $staging$
` + "Change Summary: `Plan: 1 to add, 2 to change, 3 to destroy.`" + `
<details><summary>Show Output</summary>

$$$diff
` + tfOut + `
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $staging-apply-cmd$
* :put_litter_in_its_place: To **delete** this plan click [here](staging-lock-url)
* :repeat: To **plan** this project again, comment:
    * $staging-replan-cmd$
</details>
Plan: 1 to add, 0 to change, 0 to destroy.

---
### 2. dir: $.$ workspace: $production$
` + "Change Summary: `Plan: 1 to add, 2 to change, 3 to destroy.`" + `
<details><summary>Show Output</summary>

$$$diff
` + tfOut + `
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $production-apply-cmd$
* :put_litter_in_its_place: To **delete** this plan click [here](production-lock-url)
* :repeat: To **plan** this project again, comment:
    * $production-replan-cmd$
</details>
Plan: 1 to add, 0 to change, 0 to destroy.

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`
	expWithBackticks := strings.Replace(exp, "$", "`", -1)
	Equals(t, expWithBackticks, rendered)
}

// Test rendering when there was an error in one of the plans and we deleted
// all the plans as a result.
func TestRenderProjectResults_PlansDeleted(t *testing.T) {
	cases := map[string]struct {
		cr  events.CommandResult
		exp string
	}{
		"one failure": {
			cr: events.CommandResult{
				ProjectResults: []models.ProjectResult{
					{
						RepoRelDir: ".",
						Workspace:  "staging",
						Failure:    "failure",
					},
				},
				PlansDeleted: true,
			},
			exp: `Ran Plan for dir: $.$ workspace: $staging$

**Plan Failed**: failure

`,
		},
		"two failures": {
			cr: events.CommandResult{
				ProjectResults: []models.ProjectResult{
					{
						RepoRelDir: ".",
						Workspace:  "staging",
						Failure:    "failure",
					},
					{
						RepoRelDir: ".",
						Workspace:  "production",
						Failure:    "failure",
					},
				},
				PlansDeleted: true,
			},
			exp: `Ran Plan for 2 projects:

1. dir: $.$ workspace: $staging$
1. dir: $.$ workspace: $production$

### 1. dir: $.$ workspace: $staging$
**Plan Failed**: failure

---
### 2. dir: $.$ workspace: $production$
**Plan Failed**: failure

---

`,
		},
		"one failure, one success": {
			cr: events.CommandResult{
				ProjectResults: []models.ProjectResult{
					{
						RepoRelDir: ".",
						Workspace:  "staging",
						Failure:    "failure",
					},
					{
						RepoRelDir: ".",
						Workspace:  "production",
						PlanSuccess: &models.PlanSuccess{
							TerraformOutput: "tf out",
							ChangeSummary:   "Plan: 1 to add, 2 to change, 3 to destroy.",
							LockURL:         "lock-url",
							RePlanCmd:       "re-plan cmd",
							ApplyCmd:        "apply cmd",
						},
					},
				},
				PlansDeleted: true,
			},
			exp: `Ran Plan for 2 projects:

1. dir: $.$ workspace: $staging$
1. dir: $.$ workspace: $production$

### 1. dir: $.$ workspace: $staging$
**Plan Failed**: failure

---
### 2. dir: $.$ workspace: $production$
$$$diff
tf out
$$$

This plan was not saved because one or more projects failed and automerge requires all plans pass.

---

`,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			mr := events.MarkdownRenderer{}
			rendered := mr.Render(c.cr, models.PlanCommand, "log", false, models.Github)
			expWithBackticks := strings.Replace(c.exp, "$", "`", -1)
			Equals(t, expWithBackticks, rendered)
		})
	}
}

// test that id repo locking is disabled the link to unlock the project is not rendered
func TestRenderProjectResultsWithRepoLockingDisabled(t *testing.T) {
	cases := []struct {
		Description    string
		Command        models.CommandName
		ProjectResults []models.ProjectResult
		VCSHost        models.VCSHostType
		Expected       string
	}{
		{
			"no projects",
			models.PlanCommand,
			[]models.ProjectResult{},
			models.Github,
			"Ran Plan for 0 projects:\n\n\n\n",
		},
		{
			"single successful plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:  "workspace",
					RepoRelDir: "path",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"single successful plan with master ahead",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						HasDiverged:     true,
					},
					Workspace:  "workspace",
					RepoRelDir: "path",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

:warning: The branch we're merging into is ahead, it is recommended to pull new commits first.

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"single successful plan with project name",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						RePlanCmd:       "atlantis plan -d path -w workspace",
						ApplyCmd:        "atlantis apply -d path -w workspace",
					},
					Workspace:   "workspace",
					RepoRelDir:  "path",
					ProjectName: "projectname",
				},
			},
			models.Github,
			`Ran Plan for project: $projectname$ dir: $path$ workspace: $workspace$

$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"single successful apply",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					ApplySuccess: "success",
					Workspace:    "workspace",
					RepoRelDir:   "path",
				},
			},
			models.Github,
			`Ran Apply for dir: $path$ workspace: $workspace$

$$$diff
success
$$$

`,
		},
		{
			"single successful apply with project name",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					ApplySuccess: "success",
					Workspace:    "workspace",
					RepoRelDir:   "path",
					ProjectName:  "projectname",
				},
			},
			models.Github,
			`Ran Apply for project: $projectname$ dir: $path$ workspace: $workspace$

$$$diff
success
$$$

`,
		},
		{
			"multiple successful plans",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						RePlanCmd:       "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path2",
					ProjectName: "projectname",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output2",
						LockURL:         "lock-url2",
						ApplyCmd:        "atlantis apply -d path2 -w workspace",
						RePlanCmd:       "atlantis plan -d path2 -w workspace",
					},
				},
			},
			models.Github,
			`Ran Plan for 2 projects:

1. dir: $path$ workspace: $workspace$
1. project: $projectname$ dir: $path2$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
### 2. project: $projectname$ dir: $path2$ workspace: $workspace$
$$$diff
terraform-output2
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path2 -w workspace$
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path2 -w workspace$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"multiple successful applies",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					RepoRelDir:   "path",
					Workspace:    "workspace",
					ProjectName:  "projectname",
					ApplySuccess: "success",
				},
				{
					RepoRelDir:   "path2",
					Workspace:    "workspace",
					ApplySuccess: "success2",
				},
			},
			models.Github,
			`Ran Apply for 2 projects:

1. project: $projectname$ dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$

### 1. project: $projectname$ dir: $path$ workspace: $workspace$
$$$diff
success
$$$

---
### 2. dir: $path2$ workspace: $workspace$
$$$diff
success2
$$$

---

`,
		},
		{
			"single errored plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Error:      errors.New("error"),
					RepoRelDir: "path",
					Workspace:  "workspace",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

**Plan Error**
$$$
error
$$$

`,
		},
		{
			"single failed plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					RepoRelDir: "path",
					Workspace:  "workspace",
					Failure:    "failure",
				},
			},
			models.Github,
			`Ran Plan for dir: $path$ workspace: $workspace$

**Plan Failed**: failure

`,
		},
		{
			"successful, failed, and errored plan",
			models.PlanCommand,
			[]models.ProjectResult{
				{
					Workspace:  "workspace",
					RepoRelDir: "path",
					PlanSuccess: &models.PlanSuccess{
						TerraformOutput: "terraform-output",
						LockURL:         "lock-url",
						ApplyCmd:        "atlantis apply -d path -w workspace",
						RePlanCmd:       "atlantis plan -d path -w workspace",
					},
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path2",
					Failure:    "failure",
				},
				{
					Workspace:   "workspace",
					RepoRelDir:  "path3",
					ProjectName: "projectname",
					Error:       errors.New("error"),
				},
			},
			models.Github,
			`Ran Plan for 3 projects:

1. dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$
1. project: $projectname$ dir: $path3$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
terraform-output
$$$

* :arrow_forward: To **apply** this plan, comment:
    * $atlantis apply -d path -w workspace$
* :repeat: To **plan** this project again, comment:
    * $atlantis plan -d path -w workspace$

---
### 2. dir: $path2$ workspace: $workspace$
**Plan Failed**: failure

---
### 3. project: $projectname$ dir: $path3$ workspace: $workspace$
**Plan Error**
$$$
error
$$$

---
* :fast_forward: To **apply** all unapplied plans from this pull request, comment:
    * $atlantis apply$
* :put_litter_in_its_place: To delete all plans and locks for the PR, comment:
    * $atlantis unlock$
`,
		},
		{
			"successful, failed, and errored apply",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					Workspace:    "workspace",
					RepoRelDir:   "path",
					ApplySuccess: "success",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path2",
					Failure:    "failure",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path3",
					Error:      errors.New("error"),
				},
			},
			models.Github,
			`Ran Apply for 3 projects:

1. dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$
1. dir: $path3$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
success
$$$

---
### 2. dir: $path2$ workspace: $workspace$
**Apply Failed**: failure

---
### 3. dir: $path3$ workspace: $workspace$
**Apply Error**
$$$
error
$$$

---

`,
		},
		{
			"successful, failed, and errored apply",
			models.ApplyCommand,
			[]models.ProjectResult{
				{
					Workspace:    "workspace",
					RepoRelDir:   "path",
					ApplySuccess: "success",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path2",
					Failure:    "failure",
				},
				{
					Workspace:  "workspace",
					RepoRelDir: "path3",
					Error:      errors.New("error"),
				},
			},
			models.Github,
			`Ran Apply for 3 projects:

1. dir: $path$ workspace: $workspace$
1. dir: $path2$ workspace: $workspace$
1. dir: $path3$ workspace: $workspace$

### 1. dir: $path$ workspace: $workspace$
$$$diff
success
$$$

---
### 2. dir: $path2$ workspace: $workspace$
**Apply Failed**: failure

---
### 3. dir: $path3$ workspace: $workspace$
**Apply Error**
$$$
error
$$$

---

`,
		},
	}

	r := events.MarkdownRenderer{}
	r.DisableRepoLocking = true
	for _, c := range cases {
		t.Run(c.Description, func(t *testing.T) {
			res := events.CommandResult{
				ProjectResults: c.ProjectResults,
			}
			for _, verbose := range []bool{true, false} {
				t.Run(c.Description, func(t *testing.T) {
					s := r.Render(res, c.Command, "log", verbose, c.VCSHost)
					expWithBackticks := strings.Replace(c.Expected, "$", "`", -1)
					if !verbose {
						Equals(t, expWithBackticks, s)
					} else {
						Equals(t, expWithBackticks+"<details><summary>Log</summary>\n  <p>\n\n```\nlog```\n</p></details>\n", s)
					}
				})
			}
		})
	}
}
