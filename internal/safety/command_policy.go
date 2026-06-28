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

// Package safety provides security policy enforcement including command
// allow/deny rules and secret file detection.
package safety

import (
	"strings"
)

// CommandPolicy defines a set of denied command patterns and checks shell
// commands against them before execution.
type CommandPolicy struct {
	DenyCommands []string
}

// NewCommandPolicy returns a CommandPolicy pre-populated with a set of
// dangerous command patterns (rm -rf, sudo, docker --privileged, curl, etc.)
// and any additional denyCommands provided.
func NewCommandPolicy(denyCommands []string) *CommandPolicy {
	defaultDeny := []string{
		"rm -rf", "rm -rf /", "rm -rf /*",
		"sudo", "sudo ",
		"docker run --privileged",
		"curl", "wget",
		"scp", "ssh",
	}
	if len(denyCommands) > 0 {
		defaultDeny = append(defaultDeny, denyCommands...)
	}
	return &CommandPolicy{
		DenyCommands: defaultDeny,
	}
}

// Check verifies whether command is allowed by the policy. It returns true if
// the command is permitted, along with the matched pattern if it was denied.
func (p *CommandPolicy) Check(command string) (ok bool, denied string) {
	cmdLower := strings.TrimSpace(strings.ToLower(command))
	for _, denied := range p.DenyCommands {
		if strings.Contains(cmdLower, strings.ToLower(denied)) {
			return false, denied
		}
	}
	return true, ""
}
