# Safety

AgentOS includes multiple safety mechanisms to prevent accidental damage.

## Command Denylist

Shell commands are checked against a denylist before execution. Denied by default:
- `rm -rf`, `rm -rf /`, `rm -rf /*`
- `sudo`
- `docker run --privileged`
- `curl`, `wget`
- `ssh`, `scp`

Additional deny patterns can be added in the profile YAML under `tools.deny_commands`.

## Secret File Detection

The filesystem tools reject reads and writes for the following secret-like file
names:
- `.env`, `.env.*`
- `*.pem`
- `id_rsa`, `id_rsa.pub`, `id_ed25519`, `id_ed25519.pub`
- `*.key`
- `.credentials*`, `.aws/credentials`, `.gcp/credentials*`
- `.token*`

## Main Branch Protection

- AgentOS attempts to create and work on the task branch for each run.
- Repository-level branch protection should still be enforced by GitHub or your
  Git server.

## Run Isolation

- Each run creates `${AGENTOS_HOME}/runs/{task_id}/` for all artifacts. If
  `AGENTOS_HOME` is not set, AgentOS uses `~/.agentos`.
- All file changes are tracked via git diff
- Before/after state is preserved

## Limits

- Maximum changed files: profile field exists; enforcement is planned
- Maximum retries on test/lint failure: configurable (default 3)
- Maximum runtime: configurable (default 30 minutes)
