# Configuration

## LiteLLM

AgentOS uses LiteLLM as the LLM gateway. You can start LiteLLM with your preferred providers:

```bash
litellm --model gpt-4 --api_key $OPENAI_API_KEY
# or with a config file
litellm --config litellm_config.yaml
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `LITELLM_API_KEY` | `""` | API key for LiteLLM server |
| `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM server URL |
| `AGENTOS_MODEL_CODER` | `coder` | Model alias for coding tasks |
| `AGENTOS_MODEL_EMBEDDING` | `text-embedding-ada-002` | Model alias for embeddings |

---

## Qdrant Vector Store

By default, AgentOS uses a local JSON-based vector store. To use Qdrant for production-scale vector search:

### Setup

```bash
# Start Qdrant with Docker
docker run -d --name qdrant -p 6333:6333 qdrant/qdrant
```

### Configuration

Set the `QDRANT_URL` environment variable:

```bash
export QDRANT_URL=http://localhost:6333
```

When `QDRANT_URL` is set, AgentOS will automatically use the Qdrant client instead of the local JSON store.

Qdrant collections are created automatically by AgentOS. No manual schema setup is required.

---

## Docker Sandbox

AgentOS can execute shell commands inside a Docker container for additional isolation.

### Setup

Ensure Docker is installed and the current user has permission to run Docker commands:

```bash
docker ps  # verify Docker is running
```

### Configuration

Docker sandbox is enabled via the `--sandbox docker` flag:

```bash
agentos run --task task.yaml --profile profile.yaml --sandbox docker
```

The sandbox uses the `mcr.microsoft.com/devcontainers/go:latest` image by default. You can customize the image via the profile YAML:

```yaml
sandbox:
  image: "custom-image:latest"
  memory_limit: "2g"
  cpu_limit: "1.0"
```

---

## MCP (Model Context Protocol)

AgentOS supports connecting to MCP servers to extend the available tools.

### Configuration

MCP servers are registered via the CLI:

```bash
# Register an MCP server
agentos mcp register my-server --command "python" --args "-m", "my_mcp_server"

# List registered servers
agentos mcp list

# Call a tool from an MCP server
agentos mcp call my-server tool_name '{"arg": "value"}'
```

### Environment Variables

MCP server commands inherit the AgentOS environment variables, including `LITELLM_API_KEY`, `OPENAI_API_KEY`, etc.

---

## GitHub Integration

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GITHUB_TOKEN` | `""` | GitHub personal access token for issue/PR/checks operations |

The token requires the following scopes:
- `repo` — for private repositories
- `public_repo` — for public repositories only

---

## Agent Templates

Multi-agent teams are defined in YAML template files:

```yaml
schema: "agentos/v1"
agents:
  - name: "coder"
    role: "Coding agent"
    model: "coder"
    tools:
      - read_file
      - write_file
      - shell
      - git
      - test
coordination:
  strategy: "sequential"
```

See [profiles/agents/template.yaml](../profiles/agents/template.yaml) for a complete example.
