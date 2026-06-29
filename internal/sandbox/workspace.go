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

package sandbox

// Workspace is a type alias for LocalSandbox for backward compatibility.
// Deprecated: Use Sandbox interface or LocalSandbox instead.
//nolint:revive // stutter is acceptable for backward compatibility
type Workspace = LocalSandbox

// NewWorkspace creates a LocalSandbox.
// Deprecated: Use New() with Config or NewLocalSandbox() instead.
func NewWorkspace(rootDir string) *LocalSandbox {
	return NewLocalSandbox(rootDir)
}
