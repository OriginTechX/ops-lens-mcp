package server

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/frank/ops-lens-mcp/internal/config"
	"github.com/frank/ops-lens-mcp/internal/driver"
	"github.com/frank/ops-lens-mcp/internal/driver/docker"
	"github.com/frank/ops-lens-mcp/internal/driver/k8s"
	"github.com/frank/ops-lens-mcp/internal/guardrail"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server coordinates MCP tools and infrastructure drivers.
type Server struct {
	cfg     *config.Config
	mcp     *server.MCPServer
	mu      sync.RWMutex
	drivers map[string]driver.Driver
}

// New creates and registers all tools for the ops-lens MCP server.
func New(cfg *config.Config, version string) (*Server, error) {
	s := &Server{
		cfg:     cfg,
		mcp:     server.NewMCPServer("ops-lens-mcp", version),
		drivers: make(map[string]driver.Driver),
	}

	s.registerTools()
	return s, nil
}

// MCPServer returns the underlying MCP server.
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcp
}

func (s *Server) getDriver(targetName string) (driver.Driver, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if d, ok := s.drivers[targetName]; ok {
		return d, nil
	}

	tgt, ok := s.cfg.Targets[targetName]
	if !ok {
		available := slices.Sorted(maps.Keys(s.cfg.Targets))
		return nil, fmt.Errorf("target %q not found. Available targets: %s", targetName, strings.Join(available, ", "))
	}

	var d driver.Driver
	var err error

	switch tgt.Driver {
	case "k8s":
		d, err = k8s.New(targetName, tgt)
	case "docker", "docker-compose":
		d, err = docker.New(targetName, tgt)
	default:
		return nil, fmt.Errorf("unsupported driver %q for target %q", tgt.Driver, targetName)
	}

	if err != nil {
		return nil, fmt.Errorf("initialize driver for %q: %w", targetName, err)
	}

	s.drivers[targetName] = d
	return d, nil
}

func (s *Server) registerTools() {
	// 1. ops_list_targets
	s.mcp.AddTool(mcp.NewTool("ops_list_targets",
		mcp.WithDescription("List all configured target environments (K8s clusters, Docker Compose VPSs) and their drivers"),
	), s.handleListTargets)

	// 2. ops_list_services
	s.mcp.AddTool(mcp.NewTool("ops_list_services",
		mcp.WithDescription("List running services, containers, or pods in a specific target environment"),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target environment name (from ops_list_targets)")),
	), s.handleListServices)

	// 3. ops_get_status
	s.mcp.AddTool(mcp.NewTool("ops_get_status",
		mcp.WithDescription("Get the health and runtime status of a specific service, pod, or container"),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target environment name")),
		mcp.WithString("service", mcp.Required(), mcp.Description("Service or pod name")),
	), s.handleGetStatus)

	// 4. ops_get_logs
	s.mcp.AddTool(mcp.NewTool("ops_get_logs",
		mcp.WithDescription("Fetch bounded logs for a service or pod with optional regex filter and time horizon"),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target environment name")),
		mcp.WithString("service", mcp.Required(), mcp.Description("Service or pod name")),
		mcp.WithNumber("tail", mcp.Description("Number of lines from tail (default 200, max capped)")),
		mcp.WithString("since", mcp.Description("Time window (e.g. '10m', '1h')")),
		mcp.WithString("filter", mcp.Description("Regex pattern or keyword to filter logs")),
	), s.handleGetLogs)

	// 5. ops_describe_workload
	s.mcp.AddTool(mcp.NewTool("ops_describe_workload",
		mcp.WithDescription("Inspect deep workload diagnostics (exit codes, OOMKilled reasons, recent events, restarts)"),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target environment name")),
		mcp.WithString("service", mcp.Required(), mcp.Description("Service or pod name")),
	), s.handleDescribeWorkload)

	// 6. ops_get_host_metrics
	s.mcp.AddTool(mcp.NewTool("ops_get_host_metrics",
		mcp.WithDescription("Check host/node resource saturation (CPU load, memory, disk usage, uptime)"),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target environment name")),
	), s.handleGetHostMetrics)

	// 7. ops_get_journal_logs
	s.mcp.AddTool(mcp.NewTool("ops_get_journal_logs",
		mcp.WithDescription("Query host systemd journal logs (e.g. host Traefik, Docker daemon, Nginx, or OS errors)"),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target environment name")),
		mcp.WithString("unit", mcp.Description("Systemd service unit name (e.g. 'traefik', 'docker', 'nginx')")),
		mcp.WithString("priority", mcp.Description("Syslog priority filter (e.g. 'err', 'warning', 'info')")),
		mcp.WithNumber("tail", mcp.Description("Number of log lines from tail (default 200)")),
		mcp.WithString("since", mcp.Description("Time window (e.g. '30m', '2h', 'yesterday')")),
		mcp.WithString("filter", mcp.Description("Regex pattern or keyword to filter logs")),
	), s.handleGetJournalLogs)

	// 8. ops_inspect_host_config
	s.mcp.AddTool(mcp.NewTool("ops_inspect_host_config",
		mcp.WithDescription("Inspect host configuration files within safe system directories (e.g. /etc/traefik, /etc/nginx, /etc/docker)"),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target environment name")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute file path on host (e.g. '/etc/traefik/traefik.yml')")),
	), s.handleInspectHostConfig)
}

func (s *Server) handleListTargets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	type targetSummary struct {
		Name      string `json:"name"`
		Driver    string `json:"driver"`
		Namespace string `json:"namespace,omitempty"`
		Host      string `json:"host,omitempty"`
	}

	var list []targetSummary
	for name, t := range s.cfg.Targets {
		item := targetSummary{
			Name:      name,
			Driver:    t.Driver,
			Namespace: t.Namespace,
		}
		if t.SSH != nil {
			item.Host = t.SSH.Host
		}
		list = append(list, item)
	}

	data, _ := json.MarshalIndent(list, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleListServices(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("missing 'target' parameter"), nil
	}

	d, err := s.getDriver(target)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	services, err := d.ListServices(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list services: %v", err)), nil
	}

	data, _ := json.MarshalIndent(services, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("missing 'target' parameter"), nil
	}
	service, err := req.RequireString("service")
	if err != nil {
		return mcp.NewToolResultError("missing 'service' parameter"), nil
	}

	d, err := s.getDriver(target)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	status, err := d.GetStatus(ctx, service)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get status: %v", err)), nil
	}

	data, _ := json.MarshalIndent(status, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("missing 'target' parameter"), nil
	}
	service, err := req.RequireString("service")
	if err != nil {
		return mcp.NewToolResultError("missing 'service' parameter"), nil
	}

	tail := s.cfg.Defaults.MaxLogLines
	if rawTail := req.GetInt("tail", 0); rawTail > 0 {
		tail = min(tail, rawTail)
	}

	since := req.GetString("since", "")
	filter := req.GetString("filter", "")

	d, err := s.getDriver(target)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	logs, err := d.GetLogs(ctx, driver.LogQuery{
		Service: service,
		Tail:    tail,
		Since:   since,
		Filter:  filter,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get logs: %v", err)), nil
	}

	// Apply byte and line guardrails
	if s.cfg.Defaults.StripANSI {
		logs = guardrail.StripANSI(logs)
	}
	logs, _ = guardrail.TruncateLines(logs, s.cfg.Defaults.MaxLogLines)
	logs, _ = guardrail.TruncateBytes(logs, s.cfg.Defaults.MaxBytes)

	if strings.TrimSpace(logs) == "" {
		return mcp.NewToolResultText("(no logs found matching criteria)"), nil
	}

	return mcp.NewToolResultText(logs), nil
}

func (s *Server) handleDescribeWorkload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("missing 'target' parameter"), nil
	}
	service, err := req.RequireString("service")
	if err != nil {
		return mcp.NewToolResultError("missing 'service' parameter"), nil
	}

	d, err := s.getDriver(target)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	desc, err := d.DescribeWorkload(ctx, service)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to describe workload: %v", err)), nil
	}

	return mcp.NewToolResultText(desc), nil
}

func (s *Server) handleGetHostMetrics(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("missing 'target' parameter"), nil
	}

	d, err := s.getDriver(target)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	metrics, err := d.GetMetrics(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get metrics: %v", err)), nil
	}

	data, _ := json.MarshalIndent(metrics, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetJournalLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("missing 'target' parameter"), nil
	}

	unit := req.GetString("unit", "")
	priority := req.GetString("priority", "")
	tail := s.cfg.Defaults.MaxLogLines
	if rawTail := req.GetInt("tail", 0); rawTail > 0 {
		tail = min(tail, rawTail)
	}
	since := req.GetString("since", "")
	filter := req.GetString("filter", "")

	d, err := s.getDriver(target)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	logs, err := d.GetJournalLogs(ctx, driver.JournalQuery{
		Unit:     unit,
		Priority: priority,
		Tail:     tail,
		Since:    since,
		Filter:   filter,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get journal logs: %v", err)), nil
	}

	// Apply byte and line guardrails
	if s.cfg.Defaults.StripANSI {
		logs = guardrail.StripANSI(logs)
	}
	logs, _ = guardrail.TruncateLines(logs, s.cfg.Defaults.MaxLogLines)
	logs, _ = guardrail.TruncateBytes(logs, s.cfg.Defaults.MaxBytes)

	if strings.TrimSpace(logs) == "" {
		return mcp.NewToolResultText("(no journal logs found matching criteria)"), nil
	}

	return mcp.NewToolResultText(logs), nil
}

func (s *Server) handleInspectHostConfig(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("missing 'target' parameter"), nil
	}
	filePath, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("missing 'path' parameter"), nil
	}

	d, err := s.getDriver(target)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	content, err := d.ReadHostConfig(ctx, filePath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read host config: %v", err)), nil
	}

	// Apply byte guardrail
	content, _ = guardrail.TruncateBytes(content, s.cfg.Defaults.MaxBytes)

	return mcp.NewToolResultText(content), nil
}

// Close closes all driver connections.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.drivers {
		_ = d.Close()
	}
}
