// Tauri v2 Core Invoke
const tauriInvoke = async (cmd, args) => {
  if (window.__TAURI__ && window.__TAURI__.core && typeof window.__TAURI__.core.invoke === "function") {
    return window.__TAURI__.core.invoke(cmd, args);
  }
  console.warn("Tauri API not detected. Mocking response for:", cmd, args);
  if (cmd === "get_registry_config") {
    return {
      api_url: "https://dashdemo.quixpos.com",
      printer_ip: "192.168.188.47",
      zpl_printers: ["192.168.188.47"],
      port: "41783",
      token: "40781f20191c54d8f3e72e2a72cb835f92915a645eaa7ca37f10d85dabfe3d4c",
      local_addr: "127.0.0.1:41783",
      name: "warehouse-front-01"
    };
  }
  if (cmd === "get_service_status") {
    return { running: true, background: true };
  }
  if (cmd === "get_agent_logs") {
    return [
      "2026/11/01 18:45:10 Local monitor listening on http://127.0.0.1:41783",
      "2026/11/01 18:45:10 Registry read [HKCU\\Software\\ZebraAgent]: ApiUrl=\"https://dashdemo.quixpos.com\"",
      "2026/11/01 18:45:10 Registry read [HKCU\\Software\\ZebraAgent]: ZplPrinters=[192.168.188.47]",
      "2026/11/01 18:45:10 Registry read [HKCU\\Software\\ZebraAgent]: Port=\"41783\"",
      "2026/11/01 18:45:11 SSE connected to https://dashdemo.quixpos.com/api/agent/events",
      "2026/11/01 18:45:12 Printer [192.168.188.47] status: online (model=\"ZD410-200dpi\" serial=\"50J203002218\" labelLength=261 headOpen=false pause=false other=false unreachable=false)",
      "2026/11/01 18:45:12 Posted printer state upstream: HTTP 200 (model=\"ZD410-200dpi\" serial=\"50J203002218\" type=\"zpl\" labelLength=261)"
    ];
  }
  if (cmd === "scan_local_printers") {
    return [
      { ip: "192.168.188.47", model: "ZD410-200dpi", serial: "50J203002218", status: "online", ready: true },
      { ip: "192.168.188.105", model: "ZT410-203dpi", serial: "42J150100123", status: "online", ready: true }
    ];
  }
  if (cmd === "get_ui_locale") {
    return (navigator.language || "en").toLowerCase().startsWith("de") ? "de" : "en";
  }
  return null;
};

// Localization (English default, German on German Windows)
let currentLang = "en";

const I18N = {
  en: {
    lblApiUrl: "QuixPOS API URL",
    lblPort: "local Service port",
    lblZplPrinters: "zplprinters",
    lblServiceTitle: "Background Service",
    lblRunning: "Running:",
    lblBackground: "Background:",
    lblPaired: "Paired:",
    btnSave: "💾 Save",
    btnScan: "🔍 Scan",
    btnScanTitle: "Scan local network for ZPL printers (TCP 9100)",
    btnScanning: "⏳ Scanning...",
    btnAdd: "+ Add",
    btnClear: "🗑️ Clear",
    btnClearTitle: "Clear Console",
    btnPair: "Pair",
    btnUnpair: "Unpair Agent",
    pinPlaceholder: "6-digit PIN",
    printerIpPlaceholder: "e.g. 192.168.1.100",
    removePrinterTitle: "Remove printer",
    yes: "yes",
    no: "no",
    modelLabel: "Model:",
    serialLabel: "Serial:",
    installService: "Install as background service",
    uninstallService: "Uninstall background service",
    sseConnecting: (port) => `Connecting (${port})...`,
    sseConnected: (port) => `SSE Connected (${port})`,
    sseDisconnected: "SSE Disconnected",
    sseFailed: "SSE Failed",
    discoveredHeader: (count) => `Discovered (${count}): click to use`,
    toastEnterPin: "Please enter pairing PIN",
    toastPairedSuccess: (name) => `Paired successfully as "${name || "agent"}"!`,
    toastPairingFailed: (err) => `Pairing failed: ${err || "Server rejected PIN"}`,
    toastAgentUnreachable: (port, msg) => `Could not reach agent on port ${port}: ${msg}`,
    toastUnpaired: "Agent unpaired (token cleared & connection dropped)",
    toastUnpairError: (err) => `Unpair error: ${err}`,
    toastNoPrintersFound: "No ZPL printers found on local network",
    toastFoundOnePrinter: (model, ip) => `Found ${model || "ZPL Printer"} at ${ip}`,
    toastFoundManyPrinters: (count) => `Found ${count} ZPL printers on local network`,
    toastAddedPrinter: (ip, model) => `Added ${ip} (${model || "ZPL"})`,
    toastScanFailed: (port) => `Scan failed: could not reach local agent on port ${port}`,
    toastRegReadError: (err) => `Registry read error: ${err}`,
    toastApiUrlRequired: "ApiUrl is required",
    toastRegSaved: "Registry settings saved successfully!",
    toastRegSaveError: (err) => `Failed to save registry: ${err}`,
    toastServiceUninstalled: "Background service uninstalled",
    toastServiceInstalled: "Installed as background service",
    toastError: (err) => `Error: ${err}`
  },
  de: {
    lblApiUrl: "QuixPOS API-URL",
    lblPort: "Lokaler Dienst-Port",
    lblZplPrinters: "ZPL-Drucker",
    lblServiceTitle: "Hintergrunddienst",
    lblRunning: "Aktiv:",
    lblBackground: "Hintergrund:",
    lblPaired: "Gekoppelt:",
    btnSave: "💾 Speichern",
    btnScan: "🔍 Scannen",
    btnScanTitle: "Lokales Netzwerk nach ZPL-Druckern durchsuchen (TCP 9100)",
    btnScanning: "⏳ Scanne...",
    btnAdd: "+ Neu",
    btnClear: "🗑️ Leeren",
    btnClearTitle: "Konsole leeren",
    btnPair: "Koppeln",
    btnUnpair: "Agent entkoppeln",
    pinPlaceholder: "6-stellige PIN",
    printerIpPlaceholder: "z.B. 192.168.1.100",
    removePrinterTitle: "Drucker entfernen",
    yes: "ja",
    no: "nein",
    modelLabel: "Modell:",
    serialLabel: "Seriennr.:",
    installService: "Als Hintergrunddienst installieren",
    uninstallService: "Hintergrunddienst deinstallieren",
    sseConnecting: (port) => `Verbinde (${port})...`,
    sseConnected: (port) => `SSE Verbunden (${port})`,
    sseDisconnected: "SSE Getrennt",
    sseFailed: "SSE Fehler",
    discoveredHeader: (count) => `Gefunden (${count}): zum Auswählen klicken`,
    toastEnterPin: "Bitte Kopplungs-PIN eingeben",
    toastPairedSuccess: (name) => `Erfolgreich gekoppelt als "${name || "agent"}"!`,
    toastPairingFailed: (err) => `Kopplung fehlgeschlagen: ${err || "PIN abgelehnt"}`,
    toastAgentUnreachable: (port, msg) => `Agent auf Port ${port} nicht erreichbar: ${msg}`,
    toastUnpaired: "Agent entkoppelt (Token gelöscht & Verbindung getrennt)",
    toastUnpairError: (err) => `Fehler beim Entkoppeln: ${err}`,
    toastNoPrintersFound: "Keine ZPL-Drucker im lokalen Netzwerk gefunden",
    toastFoundOnePrinter: (model, ip) => `${model || "ZPL-Drucker"} unter ${ip} gefunden`,
    toastFoundManyPrinters: (count) => `${count} ZPL-Drucker im lokalen Netzwerk gefunden`,
    toastAddedPrinter: (ip, model) => `${ip} (${model || "ZPL"}) hinzugefügt`,
    toastScanFailed: (port) => `Scan fehlgeschlagen: Agent auf Port ${port} nicht erreichbar`,
    toastRegReadError: (err) => `Fehler beim Lesen der Registry: ${err}`,
    toastApiUrlRequired: "API-URL ist erforderlich",
    toastRegSaved: "Einstellungen erfolgreich gespeichert!",
    toastRegSaveError: (err) => `Fehler beim Speichern: ${err}`,
    toastServiceUninstalled: "Hintergrunddienst deinstalliert",
    toastServiceInstalled: "Als Hintergrunddienst installiert",
    toastError: (err) => `Fehler: ${err}`
  }
};

function t(key, ...args) {
  const dict = I18N[currentLang] || I18N.en;
  const val = dict[key] !== undefined ? dict[key] : I18N.en[key];
  return typeof val === "function" ? val(...args) : val;
}

function applyLocale(lang) {
  currentLang = lang === "de" ? "de" : "en";
  document.documentElement.lang = currentLang;

  const setTxt = (id, key) => {
    const el = document.getElementById(id);
    if (el) el.textContent = t(key);
  };

  setTxt("lblApiUrl", "lblApiUrl");
  setTxt("lblPort", "lblPort");
  setTxt("lblZplPrinters", "lblZplPrinters");
  setTxt("lblServiceTitle", "lblServiceTitle");
  setTxt("lblRunning", "lblRunning");
  setTxt("lblBackground", "lblBackground");
  setTxt("lblPaired", "lblPaired");
  setTxt("btnSaveRegistry", "btnSave");
  setTxt("btnSavePrinters", "btnSave");
  setTxt("btnScanZplPrinters", "btnScan");
  setTxt("btnAddZplPrinter", "btnAdd");
  setTxt("btnClearLog", "btnClear");
  setTxt("btnPairAgent", "btnPair");
  setTxt("btnUnpairAgent", "btnUnpair");

  if (btnScanZplPrinters) btnScanZplPrinters.title = t("btnScanTitle");
  const btnClearLog = document.getElementById("btnClearLog");
  if (btnClearLog) btnClearLog.title = t("btnClearTitle");
  if (inputPairPin) inputPairPin.placeholder = t("pinPlaceholder");

  if (zplPrintersList) {
    zplPrintersList.querySelectorAll(".input-zpl-printer").forEach(inp => {
      inp.placeholder = t("printerIpPlaceholder");
    });
    zplPrintersList.querySelectorAll("button").forEach(btn => {
      btn.title = t("removePrinterTitle");
    });
  }

  refreshPrinterUIDisplays();
}

// State
let currentPort = "41783";
let eventSource = null;
let eventCount = 0;
let isBackgroundInstalled = false;
const seenLogs = new Set();
const printerInfoByIp = {};

// DOM Elements
const sseStatusPill = document.getElementById("sseStatusPill");
const sseStatusText = document.getElementById("sseStatusText");

const inputApiUrl = document.getElementById("inputApiUrl");
const zplPrintersList = document.getElementById("zplPrintersList");
const btnAddZplPrinter = document.getElementById("btnAddZplPrinter");
const btnScanZplPrinters = document.getElementById("btnScanZplPrinters");
const scanResultsBox = document.getElementById("scanResultsBox");
const inputPort = document.getElementById("inputPort");

const statRunning = document.getElementById("statRunning");
const statBackground = document.getElementById("statBackground");
const btnToggleService = document.getElementById("btnToggleService");

const statPaired = document.getElementById("statPaired");
const pairControlBox = document.getElementById("pairControlBox");
const unpairControlBox = document.getElementById("unpairControlBox");
const inputPairPin = document.getElementById("inputPairPin");
const btnPairAgent = document.getElementById("btnPairAgent");
const btnUnpairAgent = document.getElementById("btnUnpairAgent");

const logTerminal = document.getElementById("logTerminal");
const monEventCount = document.getElementById("monEventCount");
const toast = document.getElementById("toast");

// Toast helper
function showToast(msg, type = "success") {
  toast.textContent = msg;
  toast.className = `toast show ${type}`;
  setTimeout(() => {
    toast.className = "toast";
  }, 3500);
}

function timeStr() {
  const d = new Date();
  return d.toTimeString().split(" ")[0];
}

// Append log to terminal
function appendRawAgentLog(line) {
  if (!line || seenLogs.has(line)) return;
  seenLogs.add(line);
  if (seenLogs.size > 3000) seenLogs.clear();

  // Extract printer model/serial from agent log lines if present
  const mStatus = line.match(/Printer \[([^\]]+)\] status: (\S+) \(model="([^"]*)" serial="([^"]*)"/);
  if (mStatus) {
    const [, ip, status, model, serial] = mStatus;
    if (model || serial) {
      updatePrinterInfoMap({
        ip,
        status,
        ready: status === "online",
        model,
        serial
      });
    }
  } else {
    const mPost = line.match(/Posted printer state upstream: .*?\(model="([^"]*)" serial="([^"]*)"/);
    if (mPost) {
      const [, model, serial] = mPost;
      if (model || serial) {
        const ips = getZplPrintersValues();
        const ip = ips[0] || "primary";
        updatePrinterInfoMap({
          ip,
          status: "online",
          ready: true,
          model,
          serial
        });
      }
    }
  }

  eventCount++;
  if (monEventCount) monEventCount.textContent = `${eventCount} lines`;

  const entry = document.createElement("div");
  entry.className = "log-entry";
  entry.style.fontFamily = "var(--font-mono)";
  entry.style.fontSize = "0.8rem";
  entry.style.padding = "1px 0";
  entry.style.wordBreak = "break-all";

  if (line.includes("Registry read") || line.includes("Registry write")) {
    entry.style.color = "#38bdf8";
  } else if (line.includes("Printer") && line.includes("online")) {
    entry.style.color = "#4ade80";
  } else if (line.includes("Local monitor listening")) {
    entry.style.color = "#facc15";
  } else if (line.includes("error") || line.includes("fail") || line.includes("fault")) {
    entry.style.color = "#f87171";
  } else {
    entry.style.color = "#cbd5e1";
  }

  entry.textContent = line;
  logTerminal.appendChild(entry);
  logTerminal.scrollTop = logTerminal.scrollHeight;
}

function appendTerminalLog(type, msg) {
  const line = `[${timeStr()}] [${type.toUpperCase()}] ${typeof msg === "object" ? JSON.stringify(msg) : msg}`;
  appendRawAgentLog(line);
}

// Fetch logs from backend
async function fetchAgentLogs() {
  try {
    const logs = await tauriInvoke("get_agent_logs");
    if (Array.isArray(logs)) {
      logs.forEach(l => appendRawAgentLog(l));
    }
  } catch (e) {}
}

if (window.__TAURI__ && window.__TAURI__.event) {
  window.__TAURI__.event.listen("agent-stdout", (e) => {
    if (e && e.payload) {
      appendRawAgentLog(e.payload);
    }
  });
}

// Pairing Controls
function setPairedUI(isPaired, name) {
  if (isPaired) {
    statPaired.textContent = name ? `${t("yes")} (${name})` : t("yes");
    statPaired.className = "status-pill online";
    pairControlBox.style.display = "none";
    unpairControlBox.style.display = "block";
  } else {
    statPaired.textContent = t("no");
    statPaired.className = "status-pill offline";
    pairControlBox.style.display = "flex";
    unpairControlBox.style.display = "none";
  }
}

async function pairAgent() {
  const pin = inputPairPin.value.trim();
  const apiUrl = inputApiUrl.value.trim();
  const port = currentPort || inputPort.value.trim() || "41783";

  if (!pin) {
    showToast(t("toastEnterPin"), "error");
    return;
  }

  btnPairAgent.disabled = true;
  try {
    const res = await fetch(`http://127.0.0.1:${port}/pair`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        pairingpin: pin,
        apiurl: apiUrl || undefined
      })
    });

    const data = await res.json();
    if (res.ok && data.success) {
      showToast(t("toastPairedSuccess", data.name));
      inputPairPin.value = "";
      setPairedUI(true, data.name);
      await loadRegistryConfig();
    } else {
      showToast(t("toastPairingFailed", data.error), "error");
    }
  } catch (err) {
    showToast(t("toastAgentUnreachable", port, err.message), "error");
  } finally {
    btnPairAgent.disabled = false;
  }
}

async function unpairAgent() {
  const port = currentPort || inputPort.value.trim() || "41783";
  btnUnpairAgent.disabled = true;

  try {
    // 1. Send unpair request to local agent monitor
    await fetch(`http://127.0.0.1:${port}/unpair`, { method: "POST" }).catch(() => {});
    // 2. Clear token in registry and restart agent process/service to drop connection
    await tauriInvoke("unpair_agent");

    showToast(t("toastUnpaired"));
    setPairedUI(false);
    await loadRegistryConfig();

    setTimeout(() => {
      connectSSE();
      checkServiceStatus();
    }, 600);
  } catch (err) {
    showToast(t("toastUnpairError", err), "error");
  } finally {
    btnUnpairAgent.disabled = false;
  }
}

// Dynamic zplprinters list management
function updatePrinterInfoMap(info) {
  if (!info || !info.ip) return;
  printerInfoByIp[info.ip.trim()] = info;
  refreshPrinterUIDisplays();
}

function applySnapshotPrinters(snap) {
  if (!snap) return;
  if (Array.isArray(snap.printers)) {
    snap.printers.forEach(p => {
      if (p && p.ip) printerInfoByIp[p.ip.trim()] = p;
    });
  }
  if (snap.printerIp && (snap.printerModel || snap.printerSerial || snap.printerStatus)) {
    const ip = snap.printerIp.trim();
    printerInfoByIp[ip] = {
      ip,
      model: snap.printerModel || (printerInfoByIp[ip] && printerInfoByIp[ip].model) || "",
      serial: snap.printerSerial || (printerInfoByIp[ip] && printerInfoByIp[ip].serial) || "",
      status: snap.printerStatus || (printerInfoByIp[ip] && printerInfoByIp[ip].status) || "",
      ready: !!snap.printerReady
    };
  }
  refreshPrinterUIDisplays();
}

function formatPrinterMeta(info) {
  if (!info) return "";
  const model = info.model || "";
  const serial = info.serial || "";
  if (!model && !serial) return "";
  const parts = [];
  if (model) parts.push(`${t("modelLabel")} ${model}`);
  if (serial) parts.push(`${t("serialLabel")} ${serial}`);
  return parts.join(" | ");
}

function refreshPrinterUIDisplays() {
  if (!zplPrintersList) return;
  const rows = zplPrintersList.querySelectorAll(".zpl-printer-row-container");

  rows.forEach((container) => {
    const input = container.querySelector(".input-zpl-printer");
    const metaEl = container.querySelector(".zpl-printer-meta");
    if (!input || !metaEl) return;

    const ip = input.value.trim();
    const info = ip ? printerInfoByIp[ip] : null;

    if (ip && info && (info.model || info.serial)) {
      const isOnline = info.ready || info.status === "online";
      const dotColor = isOnline ? "var(--success)" : (info.status === "offline" ? "var(--danger)" : "var(--warning)");
      const model = info.model || "-";
      const serial = info.serial || "-";
      metaEl.style.display = "flex";
      metaEl.innerHTML = `
        <span style="width: 7px; height: 7px; border-radius: 50%; background: ${dotColor}; box-shadow: 0 0 6px ${dotColor}; display: inline-block; flex-shrink: 0; margin-top: 4px;"></span>
        <div style="display: flex; flex-direction: column; gap: 2px; line-height: 1.35; flex: 1; min-width: 0;">
          <div style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"><span style="color: var(--text-dim);">${t("modelLabel")}</span> <span style="color: var(--text-main); font-weight: 500;">${model}</span></div>
          <div style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"><span style="color: var(--text-dim);">${t("serialLabel")}</span> <span style="color: var(--accent);">${serial}</span></div>
        </div>
      `;
    } else {
      metaEl.style.display = "none";
      metaEl.innerHTML = "";
    }
  });
}

async function fetchAgentState() {
  const port = currentPort || (inputPort && inputPort.value.trim()) || "41783";
  try {
    const res = await fetch(`http://127.0.0.1:${port}/state`);
    if (res.ok) {
      const snap = await res.json();
      applySnapshotPrinters(snap);
    }
  } catch (e) {}
}

function renderZplPrinters(printers) {
  if (!zplPrintersList) return;
  zplPrintersList.innerHTML = "";
  const list = Array.isArray(printers) && printers.length > 0 ? printers : [""];
  list.forEach(ip => addZplPrinterRow(ip));
  refreshPrinterUIDisplays();
}

function addZplPrinterRow(ip = "") {
  if (!zplPrintersList) return;
  const container = document.createElement("div");
  container.className = "zpl-printer-row-container";
  container.style.display = "flex";
  container.style.flexDirection = "column";
  container.style.gap = "5px";

  const row = document.createElement("div");
  row.className = "zpl-printer-row";
  row.style.display = "flex";
  row.style.gap = "6px";
  row.style.alignItems = "center";

  const input = document.createElement("input");
  input.type = "text";
  input.className = "input-zpl-printer";
  input.placeholder = t("printerIpPlaceholder");
  input.value = ip;
  input.autocomplete = "off";
  input.style.flex = "1";
  input.style.padding = "5px 9px";
  input.style.fontSize = "0.84rem";
  input.addEventListener("input", refreshPrinterUIDisplays);

  const btnDel = document.createElement("button");
  btnDel.type = "button";
  btnDel.className = "btn btn-outline btn-sm";
  btnDel.textContent = "✕";
  btnDel.title = t("removePrinterTitle");
  btnDel.style.padding = "3px 7px";
  btnDel.style.fontSize = "0.72rem";
  btnDel.style.color = "#f87171";
  btnDel.style.borderColor = "rgba(248, 113, 113, 0.35)";

  btnDel.addEventListener("click", () => {
    container.remove();
    if (zplPrintersList.children.length === 0) {
      addZplPrinterRow("");
    }
    refreshPrinterUIDisplays();
  });

  const meta = document.createElement("div");
  meta.className = "zpl-printer-meta";
  meta.style.display = "none";
  meta.style.alignItems = "flex-start";
  meta.style.gap = "8px";
  meta.style.fontSize = "0.74rem";
  meta.style.fontFamily = "var(--font-mono)";
  meta.style.color = "#94a3b8";
  meta.style.padding = "5px 9px";
  meta.style.background = "rgba(15, 23, 42, 0.6)";
  meta.style.border = "1px solid var(--border)";
  meta.style.borderRadius = "var(--radius-sm)";

  row.appendChild(input);
  row.appendChild(btnDel);
  container.appendChild(row);
  container.appendChild(meta);
  zplPrintersList.appendChild(container);
}

function getZplPrintersValues() {
  if (!zplPrintersList) return [];
  const inputs = zplPrintersList.querySelectorAll(".input-zpl-printer");
  const values = [];
  inputs.forEach(input => {
    const val = input.value.trim();
    if (val) values.push(val);
  });
  return values;
}

function selectDiscoveredPrinter(info) {
  if (!info || !info.ip) return;
  const ip = info.ip.trim();
  printerInfoByIp[ip] = info;

  const inputs = zplPrintersList ? Array.from(zplPrintersList.querySelectorAll(".input-zpl-printer")) : [];
  const existing = inputs.find(i => i.value.trim() === ip);
  if (!existing) {
    const emptyInput = inputs.find(i => i.value.trim() === "");
    if (emptyInput) {
      emptyInput.value = ip;
    } else {
      addZplPrinterRow(ip);
    }
  }
  refreshPrinterUIDisplays();
}

async function scanLocalNetworkForPrinters() {
  if (!btnScanZplPrinters) return;
  const port = currentPort || (inputPort && inputPort.value.trim()) || "41783";
  btnScanZplPrinters.disabled = true;
  btnScanZplPrinters.textContent = t("btnScanning");

  try {
    let printers = [];
    try {
      const res = await fetch(`http://127.0.0.1:${port}/scan`, { method: "POST" });
      if (res.ok) {
        const data = await res.json();
        printers = (data && Array.isArray(data.printers)) ? data.printers : [];
      } else {
        throw new Error("HTTP scan unavailable");
      }
    } catch {
      const tauriRes = await tauriInvoke("scan_local_printers");
      if (Array.isArray(tauriRes)) {
        printers = tauriRes;
      }
    }

    printers.forEach(p => {
      if (p && p.ip) {
        printerInfoByIp[p.ip.trim()] = p;
      }
    });

    if (printers.length === 0) {
      showToast(t("toastNoPrintersFound"), "error");
      if (scanResultsBox) {
        scanResultsBox.style.display = "none";
        scanResultsBox.innerHTML = "";
      }
    } else if (printers.length === 1) {
      const p = printers[0];
      selectDiscoveredPrinter(p);
      showToast(t("toastFoundOnePrinter", p.model, p.ip));
      if (scanResultsBox) {
        scanResultsBox.style.display = "none";
        scanResultsBox.innerHTML = "";
      }
    } else {
      showToast(t("toastFoundManyPrinters", printers.length));
      if (scanResultsBox) {
        scanResultsBox.innerHTML = "";
        const header = document.createElement("div");
        header.style.display = "flex";
        header.style.justifyContent = "space-between";
        header.style.alignItems = "center";
        header.style.fontSize = "0.72rem";
        header.style.color = "var(--text-muted)";
        header.style.marginBottom = "2px";
        header.innerHTML = `<span>${t("discoveredHeader", printers.length)}</span><button type="button" style="background:none;border:none;color:var(--text-dim);cursor:pointer;font-size:0.75rem;">✕</button>`;
        header.querySelector("button").addEventListener("click", () => {
          scanResultsBox.style.display = "none";
        });
        scanResultsBox.appendChild(header);

        printers.forEach(p => {
          const item = document.createElement("div");
          item.style.display = "flex";
          item.style.flexDirection = "column";
          item.style.gap = "1px";
          item.style.padding = "4px 7px";
          item.style.borderRadius = "0";
          item.style.cursor = "pointer";
          item.style.fontSize = "0.71rem";
          item.style.fontFamily = "var(--font-mono)";
          item.style.background = "rgba(30, 41, 59, 0.75)";
          item.style.border = "1px solid rgba(51, 65, 85, 0.7)";
          item.innerHTML = `<div style="display: flex; justify-content: space-between; align-items: center; gap: 6px; white-space: nowrap;"><span style="color: var(--accent); font-weight: 600;">${p.ip}</span><span style="color: var(--text-main); font-weight: 500; overflow: hidden; text-overflow: ellipsis;">${p.model || "ZPL"}</span></div>${p.serial ? `<div style="font-size: 0.66rem; color: var(--text-muted); white-space: nowrap;">SN: ${p.serial}</div>` : ""}`;
          item.addEventListener("click", () => {
            selectDiscoveredPrinter(p);
            showToast(t("toastAddedPrinter", p.ip, p.model));
          });
          scanResultsBox.appendChild(item);
        });
        scanResultsBox.style.display = "flex";
      }
    }
    refreshPrinterUIDisplays();
  } catch (err) {
    showToast(t("toastScanFailed", port), "error");
  } finally {
    btnScanZplPrinters.disabled = false;
    btnScanZplPrinters.textContent = t("btnScan");
  }
}

// Load Registry Config
async function loadRegistryConfig() {
  try {
    const cfg = await tauriInvoke("get_registry_config");
    inputApiUrl.value = cfg.api_url || "https://dashdemo.quixpos.com";
    renderZplPrinters(cfg.zpl_printers && cfg.zpl_printers.length > 0 ? cfg.zpl_printers : (cfg.printer_ip ? [cfg.printer_ip] : []));
    inputPort.value = cfg.port || "41783";

    if (cfg.token && cfg.token.length > 5) {
      setPairedUI(true, cfg.name || "agent");
    } else {
      setPairedUI(false);
    }

    if (cfg.port && cfg.port !== currentPort) {
      currentPort = cfg.port;
      connectSSE();
    } else if (!eventSource) {
      if (cfg.port) currentPort = cfg.port;
      connectSSE();
    }
  } catch (err) {
    showToast(t("toastRegReadError", err), "error");
  }
}

// Save Registry Config
async function saveRegistryConfig() {
  const apiUrl = inputApiUrl.value.trim();
  const zplPrinters = getZplPrintersValues();
  const port = inputPort.value.trim();

  if (!apiUrl) {
    showToast(t("toastApiUrlRequired"), "error");
    return;
  }

  try {
    await tauriInvoke("save_registry_config", {
      apiUrl,
      zplPrinters,
      port: port || null
    });

    showToast(t("toastRegSaved"));
    appendRawAgentLog(`[Config] Saved to HKCU\\Software\\ZebraAgent: ApiUrl="${apiUrl}", zplprinters=[${zplPrinters.join(", ")}], Port="${port}"`);
    appendRawAgentLog("[Signal] Sent reload signal to running agent(s) (reloading registry & reconnecting printers)...");

    const activePort = port || currentPort || "41783";
    fetch(`http://127.0.0.1:${activePort}/reload`, { method: "POST" }).catch(() => {});

    await loadRegistryConfig();

    if (port && port !== currentPort) {
      currentPort = port;
      connectSSE();
    }
  } catch (err) {
    showToast(t("toastRegSaveError", err), "error");
  }
}

// Connect to Local Service SSE Endpoint
function connectSSE() {
  if (eventSource) {
    eventSource.close();
    eventSource = null;
  }

  const port = currentPort || inputPort.value.trim() || "41783";
  const url = `http://127.0.0.1:${port}/events`;

  setSseStatus("warning", t("sseConnecting", port));

  try {
    eventSource = new EventSource(url);

    eventSource.onopen = () => {
      setSseStatus("online", t("sseConnected", port));
      appendRawAgentLog(`[SSE] Connected to ${url}`);
      fetchAgentState();
    };

    eventSource.onerror = () => {
      setSseStatus("offline", t("sseDisconnected"));
    };

    const handleSsePayload = (raw) => {
      try {
        const msg = JSON.parse(raw);
        if (msg.type === "paired") {
          setPairedUI(true, msg.data && msg.data.name);
          loadRegistryConfig();
          appendTerminalLog("paired", msg.data);
        } else if (msg.type === "unpaired") {
          setPairedUI(false);
          loadRegistryConfig();
          appendTerminalLog("unpaired", msg.data);
        } else if (msg.type === "snapshot" && msg.data) {
          setPairedUI(!!msg.data.paired, msg.data.agentName);
          applySnapshotPrinters(msg.data);
        } else if (msg.type === "printer" && msg.data) {
          const prev = msg.data.ip ? printerInfoByIp[msg.data.ip.trim()] : null;
          const prevSig = prev ? `${prev.status}|${prev.model}|${prev.serial}|${prev.labelLength}` : "";
          const nextSig = `${msg.data.status}|${msg.data.model}|${msg.data.serial}|${msg.data.labelLength}`;
          updatePrinterInfoMap(msg.data);
          if (prevSig !== nextSig) {
            appendTerminalLog("printer", msg.data);
          }
        } else {
          appendTerminalLog(msg.type || "sse", msg.data);
        }
      } catch {
        appendRawAgentLog(raw);
      }
    };

    eventSource.onmessage = (e) => handleSsePayload(e.data);
    ["printer", "snapshot", "paired", "unpaired", "config", "connection", "job"].forEach(evt => {
      eventSource.addEventListener(evt, (e) => handleSsePayload(e.data));
    });
  } catch (e) {
    setSseStatus("offline", t("sseFailed"));
  }
}

function setSseStatus(state, text) {
  if (sseStatusPill) sseStatusPill.className = `status-pill ${state}`;
  if (sseStatusText) sseStatusText.textContent = text;
}

// Background Service Status Controls
async function checkServiceStatus() {
  try {
    const res = await tauriInvoke("get_service_status");
    if (!res) return;

    // Running: yes / no
    if (res.running) {
      statRunning.textContent = t("yes");
      statRunning.className = "status-pill online";
    } else {
      statRunning.textContent = t("no");
      statRunning.className = "status-pill offline";
    }

    // Background: yes / no
    isBackgroundInstalled = !!res.background;
    if (isBackgroundInstalled) {
      statBackground.textContent = t("yes");
      statBackground.className = "status-pill online";
      btnToggleService.textContent = t("uninstallService");
      btnToggleService.className = "btn btn-danger";
    } else {
      statBackground.textContent = t("no");
      statBackground.className = "status-pill offline";
      btnToggleService.textContent = t("installService");
      btnToggleService.className = "btn btn-primary";
    }
  } catch (err) {
    console.error("Failed to query service status:", err);
  }
}

async function toggleBackgroundService() {
  btnToggleService.disabled = true;
  try {
    if (isBackgroundInstalled) {
      await tauriInvoke("uninstall_service");
      showToast(t("toastServiceUninstalled"));
    } else {
      await tauriInvoke("install_service");
      showToast(t("toastServiceInstalled"));
    }
  } catch (err) {
    showToast(t("toastError", err), "error");
  } finally {
    btnToggleService.disabled = false;
    await checkServiceStatus();
  }
}

// Event Listeners
document.getElementById("btnSaveRegistry").addEventListener("click", saveRegistryConfig);
const btnSavePrinters = document.getElementById("btnSavePrinters");
if (btnSavePrinters) {
  btnSavePrinters.addEventListener("click", saveRegistryConfig);
}
if (btnAddZplPrinter) {
  btnAddZplPrinter.addEventListener("click", () => addZplPrinterRow(""));
}
if (btnScanZplPrinters) {
  btnScanZplPrinters.addEventListener("click", scanLocalNetworkForPrinters);
}
document.getElementById("btnClearLog").addEventListener("click", async () => {
  logTerminal.innerHTML = "";
  seenLogs.clear();
  eventCount = 0;
  if (monEventCount) monEventCount.textContent = "0 lines";
  await tauriInvoke("clear_agent_logs").catch(() => {});
});

btnToggleService.addEventListener("click", toggleBackgroundService);
btnPairAgent.addEventListener("click", pairAgent);
btnUnpairAgent.addEventListener("click", unpairAgent);
inputPairPin.addEventListener("keydown", (e) => {
  if (e.key === "Enter") pairAgent();
});

// Periodically update status and output
setInterval(() => {
  checkServiceStatus();
  fetchAgentLogs();
  fetchAgentState();
}, 1500);

// Initialize on Load
window.addEventListener("DOMContentLoaded", () => {
  setTimeout(async () => {
    try {
      const detectedLang = await tauriInvoke("get_ui_locale");
      applyLocale(detectedLang);
    } catch {
      applyLocale((navigator.language || "en").toLowerCase().startsWith("de") ? "de" : "en");
    }
    loadRegistryConfig();
    checkServiceStatus();
    fetchAgentLogs();
    fetchAgentState();
  }, 20);
});
