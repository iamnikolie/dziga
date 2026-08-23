---
name: dziga-cli
description: Use when you need to understand a video — a local file or a URL — via the dziga CLI: contact sheets of timecoded frames you can Read yourself, full-size stills at chosen moments, shot-cut lists, and Gemini native-video answers.
---

# dziga CLI — video → context

You cannot `Read` an mp4. `dziga` converts a clip into the two things you can hold:
**labelled still frames** (you look yourself, free) and **text with timestamps**
(Gemini watches for you).

## The loop that works

```bash
dziga context <src>            # 1. digest + one contact sheet → its path is the last line
# 2. Read that path            #    you now see the whole clip for ~1.5k tokens
dziga frames <src> --at 12.4   # 3. zoom into the moment that mattered → Read it
dziga ask <src> "..."          # 4. only for what stills cannot show
```

`<src>` is a local path or a URL (URLs are fetched with `yt-dlp` and cached).

## When to use which engine

| Question | Use | Cost |
|---|---|---|
| what is in this clip / what does it look like | `context` → Read sheet | free, ~1.5k tokens |
| what exactly is on screen at 0:12 | `frames --at 12` → Read | free |
| where are the shot changes | `scenes` / `context` | free |
| how does it move, what order, who does what | `ask` | Gemini tokens |
| read small on-screen text reliably | `ask --res high` | ~3x tokens |
| clip longer than a few minutes | `ask --fps 0.2` or `--from/--to` | scales with fps |

Local commands never touch the network and need no API key.

## Commands

```bash
dziga probe   <src>                       # duration, WxH, fps, codec, audio, rotation
dziga scenes  <src> [--threshold 0.3]     # cut list: seconds + MM:SS
dziga sheet   <src> [-n 16] [--scenes] [--from 0:10 --to 0:40] [--cols 4] [--png] [--keep-cells]
dziga frames  <src> --at 3.5,12,1:04 | --scenes | --every 5s [--width 900]
dziga ask     <src> "question" [--from --to] [--fps 1] [--res low|medium|high]
                               [--schema file.json] [--model flash|pro]
                               [--estimate] [--fresh] [--max-tokens N]
dziga context <src> [-n 16] [--scenes] [--ask "question"] [--no-scenes]
dziga models
dziga config init --config <profile>      # or just export GEMINI_API_KEY
dziga config show
```

Global: `--json`, `--out <dir>`, `--config <profile>`, `--timeout 10m`, `--verbose`.

Time arguments accept `12`, `12.5`, `12s`, `2m`, `1m30s`, `1:03`, `1:03.5`, `01:02:03`.

## Output contract

- **stdout** is the payload only: paths for `sheet`/`frames`, the answer for `ask`,
  markdown for `context`. Pipe it, or `Read` the path.
- **stderr** carries the status line, including the token estimate for the image you
  are about to read: `sheet=16 frames grid=4x4 1420x808 393KB ~1.5k img-tokens`.
- `--json` swaps stdout for one JSON object.
- Artifacts land in `~/.dziga/work/<clip>-<hash>/` so nothing is written into the
  repo you are working in. `--out .` overrides that.

## Reading a sheet

Every cell is burned with `NN M:SS.s` — index and timestamp. Say what you see *with
the timestamp*, then pull that exact moment full-size:

```bash
dziga sheet /clips/demo.mov -n 16
# → /Users/you/.dziga/work/demo-1a2b3c4d/sheet-even-16.jpg     ← Read this
dziga frames /clips/demo.mov --at 0:12.4 --width 1200
# → …/frame-001-0m12s400.jpg                                   ← Read this
```

Sheets are sized to land just under the point where Claude downscales an image
(1568px long edge, ~1.15MP), so a 16-cell sheet costs about 1.5k tokens whatever
the clip's shape. More cells does not cost more tokens — it costs detail per cell.
Below ~120px per cell the CLI warns you.

## What a run costs

**Local commands cost no money at all** — no API, no upload. They cost *your context*:
one sheet is ~1.5k image tokens, one full-size still 0.5–1.5k depending on `--width`.

`ask` bills Gemini tokens. Video input is predictable and was measured, not guessed:

```
video_tokens ≈ round(window_seconds × fps) × 66      (--res high: 264 per frame)
             + window_seconds × 25                    (only if the file has audio)
```

At the default model (`flash`, $0.75/1M in, $3.75/1M out through 2026-12-31):

| clip | command | tokens in | cost |
|---|---|---|---|
| 30s reel with audio | `ask` (fps 1) | ~2.7k | ~$0.002 |
| 5 min talk | `ask` (fps 1) | ~27k | ~$0.021 |
| 5 min talk | `ask --fps 0.2` | ~11k | ~$0.009 |
| 1 h recording | `ask --fps 0.2` | ~138k | ~$0.10 |
| 10s clip | `ask --res high` | ~2.7k | ~$0.002 |

Answer and reasoning tokens are billed at the output rate, so a long structured answer
can cost as much as a short clip's video.

**Check before you spend**: `dziga ask <src> --estimate "q"` prints the predicted
window, frame count, video tokens and dollars, and exits without calling the API.

```
estimate: window=0:10.0 fps=1 res=default video_tokens≈660 cost≈$0.0002 in-only
```

After a real call the status line reports what was actually spent, priced by the model
that *served* the request (`served=gemini-3.7-flash`) rather than by the alias you asked
for. `cost≈?` means no published rate — never a guess. `cost~$…(assumed)` means a
`-latest` alias was priced through its last-known model.

`dziga models` prints today's rates, the measured video rates, and which are assumed.

## Asking Gemini

```bash
dziga ask clip.mp4 "Does the person pick up the cup before or after the phone rings?"
dziga ask clip.mp4 --from 1:30 --to 2:00 --fps 4 "Frame by frame, how does the hand move?"
dziga ask clip.mp4 --res high "Transcribe every piece of text on screen with timestamps."
dziga ask clip.mp4 --schema shots.json "Break the video into shots."   # → strict JSON
```

- The first `ask` uploads the clip; the URI is cached for the 48h the API keeps it,
  so follow-ups are fast. `--fresh` forces a re-upload.
- `--fps` is the model's sampling rate (default 1/s). Raise it for fast action, drop
  it (`0.2`) for long clips — video tokens scale with it.
- `--estimate` prices the run before it happens; the status line prices it after.

## Gotchas

- ffmpeg and ffprobe must be on PATH (`brew install ffmpeg`); `yt-dlp` only for URLs.
- `--scenes` on a single-shot clip finds nothing and falls back to even sampling —
  that is an answer, not a failure.
- Cut detection decodes the whole file; `context` skips it above 20 minutes
  (`--max-scan 0` forces it).
- There is no audio surface: no transcript command, no STT. Gemini hears the track
  inside `ask` because it travels with the file.
