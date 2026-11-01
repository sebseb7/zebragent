use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::process::{Child, Command};
use std::sync::Mutex;
#[cfg(windows)]
use std::os::windows::process::CommandExt;
use tauri::{
    menu::{MenuBuilder, MenuItem, MenuItemBuilder},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Emitter, Manager,
};
use winreg::enums::*;
use winreg::RegKey;

#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x08000000;

// Embed ZebraAgent.exe into the binary
const EMBEDDED_AGENT: &[u8] = include_bytes!("../../service/ZebraAgent.exe");

// Track spawned temporary background process
static SPAWNED_AGENT: Mutex<Option<Child>> = Mutex::new(None);

// Tray menu item for exit status info
static EXIT_INFO_ITEM: Mutex<Option<MenuItem<tauri::Wry>>> = Mutex::new(None);

#[cfg(windows)]
fn is_german_windows() -> bool {
    #[link(name = "kernel32")]
    extern "system" {
        fn GetUserDefaultUILanguage() -> u16;
    }
    let lang_id = unsafe { GetUserDefaultUILanguage() };
    // Primary language ID is lower 10 bits: LANG_GERMAN == 0x07
    (lang_id & 0x03ff) == 0x07
}

#[cfg(not(windows))]
fn is_german_windows() -> bool {
    std::env::var("LANG")
        .or_else(|_| std::env::var("LC_ALL"))
        .map(|v| v.to_lowercase().starts_with("de"))
        .unwrap_or(false)
}

#[tauri::command]
fn get_ui_locale() -> String {
    if is_german_windows() {
        "de".to_string()
    } else {
        "en".to_string()
    }
}

fn get_exit_info_text() -> &'static str {
    let is_de = is_german_windows();
    if check_is_background_installed() {
        if is_de {
            "Agent läuft im Hintergrund weiter"
        } else {
            "Agent will keep running in background"
        }
    } else if is_de {
        "Dies beendet den Agenten"
    } else {
        "This will stop the agent"
    }
}

pub fn update_tray_exit_info() {
    if let Ok(guard) = EXIT_INFO_ITEM.lock() {
        if let Some(ref item) = *guard {
            let _ = item.set_text(get_exit_info_text());
        }
    }
}

// Circular in-memory log buffer for ZebraAgent.exe output
static AGENT_LOGS: Mutex<Vec<String>> = Mutex::new(Vec::new());

// App handle storage for log emission
static APP_HANDLE: Mutex<Option<tauri::AppHandle>> = Mutex::new(None);

#[allow(dead_code)]
fn append_agent_log(line: String) {
    if let Ok(guard) = APP_HANDLE.lock() {
        if let Some(ref app) = *guard {
            let _ = app.emit("agent-stdout", &line);
        }
    }
    let mut logs = AGENT_LOGS.lock().unwrap();
    if logs.len() > 1000 {
        logs.remove(0);
    }
    logs.push(line);
}

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone, Default)]
pub struct RegistryConfig {
    pub api_url: String,
    pub printer_ip: String,
    pub zpl_printers: Vec<String>,
    pub port: String,
    pub token: String,
    pub local_addr: String,
    pub name: String,
}

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone)]
pub struct SimpleStatus {
    pub running: bool,
    pub background: bool,
}

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone)]
pub struct ScannedPrinter {
    pub ip: String,
    pub model: String,
    pub serial: String,
    pub status: String,
    pub ready: bool,
}

fn unpack_embedded_agent() -> Result<PathBuf, String> {
    let temp_dir = std::env::temp_dir().join("ZebraAgent");
    let _ = std::fs::create_dir_all(&temp_dir);
    let target = temp_dir.join("ZebraAgent.exe");

    let needs_write = match std::fs::metadata(&target) {
        Ok(meta) => meta.len() != EMBEDDED_AGENT.len() as u64,
        Err(_) => true,
    };

    if needs_write {
        kill_temp_agent();
        std::thread::sleep(std::time::Duration::from_millis(100));

        std::fs::write(&target, EMBEDDED_AGENT)
            .map_err(|e| format!("Failed to unpack ZebraAgent.exe to {:?}: {}", target, e))?;
    }

    Ok(target)
}

fn get_persistent_agent_path() -> Result<PathBuf, String> {
    let dir = if let Ok(local) = std::env::var("LOCALAPPDATA") {
        PathBuf::from(local).join("ZebraAgent")
    } else {
        std::env::temp_dir().join("ZebraAgent")
    };
    let _ = std::fs::create_dir_all(&dir);
    let target = dir.join("ZebraAgent.exe");

    let needs_write = match std::fs::metadata(&target) {
        Ok(meta) => meta.len() != EMBEDDED_AGENT.len() as u64,
        Err(_) => true,
    };

    if needs_write {
        kill_temp_agent();
        std::thread::sleep(std::time::Duration::from_millis(100));
        std::fs::write(&target, EMBEDDED_AGENT)
            .map_err(|e| format!("Failed to write ZebraAgent.exe to {:?}: {}", target, e))?;
    }

    Ok(target)
}

fn is_process_running(image_name: &str) -> bool {
    let mut cmd = Command::new("tasklist.exe");
    cmd.args(["/FI", &format!("IMAGENAME eq {}", image_name), "/NH"]);
    #[cfg(windows)]
    cmd.creation_flags(CREATE_NO_WINDOW);

    if let Ok(out) = cmd.output() {
        let stdout = String::from_utf8_lossy(&out.stdout).to_lowercase();
        return stdout.contains(&image_name.to_lowercase());
    }
    false
}

fn kill_temp_agent() {
    let mut lock = SPAWNED_AGENT.lock().unwrap();
    if let Some(mut child) = lock.take() {
        let _ = child.kill();
    }
    let mut kill_cmd = Command::new("taskkill.exe");
    kill_cmd.args(["/F", "/IM", "ZebraAgent.exe"]);
    #[cfg(windows)]
    kill_cmd.creation_flags(CREATE_NO_WINDOW);
    let _ = kill_cmd.output();
}

fn launch_agent_process(target: &PathBuf) -> Result<(), String> {
    if is_process_running("ZebraAgent.exe") {
        return Ok(());
    }

    let log_file_path = std::env::temp_dir().join("ZebraAgent").join("agent.log");
    let log_file = std::fs::OpenOptions::new()
        .create(true)
        .write(true)
        .append(true)
        .open(&log_file_path)
        .map_err(|e| format!("Failed to open log file: {}", e))?;

    let log_err = log_file.try_clone().map_err(|e| format!("Clone error: {}", e))?;

    let mut cmd = Command::new(target);
    cmd.stdout(log_file);
    cmd.stderr(log_err);
    #[cfg(windows)]
    cmd.creation_flags(CREATE_NO_WINDOW);

    let child = cmd.spawn().map_err(|e| format!("Failed to launch agent ({:?}): {}", target, e))?;

    let mut lock = SPAWNED_AGENT.lock().unwrap();
    *lock = Some(child);
    Ok(())
}

fn launch_agent_from_temp() -> Result<(), String> {
    let target = unpack_embedded_agent()?;
    launch_agent_process(&target)
}

fn launch_persistent_agent() -> Result<(), String> {
    let target = get_persistent_agent_path()?;
    launch_agent_process(&target)
}

fn check_is_scm_service() -> bool {
    let mut cmd_sc = Command::new("sc.exe");
    cmd_sc.args(["query", "ZebraAgent"]);
    #[cfg(windows)]
    cmd_sc.creation_flags(CREATE_NO_WINDOW);
    if let Ok(out) = cmd_sc.output() {
        let stdout = String::from_utf8_lossy(&out.stdout).to_uppercase();
        if !stdout.contains("1060") && (out.status.success() || stdout.contains("STATE")) {
            return true;
        }
    }
    false
}

fn check_is_background_installed() -> bool {
    // 1. Check HKCU Run key (primary standard user auto-start)
    let hkcu = RegKey::predef(HKEY_CURRENT_USER);
    if let Ok(run_key) = hkcu.open_subkey(r"Software\Microsoft\Windows\CurrentVersion\Run") {
        if run_key.get_value::<String, _>("ZebraAgent").is_ok() {
            return true;
        }
    }

    // 2. Check user scheduled task
    let mut cmd_task = Command::new("schtasks.exe");
    cmd_task.args(["/Query", "/TN", "ZebraAgent"]);
    #[cfg(windows)]
    cmd_task.creation_flags(CREATE_NO_WINDOW);
    if let Ok(out) = cmd_task.output() {
        if out.status.success() {
            return true;
        }
    }

    // 3. Check Windows SCM service
    if check_is_scm_service() {
        return true;
    }

    false
}

#[tauri::command]
async fn get_registry_config() -> Result<RegistryConfig, String> {
    tauri::async_runtime::spawn_blocking(|| {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let key = match hkcu.open_subkey(r"Software\ZebraAgent") {
            Ok(k) => k,
            Err(_) => {
                return Ok(RegistryConfig {
                    api_url: "https://dashdemo.quixpos.com".to_string(),
                    printer_ip: String::new(),
                    zpl_printers: Vec::new(),
                    port: "41783".to_string(),
                    ..Default::default()
                });
            }
        };

        let mut api_url: String = key.get_value("ApiUrl").unwrap_or_default();
        if api_url.trim().is_empty() {
            let hklm = RegKey::predef(HKEY_LOCAL_MACHINE);
            if let Ok(hklm_key) = hklm.open_subkey(r"SOFTWARE\ZebraAgent") {
                api_url = hklm_key.get_value("ApiUrl").unwrap_or_default();
            }
        }
        if api_url.trim().is_empty() {
            api_url = "https://dashdemo.quixpos.com".to_string();
        }
        let printer_ip: String = key.get_value("PrinterIp").unwrap_or_default();

        let mut zpl_printers: Vec<String> = Vec::new();
        if let Ok(raw) = key.get_value::<String, _>("ZplPrinters") {
            for item in raw.split(&[',', ';', '\n', '\r'][..]) {
                let trimmed = item.trim();
                if !trimmed.is_empty() {
                    zpl_printers.push(trimmed.to_string());
                }
            }
        }
        if zpl_printers.is_empty() && !printer_ip.trim().is_empty() {
            zpl_printers.push(printer_ip.trim().to_string());
        }

        let port: String = key.get_value("Port").unwrap_or_else(|_| {
            if let Ok(dw) = key.get_value::<u32, _>("PortDWord") {
                dw.to_string()
            } else {
                "41783".to_string()
            }
        });
        let token: String = key.get_value("Token").unwrap_or_default();
        let local_addr: String = key.get_value("LocalAddr").unwrap_or_default();
        let name: String = key.get_value("Name").unwrap_or_default();

        Ok(RegistryConfig {
            api_url,
            printer_ip,
            zpl_printers,
            port,
            token,
            local_addr,
            name,
        })
    })
    .await
    .map_err(|e| e.to_string())?
}

#[cfg(windows)]
fn signal_agents_reload_event() {
    use std::ffi::OsStr;
    use std::os::windows::ffi::OsStrExt;

    #[link(name = "kernel32")]
    extern "system" {
        fn OpenEventW(dwDesiredAccess: u32, bInheritHandle: i32, lpName: *const u16) -> isize;
        fn CreateEventW(
            lpEventAttributes: *const std::ffi::c_void,
            bManualReset: i32,
            bInitialState: i32,
            lpName: *const u16,
        ) -> isize;
        fn SetEvent(hEvent: isize) -> i32;
        fn CloseHandle(hObject: isize) -> i32;
    }

    const EVENT_MODIFY_STATE: u32 = 0x0002;

    for name in &["Global\\ZebraAgentConfigReload", "Local\\ZebraAgentConfigReload"] {
        let wide: Vec<u16> = OsStr::new(name).encode_wide().chain(std::iter::once(0)).collect();
        let mut handle = unsafe { OpenEventW(EVENT_MODIFY_STATE, 0, wide.as_ptr()) };
        if handle == 0 {
            handle = unsafe { CreateEventW(std::ptr::null(), 0, 0, wide.as_ptr()) };
        }
        if handle != 0 {
            unsafe {
                SetEvent(handle);
                CloseHandle(handle);
            }
        }
    }
}

#[cfg(not(windows))]
fn signal_agents_reload_event() {}

fn signal_agents_http_reload(active_port: u16) {
    use std::io::Write;
    use std::net::{SocketAddr, TcpStream};
    use std::time::Duration;

    let mut ports = vec![active_port, 41782];
    for p in 41783..=41789 {
        if !ports.contains(&p) {
            ports.push(p);
        }
    }

    for p in ports {
        let addr = SocketAddr::from(([127, 0, 0, 1], p));
        if let Ok(mut stream) = TcpStream::connect_timeout(&addr, Duration::from_millis(100)) {
            let _ = stream.set_write_timeout(Some(Duration::from_millis(200)));
            let _ = stream.set_read_timeout(Some(Duration::from_millis(200)));
            let req = format!(
                "POST /reload HTTP/1.1\r\nHost: 127.0.0.1:{}\r\nConnection: close\r\nContent-Length: 0\r\n\r\n",
                p
            );
            let _ = stream.write_all(req.as_bytes());
        }
    }
}

#[tauri::command]
async fn save_registry_config(
    api_url: String,
    zpl_printers: Vec<String>,
    port: Option<String>,
) -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(move || {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let (key, _) = hkcu
            .create_subkey(r"Software\ZebraAgent")
            .map_err(|e| format!("Failed to open/create HKCU\\Software\\ZebraAgent: {}", e))?;

        let trimmed_api = api_url.trim();
        key.set_value("ApiUrl", &trimmed_api)
            .map_err(|e| format!("Failed to set ApiUrl: {}", e))?;

        let clean_printers: Vec<String> = zpl_printers
            .into_iter()
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
            .collect();

        let joined = clean_printers.join(",");
        key.set_value("ZplPrinters", &joined)
            .map_err(|e| format!("Failed to set ZplPrinters: {}", e))?;

        let primary_ip = clean_printers.first().cloned().unwrap_or_default();
        let _ = key.set_value("PrinterIp", &primary_ip);

        let mut saved_port = None;
        if let Some(p) = port {
            let trimmed_p = p.trim();
            if !trimmed_p.is_empty() {
                key.set_value("Port", &trimmed_p)
                    .map_err(|e| format!("Failed to set Port: {}", e))?;
                saved_port = Some(trimmed_p.to_string());
            }
        }

        // Best effort write to HKLM as well (for machine / SCM background service)
        let hklm = RegKey::predef(HKEY_LOCAL_MACHINE);
        if let Ok((hklm_key, _)) = hklm.create_subkey(r"SOFTWARE\ZebraAgent") {
            let _ = hklm_key.set_value("ApiUrl", &trimmed_api);
            let _ = hklm_key.set_value("ZplPrinters", &joined);
            let _ = hklm_key.set_value("PrinterIp", &primary_ip);
            if let Some(ref p) = saved_port {
                let _ = hklm_key.set_value("Port", p);
            }
        }

        // Signal running agents via Win32 Named Event
        signal_agents_reload_event();

        // Signal running agents via local HTTP monitor endpoint
        let active_port = saved_port
            .and_then(|p| p.parse::<u16>().ok())
            .or_else(|| key.get_value::<String, _>("Port").ok().and_then(|p| p.parse::<u16>().ok()))
            .unwrap_or(41782);

        signal_agents_http_reload(active_port);

        Ok(())
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
async fn get_service_status() -> Result<SimpleStatus, String> {
    tauri::async_runtime::spawn_blocking(|| {
        let running = is_process_running("ZebraAgent.exe");
        let background = check_is_background_installed();
        Ok(SimpleStatus { running, background })
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
async fn get_agent_logs() -> Result<Vec<String>, String> {
    tauri::async_runtime::spawn_blocking(|| {
        let temp_log = std::env::temp_dir().join("ZebraAgent").join("agent.log");
        let prog_log = if let Ok(pd) = std::env::var("ProgramData") {
            PathBuf::from(pd).join("ZebraAgent").join("agent.log")
        } else {
            PathBuf::new()
        };

        let target_file = if temp_log.is_file() {
            Some(temp_log)
        } else if prog_log.is_file() {
            Some(prog_log)
        } else {
            None
        };

        if let Some(path) = target_file {
            if let Ok(file) = std::fs::File::open(path) {
                let reader = BufReader::new(file);
                let lines: Vec<String> = reader.lines().flatten().collect();
                let start = if lines.len() > 300 { lines.len() - 300 } else { 0 };
                return Ok(lines[start..].to_vec());
            }
        }

        let logs = AGENT_LOGS.lock().unwrap().clone();
        Ok(logs)
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
async fn install_service() -> Result<String, String> {
    tauri::async_runtime::spawn_blocking(|| {
        let target = get_persistent_agent_path()?;
        let target_str = target.to_string_lossy().to_string();

        // 1. Install user-level background auto-start via HKCU Run key (no admin / elevation required)
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let (run_key, _) = hkcu
            .create_subkey(r"Software\Microsoft\Windows\CurrentVersion\Run")
            .map_err(|e| format!("Registry access error: {}", e))?;

        let run_cmd = format!("\"{}\"", target_str);
        run_key
            .set_value("ZebraAgent", &run_cmd)
            .map_err(|e| format!("Failed to register background startup: {}", e))?;

        // 2. Best-effort Task Scheduler (optional, ignores failure if non-admin)
        let mut cmd = Command::new("schtasks.exe");
        cmd.args([
            "/Create",
            "/TN", "ZebraAgent",
            "/TR", &run_cmd,
            "/SC", "ONLOGON",
            "/RL", "LIMITED",
            "/F",
        ]);
        #[cfg(windows)]
        cmd.creation_flags(CREATE_NO_WINDOW);
        let _ = cmd.output();

        // 3. Start persistent background process immediately if not running
        if !is_process_running("ZebraAgent.exe") {
            let _ = launch_persistent_agent();
        }

        update_tray_exit_info();

        Ok("Installed as background service".to_string())
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
async fn uninstall_service() -> Result<String, String> {
    tauri::async_runtime::spawn_blocking(|| {
        // 1. Remove from HKCU Run key
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        if let Ok((run_key, _)) = hkcu.create_subkey(r"Software\Microsoft\Windows\CurrentVersion\Run") {
            let _ = run_key.delete_value("ZebraAgent");
        }

        // 2. Remove Task Scheduler task if it exists
        let mut cmd = Command::new("schtasks.exe");
        cmd.args(["/Delete", "/TN", "ZebraAgent", "/F"]);
        #[cfg(windows)]
        cmd.creation_flags(CREATE_NO_WINDOW);
        let _ = cmd.output();

        // 3. Clean up SCM service if one existed
        let mut sc_cmd = Command::new("sc.exe");
        sc_cmd.args(["delete", "ZebraAgent"]);
        #[cfg(windows)]
        sc_cmd.creation_flags(CREATE_NO_WINDOW);
        let _ = sc_cmd.output();

        // 4. Terminate running background process
        kill_temp_agent();
        std::thread::sleep(std::time::Duration::from_millis(300));

        // 5. Relaunch normally under Tauri as a regular session process
        let _ = launch_agent_from_temp();

        update_tray_exit_info();

        Ok("Uninstalled background service".to_string())
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
async fn clear_agent_logs() -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(|| {
        let mut logs = AGENT_LOGS.lock().unwrap();
        logs.clear();

        let temp_log = std::env::temp_dir().join("ZebraAgent").join("agent.log");
        if temp_log.is_file() {
            let _ = std::fs::write(&temp_log, "");
        }
        Ok(())
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
async fn unpair_agent() -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(|| {
        // 1. Clear Token and Name in HKCU registry
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        if let Ok((key, _)) = hkcu.create_subkey(r"Software\ZebraAgent") {
            let _ = key.set_value("Token", &"");
            let _ = key.set_value("Name", &"");
        }

        // 2. Restart agent process/service to immediately drop upstream connection
        if check_is_scm_service() {
            let mut stop = Command::new("sc.exe");
            stop.args(["stop", "ZebraAgent"]);
            #[cfg(windows)]
            stop.creation_flags(CREATE_NO_WINDOW);
            let _ = stop.output();

            std::thread::sleep(std::time::Duration::from_millis(400));

            let mut start = Command::new("sc.exe");
            start.args(["start", "ZebraAgent"]);
            #[cfg(windows)]
            start.creation_flags(CREATE_NO_WINDOW);
            let _ = start.output();
        } else {
            kill_temp_agent();
            std::thread::sleep(std::time::Duration::from_millis(300));
            if check_is_background_installed() {
                let _ = launch_persistent_agent();
            } else {
                let _ = launch_agent_from_temp();
            }
        }

        Ok(())
    })
    .await
    .map_err(|e| e.to_string())?
}

fn parse_zpl_probe_response(ip: &str, data: &str) -> Option<ScannedPrinter> {
    if data.trim().is_empty() {
        return None;
    }

    let mut serial = String::new();
    let mut model = String::new();

    for chunk in data.split('"') {
        let c = chunk.trim();
        if c.len() >= 6 && c.len() <= 24 && !c.contains(' ') && !c.contains("device.") {
            serial = c.to_string();
            break;
        }
    }

    for token in data.split(|c| c == '\x02' || c == '\x03' || c == '\r' || c == '\n') {
        let t = token.trim();
        if t.contains(',') && !t.contains("ERRORS:") && !t.contains("WARNINGS:") {
            let parts: Vec<&str> = t.split(',').collect();
            if !parts.is_empty() && model.is_empty() {
                let mut m = parts[0].trim();
                if let Some(idx) = m.rfind('"') {
                    m = m[idx + 1..].trim();
                }
                if !m.is_empty() {
                    model = m.to_string();
                }
            }
            if parts.len() >= 7 && serial.is_empty() {
                let cand = parts[6].trim().trim_start_matches('+');
                let upper = cand.to_uppercase();
                if !upper.ends_with("KB") && !upper.ends_with("MB") && upper != "NONE" {
                    serial = cand.to_string();
                }
            }
        }
    }

    if model.is_empty() && serial.is_empty() && !data.contains("ERRORS:") && !data.contains("PRINTER STATUS") {
        return None;
    }

    Some(ScannedPrinter {
        ip: ip.to_string(),
        model,
        serial,
        status: "online".to_string(),
        ready: true,
    })
}

fn get_local_ipv4_prefixes() -> Vec<String> {
    let mut prefixes = Vec::new();
    if let Ok(sock) = std::net::UdpSocket::bind("0.0.0.0:0") {
        if sock.connect("8.8.8.8:80").is_ok() {
            if let Ok(local_addr) = sock.local_addr() {
                if let std::net::SocketAddr::V4(v4) = local_addr {
                    let octets = v4.ip().octets();
                    if octets[0] != 127 {
                        prefixes.push(format!("{}.{}.{}", octets[0], octets[1], octets[2]));
                    }
                }
            }
        }
    }
    prefixes
}

#[tauri::command]
async fn scan_local_printers() -> Result<Vec<ScannedPrinter>, String> {
    tauri::async_runtime::spawn_blocking(|| {
        use std::io::{Read, Write};
        use std::net::{IpAddr, Ipv4Addr, SocketAddr, TcpStream};
        use std::sync::Arc;
        use std::time::{Duration, Instant};

        let prefixes = get_local_ipv4_prefixes();
        if prefixes.is_empty() {
            return Ok(Vec::new());
        }

        let mut ips = Vec::new();
        for p in prefixes {
            for host in 1..=254 {
                ips.push(format!("{}.{}", p, host));
            }
        }

        let found = Arc::new(Mutex::new(Vec::new()));
        let mut handles = Vec::new();

        for chunk in ips.chunks(64) {
            for ip_str in chunk {
                let ip = ip_str.clone();
                let found_clone = Arc::clone(&found);
                handles.push(std::thread::spawn(move || {
                    if let Ok(ipv4) = ip.parse::<Ipv4Addr>() {
                        let addr = SocketAddr::new(IpAddr::V4(ipv4), 9100);
                        if let Ok(mut stream) = TcpStream::connect_timeout(&addr, Duration::from_millis(350)) {
                            let _ = stream.set_write_timeout(Some(Duration::from_millis(800)));
                            let _ = stream.set_read_timeout(Some(Duration::from_millis(400)));
                            let cmd = b"~HI~HQES! U1 getvar \"device.unique_id\"\r\n";
                            if stream.write_all(cmd).is_ok() {
                                let mut buf = [0u8; 4096];
                                let mut acc = String::new();
                                let start = Instant::now();
                                while start.elapsed() < Duration::from_millis(1600) {
                                    match stream.read(&mut buf) {
                                        Ok(0) => break,
                                        Ok(n) => {
                                            acc.push_str(&String::from_utf8_lossy(&buf[..n]));
                                            if acc.contains("WARNINGS:") && acc.matches('"').count() >= 2 {
                                                break;
                                            }
                                        }
                                        Err(_) => break,
                                    }
                                }
                                if let Some(printer) = parse_zpl_probe_response(&ip, &acc) {
                                    if let Ok(mut list) = found_clone.lock() {
                                        list.push(printer);
                                    }
                                }
                            }
                        }
                    }
                }));
            }
            for h in handles.drain(..) {
                let _ = h.join();
            }
        }

        let mut results = found.lock().unwrap().clone();
        results.sort_by(|a, b| a.ip.cmp(&b.ip));
        Ok(results)
    })
    .await
    .map_err(|e| e.to_string())?
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.unminimize();
                let _ = window.set_focus();
            }
        }))
        .setup(|app| {
            if cfg!(debug_assertions) {
                app.handle().plugin(
                    tauri_plugin_log::Builder::default()
                        .level(log::LevelFilter::Info)
                        .build(),
                )?;
            }

            // Store app handle for event emissions
            if let Ok(mut guard) = APP_HANDLE.lock() {
                *guard = Some(app.handle().clone());
            }

            // Startup requirement:
            // 1. Unpack service\ZebraAgent.exe to temp folder
            // 2. Check if background service/autostart is installed; if not, launch agent from temp
            let _ = unpack_embedded_agent();
            let is_bg = check_is_background_installed();
            if is_bg {
                if !is_process_running("ZebraAgent.exe") {
                    let _ = launch_persistent_agent();
                }
            } else {
                let _ = launch_agent_from_temp();
            }

            // Taskbar tray icon and menu setup
            let is_de = is_german_windows();
            let open_label = if is_de { "Öffnen" } else { "Open" };
            let exit_label = if is_de { "Beenden" } else { "Exit" };
            let open_item = MenuItemBuilder::with_id("open", open_label).build(app)?;
            let exit_info_item = MenuItemBuilder::with_id("exit_info", get_exit_info_text())
                .enabled(false)
                .build(app)?;
            let exit_item = MenuItemBuilder::with_id("exit", exit_label).build(app)?;

            let tray_menu = MenuBuilder::new(app)
                .item(&open_item)
                .separator()
                .item(&exit_info_item)
                .item(&exit_item)
                .build()?;

            if let Ok(mut guard) = EXIT_INFO_ITEM.lock() {
                *guard = Some(exit_info_item);
            }

            let icon = app.default_window_icon().cloned().expect("default window icon is present");

            let _tray = TrayIconBuilder::new()
                .icon(icon)
                .tooltip("Zebra Agent Manager")
                .menu(&tray_menu)
                .show_menu_on_left_click(false)
                .on_menu_event(|app, event| {
                    match event.id().as_ref() {
                        "open" => {
                            if let Some(window) = app.get_webview_window("main") {
                                let _ = window.show();
                                let _ = window.unminimize();
                                let _ = window.set_focus();
                            }
                        }
                        "exit" => {
                            if !check_is_background_installed() {
                                kill_temp_agent();
                            }
                            app.exit(0);
                        }
                        _ => {}
                    }
                })
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click { .. } = &event {
                        update_tray_exit_info();
                    }
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } = event
                    {
                        let app = tray.app_handle();
                        if let Some(window) = app.get_webview_window("main") {
                            let _ = window.show();
                            let _ = window.unminimize();
                            let _ = window.set_focus();
                        }
                    }
                })
                .build(app)?;

            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                let _ = window.hide();
            }
        })
        .invoke_handler(tauri::generate_handler![
            get_registry_config,
            save_registry_config,
            get_service_status,
            get_agent_logs,
            clear_agent_logs,
            install_service,
            uninstall_service,
            unpair_agent,
            scan_local_printers,
            get_ui_locale
        ])
        .run(tauri::generate_context!())
        .expect("error while building tauri application");
}
