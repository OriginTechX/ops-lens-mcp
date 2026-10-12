package guardrail

import (
	"bytes"
	"regexp"
	"strings"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes terminal ANSI color/style escape codes.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// TruncateLines keeps at most maxLines (from the tail end).
func TruncateLines(s string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		return s, false
	}

	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s, false
	}

	truncated := strings.Join(lines[len(lines)-maxLines:], "\n")
	return truncated, true
}

// TruncateBytes trims output if it exceeds maxBytes, preserving the tail end.
func TruncateBytes(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false
	}

	// Keep the most recent data (tail)
	tail := s[len(s)-maxBytes:]
	// Advance to next newline if possible to avoid split line
	if idx := strings.IndexByte(tail, '\n'); idx != -1 && idx < 100 {
		tail = tail[idx+1:]
	}
	return "...[TRUNCATED TO MAX BYTES]...\n" + tail, true
}

// FilterLines filters lines matching any keyword or regex pattern.
func FilterLines(s, pattern string) string {
	if pattern == "" {
		return s
	}

	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return s
	}

	var buf bytes.Buffer
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if re.MatchString(line) {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	return strings.TrimRight(buf.String(), "\n")
}
