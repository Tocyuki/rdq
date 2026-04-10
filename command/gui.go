package command

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/Tocyuki/rdq/internal/server"
)

type GUICmd struct {
	Port   int  `help:"Port to listen on." short:"P" default:"8080"`
	NoOpen bool `help:"Do not open browser automatically."`
}

func (c *GUICmd) Run(globals *Globals) error {
	if !c.NoOpen {
		url := fmt.Sprintf("http://localhost:%d", c.Port)
		go openBrowser(url)
	}
	return server.Run(c.Port)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Run()
	}
}
