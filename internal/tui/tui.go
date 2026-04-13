// Package tui implements the interactive SQL client used by `rdq tui` (and
// the bare `rdq` invocation since tui is the default subcommand). It is a
// bubbletea program: a textarea SQL editor on top, a results pane below, and
// a help bar at the bottom.
package tui

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	tea "github.com/charmbracelet/bubbletea"
)

// Config is the minimal set of inputs the TUI needs to run. The command layer
// constructs this from its Globals so that internal/tui does not have to
// import the command package (which would create an import cycle).
type Config struct {
	AWSConfig  aws.Config
	Profile    string
	ClusterArn string
	SecretArn  string
	Database   string
}

// Run launches the bubbletea program with the resolved connection. It blocks
// until the user quits.
func Run(cfg Config) error {
	if cfg.ClusterArn == "" || cfg.SecretArn == "" || cfg.Database == "" {
		return errors.New("tui requires cluster, secret, and database to be selected")
	}

	client := rdsdata.NewFromConfig(cfg.AWSConfig)
	tgt := target{
		profile:  cfg.Profile,
		region:   cfg.AWSConfig.Region,
		cluster:  cfg.ClusterArn,
		secret:   cfg.SecretArn,
		database: cfg.Database,
	}

	prog := tea.NewProgram(newModel(client, tgt), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := prog.Run()
	return err
}
