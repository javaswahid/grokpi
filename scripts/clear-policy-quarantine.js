#!/usr/bin/env node

const { baseUrl, listTokens, updateToken, writeReport } = require("./token-admin-client");

(async () => {
  const confirmed = String(process.env.SAFE_PROMPT_REPAIR_CONFIRMED || "").toLowerCase() === "true";
  const tokens = await listTokens("policy_quarantine");

  if (!confirmed) {
    const report = {
      baseUrl,
      clearedCount: 0,
      blockedCount: tokens.length,
      blocked: tokens.map((token) => ({
        id: token.id,
        status: token.status,
        reason: token.status_reason || token.warning_message || "",
      })),
      message: "Set SAFE_PROMPT_REPAIR_CONFIRMED=true only after safe-prompt-repair and safety validation pass.",
    };
    const out = writeReport("policy-quarantine-report.json", report);
    console.log(JSON.stringify({ ok: false, report: out, ...report }, null, 2));
    process.exit(tokens.length > 0 ? 2 : 0);
  }

  const cleared = [];
  const failed = [];
  for (const token of tokens) {
    try {
      const result = await updateToken(token.id, { status: "active" });
      cleared.push({ id: token.id, before: token.status, after: result.status });
    } catch (err) {
      failed.push({ id: token.id, error: err.message });
    }
  }

  const report = {
    baseUrl,
    clearedCount: cleared.length,
    failedCount: failed.length,
    cleared,
    failed,
    policyRule: "cleared only after SAFE_PROMPT_REPAIR_CONFIRMED=true",
  };
  const out = writeReport("policy-quarantine-report.json", report);
  console.log(JSON.stringify({ ok: failed.length === 0, report: out, ...report }, null, 2));
  if (failed.length > 0) process.exit(1);
})().catch((err) => {
  console.error(JSON.stringify({ ok: false, error: err.message }, null, 2));
  process.exit(1);
});
