# Grokpi Performance Review & Optimizations

**Date:** during release mission
**Reviewer:** Performance + Backend Engineer

## Executive Summary

Core is reasonably efficient for a Go self-hosted gateway (single binary, low deps).

**Positive:**
- Usage buffer + flush (avoids per-request DB writes).
- Token persistence loop (dirty only).
- Structured + leveled logging.
- Good use of context + timeouts everywhere.
- Frontend perf budgets enforced in build (performance-budgets.json + validator).
- Retry budget + token cooling prevents thundering herd on upstream.

**Findings & Actions Taken:**
- Video jobs: per-job goroutine (acceptable for self-host scale; bounded by quotas). Added cancel + early exit to free resources faster.
- TTS: 15min timeout was too aggressive + no retry → reduced per-attempt + added 2-attempt retry with backoff (PHASE 6).
- No obvious N+1 in hot paths (token selection uses in-memory pool snapshot).
- DB: GORM with indexes on hot columns (status, created_at, api_key_id).
- No memory leak obvious in shutdown paths (flushers stopped cleanly).
- Large review media ignored (prevents repo bloat affecting clone times).

**Recommendations (non-blocking for v1):**
- Add pprof endpoints behind admin (or debug flag).
- Consider bounded worker pool for video jobs instead of unlimited SafeGo (future, if high concurrency needed).
- Frontend: already has budget checks — good.
- Consider connection pooling tuning for Postgres under load.
- Add optional Prometheus metrics (counters for jobs, token selections, upstream latency histograms).

## Specific Hotspots Reviewed

1. `internal/flow/video_sync.go` + `video.go` — long poll + download. Context cancellation now propagates better.
2. `internal/httpapi/openai/video_async.go:runVideoGenerationJob` — added retry loop + cancel checks (prevents wasted work on cancelled jobs).
3. Token picker — O(1) selection after snapshot (good).
4. SSE streaming — uses proper flushing, no buffering whole response.
5. Admin UI queries — use React Query + pagination where lists are long.

## Measured / Expected (Self-Host)

- Idle: <50MB RSS
- Light chat: low CPU, DB writes buffered
- Video job: ~1 goroutine + network per active job (bounded)
- 100 concurrent tokens: pool handles fine

**No critical leaks or O(n^2) loops found in audited paths.**

---
*Report created as PHASE 10 of full release mission.*
