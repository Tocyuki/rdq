package command

import "fmt"

type ExecCmd struct {
	SQL string `arg:"" help:"SQL statement to execute."`
}

func (c *ExecCmd) Run(globals *Globals) error {
	fmt.Printf("exec: %s (profile=%s, debug=%v)\n", c.SQL, globals.Profile, globals.Debug)
	return nil
}
