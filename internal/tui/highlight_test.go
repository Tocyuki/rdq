package tui

import (
	"strings"
	"testing"
)

func TestShortenModelID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"us.anthropic.claude-sonnet-4-5-20250929-v1:0", "claude-sonnet-4-5-20250929"},
		{"anthropic.claude-3-5-sonnet-20241022-v2:0", "claude-3-5-sonnet-20241022"},
		{"meta.llama3-70b-instruct-v1:0", "llama3-70b-instruct"},
		{"apac.anthropic.claude-sonnet-4-20250514-v1:0", "claude-sonnet-4-20250514"},
		{"plain-id", "plain-id"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := shortenModelID(tc.in)
			if got != tc.want {
				t.Errorf("shortenModelID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHighlightSQLProducesAnsi(t *testing.T) {
	out := highlightSQL("SELECT * FROM users WHERE id = 1;")
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	// terminal256 formatter emits CSI escape sequences (ESC [ ...).
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes in output:\n%q", out)
	}
}

func TestHighlightSQLEmpty(t *testing.T) {
	if got := highlightSQL(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestHighlightJSONProducesAnsi(t *testing.T) {
	out := highlightJSON(`{"id": 1, "name": "alice"}`)
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes in output:\n%q", out)
	}
}

func TestPadToHeight(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		height    int
		wantLines int
	}{
		{"pad short", "one\ntwo", 5, 5},
		{"truncate long", "a\nb\nc\nd\ne", 3, 3},
		{"exact", "a\nb\nc", 3, 3},
		{"zero", "a\nb", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := padToHeight(tc.in, tc.height)
			if tc.height == 0 {
				if out != "" {
					t.Errorf("expected empty, got %q", out)
				}
				return
			}
			lines := strings.Split(out, "\n")
			if len(lines) != tc.wantLines {
				t.Errorf("got %d lines, want %d:\n%q", len(lines), tc.wantLines, out)
			}
		})
	}
}
