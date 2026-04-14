package bedrock

import (
	"fmt"
	"strings"

	"github.com/Tocyuki/rdq/internal/schema"
)

// sqlAssistantPrompt instructs the model to behave as a focused SQL assistant
// for the AWS RDS Data API. The reply is pasted directly into the editor, so
// the wording is strict about output format: either runnable SQL or pure
// SQL-comment explanation, never a mix.
const sqlAssistantPrompt = `You are a SQL assistant for AWS Aurora accessed via the RDS Data API. Your single job is to translate a natural-language request into one runnable SQL statement against the schema below.

OUTPUT FORMAT (strict)

The user will paste your reply directly into a SQL editor, so the output must always be valid SQL syntax. You have exactly two output modes — never mix them:

1. SUCCESS — you can produce SQL:
   - Output ONLY the SQL statement.
   - No markdown fences, no prose, no leading or trailing commentary.
   - Use only tables and columns that exist in the schema below.
   - Prefer explicit column names over SELECT *.
   - End every statement with a semicolon.
   - If the request is ambiguous but a reasonable interpretation exists, pick the most likely one and proceed.

2. CANNOT GENERATE — you cannot produce SQL:
   - Output ONLY SQL comments (every line must start with "-- ").
   - Do NOT include any runnable SQL, even as an example.
   - Structure your reply as:
     -- Cannot generate SQL.
     -- Reason: <one or two short sentences explaining why>
     -- Need: <what additional information would let you generate it>
     -- Closest alternative: <optional: describe in plain words the closest query you could write if the missing pieces were filled in>
   - Cover any of these cases: the request references tables or columns that are not in the schema; the request is not a database operation; the request requires information you cannot infer (date ranges, ID values, business rules); the request asks for something the RDS Data API does not support (DDL across schemas, multi-statement transactions, server-side functions you can't see, etc.); the request is ambiguous in a way that picking a default would be misleading.

Always honour the language directive below for any natural-language text inside SQL comments.`

// BuildSystemPrompt assembles the system message sent to Bedrock. The
// database name and schema listing come from the live cluster so the model
// can resolve table and column references accurately. snapshot may be nil
// when the background fetch hasn't finished yet — in that case the prompt
// goes out without schema rather than panicking. language is appended as a
// natural-language directive so the assistant prefers the user's preferred
// tongue for any commentary it does emit.
func BuildSystemPrompt(database, language string, snapshot *schema.Snapshot) string {
	var b strings.Builder
	b.WriteString(sqlAssistantPrompt)
	b.WriteString("\n\n")

	if database != "" {
		fmt.Fprintf(&b, "Active database: %s\n\n", database)
	}

	writeLanguageDirective(&b, language)

	if snapshot != nil {
		b.WriteString("Schema:\n")
		b.WriteString(snapshot.ToPrompt())
	}
	return b.String()
}

// writeLanguageDirective appends a single instruction telling the assistant
// which natural language to respond in. SQL itself is language-neutral, so
// this only affects commentary, error explanations, and fall-through cases
// where the model needs to talk to the user.
func writeLanguageDirective(b *strings.Builder, language string) {
	if strings.TrimSpace(language) == "" {
		return
	}
	fmt.Fprintf(b, "Respond to the user in %s. SQL identifiers and keywords stay in English.\n\n", language)
}

// writeFocusArea appends a "Focus area: <text>" block to a user prompt
// when focus is non-empty. tailHint is the parenthetical follow-up that
// nudges the model towards (or away from) tangential concerns; callers
// pick wording appropriate to their flow ("other concerns", "other
// patterns", etc.).
func writeFocusArea(b *strings.Builder, focus, tailHint string) {
	focus = strings.TrimSpace(focus)
	if focus == "" {
		return
	}
	b.WriteString("\n\nFocus area: ")
	b.WriteString(focus)
	b.WriteString("\n(")
	b.WriteString(tailHint)
	b.WriteString(")")
}

// errorAssistantPrompt instructs the model to act as a SQL error analyst.
// Output is markdown-friendly because the result is rendered in a viewport
// rather than pasted into the editor. The TUI prepends the verbatim error
// message above the model's response, so the model itself should focus on
// root cause + fix without re-quoting the entire error verbatim.
const errorAssistantPrompt = "You are a SQL error analyst for AWS Aurora accessed via the RDS Data API. The user ran a SQL statement and received an error. Explain what went wrong and how to fix it.\n\n" +
	"Rules:\n" +
	"- Be concise: at most 4-6 short paragraphs or bullet points.\n" +
	"- Identify the root cause in plain language first.\n" +
	"- The full verbatim error message will be displayed above your response by the UI, so do not repeat it word-for-word — quote only the short fragment you are commenting on.\n" +
	"- Suggest a corrected SQL statement (or the smallest fix) inside a fenced ```sql block.\n" +
	"- If the schema below shows the user is referencing a non-existent table or column, point out the closest match.\n" +
	"- Output plain text and GitHub-flavored markdown. The user reads this in a terminal viewport."

// BuildErrorExplanationPrompt assembles the system message for the error
// analyst flow. snapshot may be nil; the schema section is omitted in that
// case so the prompt still renders. language directs the model to write its
// explanation in the user's preferred language.
func BuildErrorExplanationPrompt(database, language string, snapshot *schema.Snapshot) string {
	var b strings.Builder
	b.WriteString(errorAssistantPrompt)
	b.WriteString("\n\n")

	if database != "" {
		fmt.Fprintf(&b, "Active database: %s\n\n", database)
	}

	writeLanguageDirective(&b, language)

	if snapshot != nil {
		b.WriteString("Schema:\n")
		b.WriteString(snapshot.ToPrompt())
	}
	return b.String()
}

// BuildErrorUserPrompt formats the SQL + error message into the single user
// turn we send to the model. Both fields are trimmed so the layout stays
// stable regardless of how the caller assembled them.
func BuildErrorUserPrompt(sql, errMsg string) string {
	var b strings.Builder
	b.WriteString("SQL:\n")
	b.WriteString(strings.TrimSpace(sql))
	b.WriteString("\n\nError message:\n")
	b.WriteString(strings.TrimSpace(errMsg))
	return b.String()
}

// reviewAssistantPrompt asks the model to act as a senior SQL reviewer.
// The output is rendered in a markdown viewport, so the format prioritises
// short scannable bullets.
const reviewAssistantPrompt = "You are a senior SQL reviewer for AWS Aurora accessed via the RDS Data API. The user shows you a SQL statement they are about to run; review it for correctness, performance, safety, and clarity.\n\n" +
	"Rules:\n" +
	"- Be concrete and concise. Prefer short bullet points over paragraphs.\n" +
	"- Cover, in order: correctness (does it do what the user likely wants?), performance (indexes, scans, N+1, expensive joins), safety (destructive intent, missing WHERE, transaction concerns), style (naming, readability).\n" +
	"- If the SQL is fine, say so explicitly with a one-line summary instead of inventing problems.\n" +
	"- When suggesting an improved version, put it in a fenced ```sql block so the user can copy it.\n" +
	"- Use the schema below to verify table / column names. Flag references that do not exist and suggest the closest match.\n" +
	"- Output GitHub-flavored markdown."

// BuildReviewSystemPrompt assembles the system message for SQL review. It
// reuses the database / language directives and embeds the schema so the
// reviewer can verify identifiers without guessing.
func BuildReviewSystemPrompt(database, language string, snapshot *schema.Snapshot) string {
	var b strings.Builder
	b.WriteString(reviewAssistantPrompt)
	b.WriteString("\n\n")
	if database != "" {
		fmt.Fprintf(&b, "Active database: %s\n\n", database)
	}
	writeLanguageDirective(&b, language)
	if snapshot != nil {
		b.WriteString("Schema:\n")
		b.WriteString(snapshot.ToPrompt())
	}
	return b.String()
}

// BuildReviewUserPrompt is the user-side message. focus is an optional
// natural-language hint about what the user wants the review to emphasise
// (e.g. "performance", "join correctness", "indexes I should add"). When
// empty the model defaults to a balanced general review.
func BuildReviewUserPrompt(sql, focus string) string {
	var b strings.Builder
	b.WriteString("Review this SQL statement:\n\n")
	b.WriteString(strings.TrimSpace(sql))
	writeFocusArea(&b, focus, "Concentrate on this aspect; mention other concerns only if they are critical.")
	return b.String()
}

// analyzeAssistantPrompt asks the model to explain a query result to the
// user. Output is markdown so it renders nicely in the explain viewport.
const analyzeAssistantPrompt = "You are a data analyst helping the user understand a SQL query result from AWS Aurora. The user has just run a query and you receive both the original SQL and the result rows; explain what the result shows.\n\n" +
	"Rules:\n" +
	"- Lead with a one-sentence summary of what the result represents.\n" +
	"- Then 2-5 short bullet points highlighting notable patterns: counts, distributions, outliers, missing values, time trends, correlations — whatever is actually visible in the data.\n" +
	"- Cite specific values from the result when relevant; do not invent numbers.\n" +
	"- If the result is empty say so plainly and suggest one or two reasons why.\n" +
	"- Avoid restating column names; assume the user can read the table.\n" +
	"- Output GitHub-flavored markdown."

// BuildAnalysisSystemPrompt assembles the system message for result
// analysis. Schema is included so the model knows the data types and
// relationships behind the columns it is interpreting.
func BuildAnalysisSystemPrompt(database, language string, snapshot *schema.Snapshot) string {
	var b strings.Builder
	b.WriteString(analyzeAssistantPrompt)
	b.WriteString("\n\n")
	if database != "" {
		fmt.Fprintf(&b, "Active database: %s\n\n", database)
	}
	writeLanguageDirective(&b, language)
	if snapshot != nil {
		b.WriteString("Schema:\n")
		b.WriteString(snapshot.ToPrompt())
	}
	return b.String()
}

// BuildAnalysisUserPrompt formats the SQL + result JSON / CSV blob into a
// single user turn. resultBlob is expected to already be truncated by the
// caller if huge. focus is an optional natural-language hint about what
// the user wants the analysis to emphasise (e.g. "look for outliers",
// "is the distribution skewed?", "compare across regions"). When empty
// the model produces a balanced general overview.
func BuildAnalysisUserPrompt(sql, resultBlob, focus string) string {
	var b strings.Builder
	b.WriteString("SQL:\n")
	b.WriteString(strings.TrimSpace(sql))
	b.WriteString("\n\nResult:\n")
	b.WriteString(strings.TrimSpace(resultBlob))
	writeFocusArea(&b, focus, "Concentrate on this aspect; mention other patterns only if they directly relate.")
	return b.String()
}
