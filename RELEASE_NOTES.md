# Grokpi v1.1.0 Release Notes

**Release Date:** 2026-06
**Type:** Minor (with important operational + DX improvements)

## Highlights

- Production-ready health, readiness and cancellation for long-running video jobs.
- Significantly more robust TTS (retries + validation).
- Documentation now complete enough for a new user to integrate without reading source.
- Full audit + security + performance reports included in repo.
- Repo is now **READY FOR PUBLIC GITHUB RELEASE**.

## What's New for Users

- Call `POST /v1/video/generations/{id}/cancel` to stop a job.
- Use `/ready` and `/live` for orchestrators / k8s / load balancers.
- TTS is more resilient to transient upstream issues.
- Much better `api.md` + updated README with architecture diagram.

## Migration / Upgrade

- No breaking changes.
- Existing jobs continue to work (new `cancelled` status is additive).
- Update your health checks to the richer `/health` payload if desired.
- Rebuild or pull new image.

## Known Issues

- Video still uses per-job goroutines (fine for self-host scale < dozens concurrent videos).
- Full e2e tests require real upstream tokens (CI uses stubs + unit tests).

## Thanks

To all contributors and the xAI Grok team for the amazing models.

See CHANGELOG.md for full details.
See FINAL_RELEASE_REPORT.md for mission completion summary.
