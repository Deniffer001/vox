# vox

Speech-to-text CLI — powered by [Qwen3-ASR](https://github.com/QwenLM/Qwen3-ASR) via the DashScope (Alibaba Bailian) API.

Transcribe an audio file, a public URL, or a quick microphone recording to text — straight from the terminal. Short clips go through the fast synchronous model; long files (up to 12 h) are handled by the async filetrans model with **no object-storage setup required**. Pipe-friendly.

## Install

```bash
# From this checkout
go build -o vox .
mv vox /usr/local/bin/   # or anywhere on your PATH
```

## Quick Start

```bash
# Authenticate with DashScope (Bailian) — stored in ~/.vox/config.json
vox auth login dashscope --token <your-api-key>

# Transcribe a local file (auto-routes: small = fast sync, large = async filetrans)
vox hear -f recording.wav
vox hear -f meeting-2h.mp3        # large file → uploaded + filetrans automatically

# Transcribe a public URL directly (async filetrans)
vox hear --url https://example.com/podcast.mp3

# Record from the microphone (5s default) and transcribe
vox hear

# Bias recognition with domain terms
vox hear -f call.wav -c "Qwen, DashScope, Clonesite"
```

## Commands

```
vox auth login dashscope --token <key>   Save DashScope API key
vox auth status                          Show auth status

vox hear [flags]                         Transcribe speech to text
  -f, --file       Transcribe a local audio file (auto-uploads large files)
  -u, --url        Transcribe a public audio URL (async filetrans)
  -d, --duration   Mic recording duration in seconds (default: 5)
  -c, --context    Text context to improve recognition (sync path only)
  --itn            Inverse text normalization, spoken numbers → digits (default on; --no-itn off)
  --long           Force the async filetrans path even for small files
  --no-cache       Skip transcription cache

vox cache                                Show cache size and file count
vox cache clear                          Delete all cached transcriptions
```

## How It Works

Two transcription paths, chosen automatically:

- **Sync** — `qwen3-asr-flash` via the multimodal-generation endpoint. Used for mic captures and local files ≤ 10 MB. Audio is base64-inlined; ~1 s latency.
- **Async filetrans** — `qwen3-asr-flash-filetrans`. Used for `--url`, local files > 10 MB, or `--long`. Supports up to 12 h / 2 GB.
  - **Local files** are uploaded to DashScope's temporary store (bucket `dashscope-file-mgr`, private objects) via the `uploads` policy endpoint, referenced as `oss://…`, and resolved server-side with the `X-DashScope-OssResourceResolve` header. **No Aliyun OSS account or credentials needed.**
  - **Public URLs** are transcribed directly.
  - Flow: submit task → poll `/tasks/{id}` → download the result JSON → emit text.

Recording is 16 kHz mono (via malgo). Transcripts are cached as text under `~/.vox/cache/` and printed to stdout (metadata goes to stderr, so the transcript pipes cleanly).

## API Key

Get a DashScope API key from [阿里云百炼](https://bailian.console.aliyun.com/) (`sk-...`). Stored locally in `~/.vox/config.json`.

## Notes

- **ITN** (inverse text normalization) is on by default on both paths — "forty two" → "42". Pass `--no-itn` to keep number words. Only affects Chinese/English audio.
- **Hotwords and context are NOT supported** by `qwen3-asr-flash-filetrans` — verified empirically: the API silently ignores `vocabulary_id` / `corpus` / `context` exactly like an unknown field, so they do nothing. If you need hotwords, use `fun-asr` or `paraformer-v2`; for context enhancement, `fun-asr-flash-2026-06-15`. (The sync `qwen3-asr-flash` path does accept a system `-c` context message.)
- The filetrans result JSON also carries word-level timestamps, per-sentence language, and emotion. `vox hear` currently surfaces plain text only — extend `downloadTranscript` in `internal/dashscope/filetrans.go` if you need the richer fields.
- DashScope's temporary upload is rate-limited (100 QPS, not for high-concurrency/production batch). For heavy workloads, host files on your own OSS and pass `--url`.

## License

MIT
