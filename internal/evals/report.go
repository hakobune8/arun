// Copyright 2026 AgentOS Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package evals

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Markdown renders the report as a release-friendly Markdown summary.
func Markdown(report *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Orchestration Eval Report\n\n")
	fmt.Fprintf(&b, "- Started: `%s`\n", report.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(&b, "- Finished: `%s`\n", report.FinishedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(&b, "- Duration: `%dms`\n", report.DurationMS)
	fmt.Fprintf(&b, "- Result: `%d/%d passed` (`%.1f%%`)\n\n", report.Passed, report.Total, report.SuccessRate*100)
	fmt.Fprintf(&b, "| Scenario | Mode | Agents | Result | Duration | Failure reasons |\n")
	fmt.Fprintf(&b, "|---|---:|---|---:|---:|---|\n")
	for i := range report.ScenarioRuns {
		scenario := &report.ScenarioRuns[i]
		reasons := "-"
		if len(scenario.FailureReasons) > 0 {
			reasons = strings.Join(scenario.FailureReasons, "<br>")
		}
		result := "pass"
		if !scenario.Passed {
			result = "fail"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | `%dms` | %s |\n", scenario.ID, scenario.Mode, strings.Join(scenario.Agents, ", "), result, scenario.DurationMS, reasons)
	}
	fmt.Fprintf(&b, "\n## Functional Coverage\n\n")
	fmt.Fprintf(&b, "| Area | Covered | Scenarios |\n")
	fmt.Fprintf(&b, "|---|---:|---|\n")
	for _, area := range report.Coverage {
		fmt.Fprintf(&b, "| `%s` | `%d/%d` | `%s` |\n", area.Name, area.Covered, area.Total, strings.Join(area.Scenarios, "`, `"))
	}
	fmt.Fprintf(&b, "\n## Required Artifacts\n\n")
	for i := range report.ScenarioRuns {
		scenario := &report.ScenarioRuns[i]
		if len(scenario.RequiredFiles) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", scenario.Name)
		for _, file := range scenario.RequiredFiles {
			mark := " "
			if file.Exists {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] `%s`\n", mark, file.Path)
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "\n## Scenario Checks\n\n")
	for i := range report.ScenarioRuns {
		scenario := &report.ScenarioRuns[i]
		if len(scenario.Checks) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", scenario.Name)
		fmt.Fprintf(&b, "| Page | Action | Result | Duration | Failure |\n")
		fmt.Fprintf(&b, "|---|---|---:|---:|---|\n")
		for _, check := range scenario.Checks {
			result := "pass"
			if !check.Passed {
				result = "fail"
			}
			failure := check.Failure
			if failure == "" {
				failure = "-"
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%dms` | %s |\n", check.Page, check.Action, result, check.DurationMS, failure)
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

// WriteMarkdown writes a report as Markdown.
func WriteMarkdown(report *Report, path string) error {
	data := []byte(Markdown(report))
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
