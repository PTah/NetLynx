const state = {
  baseURL: "http://127.0.0.1:8080",
  token: "",
  selectedDeviceId: null,
  devices: [],
  lastEvents: [],
};
const STORAGE_KEY = "invetor_desktop_state_v1";

const els = {
  status: document.getElementById("status"),
  msg: document.getElementById("message"),
  baseURL: document.getElementById("base-url"),
  username: document.getElementById("username"),
  password: document.getElementById("password"),
  loginForm: document.getElementById("login-form"),
  logoutBtn: document.getElementById("logout-btn"),
  clearLocalBtn: document.getElementById("clear-local-btn"),
  reloadBtn: document.getElementById("reload-btn"),
  devicesBody: document.querySelector("#devices-table tbody"),
  eventsBody: document.querySelector("#events-table tbody"),
  detailTitle: document.getElementById("detail-title"),
  ifacesBody: document.querySelector("#ifaces-table tbody"),
  deviceEventsBody: document.querySelector("#device-events-table tbody"),
  createForm: document.getElementById("create-device-form"),
  newName: document.getElementById("new-device-name"),
  newHost: document.getElementById("new-device-host"),
  newSNMP: document.getElementById("new-device-snmp"),
  newCommunity: document.getElementById("new-device-community"),
  newV3User: document.getElementById("new-device-v3-user"),
  newV3AuthProto: document.getElementById("new-device-v3-auth-proto"),
  newV3AuthPass: document.getElementById("new-device-v3-auth-pass"),
  newV3PrivProto: document.getElementById("new-device-v3-priv-proto"),
  newV3PrivPass: document.getElementById("new-device-v3-priv-pass"),
  newPoll: document.getElementById("new-device-poll"),
  updateDeviceBtn: document.getElementById("update-device-btn"),
  deleteDeviceBtn: document.getElementById("delete-device-btn"),
  tabButtons: Array.from(document.querySelectorAll(".tab-btn")),
  tabPanels: Array.from(document.querySelectorAll(".tab-panel")),
  eventsFilterType: document.getElementById("events-filter-type"),
  eventsFilterSeverity: document.getElementById("events-filter-severity"),
  eventsApplyFilterBtn: document.getElementById("events-apply-filter-btn"),
  eventsExportCsvBtn: document.getElementById("events-export-csv-btn"),
  settingsNotifForm: document.getElementById("settings-notif-form"),
  setWhEn: document.getElementById("set-wh-en"),
  setWhUrl: document.getElementById("set-wh-url"),
  setTgEn: document.getElementById("set-tg-en"),
  setTgTok: document.getElementById("set-tg-tok"),
  setTgChat: document.getElementById("set-tg-chat"),
  setIncidentEn: document.getElementById("set-incident-en"),
  setIncidentTypes: document.getElementById("set-incident-types"),
  settingsMsg: document.getElementById("settings-msg"),
};

function setMessage(text, isError = false) {
  els.msg.style.color = isError ? "#ff9f9f" : "#8fd38f";
  els.msg.textContent = text;
}

function setStatus(text) {
  els.status.textContent = text;
}

function saveState() {
  try {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        baseURL: state.baseURL,
        selectedDeviceId: state.selectedDeviceId,
        username: els.username.value || "",
        eventsType: els.eventsFilterType.value || "",
        eventsSeverity: els.eventsFilterSeverity.value || "",
      }),
    );
  } catch {
    // ignore
  }
}

function loadState() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    const s = JSON.parse(raw);
    if (s && typeof s.baseURL === "string") {
      state.baseURL = s.baseURL;
      els.baseURL.value = s.baseURL;
    }
    if (s && typeof s.selectedDeviceId === "number") {
      state.selectedDeviceId = s.selectedDeviceId;
    }
    if (s && typeof s.username === "string") {
      els.username.value = s.username;
    }
    if (s && typeof s.eventsType === "string") {
      els.eventsFilterType.value = s.eventsType;
    }
    if (s && typeof s.eventsSeverity === "string") {
      els.eventsFilterSeverity.value = s.eventsSeverity;
    }
  } catch {
    // ignore
  }
}

async function persistSecureToken(token) {
  try {
    if (window.invetorDesktop && typeof window.invetorDesktop.secureTokenSet === "function") {
      await window.invetorDesktop.secureTokenSet(token || "");
    }
  } catch {
    // ignore
  }
}

async function loadSecureToken() {
  try {
    if (window.invetorDesktop && typeof window.invetorDesktop.secureTokenGet === "function") {
      const tok = await window.invetorDesktop.secureTokenGet();
      if (typeof tok === "string" && tok.trim() !== "") {
        state.token = tok.trim();
      }
    }
  } catch {
    // ignore
  }
}

function switchTab(name) {
  for (const b of els.tabButtons) {
    b.classList.toggle("active", b.dataset.tab === name);
  }
  for (const p of els.tabPanels) {
    p.classList.toggle("hidden", p.dataset.panel !== name);
  }
}

async function api(path, init = {}) {
  const headers = Object.assign({}, init.headers || {});
  if (state.token) {
    headers.Authorization = `Bearer ${state.token}`;
  }
  let res = await fetch(`${state.baseURL}${path}`, Object.assign({}, init, { headers, credentials: "include" }));
  if (res.status === 401) {
    const refreshed = await tryRefreshToken();
    if (refreshed) {
      const retryHeaders = Object.assign({}, init.headers || {}, { Authorization: `Bearer ${state.token}` });
      res = await fetch(`${state.baseURL}${path}`, Object.assign({}, init, { headers: retryHeaders, credentials: "include" }));
    }
  }
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

async function tryRefreshToken() {
  const res = await fetch(`${state.baseURL}/api/v1/auth/refresh`, {
    method: "POST",
    headers: { Accept: "application/json" },
    credentials: "include",
  });
  if (!res.ok) {
    return false;
  }
  const body = await res.json();
  const token = body && body.access_token ? String(body.access_token) : "";
  if (!token) return false;
  state.token = token;
  await persistSecureToken(token);
  saveState();
  return true;
}

function escapeHtml(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function renderDevices(rows) {
  els.devicesBody.innerHTML = "";
  for (const d of rows || []) {
    const tr = document.createElement("tr");
    if (state.selectedDeviceId === d.id) {
      tr.classList.add("active");
    }
    tr.innerHTML = `
      <td>${escapeHtml(d.id)}</td>
      <td>${escapeHtml(d.name || "")}</td>
      <td>${escapeHtml(d.host || "")}</td>
      <td>${escapeHtml(d.snmp_version || "")}</td>
      <td>${d.last_cpu_pct == null ? "N/A" : `${Number(d.last_cpu_pct).toFixed(1)}%`}</td>
      <td>${d.last_snmp_ok == null ? "?" : d.last_snmp_ok ? "yes" : "no"}</td>
    `;
    tr.addEventListener("click", async () => {
      state.selectedDeviceId = d.id;
      saveState();
      fillDeviceForm(d);
      renderDevices(rows);
      await loadDeviceDetail(d.id, d.name || String(d.id));
    });
    els.devicesBody.appendChild(tr);
  }
}

function renderEvents(rows) {
  els.eventsBody.innerHTML = "";
  state.lastEvents = Array.isArray(rows) ? rows : [];
  for (const e of rows || []) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHtml(e.id)}</td>
      <td>${escapeHtml(e.device_id)}</td>
      <td>${escapeHtml(e.event_type)}</td>
      <td>${escapeHtml(e.severity)}</td>
      <td>${escapeHtml((e.created_at || "").replace("T", " ").replace("Z", ""))}</td>
    `;
    els.eventsBody.appendChild(tr);
  }
}

function buildEventsURL() {
  const q = new URLSearchParams();
  q.set("limit", "50");
  const et = (els.eventsFilterType.value || "").trim();
  const sev = (els.eventsFilterSeverity.value || "").trim();
  if (et) q.set("event_type", et);
  if (sev) q.set("severity", sev);
  return `/api/v1/events?${q.toString()}`;
}

async function reloadGlobalEvents() {
  const events = await api(buildEventsURL());
  renderEvents(Array.isArray(events) ? events : []);
}

function operText(v) {
  if (v === 1) return "up";
  if (v === 2) return "down";
  return String(v ?? "n/a");
}

function renderIfaces(rows) {
  els.ifacesBody.innerHTML = "";
  for (const p of rows || []) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHtml(p.if_index ?? "")}</td>
      <td>${escapeHtml(p.if_name || p.if_descr || "")}</td>
      <td>${escapeHtml(operText(p.oper_status))}</td>
      <td>${escapeHtml(p.port_role || "")}</td>
      <td>${p.util_max_pct == null ? "N/A" : Number(p.util_max_pct).toFixed(1)}</td>
    `;
    els.ifacesBody.appendChild(tr);
  }
}

function renderDeviceEvents(rows) {
  els.deviceEventsBody.innerHTML = "";
  for (const e of rows || []) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHtml(e.id)}</td>
      <td>${escapeHtml(e.event_type)}</td>
      <td>${escapeHtml(e.severity)}</td>
      <td>${escapeHtml((e.created_at || "").replace("T", " ").replace("Z", ""))}</td>
    `;
    els.deviceEventsBody.appendChild(tr);
  }
}

async function loadDeviceDetail(id, label) {
  const detail = await api(`/api/v1/devices/${id}/detail`);
  els.detailTitle.textContent = `#${id} ${label}`;
  renderIfaces(Array.isArray(detail.interfaces) ? detail.interfaces : []);
  renderDeviceEvents(Array.isArray(detail.events) ? detail.events : []);
}

async function reloadData() {
  if (!state.token) {
    setMessage("Сначала выполните вход.", true);
    return;
  }
  const devices = await api("/api/v1/devices");
  const drows = Array.isArray(devices) ? devices : [];
  state.devices = drows;
  renderDevices(drows);
  await reloadGlobalEvents();
  if (drows.length > 0) {
    const cur = drows.find((d) => d.id === state.selectedDeviceId);
    const target = cur || drows[0];
    state.selectedDeviceId = target.id;
    saveState();
    fillDeviceForm(target);
    renderDevices(drows);
    await loadDeviceDetail(target.id, target.name || String(target.id));
  } else {
    state.selectedDeviceId = null;
    els.detailTitle.textContent = "";
    els.ifacesBody.innerHTML = "";
    els.deviceEventsBody.innerHTML = "";
  }
  saveState();
}

function buildDevicePayload() {
  clearValidation();
  const name = (els.newName.value || "").trim();
  const host = (els.newHost.value || "").trim();
  const snmp = (els.newSNMP.value || "v2c").trim();
  const community = (els.newCommunity.value || "").trim();
  const poll = Number(els.newPoll.value || "60");
  const v3User = (els.newV3User.value || "").trim();
  const v3AuthProto = (els.newV3AuthProto.value || "SHA").trim();
  const v3AuthPass = (els.newV3AuthPass.value || "").trim();
  const v3PrivProto = (els.newV3PrivProto.value || "AES").trim();
  const v3PrivPass = (els.newV3PrivPass.value || "").trim();
  if (!name || !host) {
    markInvalid(!name ? els.newName : els.newHost);
    throw new Error("Для создания/обновления узла нужны name и host.");
  }
  if (!Number.isFinite(poll) || poll < 10) {
    markInvalid(els.newPoll);
    throw new Error("poll interval должен быть числом >= 10.");
  }
  if (snmp === "v1" || snmp === "v2c") {
    if (!community) {
      markInvalid(els.newCommunity);
      throw new Error("Для v1/v2c нужен community.");
    }
    return {
      name,
      host,
      snmp_version: snmp,
      community,
      poll_interval_seconds: poll,
    };
  }
  if (!v3User) {
    markInvalid(els.newV3User);
    throw new Error("Для v3 нужен v3_user.");
  }
  if (v3AuthPass.length < 8) {
    markInvalid(els.newV3AuthPass);
    throw new Error("Для v3_auth_pass нужно минимум 8 символов.");
  }
  if (v3PrivProto !== "NONE" && v3PrivPass.length < 8) {
    markInvalid(els.newV3PrivPass);
    throw new Error("Для v3_priv_pass нужно минимум 8 символов (или v3_priv_protocol=NONE).");
  }
  return {
    name,
    host,
    snmp_version: "v3",
    v3_user: v3User,
    v3_auth_protocol: v3AuthProto,
    v3_auth_pass: v3AuthPass,
    v3_priv_protocol: v3PrivProto,
    v3_priv_pass: v3PrivProto === "NONE" ? "" : v3PrivPass,
    poll_interval_seconds: poll,
  };
}

function markInvalid(el) {
  if (el) el.classList.add("invalid");
}

function clearValidation() {
  [
    els.newName,
    els.newHost,
    els.newCommunity,
    els.newV3User,
    els.newV3AuthPass,
    els.newV3PrivPass,
    els.newPoll,
  ].forEach((el) => el && el.classList.remove("invalid"));
}

async function createDevice() {
  const payload = buildDevicePayload();
  await api("/api/v1/devices", {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(payload),
  });
}

async function updateSelectedDevice() {
  if (!state.selectedDeviceId) {
    throw new Error("Сначала выбери узел в таблице.");
  }
  const payload = buildDevicePayload();
  await api(`/api/v1/devices/${state.selectedDeviceId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(payload),
  });
}

async function deleteSelectedDevice() {
  if (!state.selectedDeviceId) {
    throw new Error("Сначала выбери узел в таблице.");
  }
  const id = state.selectedDeviceId;
  await api(`/api/v1/devices/${id}`, { method: "DELETE", headers: { Accept: "application/json" } });
  state.selectedDeviceId = null;
  saveState();
}

function fillDeviceForm(d) {
  if (!d) return;
  els.newName.value = d.name || "";
  els.newHost.value = d.host || "";
  els.newSNMP.value = d.snmp_version || "v2c";
  els.newCommunity.value = d.community || "";
  els.newV3User.value = d.v3_user || "";
  els.newV3AuthProto.value = (d.v3_auth_protocol || "SHA").toUpperCase();
  els.newV3PrivProto.value = (d.v3_priv_protocol || "AES").toUpperCase();
  els.newPoll.value = String(d.poll_interval_seconds || 60);
  // Password fields are intentionally not prefilled for security.
  els.newV3AuthPass.value = "";
  els.newV3PrivPass.value = "";
}

els.loginForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    state.baseURL = (els.baseURL.value || "").trim().replace(/\/+$/, "");
    const body = await fetch(`${state.baseURL}/api/v1/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      credentials: "include",
      body: JSON.stringify({
        username: (els.username.value || "").trim(),
        password: els.password.value || "",
      }),
    }).then(async (r) => {
      if (!r.ok) throw new Error(await r.text());
      return r.json();
    });
    state.token = body.access_token || "";
    if (!state.token) throw new Error("access_token отсутствует");
    await persistSecureToken(state.token);
    saveState();
    setStatus(`Подключено к ${state.baseURL}`);
    setMessage("Вход выполнен.");
    await reloadData();
  } catch (err) {
    setMessage(`Ошибка входа: ${err.message}`, true);
  }
});

els.logoutBtn.addEventListener("click", async () => {
  state.token = "";
  await persistSecureToken("");
  state.selectedDeviceId = null;
  els.devicesBody.innerHTML = "";
  els.eventsBody.innerHTML = "";
  els.ifacesBody.innerHTML = "";
  els.deviceEventsBody.innerHTML = "";
  els.detailTitle.textContent = "";
  setStatus("Не подключено");
  setMessage("Сессия очищена.");
  saveState();
});

els.clearLocalBtn.addEventListener("click", async () => {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
  await persistSecureToken("");
  state.token = "";
  state.selectedDeviceId = null;
  state.baseURL = "http://127.0.0.1:8080";
  els.baseURL.value = state.baseURL;
  els.password.value = "";
  els.devicesBody.innerHTML = "";
  els.eventsBody.innerHTML = "";
  els.ifacesBody.innerHTML = "";
  els.deviceEventsBody.innerHTML = "";
  els.detailTitle.textContent = "";
  setStatus("Не подключено");
  setMessage("Локальная сессия и настройки очищены.");
});

els.reloadBtn.addEventListener("click", async () => {
  try {
    await reloadData();
    setMessage("Данные обновлены.");
  } catch (err) {
    setMessage(`Ошибка обновления: ${err.message}`, true);
  }
});

els.eventsApplyFilterBtn.addEventListener("click", async () => {
  try {
    await reloadGlobalEvents();
    saveState();
    setMessage("Фильтр событий применен.");
  } catch (err) {
    setMessage(`Ошибка фильтрации событий: ${err.message}`, true);
  }
});

els.eventsExportCsvBtn.addEventListener("click", () => {
  try {
    const rows = state.lastEvents || [];
    const lines = ["id,device_id,event_type,severity,created_at"];
    for (const e of rows) {
      const vals = [e.id, e.device_id, e.event_type, e.severity, e.created_at].map((v) =>
        `"${String(v ?? "").replace(/"/g, '""')}"`,
      );
      lines.push(vals.join(","));
    }
    const blob = new Blob([lines.join("\n")], { type: "text/csv;charset=utf-8" });
    const u = URL.createObjectURL(blob);
    const a = document.createElement("a");
    const ts = new Date().toISOString().replace(/[:.]/g, "-");
    a.href = u;
    a.download = `netlynx-events-${ts}.csv`;
    a.click();
    URL.revokeObjectURL(u);
    setMessage("CSV экспортирован.");
  } catch (err) {
    setMessage(`Ошибка экспорта CSV: ${err.message}`, true);
  }
});

els.createForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    await createDevice();
    await reloadData();
    setMessage("Узел добавлен.");
  } catch (err) {
    setMessage(`Ошибка создания узла: ${err.message}`, true);
  }
});

els.updateDeviceBtn.addEventListener("click", async () => {
  try {
    await updateSelectedDevice();
    await reloadData();
    setMessage("Изменения узла сохранены.");
  } catch (err) {
    setMessage(`Ошибка обновления узла: ${err.message}`, true);
  }
});

els.deleteDeviceBtn.addEventListener("click", async () => {
  try {
    if (!state.selectedDeviceId) {
      setMessage("Сначала выберите узел в таблице.", true);
      return;
    }
    if (!confirm(`Удалить узел #${state.selectedDeviceId}?`)) {
      return;
    }
    await deleteSelectedDevice();
    await reloadData();
    setMessage("Узел удален.");
  } catch (err) {
    setMessage(`Ошибка удаления узла: ${err.message}`, true);
  }
});

async function loadSettingsForm() {
  if (!state.token) return;
  const ns = await api("/api/v1/settings/notifications");
  els.setWhEn.checked = Boolean(ns.webhook_enabled);
  els.setWhUrl.value = ns.webhook_url || "";
  els.setTgEn.checked = Boolean(ns.telegram_enabled);
  els.setTgTok.value = ns.telegram_bot_token || "";
  els.setTgChat.value = ns.telegram_chat_id || "";
  els.setIncidentEn.checked = Boolean(ns.incident_action_enabled);
  els.setIncidentTypes.value = ns.incident_action_event_types || "UNKNOWN_MAC_ON_ACCESS_PORT";
}

if (els.settingsNotifForm) {
  els.settingsNotifForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    try {
      await api("/api/v1/settings/notifications", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          webhook_enabled: els.setWhEn.checked,
          webhook_url: els.setWhUrl.value.trim(),
          telegram_enabled: els.setTgEn.checked,
          telegram_bot_token: els.setTgTok.value.trim(),
          telegram_chat_id: els.setTgChat.value.trim(),
          incident_action_enabled: els.setIncidentEn.checked,
          incident_action_event_types: els.setIncidentTypes.value.trim(),
        }),
      });
      if (els.settingsMsg) els.settingsMsg.textContent = "Настройки сохранены.";
    } catch (err) {
      if (els.settingsMsg) els.settingsMsg.textContent = err.message;
    }
  });
}

async function init() {
  loadState();
  await loadSecureToken();
  els.tabButtons.forEach((b) => {
    b.addEventListener("click", async () => {
      const tab = b.dataset.tab || "overview";
      switchTab(tab);
      if (tab === "settings") {
        try {
          await loadSettingsForm();
        } catch (err) {
          if (els.settingsMsg) els.settingsMsg.textContent = err.message;
        }
      }
    });
  });
  switchTab("overview");
  if (state.token) {
    setStatus(`Подключено к ${state.baseURL}`);
    reloadData().catch(async () => {
      const ok = await tryRefreshToken();
      if (ok) {
        setStatus(`Подключено к ${state.baseURL}`);
        await reloadData();
      }
    });
  }
}

init();
