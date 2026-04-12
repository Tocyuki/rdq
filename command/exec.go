package command

import "fmt"

type ExecCmd struct {
	SQL string `arg:"" help:"SQL statement to execute."`
}

func (c *ExecCmd) Run(globals *Globals) error {
	fmt.Printf("exec: %s (profile=%s, region=%s, debug=%v)\n", c.SQL, globals.Profile, globals.AWSConfig.Region, globals.Debug)
	return nil
}
