package command

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/Tocyuki/rdq/internal/server"
)

type GUICmd struct {
	Port   int  `help:"Port to listen on." short:"P" default:"8080"`
	NoOpen bool `help:"Do not open browser automatically."`
	Dev    bool `help:"Allow http://localhost:5173 as an origin so the Vite dev server can talk to this API."`
}

// Run boots the GUI HTTP server with a graceful shutdown on SIGINT / SIGTERM.
// The globals struct carries any initial connection selection from the CLI
// layer; the SPA is still expected to call PUT /api/session to pick or
// confirm a destination.
func (c *GUICmd) Run(globals *Globals) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return server.Run(ctx, server.Options{
		Port:                   c.Port,
		NoOpen:                 c.NoOpen,
		Dev:                    c.Dev,
		InitialProfile:         globals.Profile,
		InitialCluster:         globals.ClusterArn,
		InitialSecret:          globals.SecretArn,
		InitialDatabase:        globals.Database,
		InitialBedrockModel:    globals.BedrockModel,
		InitialBedrockLanguage: globals.BedrockLanguage,
	})
}
