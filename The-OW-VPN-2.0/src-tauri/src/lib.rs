use anyhow::{anyhow, Result};
use once_cell::sync::OnceCell;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::Mutex;
use tauri::path::BaseDirectory;
use tauri::{AppHandle, Manager, State};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};

// Global storage for the firewall sidecar process
static FIREWALL_SIDECAR: OnceCell<Mutex<Option<CommandChild>>> = OnceCell::new();

struct AppState {
    ip_puller_path: PathBuf,
    firewall_sidecar_path: PathBuf,
}

// Run IP puller to fetch updated IP lists
#[tauri::command]
async fn run_ip_puller(app_handle: AppHandle) -> Result<String, String> {
    let state = app_handle.state::<AppState>();
    
    // Run the IP puller
    let output = Command::new(&state.ip_puller_path)
        .output()
        .map_err(|e| format!("Failed to execute IP puller: {}", e))?;
    
    if !output.status.success() {
        let error = String::from_utf8_lossy(&output.stderr);
        return Err(format!("IP puller failed: {}", error));
    }
    
    Ok("IP lists updated successfully".to_string())
}

// Get firewall sidecar status
#[tauri::command]
async fn get_firewall_status(app_handle: AppHandle) -> Result<String, String> {
    let sidecar_mutex = get_firewall_sidecar().ok_or_else(|| "Firewall sidecar not initialized".to_string())?;
    let mut sidecar_guard = sidecar_mutex.lock().map_err(|_| "Failed to lock firewall sidecar mutex".to_string())?;
    
    if let Some(child) = &mut *sidecar_guard {
        // Use the existing elevated sidecar process to get status
        let output = child
            .write("status\n")
            .map_err(|e| format!("Failed to send command to sidecar: {}", e))?;
            
        // Return the output from the sidecar
        Ok(output)
    } else {
        Err("Firewall sidecar process not available".to_string())
    }
}

// Block a region using the firewall sidecar
#[tauri::command]
async fn block_region(app_handle: AppHandle, region: String) -> Result<String, String> {
    let sidecar_mutex = get_firewall_sidecar().ok_or_else(|| "Firewall sidecar not initialized".to_string())?;
    let mut sidecar_guard = sidecar_mutex.lock().map_err(|_| "Failed to lock firewall sidecar mutex".to_string())?;
    
    if let Some(child) = &mut *sidecar_guard {
        // Use the existing elevated sidecar process to block the region
        let output = child
            .write(format!("block|{}\n", region))
            .map_err(|e| format!("Failed to send command to sidecar: {}", e))?;
            
        // Return the output from the sidecar
        Ok(output)
    } else {
        Err("Firewall sidecar process not available".to_string())
    }
}

// Unblock a region using the firewall sidecar
#[tauri::command]
async fn unblock_region(app_handle: AppHandle, region: String) -> Result<String, String> {
    let sidecar_mutex = get_firewall_sidecar().ok_or_else(|| "Firewall sidecar not initialized".to_string())?;
    let mut sidecar_guard = sidecar_mutex.lock().map_err(|_| "Failed to lock firewall sidecar mutex".to_string())?;
    
    if let Some(child) = &mut *sidecar_guard {
        // Use the existing elevated sidecar process to unblock the region
        let output = child
            .write(format!("unblock|{}\n", region))
            .map_err(|e| format!("Failed to send command to sidecar: {}", e))?;
            
        // Return the output from the sidecar
        Ok(output)
    } else {
        Err("Firewall sidecar process not available".to_string())
    }
}

// Unblock all using the firewall sidecar
#[tauri::command]
async fn unblock_all(app_handle: AppHandle) -> Result<String, String> {
    let sidecar_mutex = get_firewall_sidecar().ok_or_else(|| "Firewall sidecar not initialized".to_string())?;
    let mut sidecar_guard = sidecar_mutex.lock().map_err(|_| "Failed to lock firewall sidecar mutex".to_string())?;
    
    if let Some(child) = &mut *sidecar_guard {
        // Use the existing elevated sidecar process to unblock all
        let output = child
            .write("unblock-all\n")
            .map_err(|e| format!("Failed to send command to sidecar: {}", e))?;
            
        // Return the output from the sidecar
        Ok(output)
    } else {
        Err("Firewall sidecar process not available".to_string())
    }
}

// Initialize the firewall sidecar process
fn init_firewall_sidecar(app_handle: &AppHandle) -> Result<()> {
    let state = app_handle.state::<AppState>();

    // First, run IP puller to get latest IPs
    let ip_puller_result = Command::new(&state.ip_puller_path)
        .output()?;

    if !ip_puller_result.status.success() {
        let error = String::from_utf8_lossy(&ip_puller_result.stderr);
        return Err(anyhow!("IP puller failed: {}", error));
    }

    // Start the firewall sidecar in daemon mode (will show UAC prompt)
    let child = tauri_plugin_shell::process::Command::new_sidecar("ow-firewall-sidecar")
        .expect("failed to create sidecar command")
        .args(["daemon"])
        .spawn()
        .map_err(|e| anyhow!("Failed to start firewall sidecar: {}", e))?;

    // Register for events from the child process
    app_handle.listen_global("ow-firewall-sidecar://output", |event| {
        println!("Sidecar output: {}", event.payload().unwrap_or_default());
    });

    // Store the child process
    let mutex = Mutex::new(Some(child));
    FIREWALL_SIDECAR
        .set(mutex)
        .map_err(|_| anyhow!("Failed to store firewall sidecar process"))?;

    Ok(())
}

// Helper function to get the firewall sidecar mutex
fn get_firewall_sidecar() -> Option<&'static Mutex<Option<CommandChild>>> {
    FIREWALL_SIDECAR.get()
}

// Clean up the firewall sidecar on app exit
fn cleanup_firewall_sidecar(app_handle: &AppHandle) -> Result<()> {
    // Get the sidecar mutex, if it exists
    if let Some(mutex) = get_firewall_sidecar() {
        // Get a lock on the mutex
        let mut guard = mutex.lock().map_err(|_| anyhow!("Failed to lock firewall sidecar mutex"))?;
        
        // If there's a child process, unblock all IPs and kill it
        if let Some(child) = guard.take() {
            let state = app_handle.state::<AppState>();
            
            // Run unblock-all to clean up firewall rules
            let _cleanup_result = Command::new(&state.firewall_sidecar_path)
                .args(["-action", "unblock-all"])
                .output();
                
            // Kill the child process
            let _kill_result = child.kill();
        }
    }
    
    Ok(())
}

// Setup Tauri application
#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let mut app = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .setup(|app| {
            // Determine paths for the executables
            let app_dir = app.path().app_dir(BaseDirectory::App).expect("Failed to get app directory");
            
            let ip_puller_path = app_dir.join("ip-puller");
            let firewall_sidecar_path = app_dir.join("ow-firewall-sidecar");
            
            #[cfg(target_os = "windows")]
            let ip_puller_path = ip_puller_path.with_extension("exe");
            
            #[cfg(target_os = "windows")]
            let firewall_sidecar_path = firewall_sidecar_path.with_extension("exe");
            
            // Create and store app state
            app.manage(AppState {
                ip_puller_path,
                firewall_sidecar_path,
            });
            
            // Initialize the firewall sidecar
            if let Err(err) = init_firewall_sidecar(app.app_handle()) {
                eprintln!("Failed to initialize firewall sidecar: {}", err);
                // Show error dialog but continue
                let _ = tauri_plugin_dialog::Dialog::default().message("Error").title("Failed to initialize firewall sidecar").show();
            }
            
            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                // Clean up firewall rules and processes before exit
                if let Err(err) = cleanup_firewall_sidecar(window.app_handle()) {
                    eprintln!("Error during cleanup: {}", err);
                }
                
                // Allow the window to close
                api.prevent_close();
                window.close().unwrap();
            }
        })
        .invoke_handler(tauri::generate_handler![
            run_ip_puller,
            get_firewall_status,
            block_region,
            unblock_region,
            unblock_all,
        ])
        .build(tauri::generate_context!())
        .expect("error while building tauri application");
        
    app.run(|_, _| {});
}