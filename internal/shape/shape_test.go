package shape

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Shape
	}{
		{"empty", "", Text},
		{"whitespace", "   \n\n  ", Text},

		{"json object", `{"name":"alice","age":30}`, JSON},
		{"json array", `[1,2,3,4]`, JSON},
		{"json indented", "  {\n  \"user\": {\"id\": 42}\n}", JSON},
		{"json truncated", `{"name":"alice","age":30,"addresses":[{"city":`, JSON},
		{"json-ish unbalanced", `{"error":"oops","stack":"..."`, JSON},

		{"xml simple", `<note><to>alice</to></note>`, XML},
		{"xml with attrs", `<user id="42" />`, XML},
		{"xml indented", "  <root>\n  <child/>\n</root>", XML},

		{"yaml triple dash", "---\nname: foo\nage: 3\n", YAML},
		{"yaml plain", "name: foo\nage: 3\nactive: true\n", YAML},

		{"log iso", "2026-04-21 10:00:00 INFO started\n2026-04-21 10:00:01 ERROR oops", Log},
		{"log bracketed", "[INFO] boot\n[ERROR] something broke\n[INFO] recovered", Log},
		{"log level-colon", "INFO: running\nERROR: crashed\nINFO: restarted", Log},

		{"prose", "Hey can you help me understand what this function does and why it's slow?", Text},
		{"single yaml line ambiguous", "status: broken", Text},
		{"single word", "wat", Text},
		{"code snippet not logs", "func main() {\n  fmt.Println(\"hi\")\n}", Text},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.in)
			if got != c.want {
				t.Fatalf("Classify(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
