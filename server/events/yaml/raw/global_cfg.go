package raw

import (
	"fmt"
	"regexp"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
)

// GlobalCfg is the raw schema for server-side repo config.
type GlobalCfg struct {
	Repos      []Repo              `yaml:"repos" json:"repos"`
	Workflows  map[string]Workflow `yaml:"workflows" json:"workflows"`
	PolicySets PolicySets          `yaml:"policies" json:"policies"`
}

// Repo is the raw schema for repos in the server-side repo config.
type Repo struct {
	ID                        string         `yaml:"id" json:"id"`
	Branch                    string         `yaml:"branch" json:"branch"`
	ApplyRequirements         []string       `yaml:"apply_requirements" json:"apply_requirements"`
	PreWorkflowHooks          []WorkflowHook `yaml:"pre_workflow_hooks" json:"pre_workflow_hooks"`
	Workflow                  *string        `yaml:"workflow,omitempty" json:"workflow,omitempty"`
	PostWorkflowHooks         []WorkflowHook `yaml:"post_workflow_hooks" json:"post_workflow_hooks"`
	AllowedWorkflows          []string       `yaml:"allowed_workflows,omitempty" json:"allowed_workflows,omitempty"`
	AllowedOverrides          []string       `yaml:"allowed_overrides" json:"allowed_overrides"`
	AllowCustomWorkflows      *bool          `yaml:"allow_custom_workflows,omitempty" json:"allow_custom_workflows,omitempty"`
	DeleteSourceBranchOnMerge *bool          `yaml:"delete_source_branch_on_merge,omitempty" json:"delete_source_branch_on_merge,omitempty"`
}

func (g GlobalCfg) Validate() error {
	err := validation.ValidateStruct(&g,
		validation.Field(&g.Repos),
		validation.Field(&g.Workflows))
	if err != nil {
		return err
	}

	// Check that all workflows referenced by repos are actually defined.
	for _, repo := range g.Repos {
		if repo.Workflow == nil {
			continue
		}
		name := *repo.Workflow
		if name == valid.DefaultWorkflowName {
			// The 'default' workflow will always be defined.
			continue
		}
		found := false
		for w := range g.Workflows {
			if w == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("workflow %q is not defined", name)
		}
	}

	// Check that all allowed workflows are defined
	for _, repo := range g.Repos {
		if repo.AllowedWorkflows == nil {
			continue
		}
		for _, name := range repo.AllowedWorkflows {
			if name == valid.DefaultWorkflowName {
				// The 'default' workflow will always be defined.
				continue
			}
			found := false
			for w := range g.Workflows {
				if w == name {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("workflow %q is not defined", name)
			}
		}
	}
	return nil
}

func (g GlobalCfg) ToValid(defaultCfg valid.GlobalCfg) valid.GlobalCfg {
	workflows := make(map[string]valid.Workflow)

	// assumes: globalcfg is always initialized with one repo .*
	applyReqs := defaultCfg.Repos[0].ApplyRequirements

	var globalApplyReqs []string

	for _, req := range applyReqs {
		for _, nonOverrideableReq := range valid.NonOverrideableApplyReqs {
			if req == nonOverrideableReq {
				globalApplyReqs = append(globalApplyReqs, req)
			}
		}
	}

	for k, v := range g.Workflows {
		validatedWorkflow := v.ToValid(k)
		workflows[k] = validatedWorkflow
		if k == valid.DefaultWorkflowName {
			// Handle the special case where they're redefining the default
			// workflow. In this case, our default repo config references
			// the "old" default workflow and so needs to be redefined.
			defaultCfg.Repos[0].Workflow = &validatedWorkflow
		}
	}
	// Merge in defaults without overriding.
	for k, v := range defaultCfg.Workflows {
		if _, ok := workflows[k]; !ok {
			workflows[k] = v
		}
	}

	var repos []valid.Repo
	for _, r := range g.Repos {
		repos = append(repos, r.ToValid(workflows, globalApplyReqs))
	}
	repos = append(defaultCfg.Repos, repos...)

	return valid.GlobalCfg{
		Repos:      repos,
		Workflows:  workflows,
		PolicySets: g.PolicySets.ToValid(),
	}
}

// HasRegexID returns true if r is configured with a regex id instead of an
// exact match id.
func (r Repo) HasRegexID() bool {
	return strings.HasPrefix(r.ID, "/") && strings.HasSuffix(r.ID, "/")
}

// HasRegexBranch returns true if a branch regex was set.
func (r Repo) HasRegexBranch() bool {
	return strings.HasPrefix(r.Branch, "/") && strings.HasSuffix(r.Branch, "/")
}

func (r Repo) Validate() error {
	idValid := func(value interface{}) error {
		id := value.(string)
		if !r.HasRegexID() {
			return nil
		}
		_, err := regexp.Compile(id[1 : len(id)-1])
		return errors.Wrapf(err, "parsing: %s", id)
	}

	branchValid := func(value interface{}) error {
		branch := value.(string)
		if !r.HasRegexBranch() {
			return nil
		}
		_, err := regexp.Compile(branch[1 : len(branch)-1])
		return errors.Wrapf(err, "parsing: %s", branch)
	}

	overridesValid := func(value interface{}) error {
		overrides := value.([]string)
		for _, o := range overrides {
			if o != valid.ApplyRequirementsKey && o != valid.WorkflowKey && o != valid.DeleteSourceBranchOnMergeKey {
				return fmt.Errorf("%q is not a valid override, only %q, %q and %q are supported", o, valid.ApplyRequirementsKey, valid.WorkflowKey, valid.DeleteSourceBranchOnMergeKey)
			}
		}
		return nil
	}

	workflowExists := func(value interface{}) error {
		// We validate workflows in ParserValidator.validateRepoWorkflows
		// because we need the list of workflows to validate.
		return nil
	}

	deleteSourceBranchOnMergeValid := func(value interface{}) error {
		//TOBE IMPLEMENTED
		return nil
	}

	return validation.ValidateStruct(&r,
		validation.Field(&r.ID, validation.Required, validation.By(idValid)),
		validation.Field(&r.Branch, validation.By(branchValid)),
		validation.Field(&r.AllowedOverrides, validation.By(overridesValid)),
		validation.Field(&r.ApplyRequirements, validation.By(validApplyReq)),
		validation.Field(&r.Workflow, validation.By(workflowExists)),
		validation.Field(&r.DeleteSourceBranchOnMerge, validation.By(deleteSourceBranchOnMergeValid)),
	)
}

func (r Repo) ToValid(workflows map[string]valid.Workflow, globalApplyReqs []string) valid.Repo {
	var id string
	var idRegex *regexp.Regexp
	if r.HasRegexID() {
		withoutSlashes := r.ID[1 : len(r.ID)-1]
		// Safe to use MustCompile because we test it in Validate().
		idRegex = regexp.MustCompile(withoutSlashes)
	} else {
		id = r.ID
	}

	var branchRegex *regexp.Regexp
	if r.HasRegexBranch() {
		withoutSlashes := r.Branch[1 : len(r.Branch)-1]
		// Safe to use MustCompile because we test it in Validate().
		branchRegex = regexp.MustCompile(withoutSlashes)
	}

	var workflow *valid.Workflow
	if r.Workflow != nil {
		// This key is guaranteed to exist because we test for it in
		// ParserValidator.validateRepoWorkflows.
		ptr := workflows[*r.Workflow]
		workflow = &ptr
	}

	var preWorkflowHooks []*valid.WorkflowHook
	if len(r.PreWorkflowHooks) > 0 {
		for _, hook := range r.PreWorkflowHooks {
			preWorkflowHooks = append(preWorkflowHooks, hook.ToValid())
		}
	}

	var postWorkflowHooks []*valid.WorkflowHook
	if len(r.PostWorkflowHooks) > 0 {
		for _, hook := range r.PostWorkflowHooks {
			postWorkflowHooks = append(postWorkflowHooks, hook.ToValid())
		}
	}

	var mergedApplyReqs []string

	mergedApplyReqs = append(mergedApplyReqs, r.ApplyRequirements...)

	// only add global reqs if they don't exist already.
OUTER:
	for _, globalReq := range globalApplyReqs {
		for _, currReq := range r.ApplyRequirements {
			if globalReq == currReq {
				continue OUTER
			}
		}
		mergedApplyReqs = append(mergedApplyReqs, globalReq)
	}

	return valid.Repo{
		ID:                        id,
		IDRegex:                   idRegex,
		BranchRegex:               branchRegex,
		ApplyRequirements:         mergedApplyReqs,
		PreWorkflowHooks:          preWorkflowHooks,
		Workflow:                  workflow,
		PostWorkflowHooks:         postWorkflowHooks,
		AllowedWorkflows:          r.AllowedWorkflows,
		AllowedOverrides:          r.AllowedOverrides,
		AllowCustomWorkflows:      r.AllowCustomWorkflows,
		DeleteSourceBranchOnMerge: r.DeleteSourceBranchOnMerge,
	}
}
