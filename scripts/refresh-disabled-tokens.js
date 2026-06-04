#!/usr/bin/env node

const { baseUrl, listTokens, syncQuota, writeReport, isPolicyToken, isRefreshEligible } = require("./token-admin-client");

(async () => {
  const tokens = await listTokens();
  const refreshed = [];
  const skipped = [];
  const failed = [];

  for (const token of tokens) {
    if (isPolicyToken(token)) {
      skipped.push({ id: token.id, status: token.status, reason: "policy_quarantine_not_auto_refreshed" });
      continue;
    }
    if (!isRefreshEligible(token)) {
      skipped.push({ id: token.id, status: token.status, reason: "not_refresh_eligible" });
      continue;
    }
    try {
      const result = await syncQuota(token.id);
      refreshed.push({ id: token.id, before: token.status, after: result.status, reason: result.status_reason || "" });
    } catch (err) {
      failed.push({ id: token.id, status: token.status, error: err.message });
    }
  }

  const report = {
    baseUrl,
    refreshedCount: refreshed.length,
    skippedCount: skipped.length,
    failedCount: failed.length,
    refreshed,
    skipped,
    failed,
    policyRule: "policy_quarantine tokens require safe prompt repair and are skipped",
  };
  const out = writeReport("token-refresh-report.json", report);
  console.log(JSON.stringify({ ok: failed.length === 0, report: out, ...report }, null, 2));
  if (failed.length > 0) process.exit(1);
})().catch((err) => {
  console.error(JSON.stringify({ ok: false, error: err.message }, null, 2));
  process.exit(1);
});
