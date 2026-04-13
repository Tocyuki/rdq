package main

import (
	"context"
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
	Profile  string `help:"AWS profile to use. Pass without a value to launch an interactive picker." short:"p" env:"AWS_PROFILE"`
	Cluster  string `help:"RDS cluster ARN. Pass without a value to launch an interactive picker."`
	Secret   string `help:"Secrets Manager secret ARN. Pass without a value to launch an interactive picker."`
	Database string `help:"Database name. Pass without a value to pick from history or enter manually."`
	Debug    bool   `help:"Enable debug output." short:"d"`

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
// profile name, launching the fuzzy picker when the sentinel is present.
func resolveProfile(raw string) (string, error) {
	if raw != bareFlags[0].sentinel {
		return raw, nil
	}
	profiles, err := awsauth.ListProfiles()
	if err != nil {
		return "", err
	}
	return awsauth.SelectProfile(profiles)
}

// resolveConnection populates globals.ClusterArn / SecretArn / Database by
// applying the three-tier resolution rule per field:
//   - explicit value     → use as-is
//   - bare flag sentinel → launch picker
//   - empty + cached     → use cached
//   - empty + no cache   → launch picker
//
// On success the cache file is updated so the next invocation can skip prompts.
func resolveConnection(ctx context.Context, cfg aws.Config, profile string, globals *command.Globals) error {
	st, err := state.Load()
	if err != nil {
		log.Printf("rdq state load failed, continuing without cache: %v", err)
		st = &state.State{Profiles: map[string]state.ProfileState{}}
	}
	cached := st.Get(profile)

	cluster, err := resolveCluster(ctx, cfg, cli.Cluster, cached.Cluster)
	if err != nil {
		return err
	}
	secret, err := resolveSecret(ctx, cfg, cli.Secret, cached.Secret)
	if err != nil {
		return err
	}
	database, err := resolveDatabase(cli.Database, cached.Database, cached.DatabaseHistory)
	if err != nil {
		return err
	}

	globals.ClusterArn = cluster
	globals.SecretArn = secret
	globals.Database = database

	cached.Cluster = cluster
	cached.Secret = secret
	cached.Database = database
	st.Set(profile, cached)
	if err := st.Save(); err != nil {
		log.Printf("rdq state save failed: %v", err)
	}
	return nil
}

func resolveCluster(ctx context.Context, cfg aws.Config, raw, cached string) (string, error) {
	switch {
	case raw == bareFlags[1].sentinel:
		// fall through to picker
	case raw != "":
		return raw, nil
	case cached != "":
		if cli.Debug {
			log.Printf("using cached cluster: %s", cached)
		}
		return cached, nil
	}
	clusters, err := connection.ListClusters(ctx, cfg)
	if err != nil {
		return "", err
	}
	picked, err := connection.SelectCluster(clusters)
	if err != nil {
		return "", err
	}
	return picked.ARN, nil
}

func resolveSecret(ctx context.Context, cfg aws.Config, raw, cached string) (string, error) {
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
