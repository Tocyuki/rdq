package main

import (
	"github.com/Tocyuki/rdq/command"
	"github.com/alecthomas/kong"
)

var cli struct {
	Profile string `help:"AWS profile to use." short:"p" env:"AWS_PROFILE"`
	Debug   bool   `help:"Enable debug output." short:"d"`

	Exec command.ExecCmd `cmd:"" help:"Execute a SQL statement."`
	Ask  command.AskCmd  `cmd:"" help:"Translate natural language to SQL and execute."`
	GUI  command.GUICmd  `cmd:"" help:"Launch browser-based SQL client."`
	TUI  command.TUICmd  `cmd:"" help:"Launch interactive TUI mode." default:"1"`
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("rdq"),
		kong.Description("CLI for querying Aurora via RDS Data API."),
		kong.UsageOnError(),
	)
	err := ctx.Run(&command.Globals{
		Profile: cli.Profile,
		Debug:   cli.Debug,
	})
	ctx.FatalIfErrorf(err)
}
