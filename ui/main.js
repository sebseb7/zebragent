// Tauri v2 Core Invoke
const tauriInvoke = window.__TAURI__ ? window.__TAURI__.core.invoke : async (cmd, args) => {
  console.warn("Tauri API not detected. Mocking response for:", cmd, args);
  if (cmd === "get_registry_config") {
    return {
      api_url: "http://localhost:4209",
      printer_ip: "192.168.188.47",
      port: "41783",
      token: "40781f20191c54d8f3e72e2a72cb835f92915a645eaa7ca37f10d85dabfe3d4c",
      local_addr: "127.0.0.1:41782",
      name: "agent"
    };
  }
  if (cmd === "get_service_status") {
    return { running: true, background: false };
  }
  if (cmd === "get_agent_logs") {
    return [
      "2026/10/31 22:17:55 Local monitor listening on http://127.0.0.1:41783",
      "2026/10/31 22:17:55 SSE connected to http://localhost:4209/api/agent/events",
      "2026/10/31 22:17:55 Registry read [HKCU\\Software\\ZebraAgent]: ApiUrl=\"http://localhost:4209\"",
      "2026/10/31 22:17:55 Registry read [HKCU\\Software\\ZebraAgent]: PrinterIp=\"192.168.188.47\"",
      "2026/10/31 22:17:55 Registry read [HKCU\\Software\\ZebraAgent]: Port=\"41783\""
    ];
  }
  return null;
};

// State
let currentPort = "41783";
let eventSource = null;
let eventCount = 0;
let isBackgroundInstalled = false;
const seenLogs = new Set();

// DOM Elements
const sseStatusPill = document.getElementById("sseStatusPill");
const sseStatusText = document.getElementById("sseStatusText");

const inputApiUrl = document.getElementById("inputApiUrl");
const inputPrinterIp = document.getElementById("inputPrinterIp");
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
    statPaired.textContent = name ? `yes (${name})` : "yes";
    statPaired.className = "status-pill online";
    pairControlBox.style.display = "none";
    unpairControlBox.style.display = "block";
  } else {
    statPaired.textContent = "no";
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
    showToast("Please enter pairing PIN", "error");
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
      showToast(`Paired successfully as "${data.name || 'agent'}"!`);
      inputPairPin.value = "";
      setPairedUI(true, data.name);
      await loadRegistryConfig();
    } else {
      showToast(`Pairing failed: ${data.error || 'Server rejected PIN'}`, "error");
    }
  } catch (err) {
    showToast(`Could not reach agent on port ${port}: ${err.message}`, "error");
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

    showToast("Agent unpaired (token cleared & connection dropped)");
    setPairedUI(false);
    await loadRegistryConfig();

    setTimeout(() => {
      connectSSE();
      checkServiceStatus();
    }, 600);
  } catch (err) {
    showToast(`Unpair error: ${err}`, "error");
  } finally {
    btnUnpairAgent.disabled = false;
  }
}

// Load Registry Config
async function loadRegistryConfig() {
  try {
    const cfg = await tauriInvoke("get_registry_config");
    inputApiUrl.value = cfg.api_url || "";
    inputPrinterIp.value = cfg.printer_ip || "";
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
    showToast(`Registry read error: ${err}`, "error");
  }
}

// Save Registry Config
async function saveRegistryConfig() {
  const apiUrl = inputApiUrl.value.trim();
  const printerIp = inputPrinterIp.value.trim();
  const port = inputPort.value.trim();

  if (!apiUrl) {
    showToast("ApiUrl is required", "error");
    return;
  }

  try {
    await tauriInvoke("save_registry_config", {
      apiUrl,
      printerIp,
      port: port || null
    });

    showToast("Registry settings saved successfully!");
    appendRawAgentLog(`[Config] Saved to HKCU\\Software\\ZebraAgent: ApiUrl="${apiUrl}", PrinterIp="${printerIp}", Port="${port}"`);

    await loadRegistryConfig();

    if (port && port !== currentPort) {
      currentPort = port;
      connectSSE();
    }
  } catch (err) {
    showToast(`Failed to save registry: ${err}`, "error");
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

  setSseStatus("warning", `Connecting (${port})...`);

  try {
    eventSource = new EventSource(url);

    eventSource.onopen = () => {
      setSseStatus("online", `SSE Connected (${port})`);
      appendRawAgentLog(`[SSE] Connected to ${url}`);
    };

    eventSource.onerror = () => {
      setSseStatus("offline", "SSE Disconnected");
    };

    eventSource.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === "paired") {
          setPairedUI(true, msg.data && msg.data.name);
          loadRegistryConfig();
        } else if (msg.type === "unpaired") {
          setPairedUI(false);
          loadRegistryConfig();
        } else if (msg.type === "snapshot" && msg.data) {
          setPairedUI(!!msg.data.paired, msg.data.agentName);
        }
        appendTerminalLog(msg.type || "sse", msg.data);
      } catch {
        appendRawAgentLog(e.data);
      }
    };
  } catch (e) {
    setSseStatus("offline", "SSE Failed");
  }
}

function setSseStatus(state, text) {
  sseStatusPill.className = `status-pill ${state}`;
  sseStatusText.textContent = text;
}

// Background Service Status Controls
async function checkServiceStatus() {
  try {
    const res = await tauriInvoke("get_service_status");
    if (!res) return;

    // Running: yes / no
    if (res.running) {
      statRunning.textContent = "yes";
      statRunning.className = "status-pill online";
    } else {
      statRunning.textContent = "no";
      statRunning.className = "status-pill offline";
    }

    // Background: yes / no
    isBackgroundInstalled = !!res.background;
    if (isBackgroundInstalled) {
      statBackground.textContent = "yes";
      statBackground.className = "status-pill online";
      btnToggleService.textContent = "Uninstall background service";
      btnToggleService.className = "btn btn-danger";
    } else {
      statBackground.textContent = "no";
      statBackground.className = "status-pill offline";
      btnToggleService.textContent = "Install as background service";
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
      showToast("Background service uninstalled");
    } else {
      await tauriInvoke("install_service");
      showToast("Installed as background service");
    }
  } catch (err) {
    showToast(`Error: ${err}`, "error");
  } finally {
    btnToggleService.disabled = false;
    await checkServiceStatus();
  }
}

// Event Listeners
document.getElementById("btnSaveRegistry").addEventListener("click", saveRegistryConfig);
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
}, 1500);

// Initialize on Load
window.addEventListener("DOMContentLoaded", () => {
  setTimeout(() => {
    loadRegistryConfig();
    checkServiceStatus();
    fetchAgentLogs();
  }, 20);
});
