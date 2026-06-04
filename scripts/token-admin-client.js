#!/usr/bin/env node

const fs = require("fs");
const path = require("path");

const baseUrl = (process.env.GROKPI_ADMIN_BASE_URL || "http://localhost:8080/admin").replace(/\/+$/, "");
const appKey = process.env.GROKPI_ADMIN_APP_KEY || process.env.GROKPI_APP_KEY || process.env.APP_KEY || "";

function headers(extra = {}) {
  const h = { ...extra };
  if (appKey) {
    h.Authorization = `Bearer ${appKey}`;
  }
  return h;
}

async function request(method, route, body) {
  const res = await fetch(`${baseUrl}${route}`, {
    method,
    headers: headers(body ? { "Content-Type": "application/json" } : {}),
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { raw: text };
    }
  }
  if (!res.ok) {
    const msg = data?.error?.message || data?.message || text || `${res.status} ${res.statusText}`;
    throw new Error(`${method} ${route} failed: ${msg}`);
  }
  return data;
}

async function listTokens(status) {
  const route = status
    ? `/tokens?status=${encodeURIComponent(status)}&page_size=100`
    : "/tokens?page_size=100";
  const data = await request("GET", route);
  return Array.isArray(data?.data) ? data.data : [];
}

async function updateToken(id, patch) {
  return request("PUT", `/tokens/${id}`, patch);
}

async function syncQuota(id) {
  return request("POST", `/tokens/${id}/sync-quota`);
}

function writeReport(fileName, report) {
  const out = path.resolve(process.cwd(), fileName);
  fs.writeFileSync(out, JSON.stringify({ ...report, timestamp: new Date().toISOString() }, null, 2));
  return out;
}

function isPolicyToken(token) {
  const reason = String(token.status_reason || token.refresh_error || "").toLowerCase();
  return token.status === "policy_quarantine" || reason.includes("policy") || reason.includes("safety");
}

function isRefreshEligible(token) {
  if (isPolicyToken(token)) return false;
  if (["expired", "cooling", "rate_limited", "quota_exhausted", "refresh_required", "refresh_failed"].includes(token.status)) {
    return true;
  }
  if (token.status === "disabled") {
    const reason = String(token.status_reason || "").toLowerCase();
    return reason.includes("expired") || reason.includes("cooldown") || reason.includes("rate limit");
  }
  return false;
}

module.exports = {
  baseUrl,
  listTokens,
  updateToken,
  syncQuota,
  writeReport,
  isPolicyToken,
  isRefreshEligible,
};
