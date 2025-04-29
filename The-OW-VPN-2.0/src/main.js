// Import Tauri APIs
import { invoke } from "@tauri-apps/api/tauri";
import { appWindow } from "@tauri-apps/api/window";

// UI Elements
let statusEl;
let regionSelectEl;
let blockBtnEl;
let unblockBtnEl;
let unblockAllBtnEl;
let refreshBtnEl;
let outputEl;
let spinnerEl;

// Application state
let isProcessing = false;

// Initialize the app once DOM is loaded
window.addEventListener("DOMContentLoaded", async () => {
    // Get UI elements
    statusEl = document.querySelector("#status");
    regionSelectEl = document.querySelector("#region-select");
    blockBtnEl = document.querySelector("#block-btn");
    unblockBtnEl = document.querySelector("#unblock-btn");
    unblockAllBtnEl = document.querySelector("#unblock-all-btn");
    refreshBtnEl = document.querySelector("#refresh-btn");
    outputEl = document.querySelector("#output");
    spinnerEl = document.querySelector("#spinner");

    // Set up event listeners
    blockBtnEl.addEventListener("click", blockRegion);
    unblockBtnEl.addEventListener("click", unblockRegion);
    unblockAllBtnEl.addEventListener("click", unblockAll);
    refreshBtnEl.addEventListener("click", refreshStatus);

    // Initial actions
    showSpinner();
    await refreshStatus();
    hideSpinner();

    // Handle app close via window controls
    appWindow.onCloseRequested(async (event) => {
        showSpinner();
        setStatus("Cleaning up...");

        try {
            // Unblock all IPs before closing
            await invoke("unblock_all");
        } catch (e) {
            console.error("Error during cleanup:", e);
        }

        hideSpinner();
        // The app will close automatically after this handler completes
    });
});

// Block the selected region
async function blockRegion() {
    if (isProcessing) return;

    const region = regionSelectEl.value;
    if (!region) {
        setOutput("Please select a region to block");
        return;
    }

    showSpinner();
    setStatus(`Blocking ${getRegionName(region)}...`);

    try {
        const result = await invoke("block_region", { region });
        setOutput(result);
        await refreshStatus();
    } catch (error) {
        setOutput(`Error: ${error}`);
        setStatus("Error");
    } finally {
        hideSpinner();
    }
}

// Unblock the selected region
async function unblockRegion() {
    if (isProcessing) return;

    const region = regionSelectEl.value;
    if (!region) {
        setOutput("Please select a region to unblock");
        return;
    }

    showSpinner();
    setStatus(`Unblocking ${getRegionName(region)}...`);

    try {
        const result = await invoke("unblock_region", { region });
        setOutput(result);
        await refreshStatus();
    } catch (error) {
        setOutput(`Error: ${error}`);
        setStatus("Error");
    } finally {
        hideSpinner();
    }
}

// Unblock all regions
async function unblockAll() {
    if (isProcessing) return;

    showSpinner();
    setStatus("Unblocking all regions...");

    try {
        const result = await invoke("unblock_all");
        setOutput(result);
        await refreshStatus();
    } catch (error) {
        setOutput(`Error: ${error}`);
        setStatus("Error");
    } finally {
        hideSpinner();
    }
}

// Refresh the status
async function refreshStatus() {
    if (isProcessing) return;

    showSpinner();
    setStatus("Checking status...");

    try {
        const result = await invoke("get_firewall_status");
        setOutput(result);

        // Update UI based on status
        const isRunning = result.includes("Overwatch is currently running");
        blockBtnEl.disabled = isRunning;

        if (isRunning) {
            setStatus("Overwatch is running");
        } else {
            setStatus("Ready");
        }
    } catch (error) {
        setOutput(`Error: ${error}`);
        setStatus("Error");
    } finally {
        hideSpinner();
    }
}

// Helper function to update the status text
function setStatus(message) {
    statusEl.textContent = message;
}

// Helper function to update the output area
function setOutput(message) {
    outputEl.textContent = message;
}

// Helper functions for the spinner
function showSpinner() {
    isProcessing = true;
    spinnerEl.style.display = "inline-block";
    blockBtnEl.disabled = true;
    unblockBtnEl.disabled = true;
    unblockAllBtnEl.disabled = true;
    refreshBtnEl.disabled = true;
}

function hideSpinner() {
    isProcessing = false;
    spinnerEl.style.display = "none";
    blockBtnEl.disabled = false;
    unblockBtnEl.disabled = false;
    unblockAllBtnEl.disabled = false;
    refreshBtnEl.disabled = false;
}

// Helper function to get the full region name
function getRegionName(regionCode) {
    const regionMap = {
        EU: "Europe",
        NA: "North America",
        SA: "South America",
        AS: "Asia",
        AFR: "Africa",
        ME: "Middle East",
        OCE: "Oceania",
    };

    return regionMap[regionCode] || regionCode;
}
