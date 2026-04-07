/**
 * Monoes Agent Bridge — Background Service Worker
 *
 * Connects to the Go backend via WebSocket and dispatches commands
 * to content scripts or the chrome.tabs / chrome.scripting APIs.
 *
 * MV3 Keep-Alive Strategy:
 * - chrome.alarms fires every ~24s to wake the service worker
 * - On each alarm, check WS connection and reconnect if needed
 * - Fast retry loop (500ms) runs during the first 30s after SW start
 * - No exponential backoff — flat 500ms retry for aggressive reconnection
 */

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

let ws = null;
let connectionStatus = "disconnected"; // "connected" | "disconnected" | "connecting"
let keepAliveInterval = null;

const KEEP_ALIVE_INTERVAL = 20000; // 20s ping to prevent WS idle timeout
const DEFAULT_WS_URL = "ws://127.0.0.1:9222/monoes";
const COMMAND_TIMEOUT = 30000; // 30s default timeout for pending commands
const KEEPALIVE_ALARM = "monoes-keepalive";
const ALARM_PERIOD_MINUTES = 0.4; // ~24 seconds (minimum safe value for MV3)

// Pending navigation completions: tabId -> {resolve, timeout}
const pendingNavigations = new Map();

// ---------------------------------------------------------------------------
// Keep-Alive: Alarm-based service worker persistence
// ---------------------------------------------------------------------------

function ensureAlarm() {
  chrome.alarms.create(KEEPALIVE_ALARM, { periodInMinutes: ALARM_PERIOD_MINUTES });
}

chrome.runtime.onInstalled.addListener(() => {
  console.log("[monoes] Extension installed, starting connection loop");
  ensureAlarm();
  connect();
  fastRetryConnect();
});

chrome.runtime.onStartup.addListener(() => {
  console.log("[monoes] Chrome started, starting connection loop");
  ensureAlarm();
  connect();
  fastRetryConnect();
});

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === KEEPALIVE_ALARM) {
    if (connectionStatus !== "connected") {
      console.log("[monoes] Alarm-triggered reconnect attempt");
      connect();
    }
    // Re-create the alarm to guarantee the service worker stays alive.
    ensureAlarm();
  }
});

// Fast retry for the first 30 seconds after service worker starts.
// setTimeout is reliable while the SW is active; the alarm takes over after.
let fastRetryCount = 0;
const FAST_RETRY_MAX = 60; // 60 * 500ms = 30 seconds
const FAST_RETRY_INTERVAL = 500;

function fastRetryConnect() {
  if (connectionStatus === "connected" || fastRetryCount >= FAST_RETRY_MAX) return;
  fastRetryCount++;
  connect();
  setTimeout(fastRetryConnect, FAST_RETRY_INTERVAL);
}

// ---------------------------------------------------------------------------
// WebSocket connection
// ---------------------------------------------------------------------------

async function getWsUrl() {
  try {
    const result = await chrome.storage.local.get("wsUrl");
    return result.wsUrl || DEFAULT_WS_URL;
  } catch {
    return DEFAULT_WS_URL;
  }
}

async function connect() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
    return;
  }

  connectionStatus = "connecting";
  broadcastStatus();

  const url = await getWsUrl();

  try {
    ws = new WebSocket(url);
  } catch (err) {
    console.error("[monoes] WebSocket constructor error:", err.message);
    connectionStatus = "disconnected";
    broadcastStatus();
    return;
  }

  ws.onopen = () => {
    connectionStatus = "connected";
    fastRetryCount = FAST_RETRY_MAX; // Stop fast retry — we're connected
    console.log("[monoes] Connected to backend at", url);
    broadcastStatus();
    startKeepAlive();
  };

  ws.onmessage = (event) => {
    let cmd;
    try {
      cmd = JSON.parse(event.data);
    } catch (err) {
      console.error("[monoes] Invalid JSON from backend:", err.message);
      return;
    }
    // Ignore pong responses
    if (cmd.type === "pong") return;
    handleCommand(cmd);
  };

  ws.onerror = (err) => {
    console.error("[monoes] WebSocket error:", err);
  };

  ws.onclose = () => {
    connectionStatus = "disconnected";
    broadcastStatus();
    stopKeepAlive();
    // Don't schedule reconnect via setTimeout — the alarm handles it.
    // But do restart fast retry if we disconnected unexpectedly early.
    if (fastRetryCount < FAST_RETRY_MAX) {
      fastRetryConnect();
    }
  };
}

function startKeepAlive() {
  stopKeepAlive();
  keepAliveInterval = setInterval(() => {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: "ping" }));
    }
  }, KEEP_ALIVE_INTERVAL);
}

function stopKeepAlive() {
  if (keepAliveInterval) {
    clearInterval(keepAliveInterval);
    keepAliveInterval = null;
  }
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

function sendResponse(id, success, data, error) {
  if (ws?.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ id, success, data, error: error || undefined }));
  }
}

function broadcastStatus() {
  chrome.runtime.sendMessage({ type: "status", status: connectionStatus }).catch(() => {
    // popup not open — ignore
  });
}

// ---------------------------------------------------------------------------
// Command dispatch
// ---------------------------------------------------------------------------

async function handleCommand(cmd) {
  const id = cmd.id;
  // Merge tabId into params so all handlers can access it uniformly.
  const params = { ...cmd.params, tabId: cmd.tabId || cmd.params?.tabId };
  try {
    let result;
    switch (cmd.type) {
      case "create_tab":
        result = await createTab(params);
        break;
      case "navigate":
        result = await navigateTab(params);
        break;
      case "reload":
        result = await reloadTab(params);
        break;
      case "page_info":
        result = await pageInfo(params);
        break;
      case "eval":
        result = await evalInTab(params);
        break;
      case "wait_load":
        result = await waitForLoad(params);
        break;
      // All DOM operations are forwarded to the content script
      case "element":
      case "elements":
      case "has":
      case "click":
      case "input":
      case "text":
      case "attribute":
      case "scroll":
      case "keyboard_type":
      case "keyboard_press":
      case "wait_element":
      case "race":
      case "focus":
      case "html":
      case "property":
      case "scroll_into_view":
      case "insert_text":
        result = await sendToContent(params.tabId, { ...cmd, params });
        break;
      case "type_cdp":
        result = await typeCDP(params);
        break;
      case "eval_cdp":
        result = await evalCDP(params);
        break;
      case "get_rect":
      case "set_files":
      case "query_count":
      case "query_text":
      case "fetch_image_base64":
        result = await sendToContent(params.tabId, { ...cmd, params });
        break;
      default:
        throw new Error(`Unknown command type: ${cmd.type}`);
    }
    sendResponse(id, true, result);
  } catch (err) {
    sendResponse(id, false, null, err.message);
  }
}

// ---------------------------------------------------------------------------
// Tab operations
// ---------------------------------------------------------------------------

async function createTab({ url, active = true }) {
  const tab = await chrome.tabs.create({ url, active });
  // Wait for the tab to finish loading
  await waitForTabComplete(tab.id);
  return { tabId: tab.id, url: tab.url };
}

async function navigateTab({ tabId, url }) {
  if (!tabId) throw new Error("tabId is required");
  if (!url) throw new Error("url is required");
  await chrome.tabs.update(tabId, { url });
  await waitForTabComplete(tabId);
  const tab = await chrome.tabs.get(tabId);
  return { tabId: tab.id, url: tab.url };
}

async function reloadTab({ tabId }) {
  if (!tabId) throw new Error("tabId is required");
  await chrome.tabs.reload(tabId);
  await waitForTabComplete(tabId);
  return { tabId };
}

async function pageInfo({ tabId }) {
  if (!tabId) throw new Error("tabId is required");
  const tab = await chrome.tabs.get(tabId);
  return { tabId: tab.id, url: tab.url, title: tab.title, status: tab.status };
}

/**
 * Wait for a tab to reach "complete" loading status.
 * Uses chrome.tabs.onUpdated listener with a timeout.
 */
function waitForTabComplete(tabId, timeout = 30000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      chrome.tabs.onUpdated.removeListener(listener);
      pendingNavigations.delete(tabId);
      reject(new Error(`Tab ${tabId} did not finish loading within ${timeout}ms`));
    }, timeout);

    function listener(updatedTabId, changeInfo) {
      if (updatedTabId === tabId && changeInfo.status === "complete") {
        chrome.tabs.onUpdated.removeListener(listener);
        clearTimeout(timer);
        pendingNavigations.delete(tabId);
        resolve();
      }
    }

    pendingNavigations.set(tabId, { resolve, timeout: timer });
    chrome.tabs.onUpdated.addListener(listener);
  });
}

async function waitForLoad({ tabId, timeout = 30000 }) {
  if (!tabId) throw new Error("tabId is required");
  const tab = await chrome.tabs.get(tabId);
  if (tab.status === "complete") {
    return { tabId, status: "complete" };
  }
  await waitForTabComplete(tabId, timeout);
  return { tabId, status: "complete" };
}

// ---------------------------------------------------------------------------
// Eval in tab (via chrome.scripting)
// ---------------------------------------------------------------------------

async function evalInTab({ tabId, js, expression, args }) {
  const code = js || expression;
  if (!tabId) throw new Error("tabId is required");
  if (!code) throw new Error("js code is required");

  // chrome.scripting.executeScript with world:"MAIN" bypasses CSP.
  // We can't use eval() inside func, so we pass code+args as strings
  // and construct the evaluation via Function constructor (which is also
  // an extension privilege in MAIN world injection).
  const argsJSON = JSON.stringify(args || []);

  // Wrap executeScript in a timeout — it can hang on some pages
  const execPromise = chrome.scripting.executeScript({
    target: { tabId },
    world: "MAIN",
    args: [code, argsJSON],
    func: (codeStr, argsStr) => {
      try {
        const argsParsed = JSON.parse(argsStr);
        const fn = new Function('return (' + codeStr + ')')();
        if (typeof fn === 'function') {
          return fn(...argsParsed);
        }
        return fn;
      } catch(e) {
        return { __monoes_error: e.message };
      }
    }
  });

  const timeoutPromise = new Promise((_, reject) =>
    setTimeout(() => reject(new Error("executeScript timeout (10s)")), 10000)
  );

  let results;
  try {
    results = await Promise.race([execPromise, timeoutPromise]);
  } catch(e) {
    console.error("[monoes] evalInTab failed:", e.message);
    return null;
  }

  if (!results || results.length === 0) return null;
  const result = results[0]?.result;
  if (result && result.__monoes_error) {
    throw new Error("Eval: " + result.__monoes_error);
  }
  return result;
}

// ---------------------------------------------------------------------------
// Content script messaging
// ---------------------------------------------------------------------------

// Type text using Chrome Debugger Protocol (Input.dispatchKeyEvent).
// This produces real browser-level keyboard events that work with any
// framework (React, Lexical, Quill, etc.) — unlike synthetic JS events.
const debuggerAttached = new Set();

async function typeCDP({ tabId, text, elementId, tabCount }) {
  if (!tabId) throw new Error("tabId required");
  if (!text) throw new Error("text required");

  const target = { tabId };

  // Attach debugger if needed
  if (!debuggerAttached.has(tabId)) {
    try {
      await chrome.debugger.attach(target, "1.3");
      debuggerAttached.add(tabId);
    } catch (e) {
      if (!e.message.includes("Already attached")) {
        throw new Error("debugger attach: " + e.message);
      }
      debuggerAttached.add(tabId);
    }
  }

  // Strategy: use CDP to find the contenteditable element, focus it via
  // DOM.focus, then insert text via Input.insertText.

  // Step 1: Find the caption element via Runtime.evaluate (not blocked by CSP
  // because chrome.debugger bypasses it entirely)
  try {
    const findResult = await chrome.debugger.sendCommand(target, "Runtime.evaluate", {
      expression: `(() => {
        // Find contenteditable caption field
        const candidates = document.querySelectorAll('[contenteditable="true"]');
        for (const el of candidates) {
          const rect = el.getBoundingClientRect();
          // Caption field is visible and reasonably sized
          if (rect.width > 100 && rect.height > 30 && rect.top > 0) {
            el.focus();
            el.click();
            return { found: true, tag: el.tagName, w: rect.width, h: rect.height };
          }
        }
        // Fallback: try role=textbox
        const tb = document.querySelector('[role="textbox"]');
        if (tb) { tb.focus(); tb.click(); return { found: true, tag: 'textbox' }; }
        return { found: false };
      })()`,
      returnByValue: true,
      awaitPromise: false,
    });

    if (findResult?.result?.value?.found) {
      await new Promise(r => setTimeout(r, 300));
    }
  } catch(e) {
    // If Runtime.evaluate fails, try clicking via coordinates
    if (elementId) {
      try {
        const rect = await new Promise((resolve, reject) => {
          const timeout = setTimeout(() => reject(new Error("timeout")), 5000);
          chrome.tabs.sendMessage(tabId, { type: "get_rect", params: { elementId } }, (r) => {
            clearTimeout(timeout);
            resolve(r || {});
          });
        });
        if (rect.x !== undefined) {
          const x = rect.x + rect.width / 2;
          const y = rect.y + rect.height / 2;
          await chrome.debugger.sendCommand(target, "Input.dispatchMouseEvent", {
            type: "mousePressed", x, y, button: "left", clickCount: 1
          });
          await chrome.debugger.sendCommand(target, "Input.dispatchMouseEvent", {
            type: "mouseReleased", x, y, button: "left", clickCount: 1
          });
          await new Promise(r => setTimeout(r, 300));
        }
      } catch(e2) {}
    }
  }

  // Step 2: Insert text via CDP
  await chrome.debugger.sendCommand(target, "Input.insertText", {
    text: text,
  });

  return { typed: true, length: text.length };
}

// Clean up debugger on tab close
chrome.tabs.onRemoved.addListener((tabId) => {
  debuggerAttached.delete(tabId);
});

// Evaluate JS via CDP Runtime.evaluate — bypasses CSP completely.
async function evalCDP({ tabId, expression }) {
  if (!tabId) throw new Error("tabId required");
  if (!expression) throw new Error("expression required");

  const target = { tabId };
  if (!debuggerAttached.has(tabId)) {
    try {
      await chrome.debugger.attach(target, "1.3");
      debuggerAttached.add(tabId);
    } catch (e) {
      if (!e.message.includes("Already attached")) throw new Error("debugger attach: " + e.message);
      debuggerAttached.add(tabId);
    }
  }

  const result = await chrome.debugger.sendCommand(target, "Runtime.evaluate", {
    expression,
    returnByValue: true,
    awaitPromise: true,
  });

  if (result.exceptionDetails) {
    throw new Error("eval_cdp: " + (result.exceptionDetails.text || result.exceptionDetails.exception?.description || "unknown error"));
  }

  return { result: result.result?.value ?? null };
}

async function sendToContent(tabId, cmd) {
  if (!tabId) throw new Error("tabId is required for DOM operations");

  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error(`Content script timeout for command ${cmd.type} on tab ${tabId}`));
    }, cmd.params?.timeout || COMMAND_TIMEOUT);

    chrome.tabs.sendMessage(tabId, cmd, (response) => {
      clearTimeout(timeout);
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }
      if (response && response.error) {
        reject(new Error(response.error));
        return;
      }
      resolve(response);
    });
  });
}

// ---------------------------------------------------------------------------
// Message handler for popup and internal communication
// ---------------------------------------------------------------------------

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg.type === "get_status") {
    sendResponse({ status: connectionStatus });
    return false;
  }
  // Handle file read requests from content script (for file upload)
  if (msg.type === "read_file") {
    // The Go server needs to send us the file. We'll request it via WS.
    // For now, if the path is accessible via fetch (unlikely for local files),
    // we return an error and let the Go side handle file upload differently.
    sendResponse({ error: "Local file access not supported from extension. Use the Go server to read files." });
    return false;
  }
  if (msg.type === "set_ws_url") {
    chrome.storage.local.set({ wsUrl: msg.url }).then(() => {
      // Disconnect and reconnect with new URL
      if (ws) {
        ws.close();
      }
      // Reset fast retry so we aggressively connect to the new URL
      fastRetryCount = 0;
      connect();
      fastRetryConnect();
      sendResponse({ ok: true });
    });
    return true; // async response
  }
  return false;
});

// ---------------------------------------------------------------------------
// Initialization — runs every time the service worker starts
// ---------------------------------------------------------------------------

ensureAlarm();
connect();
fastRetryConnect();
