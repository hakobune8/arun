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
	"path/filepath"
	"strings"
)

var secretPatterns = []string{
	".env",
	".env.*",
	"*.pem",
	"id_rsa",
	"id_rsa.pub",
	"id_ed25519",
	"id_ed25519.pub",
	".secret*",
	"*.key",
	".credentials*",
	".aws/credentials",
	".gcp/credentials*",
	".token*",
}

// SecretDetector identifies files that are likely to contain secrets (e.g.
// .env, *.pem, id_rsa) by matching their names against known patterns.
type SecretDetector struct {
	patterns []string
}

// NewSecretDetector returns a SecretDetector with a built-in set of secret
// file patterns.
func NewSecretDetector() *SecretDetector {
	return &SecretDetector{patterns: secretPatterns}
}

// IsSecretFile returns true if the base name of filePath matches a known
// secret file pattern.
func (s *SecretDetector) IsSecretFile(filePath string) bool {
	name := filepath.Base(filePath)
	for _, pattern := range s.patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if strings.Contains(pattern, "*") {
			continue
		}
		if name == pattern {
			return true
		}
	}
	return false
}
