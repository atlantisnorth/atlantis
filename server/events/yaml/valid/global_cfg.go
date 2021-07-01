package valid

import (
	"fmt"
	"regexp"
	"strings"

	version "github.com/hashicorp/go-version"
	"github.com/runatlantis/atlantis/server/logging"
)

const MergeableApplyReq = "mergeable"
const ApprovedApplyReq = "approved"
const UnDivergedApplyReq = "undiverged"
const PoliciesPassedApplyReq = "policies_passed"
const ApplyRequirementsKey = "apply_requirements"
const PreWorkflowHooksKey = "pre_workflow_hooks"
const WorkflowKey = "workflow"
const PostWorkflowHooksKey = "post_workflow_hooks"
const AllowedWorkflowsKey = "allowed_workflows"
const AllowedOverridesKey = "allowed_overrides"
const AllowCustomWorkflowsKey = "allow_custom_workflows"
const DefaultWorkflowName = "default"
const DeleteSourceBranchOnMergeKey = "delete_source_branch_on_merge"

// NonOverrideableApplyReqs will get applied across all "repos" in the server side config.
// If repo config is allowed overrides, they can override this.
// TODO: Make this more customizable, not everyone wants this rigid workflow
// maybe something along the lines of defining overridable/non-overrideable apply
// requirements in the config and removing the flag to enable policy checking.
var NonOverrideableApplyReqs []string = []string{PoliciesPassedApplyReq}

// GlobalCfg is the final parsed version of server-side repo config.
type GlobalCfg struct {
	Repos      []Repo
	Workflows  map[string]Workflow
	PolicySets PolicySets
}

// Repo is the final parsed version of server-side repo config.
type Repo struct {
	// ID is the exact match id of this config.
	// If IDRegex is set then this will be empty.
	ID string
	// IDRegex is the regex match for this config.
	// If ID is set then this will be nil.
	IDRegex                   *regexp.Regexp
	BranchRegex               *regexp.Regexp
	ApplyRequirements         []string
	PreWorkflowHooks          []*WorkflowHook
	Workflow                  *Workflow
	PostWorkflowHooks         []*WorkflowHook
	AllowedWorkflows          []string
	AllowedOverrides          []string
	AllowCustomWorkflows      *bool
	DeleteSourceBranchOnMerge *bool
}

type MergedProjectCfg struct {
	ApplyRequirements         []string
	Workflow                  Workflow
	AllowedWorkflows          []string
	RepoRelDir                string
	Workspace                 string
	Name                      string
	AutoplanEnabled           bool
	AutoMergeDisabled         bool
	TerraformVersion          *version.Version
	RepoCfgVersion            int
	PolicySets                PolicySets
	DeleteSourceBranchOnMerge bool
}

// WorkflowHook is a map of custom run commands to run before or after workflows.
type WorkflowHook struct {
	StepName   string
	RunCommand string
}

// DefaultApplyStage is the Atlantis default apply stage.
var DefaultApplyStage = Stage{
	Steps: []Step{
		{
			StepName: "apply",
		},
	},
}

// DefaultPolicyCheckStage is the Atlantis default policy check stage.
var DefaultPolicyCheckStage = Stage{
	Steps: []Step{
		{
			StepName: "show",
		},
		{
			StepName: "policy_check",
		},
	},
}

// DefaultPlanStage is the Atlantis default plan stage.
var DefaultPlanStage = Stage{
	Steps: []Step{
		{
			StepName: "init",
		},
		{
			StepName: "plan",
		},
	},
}

// Deprecated: use NewGlobalCfgFromArgs
func NewGlobalCfgWithHooks(allowRepoCfg bool, mergeableReq bool, approvedReq bool, unDivergedReq bool, preWorkflowHooks []*WorkflowHook, postWorkflowHooks []*WorkflowHook) GlobalCfg {
	return NewGlobalCfgFromArgs(GlobalCfgArgs{
		AllowRepoCfg:      allowRepoCfg,
		MergeableReq:      mergeableReq,
		ApprovedReq:       approvedReq,
		UnDivergedReq:     unDivergedReq,
		PreWorkflowHooks:  preWorkflowHooks,
		PostWorkflowHooks: postWorkflowHooks,
	})
}

// NewGlobalCfg returns a global config that respects the parameters.
// allowRepoCfg is true if users want to allow repos full config functionality.
// mergeableReq is true if users want to set the mergeable apply requirement
// for all repos.
// approvedReq is true if users want to set the approved apply requirement
// for all repos.
// Deprecated: use NewGlobalCfgFromArgs
func NewGlobalCfg(allowRepoCfg bool, mergeableReq bool, approvedReq bool) GlobalCfg {
	return NewGlobalCfgFromArgs(GlobalCfgArgs{
		AllowRepoCfg: allowRepoCfg,
		MergeableReq: mergeableReq,
		ApprovedReq:  approvedReq,
	})
}

type GlobalCfgArgs struct {
	AllowRepoCfg       bool
	MergeableReq       bool
	ApprovedReq        bool
	UnDivergedReq      bool
	PolicyCheckEnabled bool
	PreWorkflowHooks   []*WorkflowHook
	PostWorkflowHooks  []*WorkflowHook
}

func NewGlobalCfgFromArgs(args GlobalCfgArgs) GlobalCfg {
	defaultWorkflow := Workflow{
		Name:        DefaultWorkflowName,
		Apply:       DefaultApplyStage,
		Plan:        DefaultPlanStage,
		PolicyCheck: DefaultPolicyCheckStage,
	}
	// Must construct slices here instead of using a `var` declaration because
	// we treat nil slices differently.
	applyReqs := []string{}
	allowedOverrides := []string{}
	allowedWorkflows := []string{}
	if args.MergeableReq {
		applyReqs = append(applyReqs, MergeableApplyReq)
	}
	if args.ApprovedReq {
		applyReqs = append(applyReqs, ApprovedApplyReq)
	}
	if args.UnDivergedReq {
		applyReqs = append(applyReqs, UnDivergedApplyReq)
	}

	if args.PolicyCheckEnabled {
		applyReqs = append(applyReqs, PoliciesPassedApplyReq)
	}

	allowCustomWorkflows := false
	deleteSourceBranchOnMerge := false
	if args.AllowRepoCfg {
		allowedOverrides = []string{ApplyRequirementsKey, WorkflowKey, DeleteSourceBranchOnMergeKey}
		allowCustomWorkflows = true
	}

	return GlobalCfg{
		Repos: []Repo{
			{
				IDRegex:                   regexp.MustCompile(".*"),
				BranchRegex:               regexp.MustCompile(".*"),
				ApplyRequirements:         applyReqs,
				PreWorkflowHooks:          args.PreWorkflowHooks,
				Workflow:                  &defaultWorkflow,
				PostWorkflowHooks:         args.PostWorkflowHooks,
				AllowedWorkflows:          allowedWorkflows,
				AllowedOverrides:          allowedOverrides,
				AllowCustomWorkflows:      &allowCustomWorkflows,
				DeleteSourceBranchOnMerge: &deleteSourceBranchOnMerge,
			},
		},
		Workflows: map[string]Workflow{
			DefaultWorkflowName: defaultWorkflow,
		},
	}
}

// IDMatches returns true if the repo ID otherID matches this config.
func (r Repo) IDMatches(otherID string) bool {
	if r.ID != "" {
		return r.ID == otherID
	}
	return r.IDRegex.MatchString(otherID)
}

// BranchMatches returns true if the branch other matches a branch regex (if preset).
func (r Repo) BranchMatches(other string) bool {
	if r.BranchRegex == nil {
		return true
	}
	return r.BranchRegex.MatchString(other)
}

// IDString returns a string representation of this config.
func (r Repo) IDString() string {
	if r.ID != "" {
		return r.ID
	}
	return "/" + r.IDRegex.String() + "/"
}

// MergeProjectCfg merges proj and rCfg with the global config to return a
// final config. It assumes that all configs have been validated.
func (g GlobalCfg) MergeProjectCfg(log logging.SimpleLogging, repoID string, proj Project, rCfg RepoCfg) MergedProjectCfg {
	log.Debug("MergeProjectCfg started")
	applyReqs, workflow, allowedOverrides, allowCustomWorkflows, deleteSourceBranchOnMerge := g.getMatchingCfg(log, repoID)

	// If repos are allowed to override certain keys then override them.
	for _, key := range allowedOverrides {
		switch key {
		case ApplyRequirementsKey:
			if proj.ApplyRequirements != nil {
				log.Debug("overriding server-defined %s with repo settings: [%s]", ApplyRequirementsKey, strings.Join(proj.ApplyRequirements, ","))
				applyReqs = proj.ApplyRequirements
			}
		case WorkflowKey:
			if proj.WorkflowName != nil {
				// We iterate over the global workflows first and the repo
				// workflows second so that repo workflows override. This is
				// safe because at this point we know if a repo is allowed to
				// define its own workflow. We also know that a workflow will
				// exist with this name due to earlier validation.
				name := *proj.WorkflowName
				for k, v := range g.Workflows {
					if k == name {
						workflow = v
					}
				}
				if allowCustomWorkflows {
					for k, v := range rCfg.Workflows {
						if k == name {
							workflow = v
						}
					}
				}
				log.Debug("overriding server-defined %s with repo-specified workflow: %q", WorkflowKey, workflow.Name)
			}
		case DeleteSourceBranchOnMergeKey:
			//We check whether the server configured value and repo-root level
			//config is different. If it is then we change to the more granular.
			if rCfg.DeleteSourceBranchOnMerge != nil && deleteSourceBranchOnMerge != *rCfg.DeleteSourceBranchOnMerge {
				log.Debug("overriding server-defined %s with repo settings: [%t]", DeleteSourceBranchOnMergeKey, rCfg.DeleteSourceBranchOnMerge)
				deleteSourceBranchOnMerge = *rCfg.DeleteSourceBranchOnMerge
			}
			//Then we check whether the more granular project based config is
			//different. If it is then we set it.
			if proj.DeleteSourceBranchOnMerge != nil && deleteSourceBranchOnMerge != *proj.DeleteSourceBranchOnMerge {
				log.Debug("overriding repo-root-defined %s with repo settings: [%t]", DeleteSourceBranchOnMergeKey, *proj.DeleteSourceBranchOnMerge)
				deleteSourceBranchOnMerge = *proj.DeleteSourceBranchOnMerge
			}
			log.Debug("merged deleteSourceBranchOnMerge: [%t]", deleteSourceBranchOnMerge)
		}
		log.Debug("MergeProjectCfg completed")
	}

	log.Debug("final settings: %s: [%s], %s: %s",
		ApplyRequirementsKey, strings.Join(applyReqs, ","), WorkflowKey, workflow.Name)

	return MergedProjectCfg{
		ApplyRequirements:         applyReqs,
		Workflow:                  workflow,
		RepoRelDir:                proj.Dir,
		Workspace:                 proj.Workspace,
		Name:                      proj.GetName(),
		AutoplanEnabled:           proj.Autoplan.Enabled,
		TerraformVersion:          proj.TerraformVersion,
		RepoCfgVersion:            rCfg.Version,
		PolicySets:                g.PolicySets,
		DeleteSourceBranchOnMerge: deleteSourceBranchOnMerge,
	}
}

// DefaultProjCfg returns the default project config for all projects under the
// repo with id repoID. It is used when there is no repo config.
func (g GlobalCfg) DefaultProjCfg(log logging.SimpleLogging, repoID string, repoRelDir string, workspace string) MergedProjectCfg {
	log.Debug("building config based on server-side config")
	applyReqs, workflow, _, _, deleteSourceBranchOnMerge := g.getMatchingCfg(log, repoID)
	return MergedProjectCfg{
		ApplyRequirements:         applyReqs,
		Workflow:                  workflow,
		RepoRelDir:                repoRelDir,
		Workspace:                 workspace,
		Name:                      "",
		AutoplanEnabled:           DefaultAutoPlanEnabled,
		TerraformVersion:          nil,
		PolicySets:                g.PolicySets,
		DeleteSourceBranchOnMerge: deleteSourceBranchOnMerge,
	}
}

// ValidateRepoCfg validates that rCfg for repo with id repoID is valid based
// on our global config.
func (g GlobalCfg) ValidateRepoCfg(rCfg RepoCfg, repoID string) error {

	sliceContainsF := func(slc []string, str string) bool {
		for _, s := range slc {
			if s == str {
				return true
			}
		}
		return false
	}
	mapContainsF := func(m map[string]Workflow, key string) bool {
		for k := range m {
			if k == key {
				return true
			}
		}
		return false
	}

	// Check allowed overrides.
	var allowedOverrides []string
	for _, repo := range g.Repos {
		if repo.IDMatches(repoID) {
			if repo.AllowedOverrides != nil {
				allowedOverrides = repo.AllowedOverrides
			}
		}
	}
	for _, p := range rCfg.Projects {
		if p.WorkflowName != nil && !sliceContainsF(allowedOverrides, WorkflowKey) {
			return fmt.Errorf("repo config not allowed to set '%s' key: server-side config needs '%s: [%s]'", WorkflowKey, AllowedOverridesKey, WorkflowKey)
		}
		if p.ApplyRequirements != nil && !sliceContainsF(allowedOverrides, ApplyRequirementsKey) {
			return fmt.Errorf("repo config not allowed to set '%s' key: server-side config needs '%s: [%s]'", ApplyRequirementsKey, AllowedOverridesKey, ApplyRequirementsKey)
		}
		if p.DeleteSourceBranchOnMerge != nil && !sliceContainsF(allowedOverrides, DeleteSourceBranchOnMergeKey) {
			return fmt.Errorf("repo config not allowed to set '%s' key: server-side config needs '%s: [%s]'", DeleteSourceBranchOnMergeKey, AllowedOverridesKey, DeleteSourceBranchOnMergeKey)
		}
	}

	// Check custom workflows.
	var allowCustomWorkflows bool
	for _, repo := range g.Repos {
		if repo.IDMatches(repoID) {
			if repo.AllowCustomWorkflows != nil {
				allowCustomWorkflows = *repo.AllowCustomWorkflows
			}
		}
	}

	if len(rCfg.Workflows) > 0 && !allowCustomWorkflows {
		return fmt.Errorf("repo config not allowed to define custom workflows: server-side config needs '%s: true'", AllowCustomWorkflowsKey)
	}

	// Check if the repo has set a workflow name that doesn't exist.
	for _, p := range rCfg.Projects {
		if p.WorkflowName != nil {
			name := *p.WorkflowName
			if !mapContainsF(rCfg.Workflows, name) && !mapContainsF(g.Workflows, name) {
				return fmt.Errorf("workflow %q is not defined anywhere", name)
			}
		}
	}

	// Check workflow is allowed
	var allowedWorkflows []string
	for _, repo := range g.Repos {
		if repo.IDMatches(repoID) {

			if repo.AllowedWorkflows != nil {
				allowedWorkflows = repo.AllowedWorkflows
			}
		}
	}

	for _, p := range rCfg.Projects {
		// default is always allowed
		if p.WorkflowName != nil && len(allowedWorkflows) != 0 {
			name := *p.WorkflowName
			if allowCustomWorkflows {
				// If we allow CustomWorkflows we need to check that workflow name is defined inside repo and not global.
				if mapContainsF(rCfg.Workflows, name) {
					break
				}
			}

			if !sliceContainsF(allowedWorkflows, name) {
				return fmt.Errorf("workflow '%s' is not allowed for this repo", name)
			}
		}
	}

	return nil
}

// getMatchingCfg returns the key settings for repoID.
func (g GlobalCfg) getMatchingCfg(log logging.SimpleLogging, repoID string) (applyReqs []string, workflow Workflow, allowedOverrides []string, allowCustomWorkflows bool, deleteSourceBranchOnMerge bool) {
	toLog := make(map[string]string)
	traceF := func(repoIdx int, repoID string, key string, val interface{}) string {
		from := "default server config"
		if repoIdx > 0 {
			from = fmt.Sprintf("repos[%d], id: %s", repoIdx, repoID)
		}
		var valStr string
		switch v := val.(type) {
		case string:
			valStr = fmt.Sprintf("%q", v)
		case []string:
			valStr = fmt.Sprintf("[%s]", strings.Join(v, ","))
		case bool:
			valStr = fmt.Sprintf("%t", v)
		default:
			valStr = "this is a bug"
		}

		return fmt.Sprintf("setting %s: %s from %s", key, valStr, from)
	}

	for _, key := range []string{ApplyRequirementsKey, WorkflowKey, AllowedOverridesKey, AllowCustomWorkflowsKey, DeleteSourceBranchOnMergeKey} {
		for i, repo := range g.Repos {
			if repo.IDMatches(repoID) {
				switch key {
				case ApplyRequirementsKey:
					if repo.ApplyRequirements != nil {
						toLog[ApplyRequirementsKey] = traceF(i, repo.IDString(), ApplyRequirementsKey, repo.ApplyRequirements)
						applyReqs = repo.ApplyRequirements
					}
				case WorkflowKey:
					if repo.Workflow != nil {
						toLog[WorkflowKey] = traceF(i, repo.IDString(), WorkflowKey, repo.Workflow.Name)
						workflow = *repo.Workflow
					}
				case AllowedOverridesKey:
					if repo.AllowedOverrides != nil {
						toLog[AllowedOverridesKey] = traceF(i, repo.IDString(), AllowedOverridesKey, repo.AllowedOverrides)
						allowedOverrides = repo.AllowedOverrides
					}
				case AllowCustomWorkflowsKey:
					if repo.AllowCustomWorkflows != nil {
						toLog[AllowCustomWorkflowsKey] = traceF(i, repo.IDString(), AllowCustomWorkflowsKey, *repo.AllowCustomWorkflows)
						allowCustomWorkflows = *repo.AllowCustomWorkflows
					}
				case DeleteSourceBranchOnMergeKey:
					if repo.DeleteSourceBranchOnMerge != nil {
						toLog[DeleteSourceBranchOnMergeKey] = traceF(i, repo.IDString(), DeleteSourceBranchOnMergeKey, *repo.DeleteSourceBranchOnMerge)
						deleteSourceBranchOnMerge = *repo.DeleteSourceBranchOnMerge
					}
				}
			}
		}
	}
	for _, l := range toLog {
		log.Debug(l)
	}
	return
}
