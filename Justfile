# Load environment variables from .env if present
set dotenv-load

# Versioning metadata
VERSION := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
GIT_COMMIT := `git rev-parse --short HEAD 2>/dev/null || echo "none"`
BUILD_DATE := `date -u +'%Y-%m-%dT%H:%M:%SZ'`
LDFLAGS := "-ldflags \"-s -w -X main.version=" + VERSION + " -X main.commit=" + GIT_COMMIT + " -X main.date=" + BUILD_DATE + "\""

# Default command when running `just`
@help:
    echo "ops-lens-mcp Management Commands (Justfile):"
    echo "  just run            - Run ops-lens-mcp locally over stdio (using config.yaml)"
    echo "  just check          - Test compilation only (no binary output)"
    echo "  just build          - Build production binary to bin/ops-lens-mcp"
    echo "  just tidy           - Clean up and sync Go module dependencies"
    echo "  just update         - Update all Go dependencies to latest versions"
    echo "  just test           - Run all unit tests with coverage"
    echo "  just test-race      - Run all unit tests with race detection (CGO_ENABLED=1)"
    echo "  just test-clean     - Clear test cache"
    echo "  just lint           - Run golangci-lint on codebase"
    echo "  just lint-fix       - Run golangci-lint and auto-fix issues"
    echo "  just deadcode       - Run deadcode analysis to detect unused functions"
    echo "  just clean          - Clear build cache, bin/, and dist/ artifacts"
    echo "  just setup-key      - Generate dedicated SSH key pair (~/.ssh/ops_lens_ed25519)"
    echo "  just install-key    - Provision 'ai-diag' user & install key on remote VPS (interactive)"
    echo "  just docker-build   - Build Docker container image"
    echo "  just docker-run     - Run Docker container interactively over stdio"
    echo "  just release-check  - Validate .goreleaser.yaml configuration syntax"
    echo "  just release-snapshot - Build local release snapshot via GoReleaser"

# Run ops-lens-mcp over stdio locally with config.yaml
run config="config.yaml":
    go run ./cmd/ops-lens-mcp --config {{config}}

# Test compilation only (no binary output)
@check:
    echo ">> Checking ops-lens-mcp compilation..."
    go build -o /dev/null ./cmd/ops-lens-mcp
    echo ">> Target compiles successfully. (No binary files created)"

# Build production binary to bin/ folder
@build:
    echo ">> Building ops-lens-mcp binary..."
    mkdir -p bin
    go build {{LDFLAGS}} -o bin/ops-lens-mcp ./cmd/ops-lens-mcp
    echo ">> Build successful. Binary is in 'bin/ops-lens-mcp'."

# Sync Go module dependencies
@tidy:
    echo ">> Running go mod tidy..."
    go mod tidy
    echo ">> Go modules synced."

# Update all dependencies to latest versions
@update:
    echo ">> Updating Go dependencies..."
    go get -u ./...
    go mod tidy
    echo ">> Update complete."

# Run all unit tests with coverage
@test:
    echo ">> Running tests with coverage..."
    go test -v -cover ./...
    echo ">> Tests complete."

# Run unit tests with race detection (requires CGO_ENABLED=1)
@test-race:
    echo ">> Running tests with race detection (CGO_ENABLED=1)..."
    CGO_ENABLED=1 go test -v -race -cover ./...
    echo ">> Tests complete."

# Clear test cache
@test-clean:
    echo ">> Cleaning test cache..."
    go clean -testcache
    echo ">> Test cache cleaned."

# Run golangci-lint on codebase
@lint:
    echo ">> Running golangci-lint on ops-lens-mcp..."
    golangci-lint run --timeout=5m
    echo ">> Lint clean."

# Run golangci-lint and auto-fix issues
@lint-fix:
    echo ">> Running golangci-lint with auto-fix..."
    golangci-lint run --fix --timeout=5m
    echo ">> Lint fix complete."

# Run deadcode analysis to detect unused functions
@deadcode:
    echo ">> Running deadcode analysis on ops-lens-mcp..."
    output=$(go run golang.org/x/tools/cmd/deadcode@latest -test ./... 2>&1 | grep -v "^go: downloading" | grep -v "^go: added" || true); \
    if [ -n "$output" ]; then \
        echo "$output"; \
        echo ">> Error: Deadcode found! Please review above functions."; \
        exit 1; \
    else \
        echo ">> Success: No deadcode found."; \
    fi

# Clear build cache, module cache, and bin/ artifacts
@clean:
    echo ">> Cleaning build cache, module cache, and binaries..."
    go clean -cache
    rm -rf bin/ dist/
    echo ">> Clean complete."

# Build Docker image
@docker-build tag="ops-lens-mcp:latest":
    echo ">> Building Docker image {{tag}}..."
    docker build -t {{tag}} .
    echo ">> Docker image built: {{tag}}"

# Run Docker container interactively over stdio
@docker-run tag="ops-lens-mcp:latest" config="config.yaml":
    echo ">> Running Docker container over stdio..."
    docker run --rm -i \
        -v "$PWD/{{config}}:/etc/ops-lens/config.yaml:ro" \
        -v "$HOME/.kube/config:/root/.kube/config:ro" \
        -v "$HOME/.ssh:/root/.ssh:ro" \
        {{tag}} --config /etc/ops-lens/config.yaml

# Generate a dedicated, isolated SSH key pair for ops-lens-mcp (~/.ssh/ops_lens_ed25519)
@setup-key key_path=(env_var_or_default("HOME", "") + "/.ssh/ops_lens_ed25519"):
    if [ -f "{{key_path}}" ]; then \
        echo ">> Dedicated key already exists at {{key_path}}"; \
    else \
        echo ">> Generating dedicated ed25519 key at {{key_path}}..."; \
        ssh-keygen -t ed25519 -N "" -f "{{key_path}}" -C "ops-lens-mcp" >/dev/null; \
        echo ">> ✅ Key generated: {{key_path}}"; \
    fi

# Provision restricted 'ai-diag' user and install dedicated key on remote VPS (interactive SSH)
@install-key remote port="22" admin_key="" key_path=(env_var_or_default("HOME", "") + "/.ssh/ops_lens_ed25519"):
    @if [ ! -f "{{key_path}}.pub" ]; then \
        echo ">> Error: Public key {{key_path}}.pub not found. Run 'just setup-key' first."; \
        exit 1; \
    fi
    @echo ">> Connecting to {{remote}} (port {{port}}) to configure user 'ai-diag'..."
    @PUBKEY=$$(cat "{{key_path}}.pub"); \
    RESTRICTED_KEY="no-port-forwarding,no-agent-forwarding,no-X11-forwarding $$PUBKEY"; \
    SSH_OPTS="-p {{port}}"; \
    if [ -n "{{admin_key}}" ]; then SSH_OPTS="$$SSH_OPTS -i {{admin_key}}"; fi; \
    ssh -t $$SSH_OPTS {{remote}} "\
        sudo id -u ai-diag &>/dev/null || sudo useradd -m -s /bin/bash -G docker,systemd-journal ai-diag; \
        sudo mkdir -p /home/ai-diag/.ssh; \
        sudo chmod 700 /home/ai-diag/.ssh; \
        echo '$$RESTRICTED_KEY' | sudo tee -a /home/ai-diag/.ssh/authorized_keys >/dev/null; \
        sudo chmod 600 /home/ai-diag/.ssh/authorized_keys; \
        sudo chown -R ai-diag:ai-diag /home/ai-diag/.ssh; \
        echo '>> ✅ Success: Restricted user \"ai-diag\" configured with ops-lens key!'"

# Validate GoReleaser configuration syntax
@release-check:
    echo ">> Checking .goreleaser.yaml configuration..."
    go run github.com/goreleaser/goreleaser/v2@latest check
    echo ">> GoReleaser configuration is valid."

# Build local release snapshot via GoReleaser (cross-compiles all binaries)
@release-snapshot:
    echo ">> Building GoReleaser snapshot release..."
    go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
    echo ">> Snapshot release artifacts available in dist/."
