# Changelog

## [Unreleased] - v1.0

### Added
- Runtime Agent interface (Plan, Execute, Review) with lifecycle hooks (#91)
- Versioned Agent definition schema (apiVersion: agentos.io/v1) (#97)
- Agent plugin registry with built-in agents (go-backend, reviewer, ci-fixer, docs) (#93)
- Structured event bus with typed events and file store persistence (#94)
- JSON memory store backend (zero dependencies) (#95)
- Sandbox interface abstraction with LocalSandbox and Docker stub (#96)
- Agent Factory from versioned Definition YAML (#98)
- Multi-agent orchestration wired to actual runtime execution (#99)
- Tool Description() method on all built-in tools and MCP adapter (#92)
- Registry validation, lifecycle support, and duplicate detection (#92)

### Changed
- Runtime delegates planning/execution/review to Agent interface
- MemoryStore renamed to VectorStore implementing Store interface
- Workspace renamed to LocalSandbox implementing Sandbox interface
- Orchestrator uses runtime.Agent interface and agent registry

### Fixed
- BuildAgentFromDefinition now returns LLM client properly

## [v0.5] - 2026-06-28

### Added
- Agent Factory: create agent instances from YAML template definitions
- Multi-agent orchestration with sequential/parallel strategies
- CLI commands: `agentos agent list/create/run`, `agentos orchestrate`
- Agent template system with coder/reviewer/tester template
- Package-level Go doc comments (ongoing)

### Changed
- Profile loading uses var instead of value receiver for DefaultProfile

## [v0.4] - 2026-06-27

### Added
- MCP client (JSON-RPC stdio) with tool registration
- Docker sandbox for isolated agent execution
- Web UI dashboard (`agentos serve`)
- GitHub CI checks integration
- CI Fix Agent for automated CI failure resolution

### Changed
- Internal: safety package structure improvements

## [v0.3] - 2026-06-26

### Added
- Vector search with local JSON store and Qdrant backend
- Agent memory system for cross-run context retention
- Coding guidelines management
- LiteLLM embedding support
- Unified search across memory, guidelines, and PRs

### Changed
- LLM client interface extended for embedding support

## [v0.2] - 2026-06-25

### Added
- GitHub API client for issue/PR/checks operations
- `agentos issue`, `agentos pr`, `agentos checks` commands
- Auto-PR creation on `agentos run --pr`
- CI Fix Agent prototype

## [v0.1] - 2026-06-24

### Added
- Initial AgentOS implementation
- CLI with `run`, `review`, `version` commands
- LLM client with LiteLLM integration
- Tool system: filesystem, shell, git, search, test tools
- Safety layer: command denylist, secret detection, branch protection
- Task/profile YAML loading
- Runtime orchestration with plan/execute/review/retry lifecycle
- Run state persistence and JSONL logging
