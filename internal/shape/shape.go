// Package shape classifies a string by surface form (JSON, log, XML, YAML,
// text) using cheap heuristics. It exists so `ses pastes --shape json` can
// filter a user's historical pastes without any indexing or ML — shape is
// detected at query time and "good enough for eyeball retrieval."
package shape

import (
	"regexp"
	"strings"
)

type Shape string

const (
	JSON Shape = "json"
	Log  Shape = "log"
	XML  Shape = "xml"
	YAML Shape = "yaml"
	Text Shape = "text"
)

// All returns every known shape (excluding Text) for CLI validation.
func All() []Shape {
	return []Shape{JSON, Log, XML, YAML}
}

// Classify inspects the first few non-empty lines of s and picks the most
// likely shape. The order of checks matters: more specific shapes must come
// before more general ones (e.g. XML before "starts with < means text").
func Classify(s string) Shape {
	s = strings.TrimLeft(s, " \t\r\n")
	if s == "" {
		return Text
	}

	switch {
	case looksJSON(s):
		return JSON
	case looksXML(s):
		return XML
	// Log before YAML: "INFO: started" matches both the YAML key:value and
	// log level-colon patterns. A line starting with a known log level is a
	// stronger signal than a generic key:value line.
	case looksLog(s):
		return Log
	case looksYAML(s):
		return YAML
	}
	return Text
}

func looksJSON(s string) bool {
	if len(s) < 2 {
		return false
	}
	first := s[0]
	if first != '{' && first != '[' {
		return false
	}
	// Trailing brace/bracket is a strong signal — we don't do a full parse
	// because pastes are often truncated or concatenated.
	last := s[len(s)-1]
	if first == '{' && last == '}' {
		return true
	}
	if first == '[' && last == ']' {
		return true
	}
	// Even unbalanced: if we see `"key":` near the start it's almost
	// certainly JSON-ish.
	head := s
	if len(head) > 512 {
		head = head[:512]
	}
	return jsonKeyRe.MatchString(head)
}

var jsonKeyRe = regexp.MustCompile(`"[A-Za-z_][A-Za-z0-9_\-]*"\s*:`)

func looksXML(s string) bool {
	if len(s) < 3 || s[0] != '<' {
		return false
	}
	// Ignore XML comments/declarations that might start HTML/config dumps.
	head := s
	if len(head) > 256 {
		head = head[:256]
	}
	return xmlTagRe.MatchString(head)
}

var xmlTagRe = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_:\-]*)(\s|>|/)`)

func looksYAML(s string) bool {
	// YAML document separator is the strongest signal.
	if strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n") {
		return true
	}
	// Otherwise: look for `key: value` patterns at line starts across
	// multiple lines, and NO JSON-style {/[ at top level.
	if s[0] == '{' || s[0] == '[' || s[0] == '<' {
		return false
	}
	lines := splitLines(s, 10)
	hits := 0
	for _, line := range lines {
		if yamlKeyRe.MatchString(line) {
			hits++
		}
	}
	// Need at least a couple of key:value lines to call it YAML. A single
	// "foo: bar" line is ambiguous with plain prose.
	return hits >= 2
}

var yamlKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_\-]*\s*:\s+\S`)

func looksLog(s string) bool {
	lines := splitLines(s, 10)
	if len(lines) < 2 {
		return false
	}
	hits := 0
	for _, line := range lines {
		if logLineRe.MatchString(line) {
			hits++
		}
	}
	return hits >= 2
}

// logLineRe matches common log prefixes: ISO-ish timestamp, bracketed level,
// or classic `LEVEL: message` shapes. Intentionally forgiving.
var logLineRe = regexp.MustCompile(
	`^(?:\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}|\[[A-Z]+\]|(?:DEBUG|INFO|WARN|WARNING|ERROR|FATAL|TRACE)[: ])`,
)

func splitLines(s string, max int) []string {
	out := make([]string, 0, max)
	for len(s) > 0 && len(out) < max {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			line := strings.TrimSpace(s)
			if line != "" {
				out = append(out, line)
			}
			break
		}
		line := strings.TrimSpace(s[:i])
		if line != "" {
			out = append(out, line)
		}
		s = s[i+1:]
	}
	return out
}
