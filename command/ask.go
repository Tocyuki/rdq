package command

import "fmt"

type AskCmd struct {
	Query string `arg:"" help:"Natural language query to convert to SQL."`
}

func (c *AskCmd) Run(globals *Globals) error {
	fmt.Printf("ask: %s (profile=%s, debug=%v)\n", c.Query, globals.Profile, globals.Debug)
	return nil
}
