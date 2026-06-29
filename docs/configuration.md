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

AgentOS has a sandbox interface, but the Docker backend is currently a stub and
is not available in v1.0. Use the local backend only for trusted repositories and
run AgentOS inside a separately isolated environment when executing untrusted
code.

The intended future configuration shape is:

```yaml
sandbox:
  backend: docker
  image: "custom-image:latest"
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
