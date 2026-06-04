#!/usr/bin/env node

const { baseUrl, listTokens, writeReport, isPolicyToken, isRefreshEligible } = require("./token-admin-client");

(async () => {
  const tokens = await listTokens();
  const statusSummary = {};
  const quarantined = [];
  const refreshEligible = [];

  for (const token of tokens) {
    statusSummary[token.status || "active"] = (statusSummary[token.status || "active"] || 0) + 1;
    if (isPolicyToken(token)) {
      quarantined.push({
        id: token.id,
        status: token.status,
        reason: token.status_reason || token.warning_message || "",
      });
      continue;
    }
    if (isRefreshEligible(token)) {
      refreshEligible.push({
        id: token.id,
        status: token.status,
        reason: token.status_reason || token.warning_message || "",
        coolUntil: token.cool_until || null,
      });
    }
  }

  const report = {
    baseUrl,
    total: tokens.length,
    statusSummary,
    policyQuarantineCount: quarantined.length,
    refreshEligibleCount: refreshEligible.length,
    quarantined,
    refreshEligible,
    policyRule: "policy_quarantine tokens are never auto-enabled by this script",
  };
  const out = writeReport("token-health-report.json", report);
  console.log(JSON.stringify({ ok: true, report: out, ...report }, null, 2));
})().catch((err) => {
  console.error(JSON.stringify({ ok: false, error: err.message }, null, 2));
  process.exit(1);
});
