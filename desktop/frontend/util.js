// Small helpers shared by the views.

export const $ = (id) => document.getElementById(id);

// h builds an element: h("div", { class: "x", onclick }, child, "text").
export function h(tag, props, ...children) {
  const el = document.createElementNS(tag === "svg" || tag === "path" || tag === "use" || tag === "defs" || tag === "linearGradient" || tag === "stop" ? "http://www.w3.org/2000/svg" : "http://www.w3.org/1999/xhtml", tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "style" && typeof v === "object") {
      for (const [p, val] of Object.entries(v)) {
        if (p.startsWith("--")) el.style.setProperty(p, val); else el.style[p] = val;
      }
    }
    else if (k === "dataset") Object.assign(el.dataset, v);
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return el;
}

export const icon = (name) => {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", "#i-" + name);
  svg.append(use);
  return svg;
};

// Bidirectional and control characters in names could reorder what is
// shown; replace them.
export const clean = (v) => String(v ?? "").replace(/[\u0000-\u001f؜‎‏‪-‮⁦-⁩]/g, "�");

export function bytes(n) {
  if (!n || n < 1) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1000 && i < units.length - 1) { n /= 1000; i++; }
  return (i === 0 ? Math.round(n) : n.toFixed(n < 10 ? 1 : 0)) + " " + units[i];
}
export const rate = (n) => bytes(n) + "/s";

export function ago(date) {
  const s = Math.max(0, (Date.now() - new Date(date).getTime()) / 1000);
  if (s < 45) return "just now";
  if (s < 3600) return Math.round(s / 60) + " min ago";
  if (s < 86400) return Math.round(s / 3600) + " h ago";
  return Math.round(s / 86400) + " d ago";
}

export function uptime(date) {
  const s = Math.max(0, (Date.now() - new Date(date).getTime()) / 1000);
  if (s < 60) return Math.round(s) + " s";
  if (s < 3600) return Math.floor(s / 60) + " min";
  if (s < 86400) return Math.floor(s / 3600) + " h " + Math.floor((s % 3600) / 60) + " min";
  return Math.floor(s / 86400) + " d " + Math.floor((s % 86400) / 3600) + " h";
}

export const clock = (date) => new Date(date).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

// Frameworks get a color and a two-letter monogram (no trademarked logos).
const FRAMEWORKS = {
  "Vite": ["#a855f7", "Vi"], "Next.js": ["#94a3b8", "N"], "Nuxt": ["#22c55e", "Nu"], "Astro": ["#f97316", "As"],
  "Remix": ["#64748b", "Rx"], "React Router": ["#ef4444", "RR"], "SvelteKit": ["#ff5a1f", "Sv"], "Angular": ["#dd0031", "Ng"],
  "Gatsby": ["#7c3aed", "Ga"], "Docusaurus": ["#3ecc5f", "Do"], "VitePress": ["#5c73e7", "VP"], "Expo": ["#4f46e5", "Ex"],
  "Electron": ["#47848f", "El"], "NestJS": ["#e0234e", "Ne"], "Create React App": ["#22d3ee", "Re"], "webpack": ["#3b82f6", "Wp"],
  "Parcel": ["#e5a00d", "Pa"], "Express": ["#9ca3af", "Ex"], "Fastify": ["#64748b", "Fy"], "Hono": ["#ff6a00", "Ho"], "Koa": ["#6b7280", "Ko"],
  "Storybook": ["#ff4785", "Sb"], "Wrangler": ["#f6821f", "Wr"],
  "Django": ["#14b87e", "Dj"], "FastAPI": ["#059669", "Fa"], "Flask": ["#94a3b8", "Fl"], "Uvicorn": ["#10b981", "Uv"], "Gunicorn": ["#16a34a", "Gu"],
  "Hypercorn": ["#0d9488", "Hc"], "Streamlit": ["#ff4b4b", "St"], "Starlette": ["#0ea5e9", "Sl"], "aiohttp": ["#2563eb", "ai"],
  "Jupyter": ["#f37626", "Ju"], "Python http.server": ["#3b82f6", "Py"], "MkDocs": ["#526cfe", "Mk"],
  "Rails": ["#e11d48", "Ra"], "Puma": ["#f59e0b", "Pu"], "Sinatra": ["#b91c1c", "Si"], "Jekyll": ["#c2410c", "Je"],
  "Laravel": ["#f43f5e", "La"], "Symfony": ["#64748b", "Sy"], "Hugo": ["#ff4088", "Hu"], "Air": ["#06b6d4", "Ai"],
  "Axum": ["#f97316", "Ax"], "Actix Web": ["#ea580c", "Ac"], "Rocket": ["#dc2626", "Ro"], "Leptos": ["#ef4444", "Le"], "Tauri": ["#eab308", "Ta"], "Trunk": ["#f97316", "Tr"],
  "Spring Boot": ["#6db33f", "Sp"], "Ktor": ["#7f52ff", "Kt"], "ASP.NET Core": ["#8b5cf6", "As"], "Phoenix": ["#fd4f00", "Ph"],
};
const LANGUAGES = {
  "Node.js": ["#5fa04e", "JS"], "Deno": ["#64748b", "De"], "Go": ["#00add8", "Go"], "Rust": ["#dea584", "Rs"], "Python": ["#3776ab", "Py"],
  "Ruby": ["#cc342d", "Rb"], "PHP": ["#777bb4", "Ph"], "Java": ["#e76f00", "Jv"], "Kotlin": ["#7f52ff", "Kt"], ".NET": ["#512bd4", "C#"],
  "Elixir": ["#6e4a7e", "Ex"], "Dart": ["#0175c2", "Da"],
};
export function brand(row) {
  const f = FRAMEWORKS[row.framework] || LANGUAGES[row.language];
  if (f) return { color: f[0], mono: f[1] };
  const name = row.project || row.process || "?";
  let hash = 0;
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) | 0;
  const hues = ["#4cc2ff", "#34e3a4", "#a78bfa", "#f472b6", "#fbbf24", "#22d3ee", "#fb7185"];
  return { color: hues[Math.abs(hash) % hues.length], mono: name.replace(/[^a-z0-9]/gi, "").slice(0, 2).replace(/^./, (c) => c.toUpperCase()) || "?" };
}

export const CATEGORIES = {
  dev: { label: "Dev", icon: "code" },
  database: { label: "Database", icon: "db" },
  web: { label: "Web server", icon: "server" },
  container: { label: "Container", icon: "box" },
  system: { label: "System", icon: "cpu" },
  app: { label: "App", icon: "app" },
};
export const CATEGORY_ORDER = ["dev", "database", "web", "container", "system", "app"];

export function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

let reduceMotion = false;
export const setReduceMotion = (on) => { reduceMotion = on; };
export const motion = () => !reduceMotion && !matchMedia("(prefers-reduced-motion: reduce)").matches;

// countUp animates a number in el from its current value to target.
export function countUp(el, target) {
  const from = Number(el.dataset.value || 0);
  el.dataset.value = target;
  if (from === target) return;
  if (!motion()) { el.textContent = target; return; }
  const start = performance.now();
  const dur = 700;
  const step = (now) => {
    const t = Math.min(1, (now - start) / dur);
    const e = 1 - Math.pow(1 - t, 4);
    el.textContent = Math.round(from + (target - from) * e);
    if (t < 1 && el.dataset.value == target) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}

// keyed reconciles container's children with items, creating, updating,
// reordering and removing elements by key, and animating moves (FLIP),
// arrivals and departures.
export function keyed(container, items, keyOf, create, update) {
  const before = new Map();
  for (const el of container.children) {
    if (el.dataset.key && !el.classList.contains("leaving")) before.set(el.dataset.key, el.getBoundingClientRect().top);
  }
  const existing = new Map([...container.children].filter((el) => el.dataset.key && !el.classList.contains("leaving")).map((el) => [el.dataset.key, el]));
  const wanted = new Set();
  let prev = null;
  for (const item of items) {
    const key = keyOf(item);
    wanted.add(key);
    let el = existing.get(key);
    if (!el) {
      el = create(item);
      el.dataset.key = key;
      if (before.size && motion()) el.classList.add("entering");
    }
    update(el, item);
    const next = prev ? prev.nextSibling : container.firstChild;
    if (el !== next) container.insertBefore(el, next);
    prev = el;
  }
  for (const [key, el] of existing) {
    if (wanted.has(key)) continue;
    if (!motion()) { el.remove(); continue; }
    el.classList.add("leaving");
    el.style.position = "absolute"; el.style.width = el.offsetWidth + "px"; el.style.top = el.offsetTop + "px";
    setTimeout(() => el.remove(), 420);
  }
  if (!motion()) return;
  for (const el of container.children) {
    const top = before.get(el.dataset.key);
    if (top === undefined || el.classList.contains("leaving")) continue;
    const dy = top - el.getBoundingClientRect().top;
    if (Math.abs(dy) < 1) continue;
    el.animate([{ transform: `translateY(${dy}px)` }, { transform: "none" }], { duration: 450, easing: "cubic-bezier(0.22, 1.25, 0.36, 1)" });
  }
}
