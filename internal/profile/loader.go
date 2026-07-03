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

package profile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads a Profile from the YAML file at path, applies defaults, and
// validates the result.
func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile file %s: %w", path, err)
	}

	prof := DefaultProfile()
	if err := yaml.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("parse profile %s: %w", path, err)
	}

	if err := validate(&prof); err != nil {
		return nil, fmt.Errorf("invalid profile %s: %w", path, err)
	}

	return &prof, nil
}

func validate(p *Profile) error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if p.Limits.MaxRetries <= 0 {
		p.Limits.MaxRetries = 3
	}
	if p.Limits.MaxIterations <= 0 {
		p.Limits.MaxIterations = 8
	}
	if p.Limits.MaxChangedFiles <= 0 {
		p.Limits.MaxChangedFiles = 20
	}
	if p.Limits.MaxRuntimeMinute <= 0 {
		p.Limits.MaxRuntimeMinute = 30
	}
	return nil
}
