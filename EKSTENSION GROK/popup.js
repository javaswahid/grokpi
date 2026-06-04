const TAB_KIND = {
  GROK: "grok",
  VEO: "veo"
};

const GROK_DOMAINS = ["grok.com", "grok.x.ai"];
const SSO_COOKIE_NAMES = ["sso-rw", "sso"];
const USER_ID_COOKIE_NAME = "x-userid";
const VEO_COOKIE_DOMAINS = [".google.com", "google.com"];
const VEO_COOKIE_NAME = "__Secure-1PSID";

const statusEl = document.getElementById("status");
const resultEl = document.getElementById("result");
const tabGrokEl = document.getElementById("tabGrok");
const tabVeoEl = document.getElementById("tabVeo");
const manualBtn = document.getElementById("manualBtn");

let currentTab = TAB_KIND.GROK;
let currentData = null;

function setStatus(message, tone = "") {
  statusEl.innerHTML = message;
  statusEl.className = tone ? `status ${tone}` : "status";
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function updateTabButtons() {
  const isGrok = currentTab === TAB_KIND.GROK;
  tabGrokEl.classList.toggle("active", isGrok);
  tabVeoEl.classList.toggle("active", !isGrok);
  manualBtn.className = `action ${isGrok ? "grok" : "veo"}`;
  manualBtn.textContent = isGrok ? "Ambil token Grok manual" : "Ambil token Veo manual";
}

function getActiveTab() {
  return new Promise((resolve, reject) => {
    chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }

      resolve(tabs[0] || null);
    });
  });
}

function getCookiesByDomain(domain) {
  return new Promise((resolve, reject) => {
    chrome.cookies.getAll({ domain }, (cookies) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }

      resolve(cookies || []);
    });
  });
}

async function collectGrokData() {
  const results = await Promise.all(
    GROK_DOMAINS.map(async (domain) => ({
      domain,
      cookies: await getCookiesByDomain(domain)
    }))
  );

  let ssoMatch = null;
  let userIdMatch = null;

  for (const result of results) {
    if (!ssoMatch) {
      const cookie = result.cookies.find((entry) => SSO_COOKIE_NAMES.includes(entry.name));
      if (cookie) {
        ssoMatch = { ...cookie, sourceDomain: result.domain };
      }
    }

    if (!userIdMatch) {
      const cookie = result.cookies.find((entry) => entry.name === USER_ID_COOKIE_NAME);
      if (cookie) {
        userIdMatch = { ...cookie, sourceDomain: result.domain };
      }
    }
  }

  return {
    kind: TAB_KIND.GROK,
    ssoToken: ssoMatch?.value || "",
    ssoCookieName: ssoMatch?.name || "",
    ssoDomain: ssoMatch?.sourceDomain || "",
    userId: userIdMatch?.value || "",
    userDomain: userIdMatch?.sourceDomain || ""
  };
}

async function collectVeoData() {
  let veoMatch = null;

  for (const domain of VEO_COOKIE_DOMAINS) {
    const cookies = await getCookiesByDomain(domain);
    const cookie = cookies.find((entry) => entry.name === VEO_COOKIE_NAME);

    if (cookie) {
      veoMatch = cookie;
      break;
    }
  }

  return {
    kind: TAB_KIND.VEO,
    veoToken: veoMatch?.value || "",
    veoDomain: veoMatch?.domain || ""
  };
}

function renderGrok(data) {
  const userIdText = data.userId || "(tidak ditemukan)";

  resultEl.innerHTML = [
    '<section class="card">',
    '  <p class="label">SSO Token</p>',
    `  <pre class="value">${escapeHtml(data.ssoToken || "(tidak ditemukan)")}</pre>`,
    `  <button id="copySsoBtn" class="action grok" type="button" ${data.ssoToken ? "" : "disabled"}>Copy SSO Token</button>`,
    '</section>',
    '<section class="card">',
    '  <p class="label">User ID</p>',
    `  <pre class="value">${escapeHtml(userIdText)}</pre>`,
    `  <button id="copyUserBtn" class="action ghost" type="button" ${data.userId ? "" : "disabled"}>Copy User ID</button>`,
    '</section>',
    '<section class="card">',
    '  <p class="label">Ringkasan</p>',
    `  <pre class="value">${escapeHtml(`SSO Cookie: ${data.ssoCookieName || "-"}\nSSO Domain: ${data.ssoDomain || "-"}\nUser Domain: ${data.userDomain || "-"}`)}</pre>`,
    `  <button id="copyAllBtn" class="action grok" type="button" ${data.ssoToken ? "" : "disabled"}>Copy Semua untuk Grok Gateway</button>`,
    '</section>'
  ].join("");

  document.getElementById("copySsoBtn").addEventListener("click", () => {
    copyText(data.ssoToken, "SSO token sudah disalin.");
  });

  document.getElementById("copyUserBtn").addEventListener("click", () => {
    copyText(data.userId, "User ID sudah disalin.");
  });

  document.getElementById("copyAllBtn").addEventListener("click", () => {
    const combined = [
      `SSO TOKEN: ${data.ssoToken}`,
      `USER ID: ${data.userId || "kosong"}`
    ].join("\n");
    copyText(combined, "Semua data Grok sudah disalin.");
  });
}

function renderVeo(data) {
  resultEl.innerHTML = [
    '<section class="card">',
    '  <p class="label">Veo Token</p>',
    `  <pre class="value">${escapeHtml(data.veoToken || "(tidak ditemukan)")}</pre>`,
    `  <button id="copyVeoBtn" class="action veo" type="button" ${data.veoToken ? "" : "disabled"}>Copy Veo Token</button>`,
    '</section>',
    '<section class="card">',
    '  <p class="label">Ringkasan</p>',
    `  <pre class="value">${escapeHtml(`Cookie: ${VEO_COOKIE_NAME}\nDomain: ${data.veoDomain || "-"}`)}</pre>`,
    '  <div class="hint">Paste token ini ke kolom Veo atau Gemini pada gateway yang Anda pakai.</div>',
    '</section>'
  ].join("");

  document.getElementById("copyVeoBtn").addEventListener("click", () => {
    copyText(data.veoToken, "Veo token sudah disalin.");
  });
}

function renderCurrentView(data) {
  currentData = data;

  if (data.kind === TAB_KIND.GROK) {
    renderGrok(data);
    return;
  }

  renderVeo(data);
}

async function copyText(text, successMessage) {
  if (!text) {
    return;
  }

  try {
    await navigator.clipboard.writeText(text);
    setStatus(successMessage, "ok");
  } catch (error) {
    setStatus(`Gagal menyalin data: ${escapeHtml(error.message)}`, "error");
  }
}

async function extractCurrentToken() {
  manualBtn.disabled = true;
  resultEl.innerHTML = "";
  setStatus(
    currentTab === TAB_KIND.GROK ? "Mengambil Grok token..." : "Mengambil Veo token dari Google...",
    ""
  );

  try {
    if (currentTab === TAB_KIND.GROK) {
      const data = await collectGrokData();

      if (!data.ssoToken) {
        currentData = data;
        setStatus(
          'Grok token tidak ditemukan. Pastikan sudah login di <a href="https://grok.com" target="_blank" rel="noreferrer">grok.com</a> atau <a href="https://grok.x.ai" target="_blank" rel="noreferrer">grok.x.ai</a>.',
          "error"
        );
        renderCurrentView(data);
        return;
      }

      renderCurrentView(data);
      setStatus(
        data.userId
          ? "Grok token dan User ID berhasil diambil."
          : "Grok token berhasil diambil, tetapi User ID belum ditemukan.",
        "ok"
      );
      return;
    }

    const data = await collectVeoData();
    if (!data.veoToken) {
      currentData = data;
      setStatus(
        'Veo token tidak ditemukan. Pastikan sudah login di <a href="https://gemini.google.com" target="_blank" rel="noreferrer">gemini.google.com</a>, lalu refresh halaman dan coba lagi.',
        "error"
      );
      renderCurrentView(data);
      return;
    }

    renderCurrentView(data);
    setStatus("Veo token berhasil diambil.", "ok");
  } catch (error) {
    setStatus(`Terjadi error saat mengambil cookie: ${escapeHtml(error.message)}`, "error");
  } finally {
    manualBtn.disabled = false;
  }
}

function switchTab(kind) {
  currentTab = kind;
  currentData = null;
  resultEl.innerHTML = "";
  updateTabButtons();
  setStatus(
    kind === TAB_KIND.GROK ? "Pilih untuk ambil token Grok." : "Pilih untuk ambil token Veo.",
    ""
  );
}

async function autoDetectTab() {
  try {
    const activeTab = await getActiveTab();
    const url = activeTab?.url || "";

    if (url.includes("gemini.google.com")) {
      switchTab(TAB_KIND.VEO);
      await extractCurrentToken();
      return;
    }

    switchTab(TAB_KIND.GROK);
    if (url.includes("grok.com") || url.includes("grok.x.ai")) {
      await extractCurrentToken();
    }
  } catch (error) {
    setStatus(`Gagal membaca tab aktif: ${escapeHtml(error.message)}`, "error");
  }
}

tabGrokEl.addEventListener("click", () => switchTab(TAB_KIND.GROK));
tabVeoEl.addEventListener("click", () => switchTab(TAB_KIND.VEO));
manualBtn.addEventListener("click", extractCurrentToken);
document.addEventListener("DOMContentLoaded", autoDetectTab);
