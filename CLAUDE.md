# dziga — repo notes

Agent-facing **video-reading** CLI, sibling of `gengoya`/`fibery`/`gl`. Same doctrine:
token-lean stdout (**paths** and payload only), human/agent status on stderr.

The premise the whole tool rests on: **an agent cannot Read an mp4.** Every feature
exists to turn video into stills it can read or text with timestamps. If a change
makes the output less readable by an agent, it is wrong however elegant it looks.

## Sources of truth

- **[`SPEC.md`](SPEC.md)** — build contract, decisions, live-verified API shapes
- **[`cmd/skill.md`](cmd/skill.md)** — agent-facing reference (embedded; `dziga skill`)
- **[`README.md`](README.md)** — human install/usage

Any CLI surface change (commands, flags, output contract, models) **must** update
`cmd/skill.md` and `README.md`.

## Layout

```
main.go
cmd/            cobra, one file per command + embedded skill.md
                sample.go = the shared "which moments" logic behind sheet/frames/context
internal/
  config/       ~/.dziga/<profile>/config.yaml, DZIGA_HOME; env-key fallback
  registry/     embedded registry.yaml (alias → api id, prices when published)
  client/       HTTP: 429/5xx backoff, key redaction, response headers exposed
  media/        ffprobe/ffmpeg wrappers + time parsing
  sheet/        grid solver, compositing, label drawing
  provider/     Gemini Files API + generateContent
  uploads/      sha256 → file URI cache (48h, matches the API's retention)
```

## Conventions

- Module `github.com/langgerone/dziga`; errors wrap as `pkg.Method: %w`
- Config is **optional** (unlike gengoya): local commands need no key at all, and
  `ask` falls back to `GEMINI_API_KEY`/`GOOGLE_API_KEY`
- Model ids and prices are **data** (`registry.yaml`), never Go constants
- Pure functions for anything testable without a process (grid math, time parsing,
  request building, response extraction); `make test` / `make vet` stay green

## Gotchas (live-verified 2026-08-23)

- **ffmpeg's `drawtext` is not available** in the Homebrew ffmpeg 8.x build (no
  libfreetype). Timecodes are therefore composited in Go with
  `x/image/font/basicfont` upscaled by an integer factor. Do not "simplify" this back
  to a drawtext filter without checking `ffmpeg -filters` on the target machine.
- **`draw.Draw`'s source point is the pixel that lands on `dst.Min`**, not an offset
  from the cell origin. Getting this wrong renders only the first cell and leaves the
  rest of the sheet black — it looked fine in the status line, and only reading the
  actual jpeg caught it. Always Read a generated sheet after touching `sheet.Compose`.
- **Two caps, not one**: Claude downscales past 1568px long edge **and** past ~1.15MP.
  Sizing to the long edge alone produces sheets that get silently resampled. `SolveArea`
  honours both; `ChooseCols` then minimises wasted cells, since with the area fixed an
  empty slot is pure wasted tokens.
- **Gemini 3.x responses carry reasoning parts** (`thoughtSignature`, `thoughtsTokenCount`).
  `ParseAskResponse` drops parts flagged `thought` — concatenating everything leaks
  chain-of-thought into the answer.
- **Files API upload is resumable-only**: start call → `X-Goog-Upload-URL` header →
  second POST with `X-Goog-Upload-Command: upload, finalize`. A plain POST of the bytes
  does not work. Offsets in `video_metadata` are `"12.5s"` **strings**; `fps` is a number.
- **Video tokens scale with sampling, not duration alone**: an 8s clip at `fps=1` cost
  364 tokens; a 4s window at `fps=2` with `MEDIA_RESOLUTION_HIGH` cost 1106. Keep
  `--fps` and `--res` exposed and documented.
- **Cost is measured, then verified twice.** Video tokens follow
  `frames × 66 (+audio 25/s)`, `--res high` × 4 — measured on clips of known length,
  recorded in `registry.yaml` with their conditions. `--estimate` predicts before the
  call; the status line prices after it, using the `modelVersion` that actually served
  the request, because `flash`/`pro` are `-latest` aliases whose model changes
  underneath them. Assumed rates print with `~`, unknown ones as `cost≈?`.
- **Introductory rates expire.** 3.6/3.7 Flash are $0.75/$3.75 only through
  2026-12-31, then double. The registry stores `price_until` + `after:`; a lapsed rate
  with no successor reports unknown rather than a stale number. Re-verify prices at
  ai.google.dev/gemini-api/docs/pricing before trusting the table.
- **`gemini-2.5-flash` / `gemini-2.5-pro` return HTTP 404** for this key ("no longer
  available to new users") even though `/v1beta/models` still lists them. They were
  removed from the registry: a dead model in `dziga models` is a trap for an agent
  choosing one. Listed ≠ callable — test before adding.

## Deferred

- **Audio** — no transcript command, no STT, by product decision. Gemini hears the
  track inside `ask` only because it travels with the file.
- **OpenAI as a second `ask` provider** — no native video input today.
- **Long-video map-reduce** (chunk → ask → merge) — revisit when a >30min clip hurts.
- **Object tracking / per-frame diffing** beyond scene cuts.
