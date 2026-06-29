# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

Only the latest released version receives security updates.

## Reporting a Vulnerability

If you discover a security vulnerability in AgentOS, please report it by emailing the project maintainer. **Do not** create a public GitHub issue for security vulnerabilities.

Please include the following in your report:

- A description of the vulnerability
- Steps to reproduce (if applicable)
- Potential impact
- Any suggested fix (if known)

You can expect:

- Acknowledgment of your report within 48 hours
- A timeline for a fix and disclosure
- Credit for discovering the vulnerability (if desired)

## Security Best Practices

When using AgentOS:

1. **API Keys**: Store API keys in environment variables or `.env` files. Never commit them to the repository.
2. **Command Denylist**: Configure `deny_commands` in your profile to restrict shell access. AgentOS ships with a built-in denylist for dangerous commands.
3. **Branch Protection**: AgentOS will refuse to commit directly to protected branches (main, master).
4. **Secret Detection**: Secret file patterns (`.pem`, `id_rsa*`, etc.) are detected and blocked from being written.
5. **Web UI**: The built-in web server has no authentication by default. Use a reverse proxy with HTTPS for production deployments.
6. **Sandboxing**: The v1.0 runtime uses the local sandbox backend. For untrusted code execution, run AgentOS inside an external container, VM, or isolated Kubernetes workload boundary.
