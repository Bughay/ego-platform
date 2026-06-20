/* ============================================================
   EGOAI chat — front-end (look & feel phase)
   Vanilla JS. No build step. Depends on CDN: marked, highlight.js, DOMPurify.
   ============================================================ */

/* --- Configuration (edit these) --- */
// Single backend for the whole Ego platform (served by backend/cmd/server).
// API_BASE comes from the shared client (frontend/shared/api.js). The /ai/chat
// endpoint is a SCAFFOLD on the backend — implement internal/ego_ai_studio, then
// flip DEMO_MODE to false.
const backendEndpoint = (window.API ? API.API_BASE : "http://localhost:8080") + "/ai/chat";
const DEMO_MODE = false;    // live: talks to the /ai/chat backend

/* --- Conversation state: array of { role, content } (for display) --- */
let messages = [];
let titleSet = false;  // page title is set from the first user message
let currentChatId = 0; // 0 = new chat; set from the backend response, sent back to continue

/* --- DOM references --- */
const messagesEl = document.getElementById("messages");
const inputEl     = document.getElementById("input");
const sendBtn     = document.getElementById("sendBtn");
const newChatBtn  = document.getElementById("newChatBtn");

/* --- Sidebar DOM references --- */
const sidebarEl      = document.getElementById("sidebar");
const sidebarToggle  = document.getElementById("sidebarToggle");
const modelEl        = document.getElementById("modelSelect");
const thinkingEl     = document.getElementById("thinkingToggle");
const sysPromptEl    = document.getElementById("sysPrompt");
const sysPromptCount = document.getElementById("sysPromptCount");
const tempEl         = document.getElementById("temp");
const tempValEl      = document.getElementById("tempVal");
const maxTokEl       = document.getElementById("maxTok");
const maxTokValEl    = document.getElementById("maxTokVal");
const resetBtn       = document.getElementById("resetBtn");
const historyListEl  = document.getElementById("historyList");

/* --- localStorage keys + control defaults --- */
const LS = {
  sidebarOpen: "egoai.sidebarOpen",
  systemPrompt: "egoai.systemPrompt",
  temperature: "egoai.temperature",
  maxTokens: "egoai.maxTokens",
  model: "egoai.model",
  thinking: "egoai.thinking",
};
const DEFAULTS = { temperature: "0.5", maxTokens: "2048", model: "deepseek-pro", thinking: "false" };

let msgSeq = 0; // monotonic id for message elements (for history scroll-to)

/* ============================================================
   Markdown rendering (marked + highlight.js) + sanitization (DOMPurify)
   ============================================================ */
if (window.marked) {
  marked.setOptions({ breaks: true });
}

// Render assistant markdown to a SANITIZED HTML string (required before DOM insert).
function renderMarkdown(text) {
  const rawHtml = window.marked ? marked.parse(text) : text;
  return window.DOMPurify ? DOMPurify.sanitize(rawHtml) : rawHtml;
}

// Highlight code blocks after they're in the DOM (version-independent).
function highlightCode(container) {
  if (!window.hljs) return;
  container.querySelectorAll("pre code").forEach((block) => {
    try {
      hljs.highlightElement(block);
    } catch (_) {
      /* leave block unhighlighted on error */
    }
  });
}

/* ============================================================
   Message rendering
   ============================================================ */
function renderMessage(role, content) {
  const wrap = document.createElement("div");
  wrap.className = "msg " + role;
  wrap.id = "msg-" + msgSeq++;

  if (role === "user") {
    // User messages: plain text only.
    wrap.textContent = content;
  } else {
    // Assistant messages: sanitized markdown + copy button.
    const label = document.createElement("div");
    label.className = "msg-label";
    label.textContent = "EGOAI";
    wrap.appendChild(label);

    const body = document.createElement("div");
    body.className = "markdown";
    body.innerHTML = renderMarkdown(content);
    highlightCode(body);
    wrap.appendChild(body);

    wrap.appendChild(buildCopyButton(content));
  }

  messagesEl.appendChild(wrap);
  return wrap;
}

// Copy button that copies the message's plain text and flashes "Copied!".
function buildCopyButton(plainText) {
  const actions = document.createElement("div");
  actions.className = "msg-actions";

  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "copy-btn";
  btn.textContent = "Copy";
  btn.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(plainText);
    } catch (_) {
      // Fallback for browsers without clipboard API / non-secure contexts.
      const ta = document.createElement("textarea");
      ta.value = plainText;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    btn.classList.add("copied");
    setTimeout(() => btn.classList.remove("copied"), 1200);
  });

  actions.appendChild(btn);
  return actions;
}

// Re-render the whole conversation from state. The sidebar chat list is independent
// of the message pane, so it is not touched here.
function renderAll() {
  messagesEl.innerHTML = "";
  msgSeq = 0;
  messages.forEach((m) => renderMessage(m.role, m.content));
  scrollToBottom(true);
}

/* ============================================================
   Chat history (persisted) — GET /ai/chats lists the user's chats,
   GET /ai/chats/{id}/messages reopens one in the message pane.
   ============================================================ */
let chats = []; // [{ chatId, title, createdAt, updatedAt }]

// Compact "time ago" label for a chat's last activity.
function relativeTime(iso) {
  const then = new Date(iso).getTime();
  if (isNaN(then)) return "";
  const secs = Math.max(0, Math.floor((Date.now() - then) / 1000));
  if (secs < 60) return "just now";
  const mins = Math.floor(secs / 60);
  if (mins < 60) return mins + "m ago";
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return hrs + "h ago";
  const days = Math.floor(hrs / 24);
  if (days < 7) return days + "d ago";
  return new Date(iso).toLocaleDateString();
}

function setHistoryEmpty(text) {
  historyListEl.innerHTML = '<div class="history-empty">' + text + "</div>";
}

// Render the sidebar list from the `chats` state, highlighting the open chat.
function renderChatList() {
  if (!chats || chats.length === 0) {
    setHistoryEmpty("No chats yet");
    return;
  }
  historyListEl.innerHTML = "";
  chats.forEach((c) => {
    const item = document.createElement("div");
    item.className = "history-item" + (c.chatId === currentChatId ? " active" : "");
    item.dataset.chatId = c.chatId;

    const titleText = c.title && c.title.trim() ? c.title : "(untitled)";
    item.title = titleText;

    const titleEl = document.createElement("span");
    titleEl.className = "history-title";
    titleEl.textContent = titleText;

    const timeEl = document.createElement("span");
    timeEl.className = "history-time";
    timeEl.textContent = relativeTime(c.updatedAt);

    const delBtn = document.createElement("button");
    delBtn.type = "button";
    delBtn.className = "history-delete";
    delBtn.textContent = "×";
    delBtn.title = "Delete chat";
    delBtn.addEventListener("click", (e) => {
      e.stopPropagation(); // don't open the chat when deleting it
      deleteChat(c.chatId);
    });

    item.appendChild(titleEl);
    item.appendChild(timeEl);
    item.appendChild(delBtn);
    item.addEventListener("click", () => openChat(c.chatId));
    historyListEl.appendChild(item);
  });
}

// Delete a chat (and its messages) immediately, then drop it from the sidebar.
async function deleteChat(chatId) {
  try {
    await API.apiFetch("/ai/chats/" + encodeURIComponent(chatId), { method: "DELETE" });
  } catch (err) {
    showError(
      "Could not delete chat: " +
        (err && err.message ? err.message : "unknown error")
    );
    return;
  }
  chats = chats.filter((c) => c.chatId !== chatId);
  if (chatId === currentChatId) newChat(); // the open chat was deleted → reset the pane
  renderChatList();
}

// Fetch the user's chats and render them in the sidebar.
async function loadChats() {
  if (!window.API) return;
  try {
    chats = await API.apiFetch("/ai/chats");
    renderChatList();
  } catch (err) {
    setHistoryEmpty("Could not load chats");
  }
}

// Highlight the active chat without re-fetching the list.
function setActiveChat(chatId) {
  historyListEl.querySelectorAll(".history-item").forEach((el) => {
    el.classList.toggle("active", Number(el.dataset.chatId) === chatId);
  });
}

// Open a chat: load its full conversation and render it in the message pane.
async function openChat(chatId) {
  if (chatId === currentChatId && messages.length) return; // already open
  try {
    const data = await API.apiFetch(
      "/ai/chats/" + encodeURIComponent(chatId) + "/messages"
    );
    messages = (data || []).map((m) => ({ role: m.role, content: m.content }));
    currentChatId = chatId; // continue this chat on the next turn
    titleSet = true;
    renderAll();
    setActiveChat(chatId);
  } catch (err) {
    showError(
      "Could not load chat: " +
        (err && err.message ? err.message : "unknown error")
    );
  }
}

/* ============================================================
   Typing indicator (shown in place of the pending assistant reply)
   ============================================================ */
function showTyping() {
  const el = document.createElement("div");
  el.className = "msg assistant typing-row";
  el.id = "typingIndicator";
  el.innerHTML =
    '<div class="typing">EGOAI is preparing the answer' +
    '<span class="dots"><span></span><span></span><span></span></span></div>';
  messagesEl.appendChild(el);
  scrollToBottom();
}

function hideTyping() {
  const el = document.getElementById("typingIndicator");
  if (el) el.remove();
}

/* ============================================================
   Auto-scroll (pause when the user scrolls up)
   ============================================================ */
let isAtBottom = true;
const BOTTOM_THRESHOLD = 60; // px tolerance

messagesEl.addEventListener("scroll", () => {
  const distance =
    messagesEl.scrollHeight - messagesEl.scrollTop - messagesEl.clientHeight;
  isAtBottom = distance <= BOTTOM_THRESHOLD;
});

// Scroll to bottom only if the user is already near the bottom (unless forced).
function scrollToBottom(force = false) {
  if (force || isAtBottom) {
    messagesEl.scrollTop = messagesEl.scrollHeight;
    isAtBottom = true;
  }
}

/* ============================================================
   Inline error (shown instead of crashing)
   ============================================================ */
function showError(text) {
  const el = document.createElement("div");
  el.className = "error-msg";
  el.textContent = text;
  messagesEl.appendChild(el);
  scrollToBottom();
}

/* ============================================================
   Network logic — keep request/response shaping in one place.
   ============================================================ */

// POST { chatId, systemPrompt, userPrompt, temperature, maxTokens } for one turn.
// The backend keeps the conversation in the DB (memory), so we send only the new
// message plus the current chat id. Parses { chatId, content }.
async function callBackend(userPrompt) {
  if (!backendEndpoint) {
    throw new Error("No backend endpoint configured.");
  }

  const body = {
    chatId: currentChatId,
    systemPrompt: sysPromptEl.value.trim(),
    userPrompt: userPrompt,
    model: modelEl.value,
    thinking: isThinkingOn(),
    temperature: parseFloat(tempEl.value),
    maxTokens: parseInt(maxTokEl.value, 10),
  };

  // Attach the shared JWT so the backend's auth middleware accepts the request.
  const headers = { "Content-Type": "application/json" };
  const token = window.API ? API.getToken() : "";
  if (token) headers["Authorization"] = "Bearer " + token;

  const response = await fetch(backendEndpoint, {
    method: "POST",
    headers: headers,
    credentials: "include",
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error("Backend returned status " + response.status);
  }

  const data = await response.json();
  if (data.chatId) currentChatId = data.chatId; // continue this chat on the next turn
  return data.content; // expected response shape: { "chatId": N, "content": "..." }
}

// Orchestrates one user turn: render user msg, show typing, fetch, render reply.
async function sendMessage() {
  const text = inputEl.value.trim();
  if (!text) return; // block empty / whitespace-only messages

  // Add and render the user message.
  messages.push({ role: "user", content: text });
  renderMessage("user", text);

  // Set the page title from the first user message.
  if (!titleSet) {
    document.title = text.slice(0, 60) + (text.length > 60 ? "…" : "");
    titleSet = true;
  }

  // Reset the input.
  inputEl.value = "";
  autoGrow();
  scrollToBottom(true);

  // Show the "preparing" indicator while we wait.
  showTyping();

  try {
    const reply = await callBackend(text);
    hideTyping();
    if (reply == null || reply === "") {
      showError("EGOAI returned an empty response.");
      return;
    }
    messages.push({ role: "assistant", content: reply });
    renderMessage("assistant", reply);
    scrollToBottom();
    // Refresh the sidebar so a newly created chat appears (with its title) and the
    // just-used chat moves to the top / stays highlighted.
    loadChats();
  } catch (err) {
    hideTyping();
    showError(
      "Could not reach EGOAI: " +
        (err && err.message ? err.message : "unknown error") +
        ". Check the backendEndpoint in script.js."
    );
  }
}

/* ============================================================
   Input behaviour: auto-grow + Enter/Shift+Enter
   ============================================================ */
function autoGrow() {
  inputEl.style.height = "auto";
  inputEl.style.height = inputEl.scrollHeight + "px";
}

inputEl.addEventListener("input", autoGrow);

inputEl.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault(); // Enter sends; Shift+Enter inserts a newline.
    sendMessage();
  }
});

sendBtn.addEventListener("click", sendMessage);

/* ============================================================
   New chat — start a fresh conversation (keeps sidebar settings)
   ============================================================ */
function newChat() {
  messages = [];
  titleSet = false;
  currentChatId = 0; // next send creates a fresh chat row in the DB
  msgSeq = 0;
  document.title = "EGOAI";
  messagesEl.innerHTML = "";
  setActiveChat(0); // keep the chat list; the new chat exists only after the first send
  inputEl.value = "";
  autoGrow();
  inputEl.focus();
}

newChatBtn.addEventListener("click", newChat);

/* ============================================================
   Demo content (so styling is visible without a backend)
   ============================================================ */
function loadDemo() {
  messages = [
    {
      role: "user",
      content: "Show me what your formatting looks like.",
    },
    {
      role: "assistant",
      content: [
        "## Welcome to EGOAI",
        "",
        "Here's a quick tour of the **rendering**. It supports *italics*, **bold**, and `inline code` like `npm run dev`.",
        "",
        "### Features",
        "- Markdown rendered with `marked`",
        "- Code highlighted via highlight.js",
        "- Output sanitized with DOMPurify",
        "",
        "A fenced code block:",
        "",
        "```js",
        "async function callBackend(messages) {",
        "  const res = await fetch(endpoint, {",
        "    method: 'POST',",
        "    headers: { 'Content-Type': 'application/json' },",
        "    body: JSON.stringify({ messages }),",
        "  });",
        "  return (await res.json()).content;",
        "}",
        "```",
        "",
        "> Paste your endpoint into `backendEndpoint` to go live.",
      ].join("\n"),
    },
  ];
  renderAll();
}

/* ============================================================
   Sidebar: controls, persistence (localStorage), toggle
   ============================================================ */

// Auto-resize the system-prompt textarea to fit its content.
function autoGrowSysPrompt() {
  sysPromptEl.style.height = "auto";
  sysPromptEl.style.height = sysPromptEl.scrollHeight + "px";
}

// Keep the character counter in sync with the system prompt.
function updateSysPromptCount() {
  sysPromptCount.textContent = sysPromptEl.value.length + " chars";
}

// Reflect the slider values next to their labels.
function updateTempLabel() {
  tempValEl.textContent = parseFloat(tempEl.value).toFixed(1);
}
function updateMaxTokLabel() {
  maxTokValEl.textContent = maxTokEl.value;
}

// Reflect the thinking-mode toggle's on/off state in the button.
function setThinking(on) {
  thinkingEl.setAttribute("aria-pressed", String(on));
  thinkingEl.querySelector(".thinking-btn__state").textContent = on ? "ON" : "OFF";
}
function isThinkingOn() {
  return thinkingEl.getAttribute("aria-pressed") === "true";
}

// Open/close the sidebar and persist the state.
function setSidebar(open) {
  sidebarEl.classList.toggle("collapsed", !open);
  sidebarToggle.classList.toggle("open", open);
  localStorage.setItem(LS.sidebarOpen, open ? "1" : "0");
}

// Restore persisted control values + sidebar state on load.
function initSidebar() {
  // System prompt
  const savedPrompt = localStorage.getItem(LS.systemPrompt);
  if (savedPrompt !== null) sysPromptEl.value = savedPrompt;
  updateSysPromptCount();
  autoGrowSysPrompt();

  // Sliders
  tempEl.value = localStorage.getItem(LS.temperature) ?? DEFAULTS.temperature;
  maxTokEl.value = localStorage.getItem(LS.maxTokens) ?? DEFAULTS.maxTokens;
  updateTempLabel();
  updateMaxTokLabel();

  // Model
  modelEl.value = localStorage.getItem(LS.model) ?? DEFAULTS.model;

  // Thinking mode (DeepSeek reasoning) — default off
  setThinking((localStorage.getItem(LS.thinking) ?? DEFAULTS.thinking) === "true");

  // Toggle state — default closed on first load (absent key → collapsed)
  setSidebar(localStorage.getItem(LS.sidebarOpen) === "1");

  // Listeners
  sidebarToggle.addEventListener("click", () => {
    setSidebar(sidebarEl.classList.contains("collapsed"));
  });

  sysPromptEl.addEventListener("input", () => {
    updateSysPromptCount();
    autoGrowSysPrompt();
    localStorage.setItem(LS.systemPrompt, sysPromptEl.value);
  });

  tempEl.addEventListener("input", () => {
    updateTempLabel();
    localStorage.setItem(LS.temperature, tempEl.value);
  });

  maxTokEl.addEventListener("input", () => {
    updateMaxTokLabel();
    localStorage.setItem(LS.maxTokens, maxTokEl.value);
  });

  modelEl.addEventListener("change", () => {
    localStorage.setItem(LS.model, modelEl.value);
  });

  thinkingEl.addEventListener("click", () => {
    const on = !isThinkingOn();
    setThinking(on);
    localStorage.setItem(LS.thinking, String(on));
  });

  // Reset: restore defaults, clear prompt, drop the saved control keys.
  resetBtn.addEventListener("click", () => {
    sysPromptEl.value = "";
    tempEl.value = DEFAULTS.temperature;
    maxTokEl.value = DEFAULTS.maxTokens;
    modelEl.value = DEFAULTS.model;
    setThinking(DEFAULTS.thinking === "true");
    updateSysPromptCount();
    autoGrowSysPrompt();
    updateTempLabel();
    updateMaxTokLabel();
    localStorage.removeItem(LS.systemPrompt);
    localStorage.removeItem(LS.temperature);
    localStorage.removeItem(LS.maxTokens);
    localStorage.removeItem(LS.model);
    localStorage.removeItem(LS.thinking);
  });
}

/* ============================================================
   Init
   ============================================================ */
// Single-auth guard: bounce to the shared login if there's no token.
if (window.API) {
  API.requireAuth();
  const logoutBtn = document.getElementById("logoutBtn");
  if (logoutBtn) logoutBtn.addEventListener("click", function () { API.logout(); });
}
initSidebar();
if (DEMO_MODE) {
  loadDemo();
} else {
  loadChats(); // populate the sidebar with the user's persisted chats
}
autoGrow();
inputEl.focus();

/* ============================================================
   Animated glowing-orb background (cyberpunk atmosphere)
   ============================================================ */
(function initBackground() {
  const canvas = document.getElementById("bg");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");

  const orbs = [];
  const COLORS = ["0, 245, 255", "255, 0, 255"]; // cyan, magenta

  function resize() {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
  }
  window.addEventListener("resize", resize);
  resize();

  // Seed a handful of slow-drifting orbs.
  for (let i = 0; i < 6; i++) {
    orbs.push({
      x: Math.random() * canvas.width,
      y: Math.random() * canvas.height,
      r: 120 + Math.random() * 180,
      dx: (Math.random() - 0.5) * 0.3,
      dy: (Math.random() - 0.5) * 0.3,
      color: COLORS[i % COLORS.length],
    });
  }

  function tick() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    for (const o of orbs) {
      o.x += o.dx;
      o.y += o.dy;
      if (o.x < -o.r) o.x = canvas.width + o.r;
      if (o.x > canvas.width + o.r) o.x = -o.r;
      if (o.y < -o.r) o.y = canvas.height + o.r;
      if (o.y > canvas.height + o.r) o.y = -o.r;

      const g = ctx.createRadialGradient(o.x, o.y, 0, o.x, o.y, o.r);
      g.addColorStop(0, "rgba(" + o.color + ", 0.10)");
      g.addColorStop(1, "rgba(" + o.color + ", 0)");
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.arc(o.x, o.y, o.r, 0, Math.PI * 2);
      ctx.fill();
    }
    requestAnimationFrame(tick);
  }
  tick();
})();
