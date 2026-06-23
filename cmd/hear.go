package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ontypehq/vox/internal/audio"
	"github.com/ontypehq/vox/internal/config"
	"github.com/ontypehq/vox/internal/dashscope"
	"github.com/ontypehq/vox/internal/ui"
)

const (
	asrSampleRate = 16000
	// syncInlineLimit: files up to this size take the fast synchronous
	// qwen3-asr-flash path (audio base64-inlined, <=5 min). Larger files are
	// uploaded and transcribed asynchronously via qwen3-asr-flash-filetrans.
	syncInlineLimit = 10 * 1024 * 1024 // 10 MB
)

type HearCmd struct {
	File     string `short:"f" help:"Transcribe an audio file (auto-uploads to filetrans if large)"`
	URL      string `short:"u" help:"Transcribe a public audio URL via async filetrans"`
	Duration int    `short:"d" default:"5" help:"Mic recording duration in seconds"`
	Context  string `short:"c" help:"Text context to improve recognition (sync path only)"`
	ITN      bool   `default:"true" negatable:"" help:"Inverse text normalization: spoken numbers -> digits (--no-itn to disable)"`
	Long     bool   `help:"Force the async filetrans path even for small files"`
	NoCache  bool   `help:"Skip transcription cache"`
}

func (c *HearCmd) Run(cfg *config.AppConfig) error {
	apiKey, err := cfg.RequireAPIKey()
	if err != nil {
		return err
	}
	client := dashscope.NewClient(apiKey)

	switch {
	case c.URL != "":
		ui.Info("%s %s", ui.Dim("url"), ui.Key(c.URL))
		return c.transcribeCached(cfg, "url:"+c.URL+":"+c.Context, func() (*dashscope.ASRResult, error) {
			ui.Info("%s %s", ui.Dim("model"), ui.Key(dashscope.ModelASRFiletrans))
			return client.TranscribeURL(c.URL, c.ITN)
		})

	case c.File != "":
		info, err := os.Stat(c.File)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		ui.Info("%s %s %s", ui.Dim("file"), ui.Key(c.File), ui.Dim(formatSize(info.Size())))

		if c.Long || info.Size() > syncInlineLimit {
			key := fmt.Sprintf("filetrans:%s:%d:%d:%s", c.File, info.Size(), info.ModTime().UnixNano(), c.Context)
			return c.transcribeCached(cfg, key, func() (*dashscope.ASRResult, error) {
				ui.Info("%s %s", ui.Dim("model"), ui.Key(dashscope.ModelASRFiletrans))
				ui.Info("%s", ui.Dim("uploading…"))
				return client.TranscribeLocalFile(c.File, c.ITN)
			})
		}

		data, err := os.ReadFile(c.File)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		h := sha256.New()
		h.Write(data)
		h.Write([]byte(":" + c.Context))
		key := "file:" + hex.EncodeToString(h.Sum(nil))
		return c.transcribeCached(cfg, key, func() (*dashscope.ASRResult, error) {
			ui.Info("%s %s", ui.Dim("model"), ui.Key(dashscope.ModelASRFlash))
			return client.Transcribe(data, c.Context, c.ITN)
		})

	default:
		ui.Info("Recording for %ds... %s", c.Duration, ui.Dim("(speak now)"))
		recorder, err := audio.NewRecorder(asrSampleRate, 1)
		if err != nil {
			return fmt.Errorf("init recorder: %w", err)
		}
		if err := recorder.Start(); err != nil {
			return fmt.Errorf("start recording: %w", err)
		}
		time.Sleep(time.Duration(c.Duration) * time.Second)
		pcm := recorder.Stop()
		ui.Info("%s %s", ui.Dim("recorded"), ui.Dim(fmt.Sprintf("%d bytes", len(pcm))))

		wavData := wrapPCMAsWAVWithRate(pcm, asrSampleRate)
		ui.Info("%s %s", ui.Dim("model"), ui.Key(dashscope.ModelASRFlash))

		t0 := time.Now()
		result, err := client.Transcribe(wavData, c.Context, c.ITN)
		if err != nil {
			return fmt.Errorf("transcribe: %w", err)
		}
		ui.Info("%s %s", ui.Dim("latency"), ui.Dim(time.Since(t0).Round(time.Millisecond).String()))
		fmt.Println(result.Text)
		return nil
	}
}

// transcribeCached returns a cached transcript for cacheKey, or runs fn and
// caches its result. The transcript is printed to stdout (pipe-friendly).
func (c *HearCmd) transcribeCached(cfg *config.AppConfig, cacheKey string, fn func() (*dashscope.ASRResult, error)) error {
	sum := sha256.Sum256([]byte(cacheKey))
	cachePath := filepath.Join(cfg.Dir, "cache", "asr-"+hex.EncodeToString(sum[:])+".txt")

	if !c.NoCache {
		if cached, err := os.ReadFile(cachePath); err == nil {
			ui.Info("%s", ui.Dim("cached"))
			fmt.Println(string(cached))
			return nil
		}
	}

	t0 := time.Now()
	result, err := fn()
	if err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}
	ui.Info("%s %s", ui.Dim("latency"), ui.Dim(time.Since(t0).Round(time.Millisecond).String()))

	if !c.NoCache && result.Text != "" {
		os.WriteFile(cachePath, []byte(result.Text), 0644)
	}
	fmt.Println(result.Text)
	return nil
}

// wrapPCMAsWAVWithRate wraps raw PCM 16-bit mono data in a WAV container at the given sample rate
func wrapPCMAsWAVWithRate(pcm []byte, sampleRate int) []byte {
	dataLen := uint32(len(pcm))
	fileLen := dataLen + 36
	sr := uint32(sampleRate)
	br := sr * 2 // 16-bit mono

	header := []byte{
		'R', 'I', 'F', 'F',
		byte(fileLen), byte(fileLen >> 8), byte(fileLen >> 16), byte(fileLen >> 24),
		'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ',
		16, 0, 0, 0,
		1, 0, // PCM
		1, 0, // mono
		byte(sr), byte(sr >> 8), byte(sr >> 16), byte(sr >> 24),
		byte(br), byte(br >> 8), byte(br >> 16), byte(br >> 24),
		2, 0,  // block align
		16, 0, // bits per sample
		'd', 'a', 't', 'a',
		byte(dataLen), byte(dataLen >> 8), byte(dataLen >> 16), byte(dataLen >> 24),
	}

	return append(header, pcm...)
}
