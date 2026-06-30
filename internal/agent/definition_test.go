package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestDef(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadDefinition_Valid(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: test-agent
spec:
  llm:
    model: coder
`
	path := writeTestDef(t, yaml)
	def, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
	if def.APIVersion != "agentos.io/v1" {
		t.Errorf("APIVersion = %q, want %q", def.APIVersion, "agentos.io/v1")
	}
	if def.Kind != "Agent" {
		t.Errorf("Kind = %q, want %q", def.Kind, "Agent")
	}
	if def.Metadata.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", def.Metadata.Name, "test-agent")
	}
	if def.Spec.LLM.Model != "coder" {
		t.Errorf("Model = %q, want %q", def.Spec.LLM.Model, "coder")
	}
}

func TestLoadDefinition_Full(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: go-backend
  labels:
    role: backend
spec:
  llm:
    model: coder
    temperature: 0.2
    maxTokens: 8192
  tools:
    allow:
      - read_file
      - write_file
      - shell
      - git
      - test
  safety:
    denyCommands:
      - rm -rf
      - sudo
  commands:
    test: go test ./...
    lint: go vet ./...
  limits:
    maxRetries: 3
    maxIterations: 8
  guidance:
    architecture:
      - Preserve existing layout
      - Prefer standard library
    outputExpectations:
      - Tests pass
`
	path := writeTestDef(t, yaml)
	def, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if len(def.Spec.Tools.Allow) != 5 {
		t.Errorf("got %d tools, want 5", len(def.Spec.Tools.Allow))
	}
	if len(def.Spec.Safety.DenyCommands) != 2 {
		t.Errorf("got %d deny commands, want 2", len(def.Spec.Safety.DenyCommands))
	}
	if def.Spec.Commands.Test != "go test ./..." {
		t.Errorf("Test command = %q", def.Spec.Commands.Test)
	}
	if def.Spec.Limits.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", def.Spec.Limits.MaxRetries)
	}
	if len(def.Spec.Guidance.Architecture) != 2 {
		t.Errorf("got %d architecture guidance items, want 2", len(def.Spec.Guidance.Architecture))
	}
	if len(def.Spec.Guidance.OutputExpectations) != 1 {
		t.Errorf("got %d output expectations, want 1", len(def.Spec.Guidance.OutputExpectations))
	}
}

func TestLoadDefinition_MissingAPIVersion(t *testing.T) {
	t.Parallel()

	yaml := `
kind: Agent
metadata:
  name: test
spec:
  llm:
    model: coder
`
	path := writeTestDef(t, yaml)
	_, err := LoadDefinition(path)
	if err == nil {
		t.Fatal("expected error for missing apiVersion")
	}
}

func TestLoadDefinition_MissingName(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: ""
spec:
  llm:
    model: coder
`
	path := writeTestDef(t, yaml)
	_, err := LoadDefinition(path)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestLoadDefinition_MissingModel(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: test
spec:
  llm:
    model: ""
`
	path := writeTestDef(t, yaml)
	_, err := LoadDefinition(path)
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestLoadDefinition_WrongKind(t *testing.T) {
	t.Parallel()

	yaml := `
apiVersion: agentos.io/v1
kind: Workflow
metadata:
  name: test
spec:
  llm:
    model: coder
`
	path := writeTestDef(t, yaml)
	_, err := LoadDefinition(path)
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
}

func TestLoadDefinition_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadDefinition("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestDefaultDefinition(t *testing.T) {
	t.Parallel()

	def := DefaultDefinition()
	if def.APIVersion != CurrentSchemaVersion {
		t.Errorf("APIVersion = %q, want %q", def.APIVersion, CurrentSchemaVersion)
	}
	if def.Kind != "Agent" {
		t.Errorf("Kind = %q, want Agent", def.Kind)
	}
	if def.Metadata.Name != "default" {
		t.Errorf("Name = %q, want default", def.Metadata.Name)
	}
	if def.Spec.LLM.Model != "coder" {
		t.Errorf("Model = %q, want coder", def.Spec.LLM.Model)
	}
}
