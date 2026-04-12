package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/Tocyuki/rdq/command"
	"github.com/Tocyuki/rdq/internal/awsauth"
	"github.com/alecthomas/kong"
)

// profileSelectSentinel marks a bare "--profile" / "-p" flag on the command
// line so that the post-parse stage knows to launch the fuzzy finder.
const profileSelectSentinel = "__rdq_select__"

// knownSubcommands enumerates the top-level subcommand names. It is used by
// preScanProfileArgs to detect the "rdq --profile exec ..." pattern, where
// the token after a bare --profile is a subcommand rather than a profile value.
var knownSubcommands = map[string]bool{
	"exec": true,
	"ask":  true,
	"gui":  true,
	"tui":  true,
}

var cli struct {
	Profile string `help:"AWS profile to use. Pass without a value to launch an interactive picker." short:"p" env:"AWS_PROFILE"`
	Debug   bool   `help:"Enable debug output." short:"d"`

	Exec command.ExecCmd `cmd:"" help:"Execute a SQL statement."`
	Ask  command.AskCmd  `cmd:"" help:"Translate natural language to SQL and execute."`
	GUI  command.GUICmd  `cmd:"" help:"Launch browser-based SQL client."`
	TUI  command.TUICmd  `cmd:"" help:"Launch interactive TUI mode." default:"1"`
}

func main() {
	os.Args = preScanProfileArgs(os.Args)

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

	err = ctx.Run(&command.Globals{
		Profile:   profile,
		Debug:     cli.Debug,
		AWSConfig: cfg,
	})
	ctx.FatalIfErrorf(err)
}

// resolveProfile turns the raw --profile value from Kong into the effective
// profile name, launching the fuzzy picker when the sentinel is present.
func resolveProfile(raw string) (string, error) {
	if raw != profileSelectSentinel {
		return raw, nil
	}
	profiles, err := awsauth.ListProfiles()
	if err != nil {
		return "", err
	}
	return awsauth.SelectProfile(profiles)
}

// preScanProfileArgs rewrites bare "--profile" / "-p" flags into
// "--profile=<sentinel>" so Kong accepts them. A flag is "bare" when the next
// token is missing, looks like another flag, or is a known subcommand name.
func preScanProfileArgs(args []string) []string {
	if len(args) < 2 {
		return args
	}
	out := make([]string, 0, len(args)+1)
	out = append(out, args[0])
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		if tok != "--profile" && tok != "-p" {
			out = append(out, tok)
			continue
		}
		if isBareProfile(rest, i) {
			out = append(out, "--profile="+profileSelectSentinel)
			continue
		}
		out = append(out, tok)
	}
	return out
}

func isBareProfile(rest []string, i int) bool {
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
