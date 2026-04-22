// Package redact scrubs transcripts before they leave the user's machine.
//
// A Redact call takes a slice of messages and returns a scrubbed copy plus a
// Report summarizing what was touched. The Report is the UX surface used by
// `ses share` to show the user what will be uploaded before they confirm.
package redact

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/timae/ses/internal/model"
)

// Mode controls which rules run.
type Mode int

const (
	ModeOff     Mode = iota // no redaction (dangerous; gated behind explicit flag)
	ModeDefault             // paths, secrets, git-remote creds, long-paste truncation
	ModeStrict              // default + aggressive URL/email scrubbing, tighter paste cap
)

// Options controls a Redact call.
type Options struct {
	Mode       Mode
	HomeDir    string // if empty, os.UserHomeDir() is used
	PasteLimit int    // max chars per message before truncation; 0 picks mode default
}

// Report summarizes a Redact run.
type Report struct {
	Counts map[string]int // rule name -> number of matches replaced
	Bytes  int            // total chars removed
}

func (r Report) Total() int {
	n := 0
	for _, c := range r.Counts {
		n += c
	}
	return n
}

// Rule is a single redaction pass over a string.
type Rule interface {
	Name() string
	Apply(s string) (out string, matches int)
}

// Redact scrubs every message Content field according to opts and returns a
// copy plus a Report. Input messages are not mutated.
func Redact(messages []model.Message, opts Options) ([]model.Message, Report) {
	rules := rulesFor(opts)
	out := make([]model.Message, len(messages))
	report := Report{Counts: map[string]int{}}

	for i, m := range messages {
		content := m.Content
		before := len(content)
		for _, rule := range rules {
			scrubbed, n := rule.Apply(content)
			if n > 0 {
				report.Counts[rule.Name()] += n
			}
			content = scrubbed
		}
		report.Bytes += before - len(content)
		m.Content = content
		// FilePath also leaks the user's machine — scrub with path rule only.
		if opts.Mode != ModeOff && m.FilePath != "" {
			scrubbed, n := pathRule(opts).Apply(m.FilePath)
			if n > 0 {
				report.Counts["path"] += n
			}
			m.FilePath = scrubbed
		}
		out[i] = m
	}
	return out, report
}

func rulesFor(opts Options) []Rule {
	if opts.Mode == ModeOff {
		return nil
	}
	rules := []Rule{
		pathRule(opts),
		gitRemoteRule{},
		secretRule{},
	}
	limit := opts.PasteLimit
	if limit == 0 {
		switch opts.Mode {
		case ModeStrict:
			limit = 2000
		default:
			limit = 8000
		}
	}
	rules = append(rules, longPasteRule{limit: limit})
	if opts.Mode == ModeStrict {
		rules = append(rules, emailRule{}, urlRule{})
	}
	return rules
}

// --- path rule -------------------------------------------------------------

type pathsRule struct {
	home string
}

func pathRule(opts Options) pathsRule {
	home := opts.HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return pathsRule{home: home}
}

func (p pathsRule) Name() string { return "path" }

func (p pathsRule) Apply(s string) (string, int) {
	if p.home == "" {
		return s, 0
	}
	n := strings.Count(s, p.home)
	if n == 0 {
		return s, 0
	}
	return strings.ReplaceAll(s, p.home, "~"), n
}

// --- git remote rule -------------------------------------------------------
//
// Catches `https://user:token@host/...` and `ssh://user@host/...` where the
// user portion is a real credential, not a GitHub-style `git@github.com`.

type gitRemoteRule struct{}

func (gitRemoteRule) Name() string { return "git-remote" }

var (
	httpCredRe = regexp.MustCompile(`https?://[^\s:/@]+:[^\s@]+@[^\s]+`)
	sshCredRe  = regexp.MustCompile(`ssh://[^\s@/]+@[^\s]+`)
)

func (r gitRemoteRule) Apply(s string) (string, int) {
	total := 0
	s = httpCredRe.ReplaceAllStringFunc(s, func(m string) string {
		total++
		// Preserve scheme + host so context survives.
		if i := strings.Index(m, "@"); i >= 0 {
			scheme := m[:strings.Index(m, "//")+2]
			return scheme + "<redacted>@" + m[i+1:]
		}
		return "<redacted-url>"
	})
	s = sshCredRe.ReplaceAllStringFunc(s, func(m string) string {
		total++
		return "ssh://<redacted>" + m[strings.Index(m, "@"):]
	})
	return s, total
}

// --- secret rule -----------------------------------------------------------
//
// Pattern-based sweep for well-known secret shapes. Intentionally
// conservative: false negatives are worse than false positives here, so we
// bias toward matching and replace with a labeled placeholder that tells the
// user which rule fired.

type secretRule struct{}

func (secretRule) Name() string { return "secret" }

type secretPattern struct {
	label string
	re    *regexp.Regexp
}

// Patterns try to match the full secret, not just the prefix, so replacement
// doesn't leave half the token behind.
var secretPatterns = []secretPattern{
	// Anthropic must precede OpenAI — sk-ant-... would otherwise match the
	// broader sk-… pattern first and get the wrong label.
	{"anthropic", regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`)},
	{"openai", regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9_\-]{20,}`)},
	{"github-pat", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{"github-oauth", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)},
	{"aws-access-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"aws-secret", regexp.MustCompile(`(?i)aws(.{0,20})?(secret|access)(.{0,20})?['"= ]+[A-Za-z0-9/+=]{40}`)},
	{"bearer", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{20,}`)},
	{"private-key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----`)},
	{"env-assignment", regexp.MustCompile(`(?m)^\s*(?:[A-Z][A-Z0-9_]*(?:TOKEN|SECRET|KEY|PASSWORD|PASSWD|API|AUTH))\s*=\s*['"]?[^\s'"]{8,}['"]?`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)},
}

func (r secretRule) Apply(s string) (string, int) {
	total := 0
	for _, p := range secretPatterns {
		matched := 0
		s = p.re.ReplaceAllStringFunc(s, func(string) string {
			matched++
			return "<redacted:" + p.label + ">"
		})
		total += matched
	}
	return s, total
}

// --- long paste rule ------------------------------------------------------

type longPasteRule struct {
	limit int
}

func (longPasteRule) Name() string { return "long-paste" }

func (r longPasteRule) Apply(s string) (string, int) {
	if r.limit <= 0 || len(s) <= r.limit {
		return s, 0
	}
	head := r.limit / 2
	tail := r.limit - head - 64
	if tail < 0 {
		tail = 0
	}
	marker := "\n\n…[truncated " + strconv.Itoa(len(s)-head-tail) + " chars]…\n\n"
	return s[:head] + marker + s[len(s)-tail:], 1
}

// --- strict-mode rules ----------------------------------------------------

type emailRule struct{}

func (emailRule) Name() string { return "email" }

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

func (emailRule) Apply(s string) (string, int) {
	n := 0
	s = emailRe.ReplaceAllStringFunc(s, func(string) string {
		n++
		return "<redacted:email>"
	})
	return s, n
}

type urlRule struct{}

func (urlRule) Name() string { return "url" }

var urlRe = regexp.MustCompile(`https?://[^\s)\]>'"]+`)

func (urlRule) Apply(s string) (string, int) {
	n := 0
	s = urlRe.ReplaceAllStringFunc(s, func(m string) string {
		n++
		// Keep scheme+host, drop path/query where tokens often hide.
		if i := strings.Index(m[8:], "/"); i > 0 {
			return m[:8+i] + "/<redacted-path>"
		}
		return m
	})
	return s, n
}

