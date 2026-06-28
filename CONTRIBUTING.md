# Contributing to AgentOS

Thank you for considering contributing to AgentOS! We welcome contributions of all kinds.

## Code of Conduct

This project and everyone participating in it is governed by our [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## How to Contribute

### Reporting Bugs

1. Check existing [issues](https://github.com/kazyamaz200/agentos/issues) to avoid duplicates.
2. Open a new issue with a clear title and description.
3. Include steps to reproduce, expected behavior, and actual behavior.
4. Include Go version, OS, and any relevant configuration.

### Suggesting Features

1. Open a feature request issue with a clear description.
2. Explain the use case and why it would benefit the project.
3. If possible, sketch an implementation approach.

### Pull Requests

1. **Branch naming**: Use `fix/` for bug fixes, `feat/` for features, `docs/` for documentation.
2. **Base branch**: Always target `main`.
3. **Commit messages**: Write clear, descriptive commit messages in English.
4. **Tests**: Add tests for new functionality. Ensure existing tests pass.
5. **Lint**: Run `go vet ./...` and ensure no warnings.
6. **Documentation**: Update relevant documentation (`README.md`, `docs/`) as needed.
7. **DCO**: You must sign off your commits (`git commit -s`) to indicate you agree to the [Developer Certificate of Origin](https://developercertificate.org/).

### Development Setup

```bash
# Prerequisites: Go 1.22+, LiteLLM (optional), Docker (optional for sandbox)

# Clone the repo
git clone https://github.com/kazyamaz200/agentos.git
cd agentos

# Build
go build ./cmd/agentos

# Run tests
go test ./...

# Run lint
go vet ./...
```

## Coding Style

- Follow [standard Go conventions](https://go.dev/doc/effective_go).
- Use `gofmt` (or `go fmt`) before committing.
- Exported types and functions must have Go doc comments.
- Prefer interfaces for testability.
- Use `filepath.Join` for path construction (cross-platform).
- Handle all errors; use `%w` for error wrapping.

## Project Structure

- `cmd/agentos/` — CLI entry point
- `internal/` — All internal packages
- `profiles/` — Agent profile and template definitions
- `examples/` — Example task files and sample repos
- `docs/` — Architecture and usage documentation

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
