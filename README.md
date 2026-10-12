# ops-lens-mcp

[![Go Reference](https://pkg.go.dev/badge/github.com/frank/ops-lens-mcp.svg)](https://pkg.go.dev/github.com/frank/ops-lens-mcp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**ops-lens-mcp** is a lightweight, read-only diagnostic Model Context Protocol (MCP) server. It enables AI coding agents (such as Antigravity, Claude Desktop, Cursor, and OpenCodeInterpreter) to safely inspect remote infrastructure across **Kubernetes clusters** and **Docker Compose VPS instances** via SSH without hallucinations, shell injections, or context-window blowouts.

---

## ✨ Features

- **Multi-Cloud & Hybrid Infra:** Single unified tool interface querying both remote Docker Compose instances over SSH and Kubernetes clusters via `client-go`.
- **Token & Context Guardrails:** Automatic truncation (tail byte/line caps) and ANSI escape stripping to protect your agent's LLM context window.
- **Strictly Non-Destructive:** Pure read-only diagnostics by default (`list`, `status`, `logs`, `describe`, `metrics`).
- **Zero-Secret Configs:** Supports environment variable expansion (`${STAGING_SSH_KEY:-~/.ssh/id_ed25519}`).

---

## 🛠️ Diagnostic Tool Suite

| Tool | Description |
| :--- | :--- |
| `ops_list_targets` | Lists all configured environments (`prod-k8s`, `staging-vps`, etc.). |
| `ops_list_services` | Lists pods, deployments, or running containers in the target. |
| `ops_get_status` | Returns container/pod state, restart counts, and health status. |
| `ops_get_logs` | Fetches bounded tail logs with regex filtering (`filter`) and time windows (`since`). |
| `ops_describe_workload`| Deep inspection: exit codes, `OOMKilled` status, termination reason, K8s events. |
| `ops_get_host_metrics` | Host/Node resource saturation: CPU load, memory usage, and root disk capacity. |

---

## 🚀 Quick Start

### 1. Installation

#### Via Go:
```bash
go install github.com/frank/ops-lens-mcp/cmd/ops-lens-mcp@latest
```

#### Or Build from Source:
```bash
git clone https://github.com/frank/ops-lens-mcp.git
cd ops-lens-mcp
go build -o /usr/local/bin/ops-lens-mcp ./cmd/ops-lens-mcp
```

### 2. Configure Targets (`config.yaml`)

Copy the example configuration:
```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml`:
```yaml
defaults:
  max_log_lines: 250
  max_bytes: 65536
  strip_ansi: true

targets:
  prod-k8s:
    driver: k8s
    kubeconfig: "${KUBECONFIG:-~/.kube/config}"
    namespace: "default"

  staging-vps:
    driver: docker-compose
    ssh:
      host: "staging.example.com"
      port: 22
      user: "ai-diag"
      key_file: "~/.ssh/id_ed25519"
    compose_path: "/opt/app/docker-compose.yml"
```

### 3. Agent Integration

Add to your agent's MCP settings (e.g. `claude_desktop_config.json` or Antigravity's `mcp_config.json`):

```json
{
  "mcpServers": {
    "ops-lens": {
      "command": "ops-lens-mcp",
      "args": ["--config", "/absolute/path/to/config.yaml"]
    }
  }
}
```

---

## 🔒 Security Best Practices

1. **Principle of Least Privilege:** When connecting to a remote VPS, use a dedicated read-only system user (e.g., `ai-diag`) that is only a member of the `docker` and `systemd-journal` groups.
2. **Key Restrictions:** Restrict the agent's SSH key in `~/.ssh/authorized_keys` on the remote VPS:
   ```text
   no-port-forwarding,no-X11-forwarding,no-agent-forwarding ssh-ed25519 AAAA...
   ```

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.