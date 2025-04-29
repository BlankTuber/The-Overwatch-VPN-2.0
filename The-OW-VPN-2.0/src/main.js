// Import Tauri APIs
import { invoke } from "@tauri-apps/api/tauri";
import { appWindow } from "@tauri-apps/api/window";

// UI Elements
let statusDotEl;
let statusTextEl;
let consoleOutputEl;
let unblockAllBtnEl;
let refreshBtnEl;
let gameRunningStatusEl;
let gameStatusMessageEl;
let gameStatusSvgEl;
let waitingModalEl;
let infoModalEl;

// Region toggle elements
const regions = ["EU", "NA", "AS", "OCE", "SA", "ME", "AFR"];
const regionToggles = {};
const regionStatuses = {};
const regionCards = {};

// Application state
let isProcessing = false;
let blockedRegions = new Set();
let isOverwatchRunning = false;

// Initialize the app once DOM is loaded
window.addEventListener("DOMContentLoaded", async () => {
    // Get UI elements
    statusDotEl = document.querySelector(".status-dot");
    statusTextEl = document.getElementById("status-text");
    consoleOutputEl = document.getElementById("console-output");
    unblockAllBtnEl = document.getElementById("unblock-all-btn");
    refreshBtnEl = document.getElementById("refresh-btn");
    gameRunningStatusEl = document.getElementById("game-running-status");
    gameStatusMessageEl = document.getElementById("game-status-message");
    gameStatusSvgEl = document.getElementById("game-status-svg");
    waitingModalEl = document.getElementById("waiting-modal");
    infoModalEl = document.getElementById("info-modal");

    // Get region elements
    regions.forEach((region) => {
        regionToggles[region] = document.getElementById(
            `${region.toLowerCase()}-toggle`,
        );
        regionStatuses[region] = document.getElementById(
            `${region.toLowerCase()}-status`,
        );
        regionCards[region] = document.querySelector(
            `.region-card[data-region="${region}"]`,
        );

        // Add event listeners to toggles
        if (regionToggles[region]) {
            regionToggles[region].addEventListener("change", () =>
                toggleRegion(region),
            );
        }
    });

    // Set up other event listeners
    unblockAllBtnEl.addEventListener("click", unblockAll);
    refreshBtnEl.addEventListener("click", refreshStatus);

    // Info modal controls
    document.getElementById("show-info").addEventListener("click", (e) => {
        e.preventDefault();
        infoModalEl.classList.add("active");
    });

    document.querySelector(".close-modal").addEventListener("click", () => {
        infoModalEl.classList.remove("active");
    });

    // Close modal when clicking outside
    infoModalEl.addEventListener("click", (e) => {
        if (e.target === infoModalEl) {
            infoModalEl.classList.remove("active");
        }
    });

    // Initial actions
    setLoading(true);
    updateStatus("Initializing...", "warning");
    addConsoleMessage("Starting Overwatch VPN...");
    addConsoleMessage("Fetching IP ranges...");

    // Wait for initial status
    await refreshStatus();

    setLoading(false);
    updateStatus("Ready", "online");
    addConsoleMessage("Overwatch VPN initialized successfully!");

    // Refresh status periodically
    setInterval(refreshStatus, 30000);

    // Handle app close via window controls
    appWindow.onCloseRequested(async (event) => {
        setLoading(true);
        updateStatus("Cleaning up...", "warning");
        addConsoleMessage("Shutting down Overwatch VPN...");
        addConsoleMessage("Removing all firewall rules...");

        try {
            // Unblock all IPs before closing
            await invoke("unblock_all");
            addConsoleMessage("All regions unblocked successfully.");
        } catch (e) {
            addConsoleMessage(`Error during cleanup: ${e}`, true);
        }

        setLoading(false);
        // The app will close automatically after this handler completes
    });
});

// Toggle a region block status
async function toggleRegion(region) {
    if (isProcessing) return;

    const isChecked = regionToggles[region].checked;

    // If trying to block and Overwatch is running, show warning
    if (isChecked && isOverwatchRunning) {
        showWaitingModal(true);
    }

    setLoading(true);

    // Add visual feedback
    regionCards[region].classList.add("state-changing");
    setTimeout(() => {
        regionCards[region].classList.remove("state-changing");
    }, 1000);

    try {
        if (isChecked) {
            // Block the region
            updateRegionStatus(region, "Blocking...", "waiting");
            addConsoleMessage(`Blocking ${getRegionName(region)}...`);

            const result = await invoke("block_region", { region });
            addConsoleMessage(result);

            blockedRegions.add(region);
            updateRegionStatus(region, "Blocked", "blocked");
        } else {
            // Unblock the region
            updateRegionStatus(region, "Unblocking...", "waiting");
            addConsoleMessage(`Unblocking ${getRegionName(region)}...`);

            const result = await invoke("unblock_region", { region });
            addConsoleMessage(result);

            blockedRegions.delete(region);
            updateRegionStatus(region, "Not Blocked", "");
        }

        await refreshStatus();
    } catch (error) {
        addConsoleMessage(`Error: ${error}`, true);

        // Reset the toggle to its previous state
        regionToggles[region].checked = !isChecked;

        if (isChecked) {
            blockedRegions.delete(region);
            updateRegionStatus(region, "Not Blocked", "");
        } else {
            blockedRegions.add(region);
            updateRegionStatus(region, "Blocked", "blocked");
        }
    } finally {
        setLoading(false);
        showWaitingModal(false);
    }
}

// Unblock all regions
async function unblockAll() {
    if (isProcessing) return;

    setLoading(true);
    updateStatus("Unblocking all regions...", "warning");
    addConsoleMessage("Unblocking all regions...");

    try {
        // Reset all toggles
        regions.forEach((region) => {
            if (regionToggles[region] && regionToggles[region].checked) {
                regionToggles[region].checked = false;
                updateRegionStatus(region, "Not Blocked", "");
            }
        });

        // Unblock all regions
        const result = await invoke("unblock_all");
        addConsoleMessage(result);

        // Clear blocked regions set
        blockedRegions.clear();

        // Refresh status
        await refreshStatus();
    } catch (error) {
        addConsoleMessage(`Error: ${error}`, true);
    } finally {
        setLoading(false);
    }
}

// Refresh the status
async function refreshStatus() {
    if (isProcessing) return;

    try {
        // Get current firewall status
        const result = await invoke("get_firewall_status");

        // Check if Overwatch is running
        const wasRunning = isOverwatchRunning;
        isOverwatchRunning = result.includes("Overwatch is currently running");

        // Update game status UI
        updateGameStatus(isOverwatchRunning);

        // If Overwatch state changed, add console message
        if (isOverwatchRunning !== wasRunning) {
            addConsoleMessage(
                isOverwatchRunning
                    ? "Overwatch is now running. Blocking operations will wait until it closes."
                    : "Overwatch is now closed. All operations available.",
            );
        }

        // Disable toggles if Overwatch is running
        regions.forEach((region) => {
            if (regionToggles[region]) {
                // Only disable if it's not already blocked
                regionToggles[region].disabled =
                    isOverwatchRunning && !blockedRegions.has(region);
            }
        });

        // Update main status
        if (isOverwatchRunning) {
            updateStatus("Overwatch is running", "warning");
        } else {
            updateStatus("Ready", "online");
        }
    } catch (error) {
        addConsoleMessage(`Error checking status: ${error}`, true);
        updateStatus("Error", "error");
        updateGameStatus(false, true);
    }
}

// Update game status UI
function updateGameStatus(isRunning, isError = false) {
    if (isError) {
        gameRunningStatusEl.textContent = "Error checking Overwatch";
        gameStatusMessageEl.textContent =
            "Could not determine if Overwatch is running";
        gameStatusSvgEl.classList.remove("running", "not-running");
        return;
    }

    if (isRunning) {
        gameRunningStatusEl.textContent = "Overwatch is Running";
        gameStatusMessageEl.textContent =
            "You must close Overwatch before blocking new regions";
        gameStatusSvgEl.classList.add("running");
        gameStatusSvgEl.classList.remove("not-running");
    } else {
        gameRunningStatusEl.textContent = "Overwatch is Not Running";
        gameStatusMessageEl.textContent = "You can block or unblock any region";
        gameStatusSvgEl.classList.remove("running");
        gameStatusSvgEl.classList.add("not-running");
    }
}

// Toggle the waiting modal
function showWaitingModal(show) {
    if (show) {
        waitingModalEl.classList.add("active");
    } else {
        waitingModalEl.classList.remove("active");
    }
}

// Update a region's status display
function updateRegionStatus(region, text, className) {
    if (regionStatuses[region]) {
        regionStatuses[region].textContent = text;
        regionStatuses[region].className = "region-status";
        if (className) {
            regionStatuses[region].classList.add(className);
        }
    }
}

// Update the main status indicator
function updateStatus(message, state) {
    statusTextEl.textContent = message;

    // Update status dot
    statusDotEl.classList.remove("online", "error", "warning");
    if (state) {
        statusDotEl.classList.add(state);
    }
}

// Add a message to the console output
function addConsoleMessage(message, isError = false) {
    const timestamp = new Date().toLocaleTimeString();
    const prefix = isError ? "❌ ERROR" : "✓";

    const formattedMessage = `[${timestamp}] ${prefix}: ${message}`;
    consoleOutputEl.textContent +=
        (consoleOutputEl.textContent ? "\n" : "") + formattedMessage;

    // Scroll to bottom
    consoleOutputEl.scrollTop = consoleOutputEl.scrollHeight;
}

// Set the loading state of the UI
function setLoading(isLoading) {
    isProcessing = isLoading;

    // Update button states
    refreshBtnEl.disabled = isLoading;
    unblockAllBtnEl.disabled = isLoading;

    // Update region toggle states
    regions.forEach((region) => {
        if (regionToggles[region]) {
            regionToggles[region].disabled =
                isLoading ||
                (isOverwatchRunning && !blockedRegions.has(region));
        }
    });
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
