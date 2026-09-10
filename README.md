# dziga

[![CI](https://github.com/iamnikolie/dziga/actions/workflows/ci.yml/badge.svg)](https://github.com/iamnikolie/dziga/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iamnikolie/dziga.svg)](https://pkg.go.dev/github.com/iamnikolie/dziga)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Turn a video into context an LLM can actually read.

Named after Dziga Vertov — `дзиґа` is Ukrainian for a spinning top, the pseudonym he
took for the crank of a movie camera. His *Kino-Eye* is the whole premise: a machine
eye that sees for someone who cannot. An agent cannot watch video; this is its eye.

Sibling of [`fibery-cli`](https://github.com/iamnikolie/fibery-cli) and
[`gitlab-cli`](https://github.com/iamnikolie/gitlab-cli): same doctrine — the CLI
writes files and prints paths, the agent reads them.

## What it does

Two engines, deliberately separate:

- **Local (ffmpeg, no key, no network, $0)** — `probe`, `scenes`, `sheet`, `frames`.
  Builds contact sheets with the timecode burned into every cell, and full-size
  stills at exact moments. The agent looks with its own eyes.
- **Gemini native video** — `ask`. For what stills cannot show: motion, ordering,
  long clips, on-screen text at scale.

## Install

`ffmpeg` is required — everything local is built on it. `yt-dlp` is only needed
if you pass URLs instead of local files.

```bash
brew install ffmpeg        # or: apt install ffmpeg
pip install yt-dlp         # optional
```

Then pick one:

**Prebuilt binary** — download the archive for your platform from
[Releases](https://github.com/iamnikolie/dziga/releases):

```bash
tar xzf dziga_*_darwin_arm64.tar.gz
sudo mv dziga /usr/local/bin/
```

**With Go** (1.24+):

```bash
go install github.com/iamnikolie/dziga@latest
```

**From source** — `make install` symlinks the binary, so a later `make build`
updates the installed CLI without reinstalling:

```bash
git clone https://github.com/iamnikolie/dziga.git
cd dziga
make install               # symlink → ~/.local/bin/dziga
```

Check what you got with `dziga version`.

## Setup

Nothing is required for the local commands. `ask` needs a Gemini key:

```bash
export GEMINI_API_KEY=...          # simplest
# or a profile:
dziga config init --config personal
dziga config show --config personal
```

Config lives at `~/.dziga/<profile>/config.yaml` (mode 0600). `DZIGA_HOME` moves the
root; `DZIGA_CONFIG` sets the default profile.

The file is mode 0600 and holds the key in plain text — the same posture as
`~/.aws/credentials` or a `.netrc`. `GEMINI_API_KEY` takes precedence if you
would rather source it from a secret manager. The local commands (`probe`,
`scenes`, `sheet`, `frames`) need no key and make no network calls at all.

## Usage

```bash
dziga context clip.mp4                     # digest + one labelled contact sheet
dziga probe clip.mp4                       # 0:31.2 · 1080x1920 · 30 fps · h264 · audio aac · 24.1MB
dziga scenes clip.mp4                      # cut list
dziga sheet clip.mp4 -n 16 --scenes        # one cell per shot
dziga frames clip.mp4 --at 3.5,12,1:04     # full-size stills
dziga sheet  clip.mp4 --keep-cells         # keep the individual cell images too
dziga ask clip.mp4 "What is written on the sign at 0:12?"
dziga ask "https://www.instagram.com/reel/…" "Summarise this reel."
```

Artifacts go to `~/.dziga/work/<clip>-<hash>/` — a tool an agent runs from inside a
repo must not scatter jpegs across it. `--out .` overrides.

Every command takes `--json`. `dziga skill` prints the agent-facing reference.

## What it costs

Local commands (`probe`, `scenes`, `sheet`, `frames`) cost nothing — no API is
involved. `ask` bills Gemini tokens, and the video part is predictable:

```
video_tokens ≈ frames(window × fps) × 66   (--res high: 264)  + 25/s if the file has audio
```

Measured against clips of known length on `gemini-3.7-flash`, not derived from docs.
On the default model that puts a 30-second reel at ~$0.002 and an hour of footage at
`--fps 0.2` around $0.10.

```bash
dziga ask clip.mp4 --estimate "question"   # predicted tokens + dollars, no API call
dziga models                               # rates in force today, measured video rates
```

After a call the status line prices what was actually spent, using the model that
served the request. Where no published rate exists the CLI prints `cost≈?` and the real
token counts rather than inventing a number.

## Why sheets are sized the way they are

Claude downscales any image past 1568px on its long edge or ~1.15 megapixels, and
bills roughly `width*height/750` tokens. Sheets are solved to land just under both
caps, so a 16-cell sheet costs ~1.5k tokens whatever the clip's aspect — more cells
buys more coverage at the price of detail, never a bigger bill. The status line
prints the estimate before you read it.

## Development

```bash
make test          # go test ./...
make vet           # go vet ./...
make fmt           # gofmt -w .
make build         # build ./dziga, version stamped from git describe
make install       # symlink to ~/.local/bin
```

CI runs gofmt, `go vet` and `go test -race` on Linux and macOS for every push
and pull request. Tests never touch the network or shell out to ffmpeg — they
exercise the pure parts (sampling math, timecode formatting, sheet geometry).

- `SPEC.md` — build contract and the live-verified API shapes.
- `cmd/skill.md` — agent reference (embedded, `dziga skill`).
- Model ids and prices are data in `internal/registry/registry.yaml`.

## Contributing

Issues and pull requests are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE) © Mykola Klitovchenko

Not affiliated with Google or the Gemini API; `ffmpeg` and `yt-dlp` are separate
projects under their own licenses.
