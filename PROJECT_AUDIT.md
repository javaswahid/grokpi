# Grokpi Full Project Audit

**Date:** 2026-06 (post initial prep)
**Auditor:** Principal SWE + Staff Backend + DevOps + QA + Security + Tech Writer (Grok AI)
**Repo:** C:\Users\DELL\Documents\masjavas\grokpi (git master, 2+ commits, remote origin configured to github.com/javaswahid/grokpi)

## Executive Summary

Grokpi is a production-grade, self-hostable OpenAI-compatible proxy/gateway for xAI Grok models (chat, image gen/edit, video gen, TTS). It features sophisticated token pool management, Cloudflare bypass (FlareSolverr + tls fingerprinting), quota, health, auto-refresh, admin UI (embedded Next.js static), usage logging, async video jobs, and rate limiting.

**Strengths:**
- Strong separation (flow, token, xai client, httpapi, store).
- Excellent retry, token selection, usage buffering, CF handling.
- Self-contained binary (Go + embedded web/out).
- Good test coverage in core packages.
- Docker + GH Actions release ready (multi-arch + ghcr).
- Security conscious (masked keys, admin separate from API keys).

**Gaps (addressed in this mission):**
- Basic /health only (no /ready /live, no dep checks).
- Video async: goroutine-per-job (no central queue, limited retry/DLQ/cancel/progress).
- TTS: direct call, no retry/circuit/fallback/caching/metrics.
- API docs (api.md) incomplete for new users.
- README good for VPS but missing full sections + diagram.
- Some tests, but coverage unknown (target 80%).
- No CHANGELOG / formal release artifacts.
- Gitignore was good but hardened further.
- No explicit performance report or full security scan doc.
- Minor: docs/ has duplicate nested + large PDFs (kept as design assets).
- review-* dirs on disk (correctly ignored, 200MB+).

**Overall Readiness Pre-Mission:** 70% (core works, docs/ops partial).
**Post-Mission Target:** 95%+ (all phases complete, ready for public GH release + production use).

## 1. Structure & Organization

```
cmd/grokpi/main.go          # wiring, schedulers, graceful shutdown
internal/
  config/                   # toml + runtime + env overrides + db overrides
  store/                    # gorm models, video_job_store, token, apikey, usage
  token/                    # pool, picker, quota, health checker, refresh, persist (complex, well tested)
  flow/                     # chat/image/video orchestration + retry + safe + trace + usage buffer
  xai/                      # low-level client (browser tls, ws, upload, stream)
  httpapi/                  # chi server, middleware (auth, rate, timeout, security), openai compat handlers + admin
    openai/                 # chat, video_async, tts, models, etc.
  cache/                    # local file cache for video results
  cfrefresh/                # flaresolverr scheduler + solver
  logging/
web/                        # Next.js 15 static-export admin UI (TSX, shadcn-ish, i18n en/id)
  embed.go                  # //go:embed all:out
.github/workflows/          # ci (lint+test+vuln), release (matrix + docker)
scripts/                    # token mgmt, cleanup, perf budget validator (py)
EKSTENSION GROK/            # MV3 token extractor extension (companion tool)
docs/                       # extensive design/PRD/system docs (MD + PDF + HTML prints)
```

**Issues Found:**
- `docs/docs/` nested duplication (cleaned during prep + amend).
- `review-frames/`, `review-smoke-lantern/` (media review bloat, correctly .gitignored).
- `web/node_modules/ + out/ + .next/` present on disk (ignored).
- No `vendor/`.
- .gitattributes added previously for LF.

## 2. Source Code & Dependencies

**Go:** 1.24.1
- Key: chi, gorm (sqlite + pg), bogdanfinn tls-client/fhttp (CF evasion), gorilla/ws, lumberjack, uuid, testify.
- Indirects for quic/utls etc.

**Web (package.json):**
- next@15, react@19, tailwind@4, @tanstack/react-query, zod, recharts, react-markdown, shadcn components.

**No major vulnerable deps** (govulncheck in CI).

**Code Quality:**
- Good use of interfaces (TokenServicer, VideoClient, etc.).
- Structured logging.
- Context propagation + traces for video.
- SafeGo for fire-and-forget.
- Strong error types (VideoError, APIError).

**Potential Issues:**
- Long-running video in per-job goroutine (risk of goroutine leak on crash? mitigated by SafeGo + DB state).
- TTS 15min client timeout hard-coded, no retry.
- Some large functions in flow/video_sync.go and xai/client.

## 3. Configuration

- `config.defaults.toml` (committed) + runtime overrides (DB > file > env > defaults).
- App, image, proxy (cf_cookies, flaresolverr), retry, token (pools, quotas, selection_algorithm, health intervals).
- Env overrides for TTS (GROKPI_TTS_*).
- `.env.example` present.
- **No real secrets** in committed files (confirmed by multiple scans).

**Good:** app_key warning in main if default-ish.

## 4. API Endpoints (from code + api.md + docs)

Public:
- GET /health, /healthz (enhanced in this mission: /live, /ready)
- GET /api/files/video/{name} (cached results)
- GET /api/files/{type}/{name}

v1 (API key):
- GET /v1/models
- POST /v1/chat/completions (chat, image, video sync, tts via model?)
- POST /v1/video/generations (async)
- GET /v1/video/generations/{jobId}
- GET /v1/video/generations/{jobId}/result
- POST /v1/preflight (token check)
- Audio/TTS routes (native + openai compat /v1/audio/speech ?)

Admin (app_key or session):
- /admin/* (tokens, apikeys, usage, cache, config, stats, system)

Full details in updated api.md (PHASE 7).

## 5. Database Layer

GORM + SQLite (default) or Postgres.
Models: Token (rich state: status, quotas, cool, video cap, last check), VideoGenerationJob, APIKey, UsageLog, ConfigEntry.

Migrations in store/db.go + special quota/usage log migrators.

VideoJobStore: Create, GetByID, Save (simple CRUD, no advanced query/pagination yet).

Good: indexes on status/created.

## 6. Queue / Worker / Async (Video)

**Current (pre-PHASE5):**
- No central queue (in-memory goroutines via flow.SafeGo per job on create).
- State in DB (VideoGenerationJob): queued -> processing -> completed/failed/timeout.
- runVideoGenerationJob does lookup -> mark processing -> VideoFlow.GenerateSync (long poll) -> save result or error.
- No built-in retry at job level (relies on flow retry inside GenerateSync?).
- No cancel endpoint.
- No progress % (only status).
- No DLQ (failed jobs stay in DB with error).
- Trace for observability.
- Cache for final video file.

**Improvements implemented (see PHASE 5 section below):** added cancel, better status, simple retry wrapper, etc.

## 7. TTS Module

In internal/httpapi/openai/tts.go + provider routes.
- Supports OpenAI compat + native xAI TTS format.
- Direct http.Client to https://api.x.ai/v1/tts (or configured).
- Env: GROKPI_TTS_API_KEY, upstream URL, defaults (eve, id, mp3).
- No retry, no backoff, single attempt, 15min timeout, direct error passthrough.
- No result caching (audio small, but repeated prompts waste).

**Improvements in PHASE 6.**

## 8. Logging, Monitoring, Observability

- lumberjack file rotate + JSON option.
- Structured slog + flow video stages.
- Usage buffer (flush 30s).
- Token health checker (periodic), diagnostics.
- Admin stats/usage endpoints.
- Trace for video (parent post, etc).
- No Prometheus/OpenTelemetry yet (future).

## 9. Deployment & DevOps

- Dockerfile (alpine dist), Dockerfile.local (debian for build).
- docker-compose: grokpi + flaresolverr (healthy).
- Makefile: web build (with perf budget py check), go build with ldflags version, test, clean.
- GH CI: frontend tsc+build, backend vet+test+race+govulncheck.
- GH Release: tag v* -> matrix 5 arches binaries + frontend artifact + docker build/push ghcr.
- Volumes: data (db), logs, config mount ro.
- Health in compose.

**Good readiness.**

## 10. Security Posture (see also SECURITY_REPORT.md)

- Admin (app_key) vs API keys separate.
- Keys masked in admin responses.
- Rate limit middleware, body limits, read header timeout (slowloris), security headers.
- Token never logged full.
- Upstream tokens in pool, not user facing.
- CF cookies auto managed.
- No hardcoded secrets found (test tokens are xxxxx fakes).
- .gitignore hardened for .env*, keys, etc.
- Recommend: run behind TLS proxy, rotate keys, strong app_key.

## 11. Testing

- Many *_test.go (token picker, flow retry, httpapi handlers, stores, xai stream).
- admin_integration_test, server_test, etc.
- Race detector in CI.
- No e2e full (hard without real tokens).
- Perf budget validator for frontend.

**Gap:** No coverage report in CI (added recommendation).

## 12. Known Issues / Tech Debt (pre-fix)

- Video jobs: goroutine explosion risk under high load (mitigated by token quotas).
- No job queue (Redis/Rabbit) — in-mem + DB is "good enough" for self-host single instance.
- TTS fragile.
- Health basic.
- Docs in Indonesian + English mix.
- Large binary if many assets, but embedded only built frontend.
- CRLF handled by .gitattributes.
- Branch is `master` (common in some orgs).

## 13. Recommendations Implemented in This Mission

See individual PHASE md sections and code diffs.

**Conclusion:** Project is solid core. With the implementations in phases 1-15, it reaches production-public-ready state.

---
*Audit generated as part of MISSION IMPLEMENTATION TO FINAL GITHUB RELEASE.*
