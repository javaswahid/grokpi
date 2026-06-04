# Grokpi API Reference (OpenAI Compatible + Extensions)

Grokpi is a self-hosted OpenAI-compatible gateway/proxy for Grok models (chat, image, video, TTS) with advanced token pooling, quotas, Cloudflare bypass, admin UI, and async job support.

**Base URL (local default):** `http://127.0.0.1:8080`

**Auth:**
- Public (no auth): `/health*`, `/live`, `/ready`, `/api/files/*`
- User API: `Authorization: Bearer <YOUR_API_KEY>` for all `/v1/*`
- Admin: `Authorization: Bearer <APP_KEY>` (from `config.toml` `[app] app_key`) for `/admin/*` and login

**Error format (consistent):**
```json
{
  "error": {
    "code": "invalid_api_key",
    "message": "Invalid API key",
    "type": "invalid_request_error"
  }
}
```

Common status codes: 200/201/202, 400, 401, 403, 404, 409, 429, 500, 502, 503.

---

## Health & Ops Endpoints (No Auth)

### GET /health (or /healthz)
Detailed health. Returns 200 even if some services degraded.

**Response 200:**
```json
{
  "status": "healthy",
  "version": "dev",
  "uptime": "1h2m3s",
  "timestamp": "2026-06-04T10:00:00Z",
  "database": "ok",
  "queue": "ok",
  "storage": "ok",
  "tts": "ok",
  "video": "ok"
}
```

**cURL:**
```bash
curl -s http://127.0.0.1:8080/health | jq
```

### GET /live
Liveness probe (process alive?).

**Response 200:** `{"status":"alive","uptime":"...","timestamp":"..."}`

Use for Kubernetes livenessProbe.

### GET /ready
Readiness probe (can accept traffic?).

Returns 200 if ready, 503 if not (with `checks` map).

**Example response (ready):**
```json
{
  "status": "ready",
  "timestamp": "...",
  "checks": { "database": "ok", "config": "ok", "token": "ok", "video": "ok" }
}
```

**cURL (expect 200 or 503):**
```bash
curl -s -w "%{http_code}" http://127.0.0.1:8080/ready
```

---

## v1 API (Requires API Key)

### GET /v1/models
List models available to the calling API key (filtered by pool + whitelist).

**Request:**
```bash
curl -s http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer $API_KEY" | jq
```

**Success 200:**
```json
{
  "object": "list",
  "data": [
    {"id": "grok-4", "object": "model", "created": 1709251200, "owned_by": "xai"},
    {"id": "grok-imagine-1.0-video", "object": "model", ...}
  ]
}
```

**Errors:** 401 invalid_api_key, 429 rate/daily limit.

### POST /v1/chat/completions
Core endpoint. Supports:
- Chat (text + tools)
- Image generation (`grok-imagine-...`)
- Image edit
- Video sync (blocking, `grok-imagine-1.0-video`)
- TTS via certain models or dedicated audio routes

See full OpenAI spec + Grok extensions (`image_config`, `video_config`, `thinking`).

**Minimal chat example:**
```bash
curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-3-mini",
    "messages": [{"role": "user", "content": "Hello from Grokpi"}],
    "stream": false
  }' | jq
```

**Image example:**
```json
{
  "model": "grok-imagine-1.0",
  "messages": [{"role":"user","content":"A serene mountain lake at dawn"}],
  "image_config": {"aspect_ratio": "16:9", "n": 1}
}
```

**Video sync (blocking, prefer async for long jobs):**
Add `"video_config": {"aspect_ratio":"16:9", "video_length": 6, "resolution_name":"480p"}`

**Streaming:** `stream: true` → SSE (text/event-stream).

**Common errors:**
- 400 invalid_request_error / missing_prompt / invalid_video_config
- 403 media_generation_disabled or model_not_allowed
- 429 (token or api key limits)

---

## Async Video (Recommended for Video)

### POST /v1/video/generations
Submit async video job. Returns 202 + jobId immediately.

**Request (same as chat video but dedicated):**
```bash
curl -s -X POST http://127.0.0.1:8080/v1/video/generations \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-imagine-1.0-video",
    "messages": [{"role":"user","content":"Cinematic drone shot over rice fields at sunrise"}],
    "video_config": {
      "aspect_ratio": "16:9",
      "video_length": 8,
      "resolution_name": "480p",
      "preset": "normal"
    }
  }'
```

**Success 202:**
```json
{
  "jobId": "vid_job_abc123def456",
  "status": "queued",
  "model": "grok-imagine-1.0-video",
  "createdAt": "2026-...",
  "updatedAt": "2026-..."
}
```

### GET /v1/video/generations/{jobId}
Poll status.

**Possible statuses:** `queued`, `processing`, `completed`, `failed`, `timeout`, `cancelled`

**Success 200:** full job object (includes `videoUrl` when completed, `errorCode`/`errorMessage` on failure).

### GET /v1/video/generations/{jobId}/result
Only for completed jobs. Returns direct video URL (or 409 if not ready).

### POST /v1/video/generations/{jobId}/cancel   *(NEW)*
Cancel a queued/processing job (best-effort).

```bash
curl -s -X POST http://127.0.0.1:8080/v1/video/generations/vid_job_xxx/cancel \
  -H "Authorization: Bearer $API_KEY"
```

Returns the updated job (status=cancelled) or 409 if already terminal.

**Notes:**
- Jobs are owned by the creating API key.
- Final videos are served via `/api/files/video/{name}` (cached).
- Long jobs use the async path to avoid  client timeouts.

---

## TTS / Audio

### POST /v1/audio/speech (OpenAI compatible)
```bash
curl -s -X POST http://127.0.0.1:8080/v1/audio/speech \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-tts",
    "input": "Halo, ini tes TTS dari Grokpi.",
    "voice": "eve",
    "response_format": "mp3"
  }' --output speech.mp3
```

### POST /tts (native xAI payload)
Use for advanced params (`voice_id`, `language`, `output_format`, `text_normalization` etc.).

Env config (server side): `GROKPI_TTS_API_KEY`, `GROKPI_TTS_UPSTREAM_URL`, defaults for voice/language.

**Improvements (PHASE 6):** retry on 5xx/429, better validation, per-attempt timeout, clear error messages.

**Errors:** 400 missing_input, 501 tts_upstream_not_configured, 502 upstream_failed (after retries).

---

## Admin Endpoints (APP_KEY auth)

See Admin UI (http://host/login) or direct:

- GET/POST/DELETE /admin/tokens
- GET/POST /admin/apikeys
- GET /admin/usage , /admin/stats
- GET/POST /admin/config
- GET /admin/cache , /admin/system

Full details in built-in docs or source `internal/httpapi/admin_*.go`.

---

## File Serving (Public for results)

`GET /api/files/video/{name}` — serves cached video results (no auth, for shareable links from jobs).

---

## Full cURL Collection (copy-paste ready)

See sections above. Also health:

```bash
curl -s http://127.0.0.1:8080/ready | jq
```

## Rate Limits & Quotas

- Per token (configurable in admin or config.toml `[token]`)
- Per API key (daily + per-minute via admin)
- 429 responses include `retry-after` hints where applicable.

## Versioning & Compatibility

- OpenAI Chat Completions + Images + Audio speech compatible.
- Grok-specific extensions via extra fields in request (`image_config`, `video_config`, `thinking`).
- Always check `/v1/models` for live available models (depends on your token pool).

---

*This document is the canonical reference. Source of truth for integrators. Updated as part of full release preparation.*
*For even more internal details see `docs/DOKUMEN_*.md`.*

**Last updated:** during PHASE 7 of release mission (added cancel, full health, retry notes, complete examples for video/TTS).
