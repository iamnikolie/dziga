# dziga — implementation spec

Agent-facing **video-reading** CLI. Sibling of `gengoya` (image/video *generation*),
`fibery`, `gl`. Named after Dziga Vertov — `дзиґа` is the Ukrainian word for a
spinning top, the pseudonym he took for the camera-crank; his *Kino-Eye* is the
whole idea here: a machine eye that sees for someone who cannot.

Module path: `github.com/langgerone/dziga`. Binary: `dziga`. Go 1.26.

---

## 1 · The problem this exists for

An LLM agent cannot ingest `mp4`. Claude Code's `Read` renders **images** (PNG/JPEG)
and PDF — nothing else. So a video must be converted into things the agent can hold:

1. **Still frames** — the agent looks with its own eyes, no third party, $0.
2. **Text with timestamps** — a video-native model (Gemini) watches and reports.

Everything below serves that inversion, exactly like gengoya's:

> **The CLI writes files to disk and prints their absolute paths to stdout.
> The agent then `Read`s the path.** Bytes never travel through stdout or context.

## 2 · Decisions locked

1. **Two engines, explicitly separate.** `sheet`/`frames`/`scenes`/`probe` are pure
   local ffmpeg — no network, no key, no cost. `ask` delegates to Gemini native video.
   Never silently swap one for the other.
2. **Timecodes are burned into every frame.** A contact sheet without labels is
   useless: the agent can see *what* but cannot say *when*, and cannot ask for a
   closer look. Every cell carries `#NN  MM:SS.s`.
3. **No audio surface.** No transcript command, no STT flag. (Gemini hears the
   track inside `ask` because it travels with the file — that is incidental, not a
   feature.)
4. **Config is optional**, unlike gengoya. Local commands need nothing. `ask` takes a
   key from `--config <profile>` or `GEMINI_API_KEY`/`GOOGLE_API_KEY`.
5. **Model ids are data** (`registry.yaml`, embedded), never Go constants.
6. **Prices are only ever measured or published-and-verified.** Unknown → print
   `cost≈?` and the real token counts. Never invent a rate (gengoya/kie lesson).
7. **Frames are written to a per-video work dir** under `~/.dziga/work/`, not the
   CWD — an agent tool must not litter the repo it is called from. `--out` overrides.

## 3 · Output contract

- **stdout** — the payload, nothing else:
  - `sheet`, `frames` → absolute paths, one per line.
  - `probe`, `scenes` → one compact human line + data lines.
  - `ask` → the model's answer.
  - `context` → a markdown digest (its final line is the sheet path).
  - `--json` on any command → a single JSON object instead.
- **stderr** — one status line per artifact, e.g.
  `sheet=16 frames grid=4x4 1536x864 jpeg 214KB ~1.6k img-tokens ms=812`
  and for `ask`: `model=flash id=gemini-flash-latest video_tokens=364 out=77 cost≈? ms=4103`.
- Exit non-zero on any error, with the provider body on stderr.

### Why token counts on stderr

The agent pays for the sheet it is about to `Read`. Claude bills an image at roughly
`w*h/750` tokens and resamples anything past **either** cap — 1568px on the long edge
**or** ~1.15 megapixels of area. Sheets are solved against both, so a sheet lands just
under the resample point whatever the clip's aspect, and a 16-cell sheet costs ~1.5k
tokens. Printing the estimate lets the agent choose between one overview and three
targeted full-res frames.

## 4 · Command surface

```
dziga probe   <src>                          # duration, WxH, fps, codec, audio, rotation
dziga scenes  <src> [--threshold 0.3]        # cut list (seconds), one per line
dziga sheet   <src> [--n 16] [--from t --to t] [--scenes] [--cols N] [--max-edge 1568]
dziga frames  <src> --at 3.5,12,44 | --scenes | --every 5s
dziga ask     <src> "question" [--from t --to t] [--fps 1] [--res low|medium|high]
                                [--schema file.json] [--model flash|pro]
dziga context <src> [--n 16] [--ask]         # probe + scenes + sheet in one markdown digest
dziga models
dziga config init|show [--config <profile>]
dziga skill
dziga version
```

`<src>` is a local path **or** a URL — a URL is fetched with `yt-dlp` into
`~/.dziga/cache/<sha1>.mp4` and reused on later calls.

Time arguments (`--at`, `--from`, `--to`, `--every`) accept `12`, `12.5`, `1:03`,
`1:03.5`, `01:02:03`, and `12s`/`2m`/`1h`.

## 5 · Contact sheet layout

- Frames are extracted with `ffmpeg -ss <t> -i <src> -frames:v 1` (per timestamp,
  bounded worker pool) so the grid holds **exact** requested moments, not an
  fps-decimated stream.
- `--scenes` samples one frame just after each detected cut instead of uniformly.
- Column count minimises **wasted cells** first (with the area capped, an empty slot in
  the last row is pure wasted tokens) and breaks ties on squareness — 16 frames land on
  4x4 for both 16:9 and 9:16, 9 frames on 3x3.
- Cell width is then solved against both caps at once: long edge ≤ `--max-edge` (1568)
  **and** area ≤ ~1.15MP (`sheet.SolveArea` solves the quadratic for the cell size).
- Labels are composited **in Go**, not by ffmpeg: `drawtext` needs a libfreetype
  build that Homebrew's ffmpeg 8.x does not ship. `golang.org/x/image/font/basicfont`
  scaled by an integer factor, white on a black bar, top-left of each cell.
- Default encode is JPEG q88 (a 16-cell PNG sheet is ~8× larger for no gain); `--png`
  when the video is text/UI heavy.

## 6 · Gemini video (`ask`) — live-verified shapes

Verified against `generativelanguage.googleapis.com/v1beta` on 2026-08-23:

1. **Resumable upload** (3 calls): `POST /upload/v1beta/files` with
   `X-Goog-Upload-Protocol: resumable`, `X-Goog-Upload-Command: start`,
   `X-Goog-Upload-Header-Content-Length`, `X-Goog-Upload-Header-Content-Type`,
   body `{"file":{"display_name":"..."}}` → reply header `X-Goog-Upload-URL`.
   Then `POST <that url>` with `X-Goog-Upload-Command: upload, finalize` and
   `X-Goog-Upload-Offset: 0` and the raw bytes → `{"file":{...,"state":"PROCESSING"}}`.
2. **Poll** `GET /v1beta/files/{name}` until `state == "ACTIVE"` (a few seconds).
   The file expires in **48h** (`expirationTime`), so its URI is cached in
   `~/.dziga/uploads.json` keyed by content sha256 — repeated questions about the
   same clip never re-upload.
3. **Generate**: parts `[{file_data:{mime_type,file_uri}, video_metadata:{start_offset,
   end_offset,fps}}, {text:prompt}]`. `generationConfig` accepts `responseMimeType`,
   `responseSchema`, and `mediaResolution: MEDIA_RESOLUTION_LOW|MEDIUM|HIGH`.
4. **Response parsing**: Gemini 3.x parts carry `thoughtSignature` and the payload
   reports `thoughtsTokenCount`. Skip any part with `"thought": true`; concatenate the
   rest. `usageMetadata.promptTokensDetails` breaks tokens down per modality — the
   `VIDEO` entry is what the clip actually cost.

### 6.1 · Cost, measured

Video-token cost was measured against clips of known length on `gemini-3.7-flash`
(2026-08-23), not taken from documentation:

| clip | fps | res | video tokens |
|---|---|---|---|
| 4s silent 1280x720 | 1 | default | 264 |
| same | 2 | default | 528 |
| same, with aac audio | 1 | default | 364 |
| 10s silent 608x1080 | 1 | default / low / medium | 660 |
| same | 1 | high | 2640 |

which resolves to a model the CLI can predict with:

```
video_tokens ≈ round(window × fps) × tokens_per_frame + (has_audio ? window × 25 : 0)
tokens_per_frame: default = low = medium = 66, high = 264
```

Frame cost did not vary with aspect ratio, and `medium` measured identical to `low`.
These rates live in `registry.yaml` under `video:` as **measurements**, with the
conditions recorded next to them — the kie-credits rule from gengoya applies here too:
never publish a rate that was not observed.

Pricing rules (`registry.yaml`, verified against ai.google.dev/gemini-api/docs/pricing):

- Rates are per model **id**, never per alias. `flash`/`pro`/`flash-lite` are
  `-latest` aliases whose served model changes underneath them, so they carry only an
  `assume:` hint used for pre-flight estimates (printed with `~`), and the real cost is
  computed from the `modelVersion` the response reports.
- Introductory rates carry `price_until` plus the successor under `after:`. Past the
  date with no successor recorded, the CLI reports `cost≈?` — a stale price is worse
  than an honest question mark.
- Long-context tiers (`long_threshold` + `long:`) apply above 200k prompt tokens.
- `--estimate` prices a run before it happens and exits without calling the API.

Sampling controls dominate cost; expose them, never hide them.

## 7 · Layout

```
main.go
cmd/            cobra, one file per command + embedded skill.md
internal/
  config/       ~/.dziga/<profile>/config.yaml, DZIGA_HOME; env-key fallback
  registry/     embedded registry.yaml (model alias → id, prices when known)
  client/       HTTP with 429/5xx backoff + key redaction (from gengoya)
  media/        ffprobe/ffmpeg: Probe, ExtractFrames, DetectScenes, time parsing
  sheet/        grid math, compositing, label drawing
  provider/     Gemini Files API + generateContent
  uploads/      sha256 → {file_uri, expires_at} cache
```

## 8 · Invariants

- Pure functions for anything testable without a process: grid math, time parsing,
  scene-line parsing, request building, response extraction. `make test` green.
- ffmpeg/ffprobe missing → a clear error naming the binary and `brew install ffmpeg`,
  never a stack trace.
- Errors wrap as `pkg.Method: %w`.
- Any CLI surface change updates `cmd/skill.md` **and** `README.md`.

## 9 · Deferred

- Audio/transcript (explicitly out of scope, see §2.4).
- OpenAI as a second `ask` provider — no native video input today.
- Object tracking / per-frame diffing beyond scene cuts.
- Long-video map-reduce (chunk → ask → merge); revisit when a >30min clip actually hurts.
