package command

import "fmt"

type TUICmd struct{}

func (c *TUICmd) Run(globals *Globals) error {
	fmt.Printf("launching TUI mode... (profile=%s, region=%s, cluster=%s, secret=%s, db=%s)\n",
		globals.Profile, globals.AWSConfig.Region, globals.ClusterArn, globals.SecretArn, globals.Database)
	return nil
}
