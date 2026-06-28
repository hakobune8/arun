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
