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
