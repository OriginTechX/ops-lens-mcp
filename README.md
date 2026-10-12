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
| `ops_get_journal_logs` | Host systemd journal logs (host Traefik, Docker daemon, Nginx, kernel errors). |
| `ops_inspect_host_config`| Safely inspects host config files in allowed paths (`/etc/traefik`, `/etc/nginx`, `/etc/docker`). |

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

### 2. Configure Targets (`config.yaml` & `.env`)

1. Copy the example configuration and environment variable templates:
```bash
cp config.example.yaml config.yaml
cp .env.example .env
```

2. Customize your `.env` with IPs, SSH keys, and usernames:
```bash
# Global limits
MAX_LOG_LINES=250

# Kubernetes
KUBECONFIG=~/.kube/config

# VPS 1 (Staging)
STAGING_SSH_HOST=staging.example.com
STAGING_SSH_USER=ai-diag
STAGING_SSH_KEY=~/.ssh/id_ed25519
STAGING_COMPOSE_PATH=/opt/app/docker-compose.yml

# VPS 2 (Worker)
WORKER_SSH_HOST=192.168.1.120
WORKER_SSH_USER=ai-diag
WORKER_SSH_KEY=~/.ssh/id_ed25519
WORKER_COMPOSE_PATH=/opt/worker/docker-compose.yml
```

3. `config.yaml` references these environment variables dynamically:
```yaml
defaults:
  max_log_lines: ${MAX_LOG_LINES:-250}
  max_bytes: ${MAX_BYTES:-65536}
  strip_ansi: ${STRIP_ANSI:-true}

targets:
  prod-k8s:
    driver: k8s
    kubeconfig: "${KUBECONFIG:-~/.kube/config}"
    namespace: "${PROD_K8S_NAMESPACE:-default}"

  staging-vps:
    driver: docker-compose
    ssh:
      host: "${STAGING_SSH_HOST}"
      port: ${STAGING_SSH_PORT:-22}
      user: "${STAGING_SSH_USER:-ai-diag}"
      key_file: "${STAGING_SSH_KEY:-~/.ssh/id_ed25519}"
    compose_path: "${STAGING_COMPOSE_PATH}"

  worker-vps:
    driver: docker-compose
    ssh:
      host: "${WORKER_SSH_HOST}"
      port: 22
      user: "${WORKER_SSH_USER:-ai-diag}"
      key_file: "${WORKER_SSH_KEY:-~/.ssh/id_ed25519}"
    compose_path: "${WORKER_COMPOSE_PATH}"
```

### 3. Agent Integration

Add to your agent's MCP settings (e.g. `claude_desktop_config.json` or Antigravity's `mcp_config.json`):

**Option A: Native Binary (Recommended)**
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

**Option B: Docker Container**
```json
{
  "mcpServers": {
    "ops-lens": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-v", "/path/to/config.yaml:/etc/ops-lens/config.yaml:ro",
        "-v", "/home/frank/.kube/config:/root/.kube/config:ro",
        "-v", "/home/frank/.ssh:/root/.ssh:ro",
        "ghcr.io/frank/ops-lens-mcp:latest",
        "--config", "/etc/ops-lens/config.yaml"
      ]
    }
  }
}
```

---

## 🔒 Security & Key Setup

### 1. Dedicated Read-Only User & Key Provisioning (Recommended)
Instead of using your personal SSH key (which typically has `sudo` privileges), use the automated `just` commands to create and install a dedicated, restricted key pair:

1. **Generate dedicated key locally (Run once):**
   ```bash
   just setup-key
   ```
   *Creates `~/.ssh/ops_lens_ed25519` without modifying your personal keys.*

2. **Provision remote VPS (Interactive bootstrap):**
   ```bash
   just install-key admin@staging.example.com
   # Or with custom port/key:
   # just install-key admin@staging.example.com port=2222 admin_key=~/.ssh/custom_admin
   ```
   *This connects via SSH, interactively prompts for password/sudo if needed, creates a dedicated system user `ai-diag` (members of `docker` & `systemd-journal` only, no `sudo`), and installs the key with restriction flags.*

3. **Restricted Key Flags:**
   The `install-key` command automatically prepends security flags to `/home/ai-diag/.ssh/authorized_keys`:
   ```text
   no-port-forwarding,no-agent-forwarding,no-X11-forwarding ssh-ed25519 AAAAC3Nza... ops-lens-mcp
   ```

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.