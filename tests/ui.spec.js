const { test, expect } = require("@playwright/test");
const path = require("path");
const { pathToFileURL } = require("url");

const UI_INDEX_URL = pathToFileURL(path.resolve(__dirname, "../ui/index.html")).href;

/**
 * Injects deterministic mocks for Tauri IPC, EventSource (SSE), and fetch (/state, /scan)
 * so screenshot tests render realistic, reproducible states.
 */
async function setupRealisticEnvironment(page, scenario) {
  await page.addInitScript((scen) => {
    // 1. Mock Tauri v2 core.invoke
    window.__TAURI__ = {
      core: {
        invoke: async (cmd, args) => {
          if (cmd === "get_registry_config") {
            return scen.registryConfig;
          }
          if (cmd === "get_service_status") {
            return scen.serviceStatus;
          }
          if (cmd === "get_agent_logs") {
            return scen.logs || [];
          }
          if (cmd === "scan_local_printers") {
            return scen.scannedPrinters || [];
          }
          if (cmd === "get_ui_locale") {
            return scen.locale || "en";
          }
          if (cmd === "save_registry_config" || cmd === "clear_agent_logs" || cmd === "unpair_agent") {
            return null;
          }
          return null;
        },
      },
      event: {
        listen: () => Promise.resolve(() => {}),
      },
    };

    // 2. Mock EventSource for deterministic SSE status & snapshot
    class MockEventSource {
      constructor(url) {
        this.url = url;
        this.listeners = {};
        setTimeout(() => {
          if (scen.sseOnline !== false) {
            if (typeof this.onopen === "function") {
              this.onopen();
            }
            if (scen.snapshot) {
              const evt = {
                data: JSON.stringify({
                  type: "snapshot",
                  timestamp: "2026-11-01T18:00:00Z",
                  data: scen.snapshot,
                }),
              };
              if (typeof this.onmessage === "function") {
                this.onmessage(evt);
              }
            }
          } else {
            if (typeof this.onerror === "function") {
              this.onerror(new Event("error"));
            }
          }
        }, 10);
      }
      addEventListener(type, cb) {
        this.listeners[type] = this.listeners[type] || [];
        this.listeners[type].push(cb);
      }
      close() {}
    }
    window.EventSource = MockEventSource;

    // 3. Mock fetch for /state, /scan, /reload, /pair
    const origFetch = window.fetch.bind(window);
    window.fetch = async (url, options) => {
      const urlStr = String(url);
      if (urlStr.endsWith("/state")) {
        return {
          ok: true,
          status: 200,
          json: async () => scen.snapshot || {},
        };
      }
      if (urlStr.endsWith("/scan")) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            printers: scen.scannedPrinters || [],
          }),
        };
      }
      if (urlStr.endsWith("/reload")) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ success: true }),
        };
      }
      return origFetch(url, options);
    };
  }, scenario);
}

test.describe("Zebra Agent Manager Visual Screenshot Tests", () => {
  test("1. Connected & paired state with single Zebra ZD410 printer", async ({ page }) => {
    await setupRealisticEnvironment(page, {
      sseOnline: true,
      registryConfig: {
        api_url: "https://dashdemo.quixpos.com",
        printer_ip: "192.168.188.47",
        zpl_printers: ["192.168.188.47"],
        port: "41783",
        token: "40781f20191c54d8f3e72e2a72cb835f92915a645eaa7ca37f10d85dabfe3d4c",
        local_addr: "127.0.0.1:41783",
        name: "warehouse-pack-01",
      },
      serviceStatus: {
        running: true,
        background: true,
      },
      snapshot: {
        agentVersion: "1",
        apiUrl: "https://dashdemo.quixpos.com",
        paired: true,
        agentId: 12,
        agentName: "warehouse-pack-01",
        backendConnected: true,
        printerIp: "192.168.188.47",
        printerModel: "ZD410-200dpi",
        printerSerial: "50J203002218",
        printerType: "zpl",
        printerReady: true,
        printerStatus: "online",
        labelLength: 261,
        printers: [
          {
            ip: "192.168.188.47",
            model: "ZD410-200dpi",
            serial: "50J203002218",
            type: "zpl",
            labelLength: 261,
            ready: true,
            status: "online",
          },
        ],
      },
      logs: [
        "2026/11/01 18:45:10 Local monitor listening on http://127.0.0.1:41783",
        "2026/11/01 18:45:10 Registry read [HKCU\\Software\\ZebraAgent]: ApiUrl=\"https://dashdemo.quixpos.com\"",
        "2026/11/01 18:45:10 Registry read [HKCU\\Software\\ZebraAgent]: ZplPrinters=[192.168.188.47]",
        "2026/11/01 18:45:10 Registry read [HKCU\\Software\\ZebraAgent]: Port=\"41783\"",
        "2026/11/01 18:45:11 SSE connected to https://dashdemo.quixpos.com/api/agent/events",
        "2026/11/01 18:45:12 Printer [192.168.188.47] status: online (model=\"ZD410-200dpi\" serial=\"50J203002218\" labelLength=261 headOpen=false pause=false other=false unreachable=false)",
        "2026/11/01 18:45:12 Posted printer state upstream: HTTP 200 (model=\"ZD410-200dpi\" serial=\"50J203002218\" type=\"zpl\" labelLength=261)",
      ],
    });

    await page.goto(UI_INDEX_URL);
    await expect(page.locator("#statPaired")).toHaveText("yes (warehouse-pack-01)");
    await expect(page.locator(".zpl-printer-meta").first()).toContainText("ZD410-200dpi");
    await expect(page.locator(".zpl-printer-meta").first()).toContainText("50J203002218");

    await expect(page).toHaveScreenshot("connected-single-printer.png");
  });

  test("2. Multiple printers & network scan discovery results", async ({ page }) => {
    await setupRealisticEnvironment(page, {
      sseOnline: true,
      registryConfig: {
        api_url: "https://dashdemo.quixpos.com",
        printer_ip: "192.168.188.47",
        zpl_printers: ["192.168.188.47"],
        port: "41783",
        token: "40781f20191c54d8f3e72e2a72cb835f92915a645eaa7ca37f10d85dabfe3d4c",
        local_addr: "127.0.0.1:41783",
        name: "shipping-dock-02",
      },
      serviceStatus: {
        running: true,
        background: true,
      },
      snapshot: {
        agentVersion: "1",
        apiUrl: "https://dashdemo.quixpos.com",
        paired: true,
        agentId: 15,
        agentName: "shipping-dock-02",
        backendConnected: true,
        printerIp: "192.168.188.47",
        printerModel: "ZD410-200dpi",
        printerSerial: "50J203002218",
        printerType: "zpl",
        printerReady: true,
        printerStatus: "online",
        labelLength: 261,
        printers: [
          {
            ip: "192.168.188.47",
            model: "ZD410-200dpi",
            serial: "50J203002218",
            type: "zpl",
            labelLength: 261,
            ready: true,
            status: "online",
          },
        ],
      },
      scannedPrinters: [
        {
          ip: "192.168.188.47",
          model: "ZD410-200dpi",
          serial: "50J203002218",
          status: "online",
          ready: true,
        },
        {
          ip: "192.168.188.105",
          model: "ZT410-203dpi",
          serial: "42J150100123",
          status: "online",
          ready: true,
        },
      ],
      logs: [
        "2026/11/01 18:46:00 Local monitor listening on http://127.0.0.1:41783",
        "2026/11/01 18:46:01 Scanning local network for ZPL printers on TCP port 9100...",
        "2026/11/01 18:46:02 Network scan complete: found 2 ZPL printer(s)",
      ],
    });

    await page.goto(UI_INDEX_URL);
    await expect(page.locator("#statPaired")).toHaveText("yes (shipping-dock-02)");

    // Click Scan button to display discovered printers list
    await page.click("#btnScanZplPrinters");
    await expect(page.locator("#scanResultsBox")).toBeVisible();
    await expect(page.locator("#scanResultsBox")).toContainText("192.168.188.105");
    await expect(page.locator("#scanResultsBox")).toContainText("ZT410-203dpi");

    // Wait for toast animation or hide toast for clean deterministic screenshot
    await page.evaluate(() => {
      const t = document.getElementById("toast");
      if (t) t.className = "toast";
    });

    await expect(page).toHaveScreenshot("network-scan-results.png");
  });

  test("3. Unpaired default state with no printer configured", async ({ page }) => {
    await setupRealisticEnvironment(page, {
      sseOnline: true,
      registryConfig: {
        api_url: "",
        printer_ip: "",
        zpl_printers: [],
        port: "41783",
        token: "",
        local_addr: "127.0.0.1:41783",
        name: "",
      },
      serviceStatus: {
        running: true,
        background: false,
      },
      snapshot: {
        agentVersion: "1",
        apiUrl: "https://dashdemo.quixpos.com",
        paired: false,
        backendConnected: false,
        printerIp: "",
        printerType: "zpl",
        printerReady: false,
        printerStatus: "unpolled",
        printers: [],
      },
      logs: [
        "2026/11/01 18:47:00 Local monitor listening on http://127.0.0.1:41783",
        "2026/11/01 18:47:00 ZebraAgent waiting for pairing PIN via local monitor (http://127.0.0.1:41783) or CLI...",
      ],
    });

    await page.goto(UI_INDEX_URL);
    await expect(page.locator("#inputApiUrl")).toHaveValue("https://dashdemo.quixpos.com");
    await expect(page.locator("#statPaired")).toHaveText("no");
    await expect(page.locator(".zpl-printer-meta").first()).toBeHidden();

    await expect(page).toHaveScreenshot("unpaired-default-state.png");
  });

  test("4. German Windows localization (de) renders German UI labels and statuses", async ({ page }) => {
    await setupRealisticEnvironment(page, {
      locale: "de",
      sseOnline: true,
      registryConfig: {
        api_url: "https://dashdemo.quixpos.com",
        printer_ip: "192.168.188.47",
        zpl_printers: ["192.168.188.47"],
        port: "41783",
        token: "40781f20191c54d8f3e72e2a72cb835f92915a645eaa7ca37f10d85dabfe3d4c",
        local_addr: "127.0.0.1:41783",
        name: "lager-versand-01",
      },
      serviceStatus: {
        running: true,
        background: true,
      },
      snapshot: {
        agentVersion: "1",
        apiUrl: "https://dashdemo.quixpos.com",
        paired: true,
        agentId: 12,
        agentName: "lager-versand-01",
        backendConnected: true,
        printerIp: "192.168.188.47",
        printerModel: "ZD410-200dpi",
        printerSerial: "50J203002218",
        printerType: "zpl",
        printerReady: true,
        printerStatus: "online",
        labelLength: 261,
        printers: [
          {
            ip: "192.168.188.47",
            model: "ZD410-200dpi",
            serial: "50J203002218",
            type: "zpl",
            labelLength: 261,
            ready: true,
            status: "online",
          },
        ],
      },
      logs: [
        "2026/11/01 18:45:10 Local monitor listening on http://127.0.0.1:41783",
      ],
    });

    await page.goto(UI_INDEX_URL);
    await expect(page.locator("#lblApiUrl")).toHaveText("QuixPOS API-URL");
    await expect(page.locator("#lblPort")).toHaveText("Lokaler Dienst-Port");
    await expect(page.locator("#lblZplPrinters")).toHaveText("ZPL-Drucker");
    await expect(page.locator("#lblServiceTitle")).toHaveText("Hintergrunddienst");
    await expect(page.locator("#lblRunning")).toHaveText("Aktiv:");
    await expect(page.locator("#statRunning")).toHaveText("ja");
    await expect(page.locator("#btnToggleService")).toHaveText("Hintergrunddienst deinstallieren");
    await expect(page.locator("#lblPaired")).toHaveText("Gekoppelt:");
    await expect(page.locator("#statPaired")).toHaveText("ja (lager-versand-01)");
    await expect(page.locator("#btnUnpairAgent")).toHaveText("Agent entkoppeln");
    await expect(page.locator(".zpl-printer-meta").first()).toContainText("Modell:");
    await expect(page.locator(".zpl-printer-meta").first()).toContainText("Seriennr.:");

    await expect(page).toHaveScreenshot("german-ui-state.png");
  });
});

