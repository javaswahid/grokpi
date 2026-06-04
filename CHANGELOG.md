# Changelog

All notable changes to Grokpi will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-06

### Added
- Full health/readiness probes: `/health` (detailed), `/live`, `/ready` (with dependency checks + timeout handling). Includes unit tests.
- Video job cancellation: `POST /v1/video/generations/{jobId}/cancel` + context propagation + early exit.
- Video job improvements: explicit `cancelled` status, simple retry (transient errors), better ctx handling in runner.
- TTS hardening: retry (2 attempts on 5xx/429), per-attempt timeouts, validation, clearer errors + logging.
- Comprehensive `api.md` rewrite with every endpoint, examples (curl), errors, status codes.
- Modernized `README.md` (Overview, Features, Architecture mermaid diagram, Env vars, Health, etc.).
- `CONTRIBUTING.md`, `PROJECT_AUDIT.md`, `SECURITY_REPORT.md`, `PERFORMANCE_REPORT.md`, `FINAL_RELEASE_REPORT.md`.
- `.gitattributes` for consistent line endings.
- Hardened `.gitignore` (more secret/build/media patterns).
- `VERSION` file + `CHANGELOG.md` + `RELEASE_NOTES.md` for release process.
- Additional health tests + structure for higher coverage.

### Changed
- Health response now includes version + per-component status (db/tts/video/queue/storage).
- Video statuses now include `cancelled`.
- CI enhanced with build smoke + health notes.
- Many small robustness improvements from full audit (no behavior changes for existing clients).

### Fixed
- Duplicate docs/ nesting cleaned during prep.
- Scripts/ no longer accidentally ignored.
- Config.toml never committed (removed duplicate + ignored).

### Security
- Confirmed clean (no secrets in tree). See SECURITY_REPORT.md.
- .gitignore and docs now explicitly call out secret rules.

## [1.0.0] - Initial public release prep

- Core token pool, flow, xai client, admin UI, async video, Docker, GH release pipeline.
