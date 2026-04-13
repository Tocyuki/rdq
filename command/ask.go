package command

import "fmt"

type AskCmd struct {
	Query string `arg:"" help:"Natural language query to convert to SQL."`
}

func (c *AskCmd) Run(globals *Globals) error {
	fmt.Printf("ask: %s (profile=%s, region=%s, cluster=%s, secret=%s, db=%s, debug=%v)\n",
		c.Query, globals.Profile, globals.AWSConfig.Region, globals.ClusterArn, globals.SecretArn, globals.Database, globals.Debug)
	return nil
}
