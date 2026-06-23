package cmd

import (
	"github.com/ontypehq/vox/internal/config"
	"github.com/ontypehq/vox/internal/ui"
)

type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Save API credentials"`
	Status AuthStatusCmd `cmd:"" help:"Show current auth status"`
}

type AuthLoginCmd struct {
	DashScope AuthLoginDashScopeCmd `cmd:"" name:"dashscope" help:"Login to DashScope (ASR)"`
}

// --- dashscope ---

type AuthLoginDashScopeCmd struct {
	Token string `required:"" help:"DashScope API key"`
}

func (c *AuthLoginDashScopeCmd) Run(cfg *config.AppConfig) error {
	cfg.Config.Services.DashScope.APIKey = c.Token
	if err := cfg.SaveConfig(); err != nil {
		return err
	}
	ui.Success("Authenticated with %s", ui.Key("dashscope"))
	return nil
}

// --- status ---

type AuthStatusCmd struct{}

func (c *AuthStatusCmd) Run(cfg *config.AppConfig) error {
	any := false

	if key := cfg.Config.Services.DashScope.APIKey; key != "" {
		any = true
		ui.Success("dashscope")
		ui.KV("  API Key", maskToken(key))
	}

	if !any {
		ui.Warn("No services configured")
		ui.Info("  %s", ui.Key("vox auth login dashscope --token <key>"))
	}

	return nil
}

func maskToken(t string) string {
	if len(t) < 10 {
		return "***"
	}
	return t[:6] + "..." + t[len(t)-4:]
}
