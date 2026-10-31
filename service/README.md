# Zebra Agent Windows Service

Windows service that connects to a backend under base path `/api/agent`, receives ZPL print jobs over SSE, sends them to a Zebra printer over TCP 9100, polls printer hardware status (`^HH` and `~HQES`), and reports state to `/api/agent/state`.

`ApiUrl` is **strictly the domain** (e.g. `https://api.example.com` or `http://localhost:4209`). All backend endpoints automatically prepend `/api/agent/...`.

## Workflow

1. **Startup**: The agent launches immediately, even if `ApiUrl` is not yet configured. It starts the local monitor server on `http://127.0.0.1:41782`.
2. **Printer Polling (`^HH` / `~HQES`)**: When `PrinterIp` is set, the agent connects to TCP 9100 and polls printer configuration (`model`, `serial`, `labelLength`) and error flags (`headOpen`, `pause`, `other`, `unreachable`).
3. **State Reporting**:
   * As soon as the printer is discovered, its state is posted upstream to `/api/agent/state`.
   * Every time printer status or error flags change, the updated state is immediately posted again upstream and broadcast to the Tauri monitor over local SSE.
4. **SSE Connection**: The agent connects to `/api/agent/events?token=<TOKEN>&agentversion=1` and keeps the long-lived connection open (which signifies the online state). If the backend returns `401 Unauthorized`, the agent automatically unpairs and waits for a new PIN.

---

## Backend Endpoints (`<ApiUrl>/api/agent/...`)

All endpoints are built by appending `/api/agent/...` to the configured `ApiUrl` domain:

| Endpoint | Method | Auth | Description |
|---|---|---|---|
| `<ApiUrl>/api/agent/pair` | `POST` | Public | Submits `pairingpin`, `clientserial`, `clientmodel`, and `agentversion: "1"`. Returns persistent `token`. |
| `<ApiUrl>/api/agent/events` | `GET` | Bearer Token | Long-lived Server-Sent Events (SSE). Sends `event: connected`, `: heartbeat`, and print jobs. |
| `<ApiUrl>/api/agent/state` | `POST` / `PUT` | Bearer Token | Telemetry and printer status (`printers`, optional `job`). |

### State Payload Example (`POST /api/agent/state`)

```json
{
  "state": {
    "printers": [
      {
        "serial": "42J150100123",
        "model": "ZT410-203dpi",
        "labelLength": 215,
        "errors": {
          "headOpen": false,
          "pause": false,
          "other": false,
          "unreachable": false
        }
      }
    ]
  }
}
```

---

## How to Re-pair

You can re-pair at any time using any of these methods:

1. **Directly via CLI** (always overrides any existing token):
   ```cmd
   ZebraAgent.exe -apiurl localhost:4209 -pairingpin <NEW_PIN>
   ```
2. **Explicit Unpair**:
   ```cmd
   ZebraAgent.exe -unpair
   ```
3. **Via Local Monitor (Tauri UI / HTTP)**:
   * Re-pair: `POST http://127.0.0.1:41782/pair` with `{"pairingpin": "<NEW_PIN>"}`
   * Disconnect / Unpair: `POST http://127.0.0.1:41782/unpair`
4. **Automatic on `HTTP 401`**: If the backend revokes the token or rejects it with 401 Unauthorized, the agent automatically clears the token and waits for a new PIN.

---

## Local Monitor Server (`127.0.0.1:41782`) for Tauri v2

- **`GET /events` (SSE)**: Streams live status to the Tauri monitor. Pushes an initial full state snapshot on connection, followed by updates (`config`, `paired`, `unpaired`, `printer`, `connection`, `job`, `fault`).
- **`POST /config`**: Configures `apiUrl` (domain) and/or `printerIp` dynamically:
  ```json
  {
    "apiurl": "http://localhost:4209",
    "printerip": "192.168.188.47"
  }
  ```
- **`POST /pair`**: Accepts `{ "pairingpin": "654321" }` (and optional `"apiurl": "..."`). Re-pairs with `<ApiUrl>/api/agent/pair`, stores the new token, and connects immediately.
- **`POST /unpair`**: Clears the stored token and sets agent state to unpaired.
- **`GET /state`**: Returns current JSON snapshot of the agent, config, and printer.
- **`POST /print`**: Accepts `{ "zpl": "^XA...^XZ" }` for test prints.

---

## Configuration & Registry

The agent reads configuration in this order:
1. Command line flags (non-service mode)
2. **Current User Registry**: `HKCU\Software\ZebraAgent` (no elevation needed)
3. **Machine Registry**: `HKLM\SOFTWARE\ZebraAgent` (machine-wide, used by Windows Service)

When saving configuration (`Token`, `ApiUrl`, `PrinterIp`):
* Tries **`HKLM`** first (succeeds when running as Windows Service or Administrator).
* Automatically falls back to **`HKCU`** if not elevated (no "Access is denied").

---

## Build & Utilities

- **Build**:
  ```cmd
  build.cmd
  ```
- **Clear Registry** (removes both HKCU and HKLM keys):
  ```cmd
  clear-registry.cmd
  ```
