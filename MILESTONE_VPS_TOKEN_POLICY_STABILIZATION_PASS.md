# MILESTONE_VPS_TOKEN_POLICY_STABILIZATION_PASS

Status: PASS
Date: 2026-05-07

## Scope

VPS GrokPi token policy stabilization is frozen as bugfix-only behavior.

## Approved Rule

```txt
policy violation
-> policy_quarantine
-> safe prompt repair
-> safety validation
-> token health check
-> clear/enable only after repair confirmation
```

## Guardrails

- `policy_quarantine` must not auto-refresh.
- `policy_quarantine` must not auto-clear.
- The original policy-blocked prompt must not be retried.
- `SyncQuota`, `RevalidateToken`, `RestoreToken`, and `ReportSuccess` must not activate a quarantined token.
- Image safety blocks must not parallel-retry the same prompt on other tokens.
- Video policy blocks must be treated as non-retryable until prompt repair passes.

## Approved Cron

Only health monitoring is approved for cron:

```bash
*/5 * * * * cd /path/to/grokpi && GROKPI_ADMIN_APP_KEY='APP_KEY_ANDA' node scripts/token-health-check.js
```

Do not run `clear-policy-quarantine.js` from cron.

## Manual Clear Requirement

Policy quarantine can only be cleared manually after prompt repair:

```bash
SAFE_PROMPT_REPAIR_CONFIRMED=true GROKPI_ADMIN_APP_KEY='APP_KEY_ANDA' node scripts/clear-policy-quarantine.js
```

## Acceptance

- Expired, cooldown, and rate-limited tokens can recover.
- Policy-quarantined tokens do not auto-clear.
- Old unsafe prompts are not sent again.
- Queue does not loop.
- Admin clear requires `SAFE_PROMPT_REPAIR_CONFIRMED=true`.
- Token health reports status honestly.
