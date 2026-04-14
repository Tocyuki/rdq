package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Tocyuki/rdq/command"
	"github.com/Tocyuki/rdq/internal/awsauth"
	"github.com/Tocyuki/rdq/internal/connection"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// bareFlagSpec describes a flag that may be passed without a value, in which
// case preScanBareFlags rewrites it to "<long>=<sentinel>" so Kong accepts it
// and the post-parse stage knows to launch an interactive picker.
type bareFlagSpec struct {
	long     string
	short    string // empty when the flag has no short form
	sentinel string
}

var bareFlags = []bareFlagSpec{
	{long: "--profile", short: "-p", sentinel: "__rdq_profile_select__"},
	{long: "--cluster", short: "", sentinel: "__rdq_cluster_select__"},
	{long: "--secret", short: "", sentinel: "__rdq_secret_select__"},
	{long: "--database", short: "", sentinel: "__rdq_database_select__"},
}

// knownSubcommands enumerates the top-level subcommand names. preScanBareFlags
// uses it to detect "rdq --profile exec ..." patterns where the token after a
// bare flag is a subcommand rather than a value.
var knownSubcommands = map[string]bool{
	"exec": true,
	"ask":  true,
	"gui":  true,
	"tui":  true,
}

// commandsNeedingConnection are the subcommands that require a resolved
// cluster/secret/database. gui has its own selection UI and is excluded.
var commandsNeedingConnection = map[string]bool{
	"exec": true,
	"ask":  true,
	"tui":  true,
}

var cli struct {
	Profile      string `help:"AWS profile to use. Pass without a value to launch an interactive picker." short:"p" env:"AWS_PROFILE"`
	Cluster      string `help:"RDS cluster ARN. Pass without a value to launch an interactive picker."`
	Secret       string `help:"Secrets Manager secret ARN. Pass without a value to launch an interactive picker."`
	Database     string `help:"Database name. Pass without a value to pick from history or enter manually."`
	BedrockModel    string `help:"Bedrock model ID for natural-language SQL generation in TUI. Overrides the cached model." env:"RDQ_BEDROCK_MODEL"`
	BedrockLanguage string `help:"Language the AI uses when responding (e.g. Japanese, English). Overrides the cached language." env:"RDQ_BEDROCK_LANGUAGE"`
	Debug           bool   `help:"Enable debug output." short:"d"`

	Exec command.ExecCmd `cmd:"" help:"Execute a SQL statement."`
	Ask  command.AskCmd  `cmd:"" help:"Translate natural language to SQL and execute."`
	GUI  command.GUICmd  `cmd:"" help:"Launch browser-based SQL client."`
	TUI  command.TUICmd  `cmd:"" help:"Launch interactive TUI mode." default:"1"`
}

func main() {
	os.Args = preScanBareFlags(os.Args)

	ctx := kong.Parse(&cli,
		kong.Name("rdq"),
		kong.Description("CLI for querying Aurora via RDS Data API."),
		kong.UsageOnError(),
	)

	profile, err := resolveProfile(cli.Profile)
	ctx.FatalIfErrorf(err)

	cfg, err := awsauth.LoadConfig(context.Background(), profile)
	ctx.FatalIfErrorf(err)

	// LoadConfig is lazy — it succeeds even when no credential provider
	// can produce a value. Eagerly resolve credentials here so the user
	// sees a friendly hint instead of the AWS SDK's raw "failed to
	// refresh cached credentials" error on the first API call.
	if _, credErr := cfg.Credentials.Retrieve(context.Background()); credErr != nil {
		fmt.Fprintln(os.Stderr, "rdq: no AWS credentials available.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Configure credentials in one of these ways:")
		fmt.Fprintln(os.Stderr, "  - export AWS_PROFILE=<name>")
		fmt.Fprintln(os.Stderr, "  - export AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... (and optionally AWS_SESSION_TOKEN)")
		fmt.Fprintln(os.Stderr, "  - run with -p (no value) to pick a profile interactively")
		fmt.Fprintln(os.Stderr, "  - populate ~/.aws/credentials or ~/.aws/config")
		if cli.Debug {
			fmt.Fprintf(os.Stderr, "\nUnderlying SDK error: %v\n", credErr)
		}
		os.Exit(1)
	}

	if cli.Debug {
		account, arn, err := awsauth.VerifyIdentity(context.Background(), cfg)
		if err != nil {
			log.Printf("aws identity verification failed: %v", err)
		} else {
			log.Printf("aws identity: account=%s arn=%s region=%s", account, arn, cfg.Region)
		}
	}

	globals := &command.Globals{
		Profile:   profile,
		Debug:     cli.Debug,
		AWSConfig: cfg,
	}

	if needsConnection(ctx) {
		err = resolveConnection(context.Background(), cfg, profile, globals)
		ctx.FatalIfErrorf(err)
	}

	err = ctx.Run(globals)
	ctx.FatalIfErrorf(err)
}

// needsConnection returns true when the selected subcommand requires a
// resolved cluster/secret/database trio.
func needsConnection(ctx *kong.Context) bool {
	cmd := strings.SplitN(ctx.Command(), " ", 2)[0]
	return commandsNeedingConnection[cmd]
}

// resolveProfile turns the raw --profile value from Kong into the effective
// profile name, with three distinct paths:
//
//   - sentinel (`-p` passed without a value): honour AWS_PROFILE if set,
//     otherwise launch the fuzzy picker.
//   - empty (no `-p` at all): honour AWS_PROFILE if set, otherwise return
//     "" so awsauth.LoadConfig falls through to the AWS SDK default
//     credentials chain (env vars, shared credentials, IMDS, ...).
//   - explicit value: use it as-is.
func resolveProfile(raw string) (string, error) {
	if raw == bareFlags[0].sentinel {
		if env := strings.TrimSpace(os.Getenv("AWS_PROFILE")); env != "" {
			return env, nil
		}
		profiles, err := awsauth.ListProfiles()
		if err != nil {
			return "", err
		}
		return awsauth.SelectProfile(profiles)
	}
	if raw == "" {
		if env := strings.TrimSpace(os.Getenv("AWS_PROFILE")); env != "" {
			return env, nil
		}
		return "", nil
	}
	return raw, nil
}

// resolveConnection populates globals.ClusterArn / SecretArn / Database.
//
// When a profile name is present it consults the per-profile state cache so
// subsequent runs can skip the pickers; on success it writes the result back
// for next time.
//
// When the profile name is empty (the user is running rdq with direct
// credentials such as AWS_ACCESS_KEY_ID, with no AWS_PROFILE) it switches
// to **ephemeral mode**: nothing is read from or written to state.json, so
// every invocation walks through the cluster / secret / database pickers
// from scratch and no traces are left on disk.
func resolveConnection(ctx context.Context, cfg aws.Config, profile string, globals *command.Globals) error {
	ephemeral := profile == ""

	var cached state.ProfileState
	if !ephemeral {
		st, err := state.Load()
		if err != nil {
			log.Printf("rdq state load failed, continuing without cache: %v", err)
			st = &state.State{Profiles: map[string]state.ProfileState{}}
		}
		cached = st.Get(profile)
	}

	cluster, err := resolveCluster(ctx, cfg, cli.Cluster, cached.Cluster)
	if err != nil {
		return err
	}
	secret, err := resolveSecret(ctx, cfg, cli.Secret, cached.Secret, cluster, cached.ClusterSecrets)
	if err != nil {
		return err
	}
	database, err := resolveDatabase(cli.Database, cached.Database, cached.DatabaseHistory)
	if err != nil {
		return err
	}

	globals.ClusterArn = cluster.ARN
	globals.SecretArn = secret
	globals.Database = database

	// Bedrock model and language: explicit CLI / env value always wins;
	// cached value only applies when we are not in ephemeral mode.
	switch {
	case cli.BedrockModel != "":
		globals.BedrockModel = cli.BedrockModel
	case !ephemeral && cached.BedrockModel != "":
		globals.BedrockModel = cached.BedrockModel
	}
	switch {
	case cli.BedrockLanguage != "":
		globals.BedrockLanguage = cli.BedrockLanguage
	case !ephemeral && cached.BedrockLanguage != "":
		globals.BedrockLanguage = cached.BedrockLanguage
	}

	if ephemeral {
		return nil
	}

	cached.Cluster = cluster.ARN
	cached.Secret = secret
	cached.Database = database
	if cached.ClusterSecrets == nil {
		cached.ClusterSecrets = map[string]string{}
	}
	if cluster.ARN != "" && secret != "" {
		cached.ClusterSecrets[cluster.ARN] = secret
	}
	st, err := state.Load()
	if err != nil {
		log.Printf("rdq state save skipped (load failed): %v", err)
		return nil
	}
	st.Set(profile, cached)
	if err := st.Save(); err != nil {
		log.Printf("rdq state save failed: %v", err)
	}
	return nil
}

// resolveCluster picks the Aurora cluster the user wants to talk to and
// returns the full ClusterInfo so downstream resolvers (specifically secret
// resolution) can take advantage of the MasterUserSecret field.
func resolveCluster(ctx context.Context, cfg aws.Config, raw, cached string) (connection.ClusterInfo, error) {
	switch {
	case raw == bareFlags[1].sentinel:
		// fall through to picker
	case raw != "":
		return lookupClusterByARN(ctx, cfg, raw)
	case cached != "":
		if cli.Debug {
			log.Printf("using cached cluster: %s", cached)
		}
		return lookupClusterByARN(ctx, cfg, cached)
	}
	clusters, err := connection.ListClusters(ctx, cfg)
	if err != nil {
		return connection.ClusterInfo{}, err
	}
	return connection.SelectCluster(clusters)
}

// lookupClusterByARN resolves an explicit / cached cluster ARN to a full
// ClusterInfo by listing clusters and matching on ARN. If the cluster can
// no longer be found we still return a ClusterInfo with just the ARN set
// so secret resolution falls back to the all-secrets picker rather than
// failing the whole startup.
func lookupClusterByARN(ctx context.Context, cfg aws.Config, arn string) (connection.ClusterInfo, error) {
	clusters, err := connection.ListClusters(ctx, cfg)
	if err != nil {
		return connection.ClusterInfo{ARN: arn}, nil
	}
	for _, c := range clusters {
		if c.ARN == arn {
			return c, nil
		}
	}
	return connection.ClusterInfo{ARN: arn}, nil
}

// resolveSecret picks the Secrets Manager secret to feed into the Data
// API. The first three tiers (explicit / cached / cluster-cache) skip
// every AWS round trip; only genuinely new clusters fall through to
// SuggestSecretsForCluster and ultimately the all-secrets picker.
func resolveSecret(ctx context.Context, cfg aws.Config, raw, cached string, cluster connection.ClusterInfo, clusterSecrets map[string]string) (string, error) {
	switch {
	case raw == bareFlags[2].sentinel:
		// fall through to picker
	case raw != "":
		return raw, nil
	case cached != "":
		if cli.Debug {
			log.Printf("using cached secret: %s", cached)
		}
		return cached, nil
	}

	// Per-cluster cache: the user already paired this cluster with a
	// secret (in TUI or in a previous CLI run) so reuse that pairing
	// without consulting AWS at all.
	if cluster.ARN != "" && clusterSecrets != nil {
		if s := clusterSecrets[cluster.ARN]; s != "" {
			if cli.Debug {
				log.Printf("using cluster-cached secret %s for cluster %s", s, cluster.ARN)
			}
			return s, nil
		}
	}

	if cluster.ARN != "" {
		suggestions, err := connection.SuggestSecretsForCluster(ctx, cfg, cluster)
		if err != nil && cli.Debug {
			log.Printf("secret suggestion failed (falling back to full list): %v", err)
		}
		switch len(suggestions) {
		case 1:
			if cli.Debug {
				log.Printf("auto-selected secret %s for cluster %s", suggestions[0].ARN, cluster.ARN)
			}
			return suggestions[0].ARN, nil
		default:
			if len(suggestions) > 1 {
				picked, perr := connection.SelectSecret(suggestions)
				if perr != nil {
					return "", perr
				}
				return picked.ARN, nil
			}
			// 0 candidates → fall through to all-secrets picker
		}
	}

	secrets, err := connection.ListSecrets(ctx, cfg)
	if err != nil {
		return "", err
	}
	picked, err := connection.SelectSecret(secrets)
	if err != nil {
		return "", err
	}
	return picked.ARN, nil
}

func resolveDatabase(raw, cached string, history []string) (string, error) {
	switch {
	case raw == bareFlags[3].sentinel:
		// fall through to prompt
	case raw != "":
		return raw, nil
	case cached != "":
		if cli.Debug {
			log.Printf("using cached database: %s", cached)
		}
		return cached, nil
	}
	return connection.PromptDatabase(history)
}

// preScanBareFlags rewrites bare "--<flag>" / short forms into
// "--<flag>=<sentinel>" so Kong accepts them as values. A flag is "bare" when
// the next token is missing, looks like another flag, or is a known subcommand.
func preScanBareFlags(args []string) []string {
	if len(args) < 2 {
		return args
	}
	out := make([]string, 0, len(args)+1)
	out = append(out, args[0])
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		spec, ok := matchBareFlag(tok)
		if !ok {
			out = append(out, tok)
			continue
		}
		if isBareFlag(rest, i) {
			out = append(out, spec.long+"="+spec.sentinel)
			continue
		}
		out = append(out, tok)
	}
	return out
}

// matchBareFlag returns the spec for a token if it is one of the registered
// long/short forms, ignoring the "<flag>=value" form which Kong handles
// natively.
func matchBareFlag(tok string) (bareFlagSpec, bool) {
	if strings.Contains(tok, "=") {
		return bareFlagSpec{}, false
	}
	for _, spec := range bareFlags {
		if tok == spec.long || (spec.short != "" && tok == spec.short) {
			return spec, true
		}
	}
	return bareFlagSpec{}, false
}

// isBareFlag decides whether the flag at position i in rest is being used
// without a value.
func isBareFlag(rest []string, i int) bool {
	next := i + 1
	if next >= len(rest) {
		return true
	}
	v := rest[next]
	if v == "" {
		return true
	}
	if strings.HasPrefix(v, "-") {
		return true
	}
	if knownSubcommands[v] {
		return true
	}
	return false
}
