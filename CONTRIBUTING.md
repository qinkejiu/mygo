# Contributing to MyGO

Thanks for helping strengthen the compiler! This document summarizes the minimum friction to land a change.

## Development Flow

1. Install Go 1.25+ and ensure `circt-opt` / `verilator` are visible on `PATH`.
2. Clone the repo and install the CLI: `go install ./cmd/mygo`.
3. Format and lint locally:
   ```bash
   gofmt -w $(git ls-files '*.go')
   go vet ./...
   ```
4. Run the full suite (unit tests plus stage harness):
   ```bash
   go test ./...
   ```
5. Keep the module graph tidy by running `scripts/tidy.sh` whenever dependencies change.

The CI workflow (`.github/workflows/ci.yml`) enforces gofmt, `go vet`, `go test`, and makes sure `go.mod` / `go.sum` stay tidy. Aim to keep the tree clean before opening a pull request.

## Commit Guidelines

- Prefer small, focused commits with descriptive messages.
- Document non-obvious design choices directly in the code or in `docs/`.
- Update or extend tests whenever behavior changes.
- Link to any relevant design docs or issues in your PR description.

## Coding Style

- Go code follows the standard `gofmt` style.
- Keep comments actionable; prefer short, high-signal notes over long prose.
- Place new integration tests under `tests/stages/` and goldens next to their `main.go`.

## Dependency Policy

- Favor the standard library when possible.
- When pulling external code, update the README's Notice section with the license details and document how it is consumed so downstream users can comply.
- Regenerate `go.mod` and `go.sum` via `scripts/tidy.sh` and include the results in your change.

## Reporting Bugs

Please file issues with:

- What you expected to happen.
- The command(s) you ran plus flags.
- A minimal reproducer (ideally a `tests/stages/<case>` directory).
- Environment info (`go version`, OS, CIRCT/Verilator versions).

