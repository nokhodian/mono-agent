/**
 * Monoes Agent Bridge — Popup Script
 *
 * Displays connection status and allows configuring the WebSocket URL.
 */

const dot = document.getElementById("dot");
const statusText = document.getElementById("status-text");
const wsUrlInput = document.getElementById("ws-url");
const saveBtn = document.getElementById("save-btn");
const savedMsg = document.getElementById("saved-msg");

const STATUS_LABELS = {
  connected: "Connected",
  disconnected: "Disconnected",
  connecting: "Connecting...",
};

function updateUI(status) {
  dot.className = `dot ${status}`;
  statusText.textContent = STATUS_LABELS[status] || status;
}

// Load current status and saved URL on popup open
async function init() {
  // Get connection status from background
  chrome.runtime.sendMessage({ type: "get_status" }, (response) => {
    if (response?.status) {
      updateUI(response.status);
    } else {
      updateUI("disconnected");
    }
  });

  // Load saved URL
  const result = await chrome.storage.local.get("wsUrl");
  wsUrlInput.value = result.wsUrl || "ws://127.0.0.1:9222/monoes";
}

// Listen for live status updates from background
chrome.runtime.onMessage.addListener((msg) => {
  if (msg.type === "status") {
    updateUI(msg.status);
  }
});

// Save button handler
saveBtn.addEventListener("click", () => {
  const url = wsUrlInput.value.trim();
  if (!url) return;

  chrome.runtime.sendMessage({ type: "set_ws_url", url }, () => {
    savedMsg.style.display = "block";
    updateUI("connecting");
    setTimeout(() => {
      savedMsg.style.display = "none";
    }, 2000);
  });
});

init();
