package tui

import (
	"context"
	"time"

	"github.com/Tocyuki/rdq/internal/awsauth"
	"github.com/Tocyuki/rdq/internal/bedrock"
	"github.com/Tocyuki/rdq/internal/connection"
	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	tea "github.com/charmbracelet/bubbletea"
)

// schemaLoadedMsg is delivered when the asynchronous information_schema
// fetch finishes. The TUI uses Snapshot for AI prompt context; an error is
// non-fatal and just means the prompt will be sent without schema.
type schemaLoadedMsg struct {
	snapshot *schema.Snapshot
	err      error
}

// modelsLoadedMsg carries the result of bedrock.ListModels for the
// first-time picker.
type modelsLoadedMsg struct {
	models []bedrock.ModelInfo
	err    error
}

// askResultMsg carries an AI-generated SQL statement (or error). Prompt is
// echoed back so the editor footer can show "ask: <prompt>" alongside the
// generated SQL.
type askResultMsg struct {
	prompt string
	sql    string
	err    error
}

// explainResultMsg carries the natural-language analysis of a SQL execution
// failure produced by Bedrock. The text may include markdown bullet lists
// and ```sql examples — the viewport renders it as-is.
type explainResultMsg struct {
	explanation string
	err         error
}

// reviewResultMsg carries the SQL review markdown produced by Bedrock for
// the F6 review-SQL flow. sql is the statement that was reviewed so the UI
// can compose a "## SQL\n<code>\n## Review" payload around the response.
type reviewResultMsg struct {
	sql    string
	review string
	err    error
}

// analyzeResultMsg carries the result-analysis markdown produced by
// Bedrock for the F7 analyze-result flow.
type analyzeResultMsg struct {
	analysis string
	err      error
}

// clustersLoadedMsg is delivered after an asynchronous DescribeDBClusters
// call completes for the in-TUI cluster picker.
type clustersLoadedMsg struct {
	clusters []connection.ClusterInfo
	err      error
}

// secretsLoadedMsg carries the result of asking Secrets Manager which
// secrets belong to a given cluster. fallback is true when the suggestion
// pipeline returned nothing and the caller should treat secrets as the
// full region listing instead of cluster-scoped suggestions.
type secretsLoadedMsg struct {
	cluster  connection.ClusterInfo
	secrets  []connection.SecretInfo
	fallback bool
	err      error
}

// profilesLoadedMsg carries the result of scanning the local AWS config /
// credentials files for available profiles.
type profilesLoadedMsg struct {
	profiles []string
	err      error
}

// profileSwitchedMsg is delivered after awsauth.LoadConfig successfully
// builds a brand-new aws.Config for the selected profile. The TUI swaps
// in the new clients on receipt and re-runs cluster/secret resolution.
type profileSwitchedMsg struct {
	profile string
	cfg     aws.Config
	err     error
}

// schemaFetchTimeout caps how long we wait for the introspection query so the
// background fetch never holds resources forever.
const schemaFetchTimeout = 30 * time.Second

// fetchSchemaCmd loads the cached snapshot first (instant) and falls back to
// a live information_schema fetch in the background. The result is delivered
// via schemaLoadedMsg.
func fetchSchemaCmd(client *rdsdata.Client, tgt target) tea.Cmd {
	return func() tea.Msg {
		if cached, err := schema.LoadCache(tgt.cluster, tgt.database); err == nil && cached != nil && len(cached.Columns) > 0 {
			return schemaLoadedMsg{snapshot: cached}
		}
		ctx, cancel := context.WithTimeout(context.Background(), schemaFetchTimeout)
		defer cancel()
		snap, err := schema.Fetch(ctx, client, tgt.cluster, tgt.secret, tgt.database)
		if err != nil {
			return schemaLoadedMsg{err: err}
		}
		// Best-effort cache write; failures are silently ignored.
		_ = schema.SaveCache(snap)
		return schemaLoadedMsg{snapshot: snap}
	}
}

// loadModelsCmd lists the available Bedrock models / inference profiles in
// the configured region.
func loadModelsCmd(client *bedrock.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := client.ListModels(ctx)
		return modelsLoadedMsg{models: models, err: err}
	}
}

// askCmd invokes Bedrock Converse with the system prompt + multi-turn
// chat history. The caller has already appended the new user message to
// messages, and the userPrompt argument is just for echo-back into the
// askResultMsg so the editor comment header has the current prompt text.
func askCmd(client *bedrock.Client, modelID, systemPrompt string, messages []bedrock.Message, userPrompt string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		sql, err := client.Ask(ctx, modelID, systemPrompt, messages)
		return askResultMsg{prompt: userPrompt, sql: sql, err: err}
	}
}

// explainCmd asks Bedrock to analyse a SQL execution failure and returns the
// raw markdown explanation. Shares Ask's 90s timeout because the analyst
// flow lives on the same critical path. The messages slice is single-turn
// today but uses the same multi-turn API for symmetry with Ask.
func explainCmd(client *bedrock.Client, modelID, systemPrompt string, messages []bedrock.Message) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		text, err := client.Explain(ctx, modelID, systemPrompt, messages)
		return explainResultMsg{explanation: text, err: err}
	}
}

// loadClustersCmd lists Aurora clusters with the Data API enabled in the
// configured region. Used by the in-TUI cluster switcher (Ctrl+T).
func loadClustersCmd(cfg aws.Config) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		clusters, err := connection.ListClusters(ctx, cfg)
		return clustersLoadedMsg{clusters: clusters, err: err}
	}
}

// reviewCmd asks Bedrock to review a SQL statement. focus is the optional
// user-supplied area of concentration (e.g. "performance"). Returns the
// raw markdown response (no fence stripping) so embedded code blocks
// survive.
func reviewCmd(client *bedrock.Client, modelID, systemPrompt, sql, focus string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		userPrompt := bedrock.BuildReviewUserPrompt(sql, focus)
		messages := []bedrock.Message{{Role: bedrock.RoleUser, Text: userPrompt}}
		text, err := client.Explain(ctx, modelID, systemPrompt, messages)
		return reviewResultMsg{sql: sql, review: text, err: err}
	}
}

// analyzeCmd asks Bedrock to interpret a SQL result. focus is the optional
// user-supplied analytical question (e.g. "look for outliers"). The result
// blob is already trimmed by the caller so the prompt stays inside model
// limits.
func analyzeCmd(client *bedrock.Client, modelID, systemPrompt, sql, resultBlob, focus string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		userPrompt := bedrock.BuildAnalysisUserPrompt(sql, resultBlob, focus)
		messages := []bedrock.Message{{Role: bedrock.RoleUser, Text: userPrompt}}
		text, err := client.Explain(ctx, modelID, systemPrompt, messages)
		return analyzeResultMsg{analysis: text, err: err}
	}
}

// loadProfilesCmd reads the local AWS config / credentials files and
// returns the available profile names for the in-TUI profile picker.
func loadProfilesCmd() tea.Cmd {
	return func() tea.Msg {
		profiles, err := awsauth.ListProfiles()
		return profilesLoadedMsg{profiles: profiles, err: err}
	}
}

// switchProfileCmd resolves a fresh aws.Config for the given profile via
// the same loader the CLI uses on startup. Errors are surfaced via the
// returned msg so the UI can show them without crashing.
func switchProfileCmd(profile string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cfg, err := awsauth.LoadConfig(ctx, profile)
		return profileSwitchedMsg{profile: profile, cfg: cfg, err: err}
	}
}

// loadSuggestedSecretsCmd resolves the secrets attached to the given
// cluster (MasterUserSecret + tag matches). When suggestions come back
// empty the cmd transparently falls back to listing every secret in the
// region so the user is never stuck with an empty picker.
func loadSuggestedSecretsCmd(cfg aws.Config, cluster connection.ClusterInfo) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		suggestions, err := connection.SuggestSecretsForCluster(ctx, cfg, cluster)
		if len(suggestions) > 0 {
			return secretsLoadedMsg{cluster: cluster, secrets: suggestions}
		}
		all, listErr := connection.ListSecrets(ctx, cfg)
		if listErr != nil {
			combined := err
			if combined == nil {
				combined = listErr
			}
			return secretsLoadedMsg{cluster: cluster, err: combined}
		}
		return secretsLoadedMsg{cluster: cluster, secrets: all, fallback: true}
	}
}
