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

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kazyamaz200/agentos/internal/evals"
	"github.com/spf13/cobra"
)

var evalsCmd = &cobra.Command{
	Use:          "evals",
	Short:        "Run orchestration regression evals",
	SilenceUsage: true,
	Long: `Run deterministic orchestration regression evals.

The default suite does not require external LLM, GitHub, or Kubernetes
credentials. It validates scenario routing, fallback recovery, required
artifacts, quality gates, and report generation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEvals(cmd.Context())
	},
}

var (
	evalsFormat               string
	evalsOutput               string
	evalsScenarios            string
	evalsWorkDir              string
	evalsLive                 bool
	evalsLiveURL              string
	evalsAuthE2E              bool
	evalsStorageCleanupE2E    bool
	evalsScheduleNotifyE2E    bool
	evalsGitHubWorkflowE2E    bool
	evalsScrumGitHubE2E       bool
	evalsKubernetesRolloutE2E bool
	evalsRealLLMSmokeE2E      bool
	evalsLiteLLMPresetEvals   bool
)

func init() {
	rootCmd.AddCommand(evalsCmd)
	evalsCmd.Flags().StringVar(&evalsFormat, "format", "markdown", "Report format: markdown or json")
	evalsCmd.Flags().StringVarP(&evalsOutput, "output", "o", "", "Report output path; defaults under AGENTOS_HOME/evals")
	evalsCmd.Flags().StringVar(&evalsScenarios, "scenario", "", "Comma-separated scenario IDs to run")
	evalsCmd.Flags().StringVar(&evalsWorkDir, "work-dir", "", "Directory for temporary eval repositories")
	evalsCmd.Flags().BoolVar(&evalsLive, "live", false, "Include opt-in live deployment smoke checks")
	evalsCmd.Flags().StringVar(&evalsLiveURL, "live-url", "", "Base URL for live smoke checks; defaults to AGENTOS_EVAL_LIVE_URL")
	evalsCmd.Flags().BoolVar(&evalsAuthE2E, "auth-e2e", false, "Include opt-in authenticated Web UI browser E2E checks")
	evalsCmd.Flags().BoolVar(&evalsStorageCleanupE2E, "storage-cleanup-e2e", false, "Include opt-in authenticated storage cleanup dry-run and execution checks")
	evalsCmd.Flags().BoolVar(&evalsScheduleNotifyE2E, "schedule-notification-e2e", false, "Include opt-in authenticated schedule execution notification checks")
	evalsCmd.Flags().BoolVar(&evalsGitHubWorkflowE2E, "github-workflow-e2e", false, "Include opt-in live GitHub issue and PR workflow checks")
	evalsCmd.Flags().BoolVar(&evalsScrumGitHubE2E, "scrum-github-e2e", false, "Include opt-in executable three-sprint scrum GitHub workflow checks")
	evalsCmd.Flags().BoolVar(&evalsKubernetesRolloutE2E, "kubernetes-rollout-e2e", false, "Include opt-in live Kubernetes rollout and rollback checks")
	evalsCmd.Flags().BoolVar(&evalsRealLLMSmokeE2E, "real-llm-smoke-e2e", false, "Include opt-in real LLM orchestration smoke checks")
	evalsCmd.Flags().BoolVar(&evalsLiteLLMPresetEvals, "litellm-preset-evals", false, "Include opt-in LiteLLM preset matrix checks")
}

func runEvals(ctx context.Context) error {
	format := strings.ToLower(strings.TrimSpace(evalsFormat))
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "json" {
		return fmt.Errorf("unsupported eval report format %q", evalsFormat)
	}
	output := evalsOutput
	if strings.TrimSpace(output) == "" {
		output = evals.DefaultOutputPath(format)
	}
	report, err := evals.Run(ctx, evals.Options{
		ScenarioIDs:                 splitComma(evalsScenarios),
		WorkDir:                     evalsWorkDir,
		IncludeLive:                 evalsLive,
		LiveURL:                     evalsLiveURL,
		IncludeAuthE2E:              evalsAuthE2E,
		IncludeStorageCleanupE2E:    evalsStorageCleanupE2E,
		IncludeScheduleNotifyE2E:    evalsScheduleNotifyE2E,
		IncludeGitHubWorkflowE2E:    evalsGitHubWorkflowE2E,
		IncludeScrumGitHubE2E:       evalsScrumGitHubE2E,
		IncludeKubernetesRolloutE2E: evalsKubernetesRolloutE2E,
		IncludeRealLLMSmokeE2E:      evalsRealLLMSmokeE2E,
		IncludeLiteLLMPresetEvals:   evalsLiteLLMPresetEvals,
	})
	if err != nil {
		return err
	}
	switch format {
	case "json":
		if err := evals.WriteJSON(report, output); err != nil {
			return err
		}
	default:
		if err := evals.WriteMarkdown(report, output); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "Orchestration evals: %d/%d passed (%.1f%%)\n", report.Passed, report.Total, report.SuccessRate*100)
	if output != "-" {
		fmt.Fprintf(os.Stderr, "Report saved to %s\n", output)
	}
	if report.Failed > 0 {
		return fmt.Errorf("%d orchestration eval scenario(s) failed", report.Failed)
	}
	return nil
}
