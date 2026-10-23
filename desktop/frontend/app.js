import { Call, Events } from "/wails/runtime.js";
import { $, h, icon, clean, bytes, rate, ago, uptime, clock, brand, CATEGORIES, CATEGORY_ORDER, countUp, keyed, motion, setReduceMotion, cssVar } from "./util.js";
import { area, spark, updateSpark } from "./charts.js";
import { MapView } from "./map.js";

const SERVICE = "github.com/padovanl/portop/desktop/backend.Backend.";
const call = (method, ...args) => Call.ByName(SERVICE + method, ...args);

const state = {
  snap: null,
  settings: null,
  about: null,
  events: [],
  view: "overview",
  stateFilter: "listen",
  category: null,
  eventKind: "all",
  eventScope: "relevant",
  unseen: 0,
  drawerPid: null,
};

const isMac = /Mac/.test(navigator.platform);
document.body.classList.toggle("is-mac", isMac);
$("palette-key").textContent = isMac ? "⌘ K" : "Ctrl K";

/* ---------- Toasts ---------- */

function toast(text, error = false) {
  const el = h("div", { class: "toast" + (error ? " error" : "") }, h("i"), h("span", {}, text));
  $("toasts").append(el);
  setTimeout(() => { el.classList.add("out"); setTimeout(() => el.remove(), 320); }, error ? 6500 : 3200);
}

/* ---------- Theme and settings ---------- */

const systemDark = matchMedia("(prefers-color-scheme: dark)");
function applyTheme() {
  const t = state.settings?.theme || "system";
  document.documentElement.dataset.theme = t === "system" ? (systemDark.matches ? "dark" : "light") : t;
  for (const b of $("theme-choice").children) b.classList.toggle("active", b.dataset.theme === t);
  if (state.snap) renderOverview();
}
systemDark.addEventListener("change", applyTheme);

async function saveSettings(patch) {
  try {
    state.settings = await call("SaveSettings", { ...state.settings, ...patch });
  } catch (e) {
    toast("Could not save settings: " + e.message, true);
  }
  renderSettings();
  if (state.snap) { renderPorts(); renderWatch(); renderTimeline(); }
}

function renderSettings() {
  const s = state.settings;
  if (!s) return;
  applyTheme();
  $("interval").value = s.interval_ms;
  $("interval-label").textContent = (s.interval_ms / 1000).toString().replace(/\.0$/, "") + " s";
  $("notify-new").checked = s.notify_new;
  $("notify-udp").checked = s.notify_udp;
  keyed($("notify-cats"), CATEGORY_ORDER, (c) => c,
    (c) => h("button", { class: "chip cat-" + c, onclick: () => saveSettings({ notify_categories: toggleIn(state.settings.notify_categories, c) }) }, h("i"), CATEGORIES[c].label),
    (el, c) => el.classList.toggle("active", s.notify_categories.includes(c)));
  const muted = $("muted-list");
  muted.replaceChildren(...(s.muted.length ? s.muted.map((m) => h("li", {}, clean(m),
    h("button", { class: "icon-btn", title: "Unmute " + m, onclick: () => saveSettings({ muted: state.settings.muted.filter((x) => x !== m) }) }, icon("x")))) : [h("li", { class: "none" }, "None")]));
  $("close-to-tray").checked = s.close_to_tray;
  $("reduce-motion").checked = s.reduce_motion;
  setReduceMotion(s.reduce_motion);
  document.documentElement.classList.toggle("reduce-motion", s.reduce_motion);
}

$("theme-choice").addEventListener("click", (e) => { const t = e.target.closest("button")?.dataset.theme; if (t) saveSettings({ theme: t }); });
$("interval").addEventListener("input", (e) => { $("interval-label").textContent = (e.target.value / 1000) + " s"; });
$("interval").addEventListener("change", (e) => saveSettings({ interval_ms: Number(e.target.value) }));
$("notify-new").addEventListener("change", (e) => saveSettings({ notify_new: e.target.checked }));
$("notify-udp").addEventListener("change", (e) => saveSettings({ notify_udp: e.target.checked }));
$("close-to-tray").addEventListener("change", (e) => saveSettings({ close_to_tray: e.target.checked }));
$("reduce-motion").addEventListener("change", (e) => saveSettings({ reduce_motion: e.target.checked }));

const isWatched = (port) => state.settings?.watched.includes(port);

// relevant mirrors Settings.Relevant in the backend: the filters that
// decide which new ports notify, also used by Activity's Relevant view.
function relevant(e) {
  const s = state.settings;
  if (!s) return true;
  if (s.watched.includes(e.port)) return true;
  if (e.protocol === "UDP" && !s.notify_udp) return false;
  if (!s.notify_categories.includes(e.category)) return false;
  return !s.muted.includes((e.process || "").toLowerCase());
}
const isPinned = (port) => state.settings?.pinned.includes(port);
const toggleIn = (list, port) => (list.includes(port) ? list.filter((p) => p !== port) : [...list, port]);

/* ---------- Navigation ---------- */

const map = new MapView($("map"), $("map-tip"), { onSelect: (row) => row && openDetails(row) });

function show(view) {
  state.view = view;
  for (const b of document.querySelectorAll(".nav-item")) b.classList.toggle("active", b.dataset.view === view);
  for (const v of document.querySelectorAll(".view")) v.classList.toggle("active", v.id === "view-" + view);
  movePill();
  if (view === "map") { requestAnimationFrame(() => { map.resize(); map.start(); }); } else map.stop();
  if (view === "activity") { state.unseen = 0; $("nav-activity").hidden = true; }
}
function movePill() {
  const active = document.querySelector(".nav-item.active");
  const pill = document.querySelector(".nav-pill");
  pill.style.transform = `translateY(${active.offsetTop}px)`;
  pill.style.height = active.offsetHeight + "px";
}
document.querySelector(".nav").addEventListener("click", (e) => { const b = e.target.closest(".nav-item"); if (b) show(b.dataset.view); });

$("map-search").addEventListener("input", (e) => map.search(e.target.value));
$("map-pause").addEventListener("click", () => {
  const paused = $("map-pause").getAttribute("aria-pressed") !== "true";
  map.setPaused(paused);
  if (!paused && state.snap) map.update(state.snap.rows);
  $("map-pause").setAttribute("aria-pressed", String(paused));
  $("map-pause").replaceChildren(icon(paused ? "play" : "pause"), h("span", {}, paused ? "Resume" : "Pause"));
  $("map-paused").hidden = !paused;
});
$("map-zoom-in").addEventListener("click", () => map.zoom(1.25));
$("map-zoom-out").addEventListener("click", () => map.zoom(0.8));
$("map-fit").addEventListener("click", () => map.fitNow());

/* ---------- Overview ---------- */

function devGroups(rows) {
  // One card per dev process, with all of its listening ports.
  const byPid = new Map();
  for (const r of rows) {
    if (!r.listening || r.category !== "dev" || !r.pid) continue;
    const g = byPid.get(r.pid) || { pid: r.pid, row: r, ports: new Map() };
    const prev = g.ports.get(r.local_port);
    if (!prev || (prev.scope !== "all" && r.scope === "all")) g.ports.set(r.local_port, r);
    byPid.set(r.pid, g);
  }
  return [...byPid.values()].sort((a, b) => Math.min(...a.ports.keys()) - Math.min(...b.ports.keys()));
}

function serviceGroups(rows) {
  const byPid = new Map();
  for (const r of rows) {
    if (!r.listening || r.category === "dev") continue;
    const key = r.pid || "u" + r.local_port;
    const g = byPid.get(key) || { key, row: r, ports: new Set() };
    g.ports.add(r.local_port);
    byPid.set(key, g);
  }
  const order = (c) => CATEGORY_ORDER.indexOf(c);
  return [...byPid.values()].sort((a, b) => order(a.row.category) - order(b.row.category) || Math.min(...a.ports) - Math.min(...b.ports));
}

function renderOverview() {
  const { snap } = state;
  const s = snap.summary;
  if (state.about) $("overview-sub").textContent = `${state.about.host} · ${state.about.os}/${state.about.arch} · updated ${clock(snap.time)}`;
  for (const el of document.querySelectorAll("[data-count]")) countUp(el, s[el.dataset.count]);
  document.querySelector('.kpi[data-tone="amber"]').classList.toggle("alert", s.exposed > 0);
  $("kpi-exposed-sub").textContent = s.exposed ? "reachable from the network" : "nothing exposed";
  $("kpi-listening-sub").textContent = `${s.processes} process${s.processes === 1 ? "" : "es"} · ${s.established} connections`;
  $("kpi-dev-sub").textContent = s.dev_servers ? "running now" : "none right now";
  $("rx-rate").textContent = rate(s.rx_rate);
  $("tx-rate").textContent = rate(s.tx_rate);
  area($("traffic-chart"), [
    { values: snap.traffic.map((p) => p.rx), color: cssVar("--mint") },
    { values: snap.traffic.map((p) => p.tx), color: cssVar("--azure") },
  ], { min: 2048 });

  const groups = devGroups(snap.rows);
  $("dev-empty").hidden = groups.length > 0;
  $("stop-all").hidden = groups.length < 2;
  keyed($("dev-cards"), groups, (g) => "pid" + g.pid, createCard, updateCard);

  const services = serviceGroups(snap.rows);
  // Sockets of other users (no PID) are one tile, not one per port.
  const hidden = services.filter((g) => !g.row.pid);
  const tiles = services.filter((g) => g.row.pid);
  if (hidden.length) tiles.push({ key: "hidden", hidden: true, row: { category: "system" }, ports: new Set(hidden.flatMap((g) => [...g.ports])) });
  keyed($("service-tiles"), tiles, (g) => String(g.key), createTile, updateTile);
  $("services-empty").hidden = tiles.length > 0;
  $("category-bar").hidden = tiles.length === 0;

  const counts = {};
  for (const g of [...groups.map((g) => ({ row: g.row })), ...services]) counts[g.row.category] = (counts[g.row.category] || 0) + 1;
  const bar = $("category-bar");
  if (!bar.children.length) for (const c of CATEGORY_ORDER) bar.append(h("span", { class: "cat-" + c, title: CATEGORIES[c].label, style: { background: "var(--cat)", flexGrow: 0 } }));
  for (const el of bar.children) {
    const c = el.className.replace("cat-", "");
    el.style.flexGrow = counts[c] || 0;
    el.title = `${CATEGORIES[c].label}: ${counts[c] || 0}`;
  }
}

function createCard(g) {
  const el = h("article", { class: "card", tabindex: "0" });
  el.addEventListener("click", (e) => { if (!e.target.closest("button")) openDetails(el.__g.row); });
  el.addEventListener("keydown", (e) => { if (e.key === "Enter") openDetails(el.__g.row); });
  return el;
}

function updateCard(el, g) {
  el.__g = g;
  const r = g.row;
  const b = brand(r);
  el.style.setProperty("--fw", b.color);
  const sig = JSON.stringify([r.project, r.framework, r.language, r.process, [...g.ports.keys()], [...g.ports.values()].map((p) => p.scope)]);
  if (el.__sig !== sig) {
    el.__sig = sig;
    el.replaceChildren(
      h("div", { class: "card-top" },
        h("div", { class: "monogram" }, b.mono),
        h("div", { class: "card-title" },
          h("h3", {}, clean(r.project || r.process)),
          h("p", {}, r.framework ? h("b", {}, r.framework) : null, r.framework && r.language ? " · " : "", r.language || (r.framework ? "" : r.process))),
        h("span", { class: "live-dot", title: "Listening" })),
      h("div", { class: "port-chips" }, [...g.ports.values()].map((p) =>
        h("button", { class: "port-chip", title: p.url ? "Open " + p.url : p.protocol + " " + p.local_port, onclick: () => p.url ? openURL(p.url) : openDetails(p) },
          h("span", { class: "scope scope-" + p.scope }), ":" + p.local_port, p.url ? icon("open") : null))),
      h("div", { class: "card-foot" },
        h("span", { class: "mono" }, "PID " + r.pid),
        spark([], b.color),
        h("div", { class: "card-actions" },
          r.project_root ? h("button", { class: "icon-btn", title: "Open folder", onclick: () => call("OpenFolder", r.project_root).catch((e) => toast(e.message, true)) }, icon("folder")) : null,
          h("button", { class: "icon-btn", title: "Restart in a terminal", onclick: () => restart(r, el) }, icon("restart")),
          h("button", { class: "icon-btn danger", title: "Stop", onclick: () => stopRow(r, el) }, icon("stop")))),
    );
  }
  const hist = state.snap.process_traffic?.[r.pid] || [];
  updateSpark(el.querySelector(".spark"), hist.map((p) => p.rx + p.tx), b.color);
}

function createTile(g) {
  const el = h("div", { class: "tile", tabindex: "0" });
  el.addEventListener("click", () => {
    if (!el.__g.hidden) { openDetails(el.__g.row); return; }
    state.category = null;
    $("filter").value = "";
    show("ports");
    renderPorts();
  });
  return el;
}
function updateTile(el, g) {
  el.__g = g;
  const r = g.row;
  if (g.hidden) {
    const ports = [...g.ports].sort((a, b) => a - b);
    const sig = "hidden" + ports.join(",");
    if (el.__sig === sig) return;
    el.__sig = sig;
    el.className = "tile cat-system";
    el.replaceChildren(
      h("div", { class: "ico" }, icon("lock")),
      h("div", {}, h("b", {}, `${ports.length} hidden port${ports.length === 1 ? "" : "s"}`),
        h("small", {}, ports.slice(0, 5).map((p) => ":" + p).join(" ") + (ports.length > 5 ? " …" : ""))));
    return;
  }
  const sig = JSON.stringify([r.process, r.category, r.unit, r.container, [...g.ports]]);
  if (el.__sig === sig) return;
  el.__sig = sig;
  el.className = "tile cat-" + r.category;
  const ports = [...g.ports].sort((a, b) => a - b);
  el.replaceChildren(
    h("div", { class: "ico" }, icon(CATEGORIES[r.category]?.icon || "app")),
    h("div", {},
      h("b", {}, clean(r.container || r.process || (r.pid ? "PID " + r.pid : "owner hidden"))),
      h("small", {}, ports.slice(0, 4).map((p) => ":" + p).join(" ") + (ports.length > 4 ? ` +${ports.length - 4}` : "") + (r.unit ? " · " + r.unit : ""))),
  );
}

/* ---------- Free a port ---------- */

function ownerOfPort(port) {
  return state.snap?.rows.find((r) => r.listening && r.local_port === port && r.pid) ||
    state.snap?.rows.find((r) => r.listening && r.local_port === port);
}
function updateFreeOwner() {
  const port = Number($("free-input").value.trim());
  const owner = port ? ownerOfPort(port) : null;
  const out = $("free-owner");
  $("free-button").disabled = !owner || !owner.pid;
  if (!port) { out.textContent = ""; return; }
  if (!owner) { out.textContent = "free"; return; }
  out.replaceChildren(h("b", {}, clean(owner.project || owner.process || "hidden")), owner.pid ? ` · PID ${owner.pid}` : " (no permission)");
}
$("free-input").addEventListener("input", updateFreeOwner);
$("free-form").addEventListener("submit", (e) => {
  e.preventDefault();
  const owner = ownerOfPort(Number($("free-input").value.trim()));
  if (owner?.pid) stopRow(owner, $("free-form"));
});

/* ---------- Ports ---------- */

function portMatches(r, terms) {
  if (!terms.length) return true;
  const hay = [r.local_port, r.local_address, r.remote_port, r.remote_address, r.remote_host, r.pid, r.process, r.project, r.framework, r.language, r.protocol, r.state, r.unit, r.container, r.user, r.category].join(" ").toLowerCase();
  return terms.every((t) => hay.includes(t));
}

function renderPorts() {
  const rows = state.snap.rows;
  const terms = $("filter").value.toLowerCase().split(/\s+/).filter(Boolean);
  const byState = (r) => state.stateFilter === "all" || (state.stateFilter === "listen" ? r.listening : r.state === "ESTABLISHED");
  $("count-listen").textContent = rows.filter((r) => r.listening).length;
  $("count-established").textContent = rows.filter((r) => r.state === "ESTABLISHED").length;
  $("count-all").textContent = rows.length;
  $("nav-ports").textContent = rows.filter((r) => r.listening).length;

  const inState = rows.filter((r) => byState(r) && portMatches(r, terms));
  const chips = $("category-chips");
  const counts = Object.fromEntries(CATEGORY_ORDER.map((c) => [c, inState.filter((r) => r.category === c).length]));
  keyed(chips, CATEGORY_ORDER.filter((c) => counts[c] || state.category === c), (c) => c,
    (c) => h("button", { class: "chip cat-" + c, onclick: () => { state.category = state.category === c ? null : c; renderPorts(); } }, h("i"), CATEGORIES[c].label, h("em")),
    (el, c) => { el.classList.toggle("active", state.category === c); el.querySelector("em").textContent = counts[c]; });

  const shown = inState.filter((r) => !state.category || r.category === state.category);
  shown.sort((a, b) => (isPinned(b.local_port) - isPinned(a.local_port)) || (b.listening - a.listening) || a.local_port - b.local_port || (a.remote_port || 0) - (b.remote_port || 0));
  const limited = shown.slice(0, 400);
  $("ports-empty").hidden = limited.length > 0;
  keyed($("ports-body"), limited, (r) => r.key, createPortRow, updatePortRow);
}

function createPortRow() {
  const el = h("div", { class: "trow", tabindex: "0" });
  el.addEventListener("click", (e) => { if (!e.target.closest("button")) openDetails(el.__r); });
  el.addEventListener("keydown", (e) => { if (e.key === "Enter") openDetails(el.__r); });
  setTimeout(() => el.classList.remove("entering"), 2600);
  return el;
}

function updatePortRow(el, r) {
  el.__r = r;
  const pinned = isPinned(r.local_port), watched = isWatched(r.local_port);
  const sig = JSON.stringify([r, pinned, watched]);
  if (el.__sig === sig) return;
  el.__sig = sig;
  const b = brand(r);
  const scopeLabel = { all: "all interfaces", loopback: "loopback", address: "one address" }[r.scope];
  const traffic = r.rx_rate !== undefined ? (() => {
    const max = Math.max(r.rx_rate, r.tx_rate, 1);
    const w = (v) => Math.max(2, Math.min(90, (Math.log10(1 + v) / Math.log10(1 + Math.max(max, 1e6))) * 90)) + "px";
    return h("div", { class: "rate-bars" }, h("span", {}, h("i", { style: { width: w(r.rx_rate) } }), "↓ " + rate(r.rx_rate)), h("span", {}, h("i", { style: { width: w(r.tx_rate) } }), "↑ " + rate(r.tx_rate)));
  })() : null;
  el.replaceChildren(
    h("div", { class: "cell-port" },
      pinned ? h("span", { class: "pin", title: "Pinned" }, icon("star")) : null,
      h("div", {}, h("strong", {}, ":" + r.local_port), h("small", {}, `${r.protocol}${r.ipv6 ? "6" : "4"} · ${clean(r.local_address)}`))),
    h("div", { class: "cell-main" },
      r.process ? h("b", {}, clean(r.process)) : h("b", { class: "unknown" }, r.pid ? "PID " + r.pid : "owner hidden"),
      h("small", {}, r.pid ? `PID ${r.pid}${r.user ? " · " + clean(r.user) : ""}` : r.user ? clean(r.user) : "run portop as administrator to see it")),
    h("div", { class: "cell-main" },
      r.project || r.framework ? h("b", {}, h("span", { class: "fw-dot", style: { "--fw": b.color } }), clean(r.project || r.framework)) : h("b", { class: "unknown" }, "—"),
      h("small", {}, [r.framework && r.project ? r.framework : "", r.language].filter(Boolean).join(" · ") || " ")),
    h("div", { class: "cell-main" },
      r.listening ? h("div", {}, h("span", { class: "badge " + r.scope }, scopeLabel), r.firewall ? h("span", { class: "badge state", style: { marginLeft: "6px" } }, "fw: " + r.firewall) : null)
        : h("b", { class: "mono", style: { fontWeight: 500, fontSize: "12.5px" } }, clean(r.remote_host || r.remote_address) + ":" + r.remote_port),
      r.listening ? h("small", {}, r.state) : traffic || h("small", {}, r.state)),
    h("div", { class: "cell-main" },
      h("span", { class: "badge cat cat-" + r.category }, CATEGORIES[r.category]?.label || r.category),
      h("small", {}, clean(r.container || r.unit || ""))),
    h("div", { class: "row-actions" },
      h("button", { class: "icon-btn" + (pinned ? " on" : ""), title: pinned ? "Unpin" : "Pin to top", onclick: () => saveSettings({ pinned: toggleIn(state.settings.pinned, r.local_port) }) }, icon("star")),
      r.listening ? h("button", { class: "icon-btn" + (watched ? " on" : ""), title: watched ? "Stop watching" : "Notify when it opens or closes", onclick: () => saveSettings({ watched: toggleIn(state.settings.watched, r.local_port) }) }, icon("bell")) : null,
      r.url ? h("button", { class: "icon-btn", title: "Open " + r.url, onclick: () => openURL(r.url) }, icon("open")) : null,
      r.pid ? h("button", { class: "icon-btn danger", title: "Stop", onclick: () => stopRow(r, el) }, icon("stop")) : null),
  );
}

$("filter").addEventListener("input", () => renderPorts());
$("state-filter").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  state.stateFilter = b.dataset.state;
  for (const x of $("state-filter").children) x.classList.toggle("active", x === b);
  renderPorts();
});

/* ---------- Actions ---------- */

async function openURL(url) {
  try { await call("OpenURL", url); } catch (e) { toast(e.message, true); }
}

// confirm asks with a hold-to-confirm button, so a stray click never
// kills anything.
function confirmHold({ title, text, label }) {
  return new Promise((resolve) => {
    $("confirm-title").textContent = title;
    $("confirm-text").replaceChildren(...text);
    const ok = $("confirm-ok");
    ok.querySelector(".hold-label").textContent = label;
    $("confirm").hidden = false;
    let timer = null;
    const close = (v) => {
      $("confirm").hidden = true;
      ok.classList.remove("holding");
      clearTimeout(timer);
      ok.onpointerdown = ok.onpointerup = ok.onpointerleave = ok.onkeydown = ok.onkeyup = null;
      $("confirm-cancel").onclick = null;
      document.removeEventListener("keydown", esc, true);
      resolve(v);
    };
    const press = () => { ok.classList.add("holding"); timer = setTimeout(() => close(true), motion() ? 900 : 0); };
    const release = () => { ok.classList.remove("holding"); clearTimeout(timer); };
    const esc = (e) => { if (e.key === "Escape") { e.stopPropagation(); close(false); } };
    ok.onpointerdown = press; ok.onpointerup = release; ok.onpointerleave = release;
    ok.onkeydown = (e) => { if ((e.key === "Enter" || e.key === " ") && !e.repeat) { e.preventDefault(); press(); } };
    ok.onkeyup = (e) => { if (e.key === "Enter" || e.key === " ") release(); };
    $("confirm-cancel").onclick = () => close(false);
    document.addEventListener("keydown", esc, true);
    setTimeout(() => $("confirm-cancel").focus(), 50);
  });
}

async function processOf(r) {
  const info = await call("Process", r.pid);
  if (!info.started) throw new Error("portop cannot read this process's start time, so it will not signal it");
  return info;
}

async function stopRow(r, el, force = false) {
  let info;
  try { info = await processOf(r); } catch (e) { toast(e.message, true); return; }
  const name = clean(info.project || info.name || r.process);
  const windows = state.about?.os === "windows";
  const how = windows ? "End " : force ? "Send SIGKILL to " : "Send SIGTERM to ";
  const ok = await confirmHold({
    title: force ? "Force kill?" : "Stop " + name + "?",
    text: [how, h("b", {}, name), ` (PID ${r.pid}). Its connections will close.`, info.own ? "" : " It belongs to another user, so your system will ask for an administrator password."],
    label: force ? "Hold to kill" : "Hold to stop",
  });
  if (!ok) return;
  try {
    await call("Stop", r.pid, info.started, force);
    toast(`${force ? "Killed" : "Stopped"} ${name}`);
    if (state.drawerPid === r.pid) closeDrawer();
  } catch (e) {
    toast(e.message, true);
  }
}

async function restart(r, el) {
  let info;
  try { info = await processOf(r); } catch (e) { toast(e.message, true); return; }
  if (!info.can_restart) { toast("portop can restart only your own processes whose command and folder it can read", true); return; }
  const name = clean(info.project || info.name);
  const ok = await confirmHold({
    title: "Restart " + name + "?",
    text: ["portop stops ", h("b", {}, name), " and runs the same command again from ", h("b", {}, clean(info.cwd || info.project_root)), " in a new terminal window."],
    label: "Hold to restart",
  });
  if (!ok) return;
  try {
    await call("Restart", r.pid, info.started);
    toast("Restarting " + name + " in a new terminal");
  } catch (e) {
    toast(e.message, true);
  }
}

$("stop-all").addEventListener("click", async () => {
  const groups = devGroups(state.snap.rows);
  const ok = await confirmHold({ title: "Stop every dev server?", text: [state.about?.os === "windows" ? "portop ends " : "portop sends SIGTERM to ", h("b", {}, `${groups.length} dev servers`), ": ", groups.map((g) => clean(g.row.project || g.row.process)).join(", "), "."], label: "Hold to stop all" });
  if (!ok) return;
  const targets = [];
  for (const g of groups) {
    try { targets.push({ pid: g.pid, started: (await processOf(g.row)).started }); } catch {}
  }
  try {
    const n = await call("StopMany", targets);
    toast(`Stopped ${n} dev server${n === 1 ? "" : "s"}`);
  } catch (e) {
    toast(e.message, true);
  }
});

/* ---------- Details drawer ---------- */

function closeDrawer() {
  $("drawer").classList.remove("open");
  $("drawer").setAttribute("aria-hidden", "true");
  state.drawerPid = null;
}
$("drawer").addEventListener("click", (e) => { if (e.target === $("drawer")) closeDrawer(); });
$("drawer-close").addEventListener("click", closeDrawer);

async function openDetails(r) {
  const body = $("drawer-body");
  const b = brand(r);
  state.drawerPid = r.pid || null;
  $("drawer").classList.add("open");
  $("drawer").setAttribute("aria-hidden", "false");
  const sockets = state.snap.rows.filter((x) => r.pid ? x.pid === r.pid : x.key === r.key);
  const head = h("div", { class: "d-head", style: { "--fw": b.color } },
    h("div", { class: "monogram" }, b.mono),
    h("div", {}, h("h2", {}, clean(r.project || r.process || "Port " + r.local_port)),
      h("p", {}, [r.framework, r.language, r.process && r.project ? r.process : ""].filter(Boolean).join(" · ") || (r.pid ? "PID " + r.pid : "owner hidden"))));
  const socketList = h("div", { class: "socket-list" }, sockets.slice(0, 30).map((x) =>
    h("div", {}, h("span", { class: "badge " + (x.listening ? x.scope : "state") }, x.listening ? ({ all: "all", loopback: "local", address: "addr" }[x.scope]) : "conn"),
      h("span", {}, `${x.protocol} ${x.local_address}:${x.local_port}${x.remote_port ? " → " + (x.remote_host || x.remote_address) + ":" + x.remote_port : ""}`),
      h("span", {}, x.rx_rate !== undefined ? `↓${rate(x.rx_rate)} ↑${rate(x.tx_rate)}` : x.state))));
  body.replaceChildren(head, h("p", { class: "muted" }, "Loading process details…"));
  if (!r.pid) {
    body.replaceChildren(head,
      h("div", { class: "note" }, icon("lock"), h("span", {}, "portop cannot see which process owns this socket. It belongs to another user or a protected process; run the portop CLI with sudo, or as administrator, to see it.")),
      h("div", { class: "d-section" }, h("h3", {}, "Sockets"), socketList));
    return;
  }
  let info;
  try { info = await call("Process", r.pid); } catch (e) {
    body.replaceChildren(head, h("div", { class: "note" }, icon("lock"), h("span", {}, e.message)), h("div", { class: "d-section" }, h("h3", {}, "Sockets"), socketList));
    return;
  }
  if (state.drawerPid !== r.pid) return;
  const listener = sockets.find((x) => x.url);
  const hist = state.snap.process_traffic?.[r.pid] || [];
  const chart = h("svg", { class: "d-spark", preserveAspectRatio: "none" });
  const kv = (pairs) => h("dl", { class: "kv" }, pairs.filter(([, v]) => v !== undefined && v !== null && v !== "").flatMap(([k, v]) => [h("dt", {}, k), h("dd", {}, v)]));
  body.replaceChildren(...[
    head,
    h("div", { class: "d-actions" },
      listener ? h("button", { class: "btn btn-primary btn-sm", onclick: () => openURL(listener.url) }, icon("open"), "Open :" + listener.local_port) : null,
      info.project_root ? h("button", { class: "btn btn-sm", onclick: () => call("OpenFolder", info.project_root).catch((e) => toast(e.message, true)) }, icon("folder"), "Folder") : null,
      info.can_restart ? h("button", { class: "btn btn-sm", onclick: () => restart(r, $("drawer-body")) }, icon("restart"), "Restart") : null,
      h("button", { class: "btn btn-danger btn-sm", onclick: () => stopRow(r, $("drawer-body")) }, icon("stop"), "Stop"),
      h("button", { class: "btn btn-ghost btn-sm", onclick: () => stopRow(r, $("drawer-body"), true) }, "Force kill")),
    !info.own ? h("div", { class: "note" }, icon("shield"), h("span", {}, `Owned by ${clean(info.user || "another user")}: stopping it asks for an administrator password.`)) : null,
    h("div", { class: "d-section" }, h("h3", {}, "Traffic"), chart),
    h("div", { class: "d-section" }, h("h3", {}, `Sockets (${sockets.length})`), socketList),
    h("div", { class: "d-section" }, h("h3", {}, "Command"),
      h("div", { class: "cmd selectable" }, clean(info.cmdline || info.name),
        h("button", { class: "icon-btn", title: "Copy", onclick: () => navigator.clipboard.writeText(info.cmdline || "").then(() => toast("Command copied")) }, icon("copy")))),
    h("div", { class: "d-section" }, h("h3", {}, "Process"), kv([
      ["PID", h("span", { class: "mono-value" }, String(r.pid))],
      ["User", clean(info.user)],
      ["Running for", info.started ? uptime(info.started) : ""],
      ["Memory", info.rss_bytes ? bytes(info.rss_bytes) : ""],
      ["CPU", r.cpu !== undefined ? r.cpu.toFixed(1) + "%" : ""],
      ["Threads", info.num_threads || ""],
      ["Directory", info.cwd ? h("span", { class: "mono-value" }, clean(info.cwd)) : ""],
      ["Executable", info.exe ? h("span", { class: "mono-value" }, clean(info.exe)) : ""],
      ["Unit", r.unit], ["Container", r.container],
      ["Workers", r.workers?.length ? r.workers.join(", ") : ""],
    ])),
  ].filter(Boolean));
  area(chart, [{ values: hist.map((p) => p.rx), color: cssVar("--mint") }, { values: hist.map((p) => p.tx), color: cssVar("--azure") }], { min: 1024 });
}

/* ---------- Activity ---------- */

function renderTimeline() {
  const list = $("timeline");
  const events = state.events.filter((e) => (state.eventKind === "all" || e.kind === state.eventKind) && (state.eventScope === "everything" || relevant(e))).slice(-300).reverse();
  $("timeline-empty").hidden = events.length > 0;
  const items = [];
  let day = "";
  for (const e of events) {
    const d = new Date(e.time).toDateString();
    if (d !== day) { day = d; items.push({ day: d === new Date().toDateString() ? "Today" : new Date(e.time).toLocaleDateString([], { weekday: "long", day: "numeric", month: "short" }) }); }
    items.push(e);
  }
  keyed(list, items, (x) => x.day ? "d" + x.day : "e" + x.id,
    (x) => x.day ? h("li", { class: "day" }, x.day) : timelineItem(x),
    () => {});
}
function timelineItem(e) {
  const who = clean(e.project || e.process || "unknown process");
  return h("li", { class: e.kind },
    h("span", { class: "dot" }, icon(e.kind === "opened" ? "bolt" : "stop")),
    h("div", { class: "body", onclick: () => { $("filter").value = String(e.port); show("ports"); state.stateFilter = "all"; for (const x of $("state-filter").children) x.classList.toggle("active", x.dataset.state === "all"); renderPorts(); } },
      h("div", { class: "row" },
        h("b", {}, `${e.kind === "opened" ? "Opened" : "Closed"} ${e.protocol} :${e.port}`),
        h("time", { title: new Date(e.time).toLocaleString() }, clock(e.time))),
      h("small", {}, [who, e.framework, e.pid ? "PID " + e.pid : "", { all: "all interfaces", loopback: "loopback", address: "one address" }[e.scope]].filter(Boolean).join(" · ")),
      e.process ? h("button", { class: "icon-btn mute", title: `Mute ${e.process}: no notifications, hidden from Relevant`, onclick: (ev) => {
        ev.stopPropagation();
        saveSettings({ muted: [...state.settings.muted, e.process.toLowerCase()] }).then(() => { toast(`Muted ${e.process}`); $("timeline").replaceChildren(); renderTimeline(); });
      } }, icon("bell-off")) : null));
}
$("event-scope").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  state.eventScope = b.dataset.scope;
  for (const x of $("event-scope").children) x.classList.toggle("active", x === b);
  $("timeline").replaceChildren();
  renderTimeline();
});
$("event-filter").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  state.eventKind = b.dataset.kind;
  for (const x of $("event-filter").children) x.classList.toggle("active", x === b);
  $("timeline").replaceChildren();
  renderTimeline();
});

function renderWatch() {
  const list = $("watch-list");
  const watched = state.settings?.watched || [];
  keyed(list, watched, String, (p) => h("li"), (el, p) => {
    const owner = ownerOfPort(p);
    el.replaceChildren(
      h("strong", {}, ":" + p),
      h("small", { class: owner ? "up" : "down" }, owner ? "open · " + clean(owner.project || owner.process || "hidden") : "closed"),
      h("button", { class: "icon-btn", title: "Stop watching", onclick: () => saveSettings({ watched: watched.filter((x) => x !== p) }) }, icon("x")));
  });
}
$("watch-form").addEventListener("submit", (e) => {
  e.preventDefault();
  const p = Number($("watch-input").value.trim());
  if (!Number.isInteger(p) || p < 1 || p > 65535) { toast("Enter a port between 1 and 65535", true); return; }
  $("watch-input").value = "";
  saveSettings({ watched: [...state.settings.watched, p] });
});

/* ---------- Command palette ---------- */

let paletteSel = 0;
function paletteItems() {
  const q = $("palette-input").value.toLowerCase().trim();
  const terms = q.split(/\s+/).filter(Boolean);
  const seen = new Set();
  const out = [];
  for (const r of state.snap?.rows || []) {
    if (!r.listening) continue;
    const key = r.local_port + ":" + r.pid;
    if (seen.has(key) || !portMatches(r, terms)) continue;
    seen.add(key);
    out.push(r);
  }
  if (/^\d+$/.test(q)) out.sort((a, b) => (String(b.local_port).startsWith(q) - String(a.local_port).startsWith(q)) || a.local_port - b.local_port);
  else out.sort((a, b) => a.local_port - b.local_port);
  return out.slice(0, 40);
}
function renderPalette() {
  const items = paletteItems();
  paletteSel = Math.min(paletteSel, Math.max(0, items.length - 1));
  $("palette-list").replaceChildren(...items.map((r, i) => h("li", { class: i === paletteSel ? "sel" : "", onclick: () => { closePalette(); openDetails(r); } },
    h("strong", {}, ":" + r.local_port),
    h("span", {}, clean(r.project || r.process || "owner hidden") + (r.framework ? " · " + r.framework : "")),
    h("span", { class: "badge cat cat-" + r.category }, CATEGORIES[r.category]?.label || ""))));
  $("palette-list").__items = items;
  $("palette-list").children[paletteSel]?.scrollIntoView({ block: "nearest" });
}
function openPalette() {
  $("palette").hidden = false;
  $("palette-input").value = "";
  paletteSel = 0;
  renderPalette();
  setTimeout(() => $("palette-input").focus(), 10);
}
function closePalette() { $("palette").hidden = true; }
$("open-palette").addEventListener("click", openPalette);
$("palette").addEventListener("click", (e) => { if (e.target === $("palette")) closePalette(); });
$("palette-input").addEventListener("input", () => { paletteSel = 0; renderPalette(); });
$("palette-input").addEventListener("keydown", (e) => {
  const items = $("palette-list").__items || [];
  if (e.key === "ArrowDown") { paletteSel = Math.min(items.length - 1, paletteSel + 1); renderPalette(); e.preventDefault(); }
  else if (e.key === "ArrowUp") { paletteSel = Math.max(0, paletteSel - 1); renderPalette(); e.preventDefault(); }
  else if (e.key === "Enter" && items[paletteSel]) {
    const r = items[paletteSel];
    closePalette();
    if ((e.ctrlKey || e.metaKey) && r.url) openURL(r.url); else openDetails(r);
  } else if (e.key === "Escape") closePalette();
});

document.addEventListener("keydown", (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") { e.preventDefault(); $("palette").hidden ? openPalette() : closePalette(); return; }
  if (e.key === "Escape") { if (!$("palette").hidden) closePalette(); else if ($("drawer").classList.contains("open")) closeDrawer(); return; }
  const typing = /INPUT|TEXTAREA/.test(document.activeElement?.tagName);
  if (e.key === "/" && !typing) { e.preventDefault(); show("ports"); $("filter").focus(); }
  if (!typing && !e.ctrlKey && !e.metaKey && !e.altKey && /^[1-5]$/.test(e.key)) show(["overview", "ports", "map", "activity", "settings"][Number(e.key) - 1]);
});

document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-external]");
  if (!a) return;
  e.preventDefault();
  call("OpenURL", a.href).catch(() => window.open(a.href, "_blank"));
});

/* ---------- Data flow ---------- */

function onSnapshot(snap) {
  state.snap = snap;
  const live = $("live");
  live.classList.toggle("error", Boolean(snap.error));
  $("live-text").textContent = snap.error ? "Scan failed: " + snap.error : `Live · ${clock(snap.time)}`;
  if (snap.new_events?.length) {
    state.events.push(...snap.new_events);
    if (state.events.length > 1000) state.events = state.events.slice(-1000);
    const counted = snap.new_events.filter(relevant).length;
    if (state.view !== "activity" && counted) {
      state.unseen += counted;
      $("nav-activity").hidden = false;
      $("nav-activity").textContent = state.unseen > 99 ? "99+" : state.unseen;
    }
  }
  renderOverview();
  renderPorts();
  renderTimeline();
  renderWatch();
  updateFreeOwner();
  map.update(snap.rows);
  if (!map.paused) {
    const st = map.stats();
    $("map-processes").textContent = st.processes;
    $("map-links").textContent = st.links;
    $("map-remotes").textContent = st.remotes;
  }
  if (!$("palette").hidden) renderPalette();
}

Events.On("snapshot", (ev) => onSnapshot(ev.data));
Events.On("navigate", (ev) => show(ev.data));
Events.On("notify", (ev) => toast(ev.data.title + ": " + ev.data.body));

async function boot() {
  try {
    [state.settings, state.about, state.events] = await Promise.all([call("Settings"), call("About"), call("Events")]);
    state.events = state.events || [];
  } catch (e) {
    toast("portop could not start: " + e.message, true);
    return;
  }
  renderSettings();
  $("host").textContent = state.about.host;
  $("version").textContent = state.about.version;
  $("about-line").textContent = `${state.about.host} · ${state.about.os}/${state.about.arch}`;
  movePill();
  const snap = await call("Snapshot");
  if (snap?.time && !snap.time.startsWith("0001")) onSnapshot(snap);
}
boot();
