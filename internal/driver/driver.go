package driver

import (
	"context"
)

// WorkloadStatus describes the runtime status of a service or pod.
type WorkloadStatus struct {
	Name      string            `json:"name"`
	Target    string            `json:"target"`
	State     string            `json:"state"` // Running, CrashLoopBackOff, Restarting, Stopped, etc.
	Restarts  int32             `json:"restarts"`
	Uptime    string            `json:"uptime,omitempty"`
	Image     string            `json:"image,omitempty"`
	Health    string            `json:"health,omitempty"` // Healthy, Unhealthy, None
	Details   map[string]string `json:"details,omitempty"`
}

// HostMetrics captures CPU, RAM, Disk, and load statistics.
type HostMetrics struct {
	Target      string `json:"target"`
	Uptime      string `json:"uptime"`
	LoadAvg     string `json:"load_avg"`
	MemoryTotal string `json:"memory_total"`
	MemoryFree  string `json:"memory_free"`
	MemoryUsed  string `json:"memory_used"`
	DiskUsage   string `json:"disk_usage"`
	Raw         string `json:"raw,omitempty"`
}

// LogQuery defines parameters for log retrieval.
type LogQuery struct {
	Service string
	Tail    int
	Since   string // e.g. "15m", "1h"
	Filter  string // regex or keyword filter
}

// JournalQuery defines parameters for systemd journal inspection.
type JournalQuery struct {
	Unit     string // systemd unit name (e.g., "traefik", "docker", "nginx")
	Priority string // e.g. "err", "warning", "info" (maps to -p)
	Tail     int    // number of lines
	Since    string // e.g. "1h", "24h"
	Filter   string // regex or keyword filter
}

// Driver defines the standard diagnostic interface across both K8s and Docker.
type Driver interface {
	// ListServices lists observable services, containers, or pods.
	ListServices(ctx context.Context) ([]string, error)

	// GetStatus checks health and state of a specific service or workload.
	GetStatus(ctx context.Context, service string) (*WorkloadStatus, error)

	// GetLogs fetches bounded log streams with optional regex filtering.
	GetLogs(ctx context.Context, query LogQuery) (string, error)

	// DescribeWorkload returns deep diagnostics (exit codes, events, restarts, termination reason).
	DescribeWorkload(ctx context.Context, service string) (string, error)

	// GetMetrics returns node / host resource saturation (CPU, RAM, Disk).
	GetMetrics(ctx context.Context) (*HostMetrics, error)

	// GetJournalLogs inspects host systemd logs (e.g. host Traefik, Docker daemon, OS errors).
	GetJournalLogs(ctx context.Context, query JournalQuery) (string, error)

	// ReadHostConfig safely inspects host configuration files within safe directories.
	ReadHostConfig(ctx context.Context, filePath string) (string, error)

	// Close cleans up network connections or clients.
	Close() error
}
