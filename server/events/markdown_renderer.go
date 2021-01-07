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
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/Masterminds/sprig/v3"
	"github.com/runatlantis/atlantis/server/events/models"
)

const (
	planCommandTitle  = "Plan"
	applyCommandTitle = "Apply"
)

// MarkdownRenderer renders responses as markdown.
type MarkdownRenderer struct {
	// GitlabSupportsCommonMark is true if the version of GitLab we're
	// using supports the CommonMark markdown format.
	// If we're not configured with a GitLab client, this will be false.
	GitlabSupportsCommonMark bool
	DisableApplyAll          bool
	DisableApply             bool
	DisableMarkdownFolding   bool
}

// commonData is data that all responses have.
type commonData struct {
	Command         string
	Verbose         bool
	Log             string
	PlansDeleted    bool
	DisableApplyAll bool
	DisableApply    bool
}

// errData is data about an error response.
type errData struct {
	Error string
	commonData
}

// failureData is data about a failure response.
type failureData struct {
	Failure string
	commonData
}

// resultData is data about a successful response.
type resultData struct {
	Results  []projectResultTmplData
	Overview string
	commonData
}

type planSuccessData struct {
	models.PlanSuccess
	Summary        string
	PlanWasDeleted bool
	DisableApply   bool
}

type projectResultTmplData struct {
	Workspace   string
	RepoRelDir  string
	ProjectName string
	Rendered    string
}

type idxProjectResult struct {
	models.ProjectResult
	Index int
}

type projectPlanSummary struct {
	FailedProjects  []idxProjectResult
	ErroredProjects []idxProjectResult
	DestroyProjects []idxProjectResult
	CreateProjects  []idxProjectResult
	ReplaceProjects []idxProjectResult
	ModifyProjects  []idxProjectResult
	NoopProjects    []idxProjectResult
}

func summarizePlanResults(results []models.ProjectResult) projectPlanSummary {
	sum := projectPlanSummary{
		FailedProjects:  []idxProjectResult{},
		ErroredProjects: []idxProjectResult{},
		DestroyProjects: []idxProjectResult{},
		CreateProjects:  []idxProjectResult{},
		ReplaceProjects: []idxProjectResult{},
		ModifyProjects:  []idxProjectResult{},
		NoopProjects:    []idxProjectResult{},
	}
	for idx, result := range results {
		idxResult := idxProjectResult{
			result,
			idx,
		}
		if result.Error != nil {
			sum.ErroredProjects = append(sum.ErroredProjects, idxResult)
		} else if result.Failure != "" {
			sum.FailedProjects = append(sum.FailedProjects, idxResult)
		} else if result.PlanSuccess != nil {
			cnt := countResources(result.PlanSuccess.TerraformOutput)
			if cnt.Destroy > 0 {
				sum.DestroyProjects = append(sum.DestroyProjects, idxResult)
			}
			if cnt.Create > 0 {
				sum.CreateProjects = append(sum.CreateProjects, idxResult)
			}
			if cnt.Replace > 0 {
				sum.ReplaceProjects = append(sum.ReplaceProjects, idxResult)
			}
			if cnt.Modify > 0 {
				sum.ModifyProjects = append(sum.ModifyProjects, idxResult)
			}
			if cnt.Noop() {
				sum.NoopProjects = append(sum.NoopProjects, idxResult)
			}
		}
	}
	return sum
}

func renderPlanSummarySection(section string, results []idxProjectResult) string {
	if len(results) == 0 {
		return ""
	}
	plural := "s"
	if len(results) == 1 {
		plural = ""
		section = strings.ReplaceAll(section, "have", "has")
	}
	header := fmt.Sprintf("* %d plan%s %s:\n", len(results), plural, section)
	indent := "    * "
	lines := []string{}
	for _, result := range results {
		line := fmt.Sprintf("%s_%d.", indent, result.Index+1)
		if result.ProjectName != "" {
			line += fmt.Sprintf(" project: `%s`", result.ProjectName)
		}
		// If ProjectName and Workspace are both unset, make
		// sure we are going to at least print RepoRelDir so
		// we don't end up with an empty string.
		if result.RepoRelDir != "." || (result.ProjectName == "" && result.Workspace == "default") {
			line += fmt.Sprintf(" dir: `%s`", result.RepoRelDir)
		}
		if result.Workspace != "default" {
			line += fmt.Sprintf(" workspace: `%s`", result.Workspace)
		}
		line += "_\n"
		lines = append(lines, line)
	}
	return header + strings.Join(lines, "")
}

func (sum *projectPlanSummary) render() string {
	return strings.Join([]string{
		renderPlanSummarySection("have **failed**", sum.ErroredProjects),
		renderPlanSummarySection("have **Atlantis errors**", sum.FailedProjects),
		renderPlanSummarySection("to **destroy resources**", sum.DestroyProjects),
		renderPlanSummarySection("to **create resources**", sum.CreateProjects),
		renderPlanSummarySection("to **replace resources**", sum.ReplaceProjects),
		renderPlanSummarySection("to **modify resources**", sum.ModifyProjects),
		renderPlanSummarySection("with **no changes** (not shown below)", sum.NoopProjects),
	}, "")
}

type resourceCount struct {
	Destroy      int
	Create       int
	Replace      int
	Modify       int
	ModifyDelete int
	ModifyAdd    int
	ModifyChange int
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

var diffMarkerRegexp = regexp.MustCompile(`(?m)^( *)([-+~]|-/\+) `)

func countResources(output string) resourceCount {
	cnt := resourceCount{
		// Subtract 1 from each match count to account for the
		// symbol legend that Terraform prints at the top. The
		// counts will get incremented for each appearance of
		// the symbol. There will be one occurrence in the
		// legend, and then each other occurrence is for real.
		// By starting at -1, we will end up with a correct
		// count.
		Destroy:      -1,
		Create:       -1,
		Replace:      -1,
		Modify:       -1,
		ModifyDelete: 0,
		ModifyAdd:    0,
		ModifyChange: 0,
	}
	// We'll process lines that have a diff indicator (either +,
	// -, ~, or -/+) one at a time. This boolean may be toggled
	// when we get to a new top-level diff indicator (introducing
	// a new resource), since we'll only want to change
	// ModifyDelete and friends when we're within a top-level
	// resource with indicator ~.
	withinModifiedResource := false
	// Keep track of the last diff indicator line we looked at
	// (partly... we only update these variables sometimes). It's
	// needed to track state like this because avoiding
	// double-counting means keying off the relative indentation
	// of adjacent lines.
	lastIndent := 0
	lastMarker := ""
	for _, match := range diffMarkerRegexp.FindAllStringSubmatch(output, -1) {
		// Regex groups: the whitespace before the diff
		// indicator, and the diff indicator itself.
		indent := len(match[1])
		marker := match[2]
		// Top-level indicators are at the left margin, and
		// indicate a new resource being introduced.
		topLevel := indent == 0
		// Is this diff indicator more indented than the last
		// one we looked at? If so, we assume it corresponds
		// to a property that's nested under the one on the
		// last line we looked at.
		moreIndented := indent > lastIndent
		if topLevel {
			// This variable should be set to true only
			// for ~.
			withinModifiedResource = false
			switch marker {
			case "+":
				cnt.Create += 1
			case "-":
				cnt.Destroy += 1
			case "-/+":
				cnt.Replace += 1
			case "~":
				cnt.Modify += 1
				// The very first time we see a ~ it
				// is in the symbol legend, so we
				// shouldn't count it as introducing a
				// resource.
				if cnt.Modify > 0 {
					withinModifiedResource = true
				}
			}
		} else if withinModifiedResource {
			// Normally if we're not at the top level, and
			// we're inside a ~ resource, then we'll want
			// to count +, -, and ~ indicators as counting
			// toward the total number of sub-properties
			// added, deleted, and changed. However, there
			// are two exceptions to avoid
			// double-counting.
			//
			// First exception: if a property is marked as
			// ~, but it also has sub-properties, then we
			// should not count the ~. Instead we should
			// count the individual properties that are
			// added, deleted, or changed. The leaf nodes,
			// if you will. We could do this with
			// lookahead, but instead we just fix it after
			// the fact: if we realize the last property
			// we looked at was a ~ that we shouldn't have
			// counted, then we just
			if moreIndented && lastMarker == "~" {
				cnt.ModifyChange -= 1
			}
			// Second exception: if a property is marked
			// as + (added), then Terraform also marks all
			// sub-properties as +. However, really this
			// is only one property being added, so we
			// should only count a single +. For that
			// reason, if the current + is nested under
			// the previous indicator which was also a +,
			// then we will skip counting the current +.
			// Also we will skip updating the lastIndent
			// and lastMarker variables, so that the
			// lastIndent and lastMarker continue to point
			// at the + indicator that introduced the
			// entire property being added. That will
			// prevent us from starting to double-count
			// again halfway through the resource, if we
			// recurse back upwards from a
			// sub-sub-property. Same for - as for +.
			// However, for ~, we actually want to count
			// it and update the lastIndent and lastMarker
			// variables unconditionally, because nested ~
			// indicators don't have the same meaning as
			// nested + and - indicators. If the increment
			// is wrong, it'll be undone by the decrement
			// above.
			if !(moreIndented && marker == lastMarker) || marker == "~" {
				switch marker {
				case "+":
					cnt.ModifyAdd += 1
				case "-":
					cnt.ModifyDelete += 1
				case "~":
					cnt.ModifyChange += 1
				}
				lastIndent = indent
				lastMarker = marker
			}
		} else {
			// Not sure this is needed, but seems good for
			// cleanliness.
			lastIndent = 0
			lastMarker = ""
		}
	}
	// In case of a Terraform diff that does not include a
	// particular match, we'll end up with a value of -1 which we
	// should adjust to 0. (Terraform only includes a symbol in
	// the legend if it also appears at least once in the diff at
	// the top level.)
	cnt.Destroy = max(0, cnt.Destroy)
	cnt.Create = max(0, cnt.Create)
	cnt.Replace = max(0, cnt.Replace)
	cnt.Modify = max(0, cnt.Modify)
	return cnt
}

func capitalize(text string) string {
	tmp := []rune(text)
	if len(tmp) > 0 {
		tmp[0] = unicode.ToUpper(tmp[0])
	}
	return string(tmp)
}

func (cnt *resourceCount) Noop() bool {
	return cnt.Destroy == 0 && cnt.Create == 0 && cnt.Replace == 0 && cnt.Modify == 0
}

func (cnt *resourceCount) render() string {
	clauses := []string{}
	if cnt.Destroy > 0 {
		clauses = append(clauses, fmt.Sprintf("destroy %d", cnt.Destroy))
	}
	if cnt.Create > 0 {
		clauses = append(clauses, fmt.Sprintf("create %d", cnt.Create))
	}
	if cnt.Replace > 0 {
		clauses = append(clauses, fmt.Sprintf("replace %d", cnt.Replace))
	}
	if cnt.Modify > 0 {
		clause := fmt.Sprintf("modify %d", cnt.Modify)
		subclauses := []string{}
		if cnt.ModifyDelete > 0 {
			subclauses = append(subclauses, fmt.Sprintf("delete %d", cnt.ModifyDelete))
		}
		if cnt.ModifyAdd > 0 {
			subclauses = append(subclauses, fmt.Sprintf("add %d", cnt.ModifyAdd))
		}
		if cnt.ModifyChange > 0 {
			subclauses = append(subclauses, fmt.Sprintf("change %d", cnt.ModifyChange))
		}
		if len(subclauses) > 0 {
			clause += fmt.Sprintf(" (modified properties: %s)", strings.Join(subclauses, ", "))
		}
		clauses = append(clauses, clause)
	}
	var text string
	if len(clauses) > 0 {
		text = strings.Join(clauses, ", ")
	} else {
		text = "no changes"
	}
	return capitalize(text)
}

type projectApplySummary struct {
	FailedProjects    []idxProjectResult
	ErroredProjects   []idxProjectResult
	SucceededProjects []idxProjectResult
	NoopProjects      []idxProjectResult
}

func isApplyNoop(apply string) bool {
	return strings.Contains(
		apply,
		"Apply complete! Resources: 0 added, 0 changed, 0 destroyed.",
	)
}

func summarizeApplyResults(results []models.ProjectResult) projectApplySummary {
	sum := projectApplySummary{
		FailedProjects:    []idxProjectResult{},
		ErroredProjects:   []idxProjectResult{},
		SucceededProjects: []idxProjectResult{},
		NoopProjects:      []idxProjectResult{},
	}
	for idx, result := range results {
		idxResult := idxProjectResult{
			result,
			idx,
		}
		if result.Error != nil {
			sum.FailedProjects = append(sum.FailedProjects, idxResult)
		} else if result.Failure != "" {
			sum.ErroredProjects = append(sum.ErroredProjects, idxResult)
		} else if result.ApplySuccess != "" {
			if isApplyNoop(result.ApplySuccess) {
				sum.NoopProjects = append(sum.NoopProjects, idxResult)
			} else {
				sum.SucceededProjects = append(sum.SucceededProjects, idxResult)
			}
		}
	}
	return sum
}

func renderApplySummarySection(section string, results []idxProjectResult) string {
	if len(results) == 0 {
		return ""
	}
	plural := "s"
	if len(results) == 1 {
		plural = " has"
		section = strings.ReplaceAll(section, "have", "has")
	}
	header := fmt.Sprintf("* %d plan%s %s:\n", len(results), plural, section)
	indent := "    * "
	lines := []string{}
	for _, result := range results {
		line := fmt.Sprintf("%s_%d. dir: `%s`_", indent, result.Index+1, result.RepoRelDir)
		if result.Workspace != "default" {
			line += fmt.Sprintf(" workspace: `%s`", result.Workspace)
		}
		line += "\n"
		lines = append(lines, line)
	}
	return header + strings.Join(lines, "")
}

func (sum *projectApplySummary) render() string {
	return strings.Join([]string{
		renderApplySummarySection("have **failed to apply**", sum.ErroredProjects),
		renderApplySummarySection("have **Atlantis errors**", sum.FailedProjects),
		renderApplySummarySection("have **applied successfully**", sum.SucceededProjects),
		renderApplySummarySection("with **no changes** (not shown below)", sum.NoopProjects),
	}, "")
}

// Render formats the data into a markdown string.
// nolint: interfacer
func (m *MarkdownRenderer) Render(res CommandResult, cmdName models.CommandName, log string, verbose bool, vcsHost models.VCSHostType) string {
	commandStr := strings.Title(cmdName.String())
	common := commonData{
		Command:         commandStr,
		Verbose:         verbose,
		Log:             log,
		PlansDeleted:    res.PlansDeleted,
		DisableApplyAll: m.DisableApplyAll || m.DisableApply,
		DisableApply:    m.DisableApply,
	}
	if res.Error != nil {
		return m.renderTemplate(unwrappedErrWithLogTmpl, errData{res.Error.Error(), common})
	}
	if res.Failure != "" {
		return m.renderTemplate(failureWithLogTmpl, failureData{res.Failure, common})
	}
	return m.renderProjectResults(res.ProjectResults, common, vcsHost)
}

var diffMarkerMovementRegexp = regexp.MustCompile(`(?m)^( +)([-+~]) `)
var diffTildeRegexp = regexp.MustCompile(`(?m)^~ `)

func enhanceDiffColorCoding(output string) string {
	// Move +, -, ~ indicators to left margin, without changing
	// line indentation
	output = diffMarkerMovementRegexp.ReplaceAllString(output, "$2$1 ")
	// Replace ~ with !
	output = diffTildeRegexp.ReplaceAllString(output, "! ")
	return output
}

func (m *MarkdownRenderer) renderProjectResults(results []models.ProjectResult, common commonData, vcsHost models.VCSHostType) string {
	var resultsTmplData []projectResultTmplData
	numPlanSuccesses := 0

	overview := ""
	if common.Command == planCommandTitle {
		planSummary := summarizePlanResults(results)
		overview = planSummary.render()
	} else if common.Command == applyCommandTitle {
		applySummary := summarizeApplyResults(results)
		overview = applySummary.render()
	}

	for _, result := range results {
		resultData := projectResultTmplData{
			Workspace:   result.Workspace,
			RepoRelDir:  result.RepoRelDir,
			ProjectName: result.ProjectName,
		}
		if result.Error != nil {
			tmpl := unwrappedErrTmpl
			if m.shouldUseWrappedTmpl(vcsHost, result.Error.Error()) {
				tmpl = wrappedErrTmpl
			}
			resultData.Rendered = m.renderTemplate(tmpl, struct {
				Command string
				Error   string
				Summary string
			}{
				Command: common.Command,
				Error:   result.Error.Error(),
				Summary: fmt.Sprintf("<b>%s failed</b>", common.Command),
			})
		} else if result.Failure != "" {
			resultData.Rendered = m.renderTemplate(failureTmpl, struct {
				Command string
				Failure string
				Summary string
			}{
				Command: common.Command,
				Failure: result.Failure,
				Summary: "<b>Atlantis error</b>",
			})
		} else if result.PlanSuccess != nil {
			cnt := countResources(result.PlanSuccess.TerraformOutput)
			colorCodedPlan := *result.PlanSuccess
			colorCodedPlan.TerraformOutput = enhanceDiffColorCoding(colorCodedPlan.TerraformOutput)
			if cnt.Noop() {
				resultData.Rendered = ""
			} else {
				summary := cnt.render()
				if m.shouldUseWrappedTmpl(vcsHost, result.PlanSuccess.TerraformOutput) {
					resultData.Rendered = m.renderTemplate(planSuccessWrappedTmpl, planSuccessData{PlanSuccess: colorCodedPlan, PlanWasDeleted: common.PlansDeleted, Summary: summary})
				} else {
					resultData.Rendered = m.renderTemplate(planSuccessUnwrappedTmpl, planSuccessData{PlanSuccess: colorCodedPlan, PlanWasDeleted: common.PlansDeleted})
				}
			}
			numPlanSuccesses++
		} else if result.ApplySuccess != "" {
			if isApplyNoop(result.ApplySuccess) {
				resultData.Rendered = ""
			} else {
				summary := "Applied successfully"
				if m.shouldUseWrappedTmpl(vcsHost, result.ApplySuccess) {
					resultData.Rendered = m.renderTemplate(applyWrappedSuccessTmpl, struct {
						Output  string
						Summary string
					}{result.ApplySuccess, summary})
				} else {
					resultData.Rendered = m.renderTemplate(applyUnwrappedSuccessTmpl, struct{ Output string }{result.ApplySuccess})
				}
			}
		} else {
			resultData.Rendered = "Found no template. This is a bug!"
		}
		resultsTmplData = append(resultsTmplData, resultData)
	}

	var tmpl *template.Template
	switch {
	case len(resultsTmplData) == 1 && common.Command == planCommandTitle && numPlanSuccesses > 0:
		tmpl = singleProjectPlanSuccessTmpl
	case len(resultsTmplData) == 1 && common.Command == planCommandTitle && numPlanSuccesses == 0:
		tmpl = singleProjectPlanUnsuccessfulTmpl
	case len(resultsTmplData) == 1 && common.Command == applyCommandTitle:
		tmpl = singleProjectApplyTmpl
	case common.Command == planCommandTitle:
		tmpl = multiProjectPlanTmpl
	case common.Command == applyCommandTitle:
		tmpl = multiProjectApplyTmpl
	default:
		return "no template matched–this is a bug"
	}
	return m.renderTemplate(tmpl, resultData{resultsTmplData, overview, common})
}

// shouldUseWrappedTmpl returns true if we should use the wrapped markdown
// templates that collapse the output to make the comment smaller on initial
// load. Some VCS providers or versions of VCS providers don't support this
// syntax.
func (m *MarkdownRenderer) shouldUseWrappedTmpl(vcsHost models.VCSHostType, output string) bool {
	if m.DisableMarkdownFolding {
		return false
	}

	// Bitbucket Cloud and Server don't support the folding markdown syntax.
	if vcsHost == models.BitbucketServer || vcsHost == models.BitbucketCloud {
		return false
	}

	if vcsHost == models.Gitlab && !m.GitlabSupportsCommonMark {
		return false
	}

	// Original code only uses a wrapped template when the output
	// is long enough. This makes large comments hard to parse
	// because the format is inconsistent. We always wrap them.
	// The inconvenience is offset by including a useful <summary>
	// description for each <details> tag, unlike in the original
	// Atlantis.
	return true
}

func (m *MarkdownRenderer) renderTemplate(tmpl *template.Template, data interface{}) string {
	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, data); err != nil {
		return fmt.Sprintf("Failed to render template, this is a bug: %v", err)
	}
	return buf.String()
}

// todo: refactor to remove duplication #refactor
//
// todo: Plaid modifications to these templates do not support
// DisableApply and DisableApplyAll.
var singleProjectApplyTmpl = template.Must(template.New("").Parse(
	"{{$result := index .Results 0}}Ran {{.Command}} for {{ if $result.ProjectName }}project: `{{$result.ProjectName}}` {{ end }}dir: `{{$result.RepoRelDir}}`{{ if ne $result.Workspace \"default\" }} workspace: `{{$result.Workspace}}`{{end}}\n\n{{if $result.Rendered}}{{$result.Rendered}}{{else}}**No changes to infrastructure**{{end}}\n" + logTmpl))
var singleProjectPlanSuccessTmpl = template.Must(template.New("").Parse(
	"{{$result := index .Results 0}}Ran {{.Command}} for {{ if $result.ProjectName }}project: `{{$result.ProjectName}}` {{ end }}dir: `{{$result.RepoRelDir}}`{{ if ne $result.Workspace \"default\" }} workspace: `{{$result.Workspace}}`{{end}}\n\n{{if $result.Rendered}}{{$result.Rendered}}{{else}}**No changes to infrastructure**{{end}}\n\n" + logTmpl + "\n\n_To apply this plan, comment `atlantis apply`. To discard it, comment `atlantis unlock`._"))
var singleProjectPlanUnsuccessfulTmpl = template.Must(template.New("").Parse(
	"{{$result := index .Results 0}}Ran {{.Command}} for dir: `{{$result.RepoRelDir}}`{{ if ne $result.Workspace \"default\" }} workspace: `{{$result.Workspace}}`{{end}}\n\n" +
		"{{$result.Rendered}}\n" + logTmpl))
var multiProjectPlanTmpl = template.Must(template.New("").Funcs(sprig.TxtFuncMap()).Parse(
	"## atlantis plan [{{ len .Results }} projects]\n\n" +
		"{{ .Overview }}\n\n" +
		"{{ range $i, $result := .Results }}{{ if $result.Rendered }}" +
		"### {{add $i 1}}. {{ if $result.ProjectName }}project: `{{$result.ProjectName}}` {{ end }}dir: `{{$result.RepoRelDir}}`{{ if ne $result.Workspace \"default\" }} workspace: `{{$result.Workspace}}`{{end}}\n" +
		"{{$result.Rendered}}\n\n" +
		"{{end}}{{end}}" +
		logTmpl + "\n\n_To apply this plan, comment `atlantis apply`. To discard it, comment `atlantis unlock`._"))
var multiProjectApplyTmpl = template.Must(template.New("").Funcs(sprig.TxtFuncMap()).Parse(
	"## atlantis apply [{{ len .Results }} projects]\n\n" +
		"{{ .Overview }}\n\n" +
		"{{ range $i, $result := .Results }}{{if $result.Rendered }}" +
		"### {{add $i 1}}. {{ if $result.ProjectName }}project: `{{$result.ProjectName}}` {{ end }}dir: `{{$result.RepoRelDir}}`{{ if ne $result.Workspace \"default\" }} workspace: `{{$result.Workspace}}`{{end}}\n" +
		"{{$result.Rendered}}\n\n" +
		"{{end}}{{end}}" +
		logTmpl))
var planSuccessUnwrappedTmpl = template.Must(template.New("").Parse(
	"```diff\n" +
		"{{.TerraformOutput}}\n" +
		"```\n\n" + planNextSteps +
		"{{ if .HasDiverged }}\n\n:warning: The branch we're merging into is ahead, it is recommended to pull new commits first.{{end}}"))

var planSuccessWrappedTmpl = template.Must(template.New("").Parse(
	"<details><summary>{{.Summary}}</summary>\n\n" +
		"```diff\n" +
		"{{.TerraformOutput}}\n" +
		"```\n\n" +
		planNextSteps + "\n" +
		"</details>" +
		"{{ if .HasDiverged }}\n\n:warning: The branch we're merging into is ahead, it is recommended to pull new commits first.{{end}}"))

// planNextSteps are instructions appended after successful plans as to what
// to do next.
var planNextSteps = "{{ if .PlanWasDeleted }}This plan was not saved because one or more projects failed and automerge requires all plans pass.{{end}}"
var applyUnwrappedSuccessTmpl = template.Must(template.New("").Parse(
	"```diff\n" +
		"{{.Output}}\n" +
		"```"))
var applyWrappedSuccessTmpl = template.Must(template.New("").Parse(
	"<details><summary>{{.Summary}}</summary>\n\n" +
		"```diff\n" +
		"{{.Output}}\n" +
		"```\n" +
		"</details>"))
var unwrappedErrTmplText = "**{{.Command}} Error**\n" +
	"```\n" +
	"{{.Error}}\n" +
	"```"
var wrappedErrTmplText = "<details><summary>{{.Summary}}</summary>\n\n" +
	"```\n" +
	"{{.Error}}\n" +
	"```\n</details>"
var unwrappedErrTmpl = template.Must(template.New("").Parse(unwrappedErrTmplText))
var unwrappedErrWithLogTmpl = template.Must(template.New("").Parse(unwrappedErrTmplText + logTmpl))
var wrappedErrTmpl = template.Must(template.New("").Parse(wrappedErrTmplText))
var failureTmplText = "<details><summary>{{.Summary}}</summary>\n\n" +
	"{{.Failure}}\n\n" +
	"</details>"
var failureTmpl = template.Must(template.New("").Parse(failureTmplText))
var failureWithLogTmpl = template.Must(template.New("").Parse(failureTmplText + logTmpl))
var logTmpl = "{{if .Verbose}}\n<details><summary>Log</summary>\n  <p>\n\n```\n{{.Log}}```\n</p></details>{{end}}\n"
