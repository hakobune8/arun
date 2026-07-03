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
	"strings"
	"testing"
)

func TestRedactor_RedactString(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		`Authorization: Bearer ghp_123456789012345678901234567890123456`,
		`Cookie: arun_session=signed-session-value; other=value`,
		`LITELLM_API_KEY=sk-123456789012345678901234567890`,
		`client-key-data: kube-private-key`,
		`-----BEGIN PRIVATE KEY-----`,
		`abcdef`,
		`-----END PRIVATE KEY-----`,
	}, "\n")

	got := NewRedactor().RedactString(raw)
	for _, leaked := range []string{
		"ghp_123456789012345678901234567890123456",
		"signed-session-value",
		"sk-123456789012345678901234567890",
		"kube-private-key",
		"abcdef",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted output leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, redactedValue) {
		t.Fatalf("redacted output did not contain placeholder: %s", got)
	}
}

func TestRedactor_RedactValueNestedJSON(t *testing.T) {
	t.Parallel()

	value := map[string]any{
		"headers": map[string]string{
			"Authorization": "Bearer ghp_123456789012345678901234567890123456",
		},
		"kubeconfig": "apiVersion: v1\nusers:\n- token: secret-token",
	}

	got := NewRedactor().RedactValue(value)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, leaked := range []string{"ghp_123456789012345678901234567890123456", "secret-token"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("redacted value leaked %q: %s", leaked, text)
		}
	}
}
