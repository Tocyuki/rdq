package command

import "fmt"

type TUICmd struct{}

func (c *TUICmd) Run(globals *Globals) error {
	fmt.Printf("launching TUI mode... (profile=%s, region=%s)\n", globals.Profile, globals.AWSConfig.Region)
	return nil
}
