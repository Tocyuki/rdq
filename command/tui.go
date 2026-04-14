package command

import "github.com/Tocyuki/rdq/internal/tui"

type TUICmd struct{}

func (c *TUICmd) Run(globals *Globals) error {
	return tui.Run(tui.Config{
		AWSConfig:       globals.AWSConfig,
		Profile:         globals.Profile,
		ClusterArn:      globals.ClusterArn,
		SecretArn:       globals.SecretArn,
		Database:        globals.Database,
		BedrockModel:    globals.BedrockModel,
		BedrockLanguage: globals.BedrockLanguage,
	})
}
