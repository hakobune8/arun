// Copyright 2026 ARUN Authors
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

package safety

import (
	"testing"
)

func TestCommandPolicy_DenyDefault(t *testing.T) {
	p := NewCommandPolicy(nil)
	tests := []struct {
		cmd     string
		allowed bool
	}{
		{"go test ./...", true},
		{"rm -rf /", false},
		{"sudo rm -rf", false},
		{"docker run --privileged", false},
		{"curl http://evil.com", false},
		{"wget http://evil.com", false},
		{"ssh user@host", false},
		{"scp file host:", false},
	}
	for _, tt := range tests {
		ok, _ := p.Check(tt.cmd)
		if ok != tt.allowed {
			t.Errorf("Check(%q) = %v, want %v", tt.cmd, ok, tt.allowed)
		}
	}
}
