package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/frank/ops-lens-mcp/internal/config"
	"github.com/frank/ops-lens-mcp/internal/driver"
	"github.com/frank/ops-lens-mcp/internal/guardrail"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Driver implements driver.Driver for Kubernetes using client-go.
type Driver struct {
	targetName string
	cfg        config.Target
	client     *kubernetes.Clientset
	namespace  string
}

// New creates a new Kubernetes diagnostic driver.
func New(targetName string, cfg config.Target) (*Driver, error) {
	kubeconfigPath := cfg.Kubeconfig
	if kubeconfigPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	} else if strings.HasPrefix(kubeconfigPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfigPath = filepath.Join(home, kubeconfigPath[2:])
		}
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}
	if cfg.Context != "" {
		configOverrides.CurrentContext = cfg.Context
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	ns := cfg.Namespace
	if ns == "" {
		if rawNs, _, err := kubeConfig.Namespace(); err == nil && rawNs != "" {
			ns = rawNs
		} else {
			ns = "default"
		}
	}

	return &Driver{
		targetName: targetName,
		cfg:        cfg,
		client:     clientset,
		namespace:  ns,
	}, nil
}

// ListServices lists deployments and pods in the target namespace.
func (d *Driver) ListServices(ctx context.Context) ([]string, error) {
	pods, err := d.client.CoreV1().Pods(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	var names []string
	for _, p := range pods.Items {
		names = append(names, p.Name)
	}
	return names, nil
}

// GetStatus checks pod phase and container restart counts.
func (d *Driver) GetStatus(ctx context.Context, service string) (*driver.WorkloadStatus, error) {
	pod, err := d.client.CoreV1().Pods(d.namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %q: %w", service, err)
	}

	var restarts int32
	var state string = string(pod.Status.Phase)
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			state = cs.State.Waiting.Reason
		}
	}

	return &driver.WorkloadStatus{
		Name:     pod.Name,
		Target:   d.targetName,
		State:    state,
		Restarts: restarts,
		Uptime:   pod.CreationTimestamp.Time.Format("2006-01-02 15:04:05"),
		Health:   string(pod.Status.Phase),
	}, nil
}

// GetLogs fetches pod logs with tail limits.
func (d *Driver) GetLogs(ctx context.Context, query driver.LogQuery) (string, error) {
	tailLines := int64(query.Tail)
	if tailLines <= 0 {
		tailLines = 200
	}

	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
	}

	req := d.client.CoreV1().Pods(d.namespace).GetLogs(query.Service, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("stream pod logs: %w", err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, stream); err != nil {
		return "", fmt.Errorf("read pod logs: %w", err)
	}

	cleaned := guardrail.StripANSI(buf.String())
	if query.Filter != "" {
		cleaned = guardrail.FilterLines(cleaned, query.Filter)
	}
	return cleaned, nil
}

// DescribeWorkload returns pod exit codes, events, and termination details.
func (d *Driver) DescribeWorkload(ctx context.Context, service string) (string, error) {
	pod, err := d.client.CoreV1().Pods(d.namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod: %w", err)
	}

	// Fetch events related to this pod
	events, _ := d.client.CoreV1().Events(d.namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", service),
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Pod: %s (Namespace: %s)\n", pod.Name, pod.Namespace))
	b.WriteString(fmt.Sprintf("Phase: %s | Node: %s | IP: %s\n", pod.Status.Phase, pod.Spec.NodeName, pod.Status.PodIP))

	b.WriteString("\n--- Containers ---\n")
	for _, cs := range pod.Status.ContainerStatuses {
		b.WriteString(fmt.Sprintf("Container %s: Ready=%v, Restarts=%d\n", cs.Name, cs.Ready, cs.RestartCount))
		if cs.State.Waiting != nil {
			b.WriteString(fmt.Sprintf("  Waiting: %s - %s\n", cs.State.Waiting.Reason, cs.State.Waiting.Message))
		}
		if cs.State.Terminated != nil {
			b.WriteString(fmt.Sprintf("  Terminated: ExitCode=%d Reason=%s Message=%s\n", cs.State.Terminated.ExitCode, cs.State.Terminated.Reason, cs.State.Terminated.Message))
		}
	}

	if events != nil && len(events.Items) > 0 {
		b.WriteString("\n--- Recent Events ---\n")
		for _, e := range events.Items {
			b.WriteString(fmt.Sprintf("[%s] %s: %s\n", e.Type, e.Reason, e.Message))
		}
	}

	return b.String(), nil
}

// GetMetrics returns cluster node summary.
func (d *Driver) GetMetrics(ctx context.Context) (*driver.HostMetrics, error) {
	nodes, err := d.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Cluster Nodes: %d\n", len(nodes.Items)))
	for _, n := range nodes.Items {
		ready := "NotReady"
		for _, cond := range n.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = "Ready"
				break
			}
		}
		b.WriteString(fmt.Sprintf("Node: %s (%s) | InternalIP: %s | Arch: %s/%s\n",
			n.Name, ready, n.Status.Addresses[0].Address, n.Status.NodeInfo.OperatingSystem, n.Status.NodeInfo.Architecture))
	}

	return &driver.HostMetrics{
		Target: d.targetName,
		Raw:    b.String(),
	}, nil
}

// Close is a no-op for client-go.
func (d *Driver) Close() error {
	return nil
}
