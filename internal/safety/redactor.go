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
	"encoding/json"
	"regexp"
)

const redactedValue = "[REDACTED]"

var (
	privateKeyBlockPattern = regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	authHeaderPattern      = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer|token|basic)\s+[A-Za-z0-9._~+/=-]+`)
	cookieHeaderPattern    = regexp.MustCompile(`(?i)(cookie\s*[:=]\s*)[^\r\n]+`)
	agentOSSessionPattern  = regexp.MustCompile(`(?i)(arun_session=)[^;\s]+`)
	githubTokenPattern     = regexp.MustCompile(`\b(?:github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9_]{20,}|ghs_[A-Za-z0-9_]{20,})\b`)
	openAITokenPattern     = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`)
	jwtPattern             = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	keyValuePattern        = regexp.MustCompile(`(?i)(["']?(?:access[_-]?token|api[_-]?key|authorization|client[_-]?key[_-]?data|client[_-]?secret|cookie|github[_-]?token|id[_-]?token|kubeconfig|password|private[_-]?key|refresh[_-]?token|secret|session|token)["']?\s*[:=]\s*)(["'][^"'\r\n]*["']|[^\s,}\]]+)`)
)

// Redactor removes high-risk secret material before logs, reports, or
// artifacts are persisted or returned through the API.
type Redactor struct{}

// NewRedactor creates a Redactor with the built-in secret patterns.
func NewRedactor() *Redactor {
	return &Redactor{}
}

// RedactString replaces known tokens, auth headers, cookies, kubeconfig
// credentials, and private keys with a stable placeholder.
func (r *Redactor) RedactString(s string) string {
	if s == "" {
		return s
	}
	out := privateKeyBlockPattern.ReplaceAllString(s, redactedValue)
	out = authHeaderPattern.ReplaceAllString(out, `${1}${2} `+redactedValue)
	out = cookieHeaderPattern.ReplaceAllString(out, `${1}`+redactedValue)
	out = agentOSSessionPattern.ReplaceAllString(out, `${1}`+redactedValue)
	out = keyValuePattern.ReplaceAllString(out, `${1}"`+redactedValue+`"`)
	out = githubTokenPattern.ReplaceAllString(out, redactedValue)
	out = openAITokenPattern.ReplaceAllString(out, redactedValue)
	out = jwtPattern.ReplaceAllString(out, redactedValue)
	return out
}

// RedactValue redacts strings inside arbitrary JSON-like values. Structs and
// maps are round-tripped through JSON so nested values are covered uniformly.
func (r *Redactor) RedactValue(v any) any {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return r.RedactString(s)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	redacted := r.RedactString(string(data))
	var out any
	if err := json.Unmarshal([]byte(redacted), &out); err != nil {
		return redacted
	}
	return out
}
