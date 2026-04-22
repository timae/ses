package redact

import (
	"strings"
	"testing"

	"github.com/timae/ses/internal/model"
)

func TestPathRule(t *testing.T) {
	r := pathsRule{home: "/Users/tim"}
	got, n := r.Apply("open /Users/tim/project/file.go and /Users/tim/other")
	if n != 2 {
		t.Fatalf("match count: want 2, got %d", n)
	}
	want := "open ~/project/file.go and ~/other"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPathRuleNoHome(t *testing.T) {
	r := pathsRule{home: ""}
	got, n := r.Apply("/Users/tim/foo")
	if n != 0 || got != "/Users/tim/foo" {
		t.Fatalf("no-home should be no-op, got %q n=%d", got, n)
	}
}

func TestGitRemoteRule(t *testing.T) {
	r := gitRemoteRule{}
	cases := []struct {
		in   string
		want string
	}{
		{
			"clone https://tim:ghp_abcdefghij1234567890@github.com/org/repo.git",
			"clone https://<redacted>@github.com/org/repo.git",
		},
		{
			"push ssh://deploy@prod.example.com/var/app",
			"push ssh://<redacted>@prod.example.com/var/app",
		},
		{
			"normal url https://github.com/org/repo", // no creds
			"normal url https://github.com/org/repo",
		},
	}
	for _, c := range cases {
		got, _ := r.Apply(c.in)
		if got != c.want {
			t.Errorf("in=%q\n got=%q\nwant=%q", c.in, got, c.want)
		}
	}
}

func TestSecretRule(t *testing.T) {
	r := secretRule{}
	cases := []struct {
		name  string
		in    string
		label string
	}{
		{"openai", "key is sk-proj-abcdefghij1234567890XYZ", "openai"},
		{"anthropic", "use sk-ant-abcdefghij1234567890XYZ", "anthropic"},
		{"github pat", "token ghp_abcdefghij1234567890XY", "github-pat"},
		{"aws access", "id AKIAIOSFODNN7EXAMPLE", "aws-access-key"},
		{"bearer", "Authorization: Bearer abcdefghij1234567890XY", "bearer"},
		{"env var", "GITHUB_TOKEN=ghp_abcdefghij1234567890", "env-assignment"},
		{"jwt", "jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.abcdefghij1234567890", "jwt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, n := r.Apply(c.in)
			if n == 0 {
				t.Fatalf("expected match for %q, got %q", c.in, got)
			}
			if !strings.Contains(got, "<redacted:"+c.label+">") {
				t.Fatalf("expected label %q in %q", c.label, got)
			}
		})
	}
}

func TestSecretRulePrivateKey(t *testing.T) {
	in := "key:\n-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----\ndone"
	got, n := secretRule{}.Apply(in)
	if n != 1 {
		t.Fatalf("want 1 match, got %d: %q", n, got)
	}
	if strings.Contains(got, "BEGIN PRIVATE KEY") {
		t.Fatalf("private key body leaked: %q", got)
	}
	if !strings.Contains(got, "<redacted:private-key>") {
		t.Fatalf("want private-key label, got %q", got)
	}
}

func TestLongPasteRule(t *testing.T) {
	r := longPasteRule{limit: 100}
	short := strings.Repeat("x", 50)
	if got, n := r.Apply(short); n != 0 || got != short {
		t.Fatalf("short input should pass through")
	}
	long := strings.Repeat("a", 200) + strings.Repeat("b", 200)
	got, n := r.Apply(long)
	if n != 1 {
		t.Fatalf("want 1 truncation, got %d", n)
	}
	if len(got) >= len(long) {
		t.Fatalf("truncated output not shorter: %d >= %d", len(got), len(long))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("missing truncation marker: %q", got)
	}
}

func TestRedactEndToEnd(t *testing.T) {
	msgs := []model.Message{
		{Role: "user", Content: "run in /Users/tim/code and use sk-ant-abcdefghij1234567890XY"},
		{Role: "assistant", Content: "cloned https://tim:ghp_abcdefghij1234567890@github.com/me/repo"},
		{Role: "user", Content: "done", FilePath: "/Users/tim/code/a.go"},
	}
	out, report := Redact(msgs, Options{Mode: ModeDefault, HomeDir: "/Users/tim"})

	if len(out) != len(msgs) {
		t.Fatalf("len mismatch")
	}
	if strings.Contains(out[0].Content, "/Users/tim") {
		t.Errorf("path leaked: %q", out[0].Content)
	}
	if strings.Contains(out[0].Content, "sk-ant-abc") {
		t.Errorf("secret leaked: %q", out[0].Content)
	}
	if strings.Contains(out[1].Content, "ghp_abc") {
		t.Errorf("git cred leaked: %q", out[1].Content)
	}
	if out[2].FilePath != "~/code/a.go" {
		t.Errorf("file path not scrubbed: %q", out[2].FilePath)
	}
	if report.Counts["path"] < 2 {
		t.Errorf("path count low: %v", report.Counts)
	}
	if report.Counts["secret"] < 1 {
		t.Errorf("secret count low: %v", report.Counts)
	}
	if report.Counts["git-remote"] < 1 {
		t.Errorf("git-remote count low: %v", report.Counts)
	}
	if report.Total() < 4 {
		t.Errorf("total low: %d (%v)", report.Total(), report.Counts)
	}
	// Original messages must not be mutated.
	if !strings.Contains(msgs[0].Content, "/Users/tim") {
		t.Errorf("Redact mutated input")
	}
}

func TestRedactModeOff(t *testing.T) {
	msgs := []model.Message{{Role: "user", Content: "sk-ant-abcdefghij1234567890XY at /Users/tim"}}
	out, report := Redact(msgs, Options{Mode: ModeOff, HomeDir: "/Users/tim"})
	if out[0].Content != msgs[0].Content {
		t.Fatalf("ModeOff should not modify content")
	}
	if report.Total() != 0 {
		t.Fatalf("ModeOff should report 0 matches, got %d", report.Total())
	}
}

func TestRedactStrictAddsEmailAndURL(t *testing.T) {
	msgs := []model.Message{{Role: "user", Content: "mail me at person@example.com see https://example.com/secret/path"}}
	out, _ := Redact(msgs, Options{Mode: ModeStrict, HomeDir: "/Users/tim"})
	if strings.Contains(out[0].Content, "person@example.com") {
		t.Errorf("email leaked in strict mode: %q", out[0].Content)
	}
	if strings.Contains(out[0].Content, "/secret/path") {
		t.Errorf("url path leaked in strict mode: %q", out[0].Content)
	}
}
