package bedrock

import (
	"strings"
	"testing"

	"github.com/Tocyuki/rdq/internal/schema"
)

func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "SELECT 1;", "SELECT 1;"},
		{"sql fence", "```sql\nSELECT 1;\n```", "SELECT 1;"},
		{"unlabeled fence", "```\nSELECT 1;\n```", "SELECT 1;"},
		{"trailing whitespace", "```sql\nSELECT 1;\n```   \n", "SELECT 1;"},
		{"multiline", "```sql\nSELECT id\nFROM users;\n```", "SELECT id\nFROM users;"},
		{"no fence multiline", "SELECT id\nFROM users;", "SELECT id\nFROM users;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCodeFence(tc.in)
			if got != tc.want {
				t.Errorf("stripCodeFence(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildSystemPromptIncludesSchema(t *testing.T) {
	snap := &schema.Snapshot{
		Database: "myapp",
		Columns: []schema.Column{
			{Schema: "myapp", Table: "users", Name: "id", Type: "bigint"},
			{Schema: "myapp", Table: "users", Name: "email", Type: "varchar"},
		},
	}
	got := BuildSystemPrompt("myapp", "Japanese", snap)
	if !strings.Contains(got, "Active database: myapp") {
		t.Errorf("expected active database in prompt:\n%s", got)
	}
	if !strings.Contains(got, "TABLE users") {
		t.Errorf("expected schema table in prompt:\n%s", got)
	}
	if !strings.Contains(got, "Output ONLY the SQL statement") {
		t.Errorf("expected SUCCESS-mode rule in prompt:\n%s", got)
	}
	if !strings.Contains(got, "CANNOT GENERATE") {
		t.Errorf("expected CANNOT GENERATE branch in prompt:\n%s", got)
	}
	if !strings.Contains(got, "-- Cannot generate SQL.") {
		t.Errorf("expected SQL-comment template in prompt:\n%s", got)
	}
	if !strings.Contains(got, "Respond to the user in Japanese") {
		t.Errorf("expected language directive in prompt:\n%s", got)
	}
}

func TestBuildSystemPromptHandlesEmptySchema(t *testing.T) {
	got := BuildSystemPrompt("myapp", "", &schema.Snapshot{})
	if !strings.Contains(got, "(no schema available)") {
		t.Errorf("expected fallback for empty schema:\n%s", got)
	}
	if strings.Contains(got, "Respond to the user in") {
		t.Errorf("did not expect language directive when language is empty:\n%s", got)
	}
}

func TestBuildSystemPromptNilSnapshotDoesNotPanic(t *testing.T) {
	got := BuildSystemPrompt("myapp", "", nil)
	if !strings.Contains(got, "Active database: myapp") {
		t.Errorf("expected active database in prompt:\n%s", got)
	}
	if strings.Contains(got, "Schema:") {
		t.Errorf("did not expect schema section without snapshot:\n%s", got)
	}
}

func TestBuildErrorExplanationPromptIncludesSchema(t *testing.T) {
	snap := &schema.Snapshot{
		Database: "myapp",
		Columns: []schema.Column{
			{Schema: "myapp", Table: "users", Name: "id", Type: "bigint"},
		},
	}
	got := BuildErrorExplanationPrompt("myapp", "Japanese", snap)
	if !strings.Contains(got, "SQL error analyst") {
		t.Errorf("expected analyst preamble:\n%s", got)
	}
	if !strings.Contains(got, "Active database: myapp") {
		t.Errorf("expected database line:\n%s", got)
	}
	if !strings.Contains(got, "TABLE users") {
		t.Errorf("expected schema content:\n%s", got)
	}
	if !strings.Contains(got, "Respond to the user in Japanese") {
		t.Errorf("expected language directive in prompt:\n%s", got)
	}
}

func TestBuildErrorExplanationPromptNilSnapshot(t *testing.T) {
	got := BuildErrorExplanationPrompt("myapp", "", nil)
	if !strings.Contains(got, "SQL error analyst") {
		t.Errorf("expected analyst preamble even without schema:\n%s", got)
	}
	if strings.Contains(got, "Schema:") {
		t.Errorf("did not expect schema section when snapshot is nil:\n%s", got)
	}
}

func TestBuildErrorUserPromptFormatsBothFields(t *testing.T) {
	got := BuildErrorUserPrompt("  SELECT * FROM userz;  ", "  ERROR 1146: Table 'myapp.userz' doesn't exist  ")
	if !strings.Contains(got, "SQL:\nSELECT * FROM userz;") {
		t.Errorf("expected trimmed SQL section:\n%s", got)
	}
	if !strings.Contains(got, "Error message:\nERROR 1146") {
		t.Errorf("expected trimmed error section:\n%s", got)
	}
}
