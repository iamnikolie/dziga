# Security Policy

## Supported versions

The latest release is the supported one. Fixes land on `main` and go out in the
next tag.

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

Use GitHub's private vulnerability reporting instead:
[Security → Report a vulnerability](https://github.com/iamnikolie/dziga/security/advisories/new).
That opens a private advisory visible only to the maintainers.

Include what you did, what happened, and the impact you think it has. Expect a
first response within a week — this is a spare-time project, not a product with
an on-call rotation.

## Scope notes

Some things are known and by design rather than vulnerabilities:

- **The API key is stored in plain text** in `~/.dziga/<profile>/config.yaml`
  with mode 0600 — the same posture as `~/.aws/credentials` or `.netrc`. Use
  `GEMINI_API_KEY` from a secret manager if you need better than that.
- **`ask` uploads video to Google.** That is the whole point of the command,
  but it is worth stating: anything you pass to `ask` leaves your machine. The
  local commands (`probe`, `scenes`, `sheet`, `frames`) never do.
- **`dziga` shells out to `ffmpeg` and, for URLs, `yt-dlp`.** Both are treated
  as trusted local binaries. A malicious media file is `ffmpeg`'s threat model,
  not this tool's — keep `ffmpeg` current.
- **Downloaded and generated files land on disk** under the working directory
  with default permissions. Nothing is cleaned up automatically.

A key that leaks is revoked in Google AI Studio.
