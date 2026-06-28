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
