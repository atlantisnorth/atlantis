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
//
package events

import "github.com/runatlantis/atlantis/server/events/vcs"

// ProjectResult is the result of executing a plan/apply for a project.
type ProjectResult struct {
	Path         string
	Error        error
	Failure      string
	PlanSuccess  *PlanSuccess
	ApplySuccess string
}

// Status returns the vcs commit status of this project result.
func (p ProjectResult) Status() vcs.CommitStatus {
	if p.Error != nil {
		return vcs.Failed
	}
	if p.Failure != "" {
		return vcs.Failed
	}
	return vcs.Success
}
