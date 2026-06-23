package main

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/ontypehq/vox/cmd"
	"github.com/ontypehq/vox/internal/config"
	"github.com/ontypehq/vox/internal/ui"
)

var cli struct {
	Auth  cmd.AuthCmd  `cmd:"" help:"Manage authentication"`
	Hear  cmd.HearCmd  `cmd:"" help:"Transcribe speech to text"`
	Cache cmd.CacheCmd `cmd:"" help:"Manage transcription cache"`
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("vox"),
		kong.Description("Speech-to-text CLI — powered by Qwen3-ASR via DashScope"),
		kong.UsageOnError(),
	)

	cfg, err := config.Load()
	if err != nil {
		ui.Error("Failed to load config: %v", err)
		os.Exit(1)
	}

	err = ctx.Run(cfg)
	ctx.FatalIfErrorf(err)
}
