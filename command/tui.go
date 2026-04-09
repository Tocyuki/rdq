package command

import "fmt"

type TUICmd struct{}

func (c *TUICmd) Run(globals *Globals) error {
	fmt.Println("launching TUI mode...")
	return nil
}
