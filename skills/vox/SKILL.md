---
name: vox
description: Transcribe speech to text from a local audio file, a public URL, or the microphone, via Qwen3-ASR on DashScope. Handles short clips and multi-hour files.
---

# vox

Speech-to-text in the terminal. Qwen3-ASR via DashScope. Transcribes a local audio file, a public URL, or a short microphone recording; outputs plain text to stdout (pipe-friendly). Short clips use the fast sync model; long files auto-route to the async filetrans model (no object-storage setup needed).

## When to Use

- User wants to **transcribe audio** to text — a file, a URL, or a recording
- User says "transcribe this", "what does this audio say", "speech to text"
- User has a **long recording** (a meeting, a podcast) to transcribe

## Commands

### Transcribe (ASR)

```bash
# Local file — auto-routes: <=10MB sync, larger uploads to async filetrans
vox hear -f ~/recording.wav
vox hear -f ~/meeting-2h.mp3

# Public URL (async filetrans, up to 12h / 2GB)
vox hear --url https://example.com/podcast.mp3

# Microphone (5 seconds default)
vox hear
vox hear -d 10

# Force the async filetrans path for a small file
vox hear -f short.wav --long

# Provide context for better recognition of domain terms
vox hear -f call.wav -c "Qwen, DashScope, Clonesite"
```

### Auth

```bash
vox auth status                              # check if authenticated
vox auth login dashscope --token <api-key>   # login (once)
```

### Cache

```bash
vox cache         # show cache size / file count
vox cache clear   # delete cached transcriptions
```

## Behavior

- **Pipeable**: `vox hear` writes the transcript to stdout; metadata (model, latency) goes to stderr. Pipe it: `vox hear -f a.wav | pbcopy`.
- **Auto-routing**: local files ≤ 10 MB use the fast synchronous `qwen3-asr-flash`; larger files (or `--long`, or `--url`) use the async `qwen3-asr-flash-filetrans`.
- **No OSS needed**: large local files are uploaded to DashScope's temporary store automatically — the user does not need an Aliyun OSS bucket.
- **ITN on by default**: spoken numbers render as digits ("forty two" → "42"); pass `--no-itn` to keep words. Hotwords/context are NOT supported by the filetrans model.
- **Caching**: same input + context returns instantly from `~/.vox/cache/` on repeat. Use `--no-cache` to force a fresh call.

## Tips

- Check `vox auth status` first if you get auth errors.
- Pass `-c` with names/jargon the model might not know (product names, people) to improve accuracy.
- For a file already reachable on the public web, prefer `--url` — it skips the upload step.
