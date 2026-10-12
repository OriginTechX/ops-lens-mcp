package docker

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frank/ops-lens-mcp/internal/config"
	"github.com/frank/ops-lens-mcp/internal/driver"
	"github.com/frank/ops-lens-mcp/internal/guardrail"
	"golang.org/x/crypto/ssh"
)

// Driver implements driver.Driver for remote Docker Compose over SSH.
type Driver struct {
	targetName string
	cfg        config.Target
	client     *ssh.Client
}

// New creates a new SSH-backed Docker Compose driver.
func New(targetName string, cfg config.Target) (*Driver, error) {
	if cfg.SSH == nil {
		return nil, fmt.Errorf("target %q missing ssh configuration", targetName)
	}

	d := &Driver{
		targetName: targetName,
		cfg:        cfg,
	}

	return d, nil
}

func (d *Driver) getSSHClient() (*ssh.Client, error) {
	if d.client != nil {
		// Quick ping check
		_, _, err := d.client.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			return d.client, nil
		}
		_ = d.client.Close()
		d.client = nil
	}

	sshConf := d.cfg.SSH
	var authMethods []ssh.AuthMethod

	if sshConf.KeyFile != "" {
		keyPath := sshConf.KeyFile
		if strings.HasPrefix(keyPath, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				keyPath = filepath.Join(home, keyPath[2:])
			}
		}
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key %q: %w", keyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if sshConf.Password != "" {
		authMethods = append(authMethods, ssh.Password(sshConf.Password))
	}

	port := sshConf.Port
	if port <= 0 {
		port = 22
	}

	clientConfig := &ssh.ClientConfig{
		User:            sshConf.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Diagnostic probe convenience; can be tightened
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(sshConf.Host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	d.client = client
	return d.client, nil
}

func (d *Driver) runCommand(ctx context.Context, cmd string) (string, error) {
	client, err := d.getSSHClient()
	if err != nil {
		return "", err
	}

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return "", ctx.Err()
	case err := <-done:
		out := stdout.String()
		if err != nil {
			errStr := strings.TrimSpace(stderr.String())
			if errStr != "" {
				return out, fmt.Errorf("command failed (%w): %s", err, errStr)
			}
			return out, err
		}
		return out, nil
	}
}

// ListServices lists running or defined docker compose services.
func (d *Driver) ListServices(ctx context.Context) ([]string, error) {
	composeFlag := ""
	if d.cfg.ComposePath != "" {
		composeFlag = fmt.Sprintf("-f %s", d.cfg.ComposePath)
	}

	// docker compose ps --services
	cmd := fmt.Sprintf("docker compose %s ps --services 2>/dev/null || docker-compose %s ps --services", composeFlag, composeFlag)
	out, err := d.runCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	var services []string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			services = append(services, trimmed)
		}
	}
	return services, nil
}

// GetStatus returns the status of a specific container or service.
func (d *Driver) GetStatus(ctx context.Context, service string) (*driver.WorkloadStatus, error) {
	composeFlag := ""
	if d.cfg.ComposePath != "" {
		composeFlag = fmt.Sprintf("-f %s", d.cfg.ComposePath)
	}

	// Query docker inspect format
	cmd := fmt.Sprintf("docker compose %s ps %s --format json 2>/dev/null", composeFlag, service)
	out, err := d.runCommand(ctx, cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		// Fallback to plain ps
		cmd = fmt.Sprintf("docker compose %s ps %s", composeFlag, service)
		out, err = d.runCommand(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("get service status: %w", err)
		}
	}

	status := &driver.WorkloadStatus{
		Name:    service,
		Target:  d.targetName,
		State:   "Unknown",
		Details: map[string]string{"raw_ps": strings.TrimSpace(out)},
	}

	lower := strings.ToLower(out)
	if strings.Contains(lower, "running") || strings.Contains(lower, "up ") {
		status.State = "Running"
	} else if strings.Contains(lower, "restarting") {
		status.State = "Restarting"
	} else if strings.Contains(lower, "exited") || strings.Contains(lower, "down") {
		status.State = "Stopped"
	}

	return status, nil
}

// GetLogs fetches container logs with tail and grep.
func (d *Driver) GetLogs(ctx context.Context, query driver.LogQuery) (string, error) {
	composeFlag := ""
	if d.cfg.ComposePath != "" {
		composeFlag = fmt.Sprintf("-f %s", d.cfg.ComposePath)
	}

	tail := query.Tail
	if tail <= 0 {
		tail = 200
	}

	sinceFlag := ""
	if query.Since != "" {
		sinceFlag = fmt.Sprintf("--since %s", query.Since)
	}

	cmd := fmt.Sprintf("docker compose %s logs --no-color --tail=%d %s %s 2>&1", composeFlag, tail, sinceFlag, query.Service)
	out, err := d.runCommand(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("fetch logs: %w", err)
	}

	cleaned := guardrail.StripANSI(out)
	if query.Filter != "" {
		cleaned = guardrail.FilterLines(cleaned, query.Filter)
	}

	return cleaned, nil
}

// DescribeWorkload gives container inspect details and exit codes.
func (d *Driver) DescribeWorkload(ctx context.Context, service string) (string, error) {
	composeFlag := ""
	if d.cfg.ComposePath != "" {
		composeFlag = fmt.Sprintf("-f %s", d.cfg.ComposePath)
	}

	// Get container id, inspect state, restart count, and OOMKilled flag
	cmd := fmt.Sprintf(`CID=$(docker compose %s ps -q %s 2>/dev/null); if [ -n "$CID" ]; then docker inspect "$CID" --format 'Status: {{.State.Status}} | ExitCode: {{.State.ExitCode}} | OOMKilled: {{.State.OOMKilled}} | Error: {{.State.Error}} | FinishedAt: {{.State.FinishedAt}} | RestartCount: {{.RestartCount}}'; else echo "Container for %s not found"; fi`, composeFlag, service, service)
	out, err := d.runCommand(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("describe workload: %w", err)
	}

	return strings.TrimSpace(out), nil
}

// GetMetrics returns host CPU load, Memory, and Disk usage via SSH.
func (d *Driver) GetMetrics(ctx context.Context) (*driver.HostMetrics, error) {
	cmd := `echo "=== UPTIME ==="; uptime; echo "=== MEMORY ==="; free -h; echo "=== DISK ==="; df -h /`
	out, err := d.runCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("get host metrics: %w", err)
	}

	metrics := &driver.HostMetrics{
		Target: d.targetName,
		Raw:    out,
	}

	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, "=== UPTIME ===") && i+1 < len(lines) {
			metrics.Uptime = strings.TrimSpace(lines[i+1])
		}
	}

	return metrics, nil
}

// Close closes the SSH connection.
func (d *Driver) Close() error {
	if d.client != nil {
		err := d.client.Close()
		d.client = nil
		return err
	}
	return nil
}
