package guardrail

import (
	"testing"
)

func TestStripANSI(t *testing.T) {
	colored := "\x1b[31mError:\x1b[0m Connection failed"
	expected := "Error: Connection failed"
	if got := StripANSI(colored); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestTruncateLines(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5"
	got, truncated := TruncateLines(input, 3)
	if !truncated {
		t.Fatalf("expected truncated to be true")
	}
	expected := "line3\nline4\nline5"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestFilterLines(t *testing.T) {
	input := "info: start\nerror: timeout\ninfo: retry\nFATAL: crash"
	got := FilterLines(input, "error|fatal")
	expected := "error: timeout\nFATAL: crash"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestIsSafeHostConfigPath(t *testing.T) {
	tests := []struct {
		path string
		safe bool
	}{
		{"/etc/traefik/traefik.yml", true},
		{"/etc/traefik/dynamic_conf/dmei.yml", true},
		{"/etc/nginx/nginx.conf", true},
		{"/var/log/syslog", true},
		{"/etc/shadow", false},
		{"/etc/traefik/../shadow", false},
		{"/root/.ssh/id_ed25519", false},
		{"/home/user/.env", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := IsSafeHostConfigPath(tc.path); got != tc.safe {
			t.Errorf("IsSafeHostConfigPath(%q) = %v, want %v", tc.path, got, tc.safe)
		}
	}
}
