# Contributing to Grokpi

Thank you for your interest in contributing to Grokpi! This document provides guidelines and instructions for contributing.

## Code of Conduct

- Be respectful and inclusive.
- Focus on constructive feedback.
- Follow the project's coding style and patterns.

## How to Contribute

### Reporting Bugs

1. Search existing issues first to avoid duplicates.
2. Use a clear title and include:
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment (OS, Go version, Docker, etc.)
   - Relevant logs (sanitized, no tokens/secrets)
   - Config snippet (redacted)

### Suggesting Enhancements

Open an issue with:
- Use case / motivation
- Proposed API or behavior changes
- Alternatives considered

### Pull Requests

1. Fork the repo and create a feature branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```
2. Make focused, atomic commits.
3. Ensure tests pass:
   ```bash
   make test
   # or
   go test -race ./...
   ```
4. For frontend changes (in `web/`):
   ```bash
   cd web
   npm ci
   npm run build
   ```
5. Update documentation (README, api.md, comments) as needed.
6. Open PR with clear description, linked issue if any.

## Development Setup (Local)

Prerequisites: Go 1.24+, Node.js 20+, make (optional).

```bash
# 1. Clone
git clone https://github.com/crmmc/grokpi.git
cd grokpi

# 2. Prepare config (never commit real secrets)
cp config.defaults.toml config.toml
# Edit config.toml: change app.app_key to a strong value

# 3. (Optional) Build frontend + Go binary
make build

# 4. Run directly (dev mode, auto-rebuild web not automatic)
go run ./cmd/grokpi -config config.toml
# or
make dev
```

Frontend dev (separate, for UI work):
```bash
cd web
npm ci
npm run dev   # note: the embedded UI uses static export; dev server is for rapid iteration only
```

Admin UI is served at `/` (or `/login`) from the Go binary (embedded `web/out`).

## Testing

- Backend: `go test -race -v ./...`
- With coverage: `go test -race -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`
- Frontend type check + build is run in CI.

Some integration tests require real tokens or mocks; they are skipped or use test tags when appropriate.

## Security & Secrets

**CRITICAL**:
- Never commit `config.toml`, `.env*`, real API tokens, cookies (`cf_cookies`, `cf_clearance`), passwords, or private keys.
- `.gitignore` is configured to exclude these.
- Use `config.defaults.toml` as the committed template.
- Tokens belong in the Admin UI (Token Management) or environment variables (see docker-compose.yml and .env.example).
- If you accidentally commit a secret, rotate it immediately and remove from history (e.g. `git filter-repo` or BFG).

## Style Guidelines

### Go
- Follow standard Go conventions (`gofmt`, `goimports`).
- Use structured logging via `internal/logging`.
- Prefer explicit error handling.
- Add tests for new logic (especially token picker, flow, httpapi handlers).
- Keep HTTP handlers thin; logic in `internal/flow`, `internal/token`, etc.

### TypeScript / React (web/)
- Strict TypeScript.
- Use existing UI patterns (Tailwind + shadcn-inspired components in `src/components`).
- Prefer server actions or TanStack Query for data.
- Run `npm run lint` / build before PR.

### Config & Docs
- TOML for config.
- Update `api.md` for API changes.
- Keep `README.md` installation/run/deploy sections accurate.

## Commit Messages

Use conventional style:
- `feat: add video job status polling`
- `fix(token): prevent quota underflow on concurrent use`
- `docs: clarify docker volume permissions`
- `refactor(httpapi): extract admin middleware`

## Release Process

Releases are handled via GitHub Actions (see `.github/workflows/release.yml`).
- Tag with semver: `git tag v1.2.3 && git push --tags`
- CI builds multi-arch binaries + Docker images.

## Questions?

Open a discussion or issue. For security issues, please contact the maintainer privately (do not use public issues).

Thank you for helping make Grokpi better!
