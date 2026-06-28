// Package task provides types and functions for defining and loading agent
// tasks from YAML files or GitHub issues.
package task

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Task represents an agent task with its metadata, repository, branch, and
// description.
type Task struct {
	ID          string `yaml:"id"`
	Type        string `yaml:"type"`
	Repo        string `yaml:"repo"`
	BaseBranch  string `yaml:"base_branch"`
	Branch      string `yaml:"branch"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

// Load reads a Task from a YAML file at path, validates it, and returns the
// parsed Task.
func Load(path string) (*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read task file %s: %w", path, err)
	}

	var t Task
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse task %s: %w", path, err)
	}

	if err := validate(&t); err != nil {
		return nil, fmt.Errorf("invalid task %s: %w", path, err)
	}

	return &t, nil
}

func validate(t *Task) error {
	if t.ID == "" {
		return fmt.Errorf("id is required")
	}
	if t.Type == "" {
		t.Type = "issue_to_patch"
	}
	if t.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if t.BaseBranch == "" {
		t.BaseBranch = "main"
	}
	if t.Branch == "" {
		t.Branch = "agent/" + t.ID
	}
	return nil
}
