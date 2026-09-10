# Contributing

Thanks for taking a look. This is a small, focused CLI — bug reports and pull
requests are welcome, and so is a plain question in an issue.

## Reporting a bug

Include the output of `dziga version`, your `ffmpeg -version` (first line), the
exact command you ran, and what you expected instead. For anything about
sampling, sheets or timecodes, `dziga probe <file>` output is usually the piece
that explains it — container metadata differs wildly between sources.

If the video that triggers it can be shared, a 10-second clip that reproduces
the problem is worth more than any description.

## Pull requests

Before opening one:

```bash
make fmt           # gofmt -w .
make vet           # go vet ./...
make test          # go test ./...
```

CI runs the same three (plus `-race`) on Linux and macOS, so a green local run
usually means a green PR.

House rules:

- **One concern per PR.** A bug fix and a refactor in the same diff take three
  times as long to review.
- **Tests stay hermetic.** The suite does not shell out to ffmpeg and does not
  hit the network. Keep the math — sampling, geometry, timecode formatting —
  in functions that can be tested on their own, which is how the existing code
  is structured.
- **The local engine stays free and offline.** `probe`, `scenes`, `sheet` and
  `frames` must keep working with no API key and no network. Anything that
  needs a model belongs behind `ask`.
- **Write files, print paths.** The CLI's contract with an agent is that it
  leaves artifacts on disk and says where they are. Don't add commands that
  dump binary or huge output to stdout.
- **Update the docs in the same commit.** Any change to the CLI surface must
  also update `cmd/skill.md` (embedded in the binary, printed by `dziga skill`)
  and `README.md`. `SPEC.md` carries the build contract and the verified API
  shapes — keep it true.
- **Conventional commit subjects** — `feat:`, `fix:`, `docs:`, `refactor:`,
  `test:`, `chore:`. Release notes are generated from them.

## Model ids and prices

Those are data, not code: `internal/registry/registry.yaml`. Updating a price or
adding a model should be a one-file change.

## Releases

Maintainer-only. Tag and push:

```bash
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

GoReleaser builds archives for linux/darwin/windows on amd64 and arm64 and
publishes the GitHub release with a generated changelog.
