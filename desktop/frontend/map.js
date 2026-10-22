// The Connections view: a service map. Each process is a card; arrows go
// from a client to the server it talks to, so the cards fall into columns
// by role (entry points, applications, their dependencies, remote hosts).
// The layout comes from the structure, not a simulation: cards stay put
// between scans.

import { h, icon, brand, clean, rate, CATEGORIES } from "./util.js";

const CARD_W = 214, CARD_H = 66, COL_GAP = 120, ROW_GAP = 16, PAD = 40;
const MAX_REMOTES = 14;
const SVG = "http://www.w3.org/2000/svg";

function isLocal(addr, locals) {
  return !addr || addr === "::1" || addr.startsWith("127.") || addr.startsWith("::ffff:127.") || locals.has(addr);
}

function svg(tag, attrs) {
  const el = document.createElementNS(SVG, tag);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, v);
  return el;
}

export class MapView {
  constructor(root, tip, { onSelect }) {
    this.root = root;
    this.tip = tip;
    this.onSelect = onSelect;
    this.stage = h("div", { class: "map-stage" });
    this.edgesEl = svg("svg", { class: "map-edges" });
    this.cardsEl = h("div", { class: "map-cards" });
    this.stage.append(this.edgesEl, this.cardsEl);
    root.append(this.stage);
    this.cards = new Map();
    this.nodes = [];
    this.edges = [];
    this.view = { x: 0, y: 0, k: 1 };
    this.userView = false;
    this.paused = false;
    this.query = "";
    this.hover = null;
    this.size = { w: 0, h: 0 };
    this.bind();
  }

  // ---- data ----

  update(rows) {
    if (this.paused) return;
    const locals = new Set(rows.filter((r) => r.listening).map((r) => r.local_address));
    const nodes = new Map();
    const node = (id, init) => {
      if (!nodes.has(id)) nodes.set(id, { id, ports: new Set(), rx: 0, tx: 0, ...init });
      return nodes.get(id);
    };
    const procId = (r) => (r.pid ? "p:" + r.pid : "hidden");
    const procInit = (r) => r.pid
      ? { kind: "proc", row: r, category: r.category, name: r.category === "dev" ? r.project || r.process : r.process || r.project, sub: r.framework || "" }
      : { kind: "proc", row: null, category: "system", name: "Other users", sub: "owners hidden" };

    const listener = new Map(); // port -> node id
    for (const r of rows) {
      if (!r.listening) continue;
      const n = node(procId(r), procInit(r));
      n.ports.add(r.local_port);
      listener.set(r.local_port, n.id);
    }

    const links = new Map();
    const link = (from, to, kind, r) => {
      if (from === to) return;
      const key = from + ">" + to;
      const l = links.get(key) || { key, from, to, kind, rate: 0, count: 0 };
      l.rate += (r.rx_rate || 0) + (r.tx_rate || 0);
      l.count++;
      links.set(key, l);
    };
    const remotes = new Map();
    for (const r of rows) {
      if (r.state !== "ESTABLISHED" || !r.remote_port) continue;
      const self = procId(r);
      if (isLocal(r.remote_address, locals)) {
        // A local connection appears twice, once from each end. Only the
        // client's end counts: the one whose local port is not a listener
        // of its own process.
        if (listener.get(r.local_port) === self) continue;
        const server = listener.get(r.remote_port);
        if (!server || !r.pid) continue;
        node(self, procInit(r));
        link(self, server, "local", r);
        continue;
      }
      if (!r.pid) continue;
      const host = r.remote_host || r.remote_address;
      const rem = remotes.get(host) || { host, rate: 0, rows: [] };
      rem.rate += (r.rx_rate || 0) + (r.tx_rate || 0);
      rem.rows.push(r);
      remotes.set(host, rem);
      node(self, procInit(r));
    }
    const sorted = [...remotes.values()].sort((a, b) => b.rate - a.rate || b.rows.length - a.rows.length);
    const groups = sorted.slice(0, MAX_REMOTES).map((rem) => ({ id: "r:" + rem.host, name: rem.host, rows: rem.rows }));
    const rest = sorted.slice(MAX_REMOTES);
    if (rest.length) groups.push({ id: "r:more", name: `${rest.length} more hosts`, rows: rest.flatMap((r) => r.rows) });
    for (const g of groups) {
      node(g.id, { kind: "remote", name: g.name, sub: "", category: "remote" });
      for (const r of g.rows) {
        // A connection to one of this process's listeners came in from the
        // host; any other went out to it.
        if (listener.get(r.local_port) === procId(r)) link(g.id, procId(r), "remote", r);
        else link(procId(r), g.id, "remote", r);
      }
    }
    for (const r of rows) {
      const n = nodes.get(procId(r));
      if (!n) continue;
      n.rx += r.rx_rate || 0;
      n.tx += r.tx_rate || 0;
    }
    for (const n of nodes.values()) {
      if (n.kind !== "remote") continue;
      const c = [...links.values()].filter((l) => l.from === n.id || l.to === n.id).reduce((s, l) => s + l.count, 0);
      n.sub = `${c} connection${c === 1 ? "" : "s"}`;
    }

    this.nodes = [...nodes.values()];
    this.edges = [...links.values()];
    this.layout();
    this.render();
  }

  // ---- layout ----

  layout() {
    const out = new Map(), inc = new Map();
    for (const n of this.nodes) { out.set(n.id, []); inc.set(n.id, []); }
    for (const e of this.edges) { out.get(e.from).push(e.to); inc.get(e.to).push(e.from); }

    const connected = this.nodes.filter((n) => out.get(n.id).length || inc.get(n.id).length);
    const alone = this.nodes.filter((n) => !out.get(n.id).length && !inc.get(n.id).length);

    // Column = longest path from a source, bounded so cycles end.
    const depth = new Map(connected.map((n) => [n.id, 0]));
    for (let pass = 0; pass < connected.length; pass++) {
      let changed = false;
      for (const e of this.edges) {
        const d = depth.get(e.from) + 1;
        if (d > depth.get(e.to) && d < connected.length) { depth.set(e.to, d); changed = true; }
      }
      if (!changed) break;
    }
    // Remote hosts that only receive go to the last column.
    const maxProc = Math.max(0, ...connected.filter((n) => n.kind === "proc").map((n) => depth.get(n.id)));
    for (const n of connected) if (n.kind === "remote" && !out.get(n.id).length) depth.set(n.id, maxProc + 1);

    const cols = [];
    for (const n of connected) (cols[depth.get(n.id)] ||= []).push(n);
    const order = new Map();
    const place = (col) => col.forEach((n, i) => order.set(n.id, i));
    for (const col of cols) if (col) { col.sort((a, b) => (b.rx + b.tx) - (a.rx + a.tx) || a.name.localeCompare(b.name)); place(col); }
    // Barycenter sweeps: each card moves near the cards it links to.
    for (let sweep = 0; sweep < 3; sweep++) {
      for (const col of cols) {
        if (!col) continue;
        const bary = (n) => {
          const ns = [...inc.get(n.id), ...out.get(n.id)];
          return ns.length ? ns.reduce((s, id) => s + (order.get(id) ?? 0), 0) / ns.length : order.get(n.id);
        };
        const b = new Map(col.map((n) => [n.id, bary(n)]));
        col.sort((x, y) => b.get(x.id) - b.get(y.id));
        place(col);
      }
    }

    const tallest = Math.max(1, ...cols.map((c) => (c ? c.length : 0)));
    const graphH = tallest * (CARD_H + ROW_GAP) - ROW_GAP;
    let ci = 0;
    for (const col of cols) {
      if (!col) continue;
      const colH = col.length * (CARD_H + ROW_GAP) - ROW_GAP;
      col.forEach((n, i) => {
        n.x = PAD + ci * (CARD_W + COL_GAP);
        n.y = PAD + (graphH - colH) / 2 + i * (CARD_H + ROW_GAP);
      });
      ci++;
    }
    const graphW = ci ? PAD * 2 + ci * CARD_W + (ci - 1) * COL_GAP : 0;

    // Processes that talk to no other process: a grid under the map.
    const gridTop = connected.length ? PAD + graphH + 70 : PAD;
    const perRow = Math.max(1, Math.min(5, Math.floor((Math.max(graphW, 4 * (CARD_W + ROW_GAP))) / (CARD_W + ROW_GAP))));
    alone.sort((a, b) => a.category.localeCompare(b.category) || a.name.localeCompare(b.name));
    alone.forEach((n, i) => {
      n.x = PAD + (i % perRow) * (CARD_W + ROW_GAP);
      n.y = gridTop + Math.floor(i / perRow) * (CARD_H + ROW_GAP);
    });
    this.aloneTop = alone.length && connected.length ? gridTop - 26 : null;
    const aloneW = alone.length ? PAD * 2 + Math.min(alone.length, perRow) * (CARD_W + ROW_GAP) : 0;
    this.size = {
      w: Math.max(graphW, aloneW, 400),
      h: (alone.length ? gridTop + Math.ceil(alone.length / perRow) * (CARD_H + ROW_GAP) : PAD + graphH) + PAD,
    };
  }

  // ---- rendering ----

  render() {
    const { w, h: hgt } = this.size;
    this.stage.style.width = w + "px";
    this.stage.style.height = hgt + "px";
    this.edgesEl.setAttribute("width", w);
    this.edgesEl.setAttribute("height", hgt);
    this.edgesEl.setAttribute("viewBox", `0 0 ${w} ${hgt}`);

    // Cards are keyed, so they keep their element and slide when the
    // layout changes.
    const live = new Set();
    for (const n of this.nodes) {
      live.add(n.id);
      let el = this.cards.get(n.id);
      if (!el) {
        el = h("div", { class: "map-card entering", tabindex: "0" });
        el.addEventListener("click", () => { if (el.__n.row) this.onSelect(el.__n.row); });
        el.addEventListener("keydown", (e) => { if (e.key === "Enter" && el.__n.row) this.onSelect(el.__n.row); });
        el.addEventListener("pointerenter", () => { this.hover = el.__n.id; this.highlight(); });
        el.addEventListener("pointerleave", () => { this.hover = null; this.highlight(); });
        el.style.transform = `translate(${n.x}px, ${n.y}px)`;
        this.cardsEl.append(el);
        this.cards.set(n.id, el);
        setTimeout(() => el.classList.remove("entering"), 400);
      }
      el.__n = n;
      this.fillCard(el, n);
      el.style.transform = `translate(${n.x}px, ${n.y}px)`;
    }
    for (const [id, el] of this.cards) if (!live.has(id)) { el.remove(); this.cards.delete(id); }

    const defs = svg("defs");
    for (const kind of ["local", "remote"]) {
      const m = svg("marker", { id: "arrow-" + kind, viewBox: "0 0 10 10", refX: "8.5", refY: "5", markerWidth: "8", markerHeight: "8", markerUnits: "userSpaceOnUse", orient: "auto" });
      m.append(svg("path", { d: "M0,0.5 L9,5 L0,9.5 z", class: "arrow " + kind }));
      defs.append(m);
    }
    const parts = [defs];
    if (this.aloneTop !== null) {
      const t = svg("text", { x: PAD, y: this.aloneTop, class: "map-section" });
      t.textContent = "Not connected to other processes";
      parts.push(t);
    }
    const byId = new Map(this.nodes.map((n) => [n.id, n]));
    // Fan the arrows out on each side of a card instead of piling them
    // on one point.
    const slots = (id, side) => this.edges.filter((e) => (side === "out" ? e.from : e.to) === id);
    const offset = (id, side, e) => {
      const list = slots(id, side).sort((a, b) => (byId.get(side === "out" ? a.to : a.from)?.y ?? 0) - (byId.get(side === "out" ? b.to : b.from)?.y ?? 0));
      const i = list.indexOf(e), n = list.length;
      return n < 2 ? 0 : (i - (n - 1) / 2) * Math.min(12, (CARD_H - 20) / (n - 1));
    };
    for (const e of this.edges) {
      const a = byId.get(e.from), b = byId.get(e.to);
      if (!a || !b) continue;
      const x1 = a.x + CARD_W, y1 = a.y + CARD_H / 2 + offset(a.id, "out", e);
      const x2 = b.x - 2, y2 = b.y + CARD_H / 2 + offset(b.id, "in", e);
      let d;
      if (x2 > x1 + 20) {
        const dx = (x2 - x1) / 2;
        d = `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`;
      } else {
        // A link against the flow (a cycle) loops under the cards.
        const low = Math.max(a.y, b.y) + CARD_H + 36;
        d = `M${a.x + CARD_W / 2},${a.y + CARD_H} C${a.x + CARD_W / 2},${low} ${b.x + CARD_W / 2},${low} ${b.x + CARD_W / 2},${b.y + CARD_H + 2}`;
      }
      const width = 1.5 + Math.min(2.5, Math.log10(1 + e.rate / 1000) * 0.8);
      const g = svg("g", { class: `map-edge ${e.kind}${e.rate > 0 ? " busy" : ""}`, "data-from": e.from, "data-to": e.to });
      g.append(svg("path", { d, class: "line", "stroke-width": width, "marker-end": `url(#arrow-${e.kind})` }));
      if (e.rate > 0) g.append(svg("path", { d, class: "flow", "stroke-width": width }));
      if (e.rate > 0 && x2 > x1 + 20) {
        const label = rate(e.rate);
        const mx = (x1 + x2) / 2, my = (y1 + y2) / 2;
        const tw = label.length * 6.3 + 14;
        g.append(svg("rect", { x: mx - tw / 2, y: my - 10, width: tw, height: 20, rx: 10, class: "rate-pill" }));
        const t = svg("text", { x: mx, y: my + 4, class: "rate-text" });
        t.textContent = label;
        g.append(t);
      }
      const title = svg("title");
      title.textContent = `${a.name} → ${b.name}: ${e.count} connection${e.count === 1 ? "" : "s"}${e.rate > 0 ? ", " + rate(e.rate) : ""}`;
      g.append(title);
      parts.push(g);
    }
    this.edgesEl.replaceChildren(...parts);
    this.highlight();
    this.fit();
  }

  fillCard(el, n) {
    const ports = [...n.ports].sort((a, b) => a - b);
    const traffic = n.rx + n.tx;
    const sig = JSON.stringify([n.kind, n.name, n.sub, n.category, ports, traffic > 0 ? rate(traffic) : ""]);
    if (el.__sig === sig) return;
    el.__sig = sig;
    const r = n.row || {};
    el.className = `map-card cat-${n.category}${n.kind === "remote" ? " remote" : ""}${n.row ? "" : " static"}${el.classList.contains("entering") ? " entering" : ""}`;
    let badge;
    if (n.kind === "remote") badge = h("span", { class: "map-icon" }, icon("globe"));
    else if (n.category === "dev" && n.row) {
      const b = brand(r);
      badge = h("span", { class: "map-icon mono", style: { "--fw": b.color } }, b.mono);
    } else badge = h("span", { class: "map-icon" }, icon(n.row ? CATEGORIES[n.category]?.icon || "app" : "lock"));
    const sub = n.kind === "remote" ? n.sub : [n.sub, n.row ? "PID " + r.pid : ""].filter(Boolean).join(" · ");
    el.replaceChildren(
      badge,
      h("div", { class: "map-text" },
        h("b", { title: n.name }, clean(n.name || "unknown")),
        h("small", {}, sub || CATEGORIES[n.category]?.label || ""),
        h("div", { class: "map-meta" },
          ports.length ? h("span", { class: "map-ports" }, ports.slice(0, 3).map((p) => ":" + p).join(" ") + (ports.length > 3 ? ` +${ports.length - 3}` : "")) : null,
          traffic > 0 ? h("span", { class: "map-rate" }, rate(traffic)) : null)),
    );
  }

  // highlight dims everything but the hovered card and its links, or the
  // cards matching the search.
  highlight() {
    let keep = null;
    if (this.hover) {
      keep = new Set([this.hover]);
      for (const e of this.edges) {
        if (e.from === this.hover) keep.add(e.to);
        if (e.to === this.hover) keep.add(e.from);
      }
    } else if (this.query) {
      keep = new Set(this.nodes.filter((n) => this.matches(n)).map((n) => n.id));
    }
    for (const [id, el] of this.cards) el.classList.toggle("dim", Boolean(keep) && !keep.has(id));
    for (const g of this.edgesEl.querySelectorAll(".map-edge")) {
      const touches = !this.hover || g.dataset.from === this.hover || g.dataset.to === this.hover;
      g.classList.toggle("dim", Boolean(keep) && !(keep.has(g.dataset.from) && keep.has(g.dataset.to) && touches));
    }
  }

  matches(n) {
    const r = n.row || {};
    return [n.name, n.sub, r.process, r.project, r.framework, r.pid, ...n.ports].some((v) => String(v ?? "").toLowerCase().includes(this.query));
  }

  // ---- view ----

  search(q) { this.query = q.trim().toLowerCase(); this.highlight(); }
  setPaused(p) { this.paused = p; }
  start() { this.fit(); }
  stop() {}
  resize() { this.fit(); }
  fitNow() { this.userView = false; this.fit(); }

  fit() {
    if (this.userView) { this.apply(); return; }
    const w = this.root.clientWidth, hgt = this.root.clientHeight;
    if (!w || !hgt || !this.size.w) return;
    const k = Math.min(1, Math.max(0.35, Math.min(w / this.size.w, hgt / this.size.h)));
    this.view = { k, x: Math.max(0, (w - this.size.w * k) / 2), y: Math.max(0, (hgt - this.size.h * k) / 2) };
    this.apply();
  }

  zoom(f, cx, cy) {
    const w = this.root.clientWidth, hgt = this.root.clientHeight;
    cx ??= w / 2; cy ??= hgt / 2;
    const k = Math.min(2, Math.max(0.3, this.view.k * f));
    this.view.x = cx - ((cx - this.view.x) / this.view.k) * k;
    this.view.y = cy - ((cy - this.view.y) / this.view.k) * k;
    this.view.k = k;
    this.userView = true;
    this.apply();
  }

  apply() {
    this.stage.style.transform = `translate(${this.view.x}px, ${this.view.y}px) scale(${this.view.k})`;
  }

  stats() {
    return {
      processes: this.nodes.filter((n) => n.kind === "proc").length,
      remotes: this.nodes.filter((n) => n.kind === "remote").length,
      links: this.edges.length,
    };
  }

  bind() {
    let drag = null;
    this.root.addEventListener("pointerdown", (e) => {
      if (e.target.closest(".map-card, .map-zoom")) return;
      drag = { x: e.clientX, y: e.clientY, vx: this.view.x, vy: this.view.y };
      this.root.setPointerCapture(e.pointerId);
      this.root.classList.add("dragging");
    });
    this.root.addEventListener("pointermove", (e) => {
      if (!drag) return;
      this.userView = true;
      this.view.x = drag.vx + e.clientX - drag.x;
      this.view.y = drag.vy + e.clientY - drag.y;
      this.apply();
    });
    const end = () => { drag = null; this.root.classList.remove("dragging"); };
    this.root.addEventListener("pointerup", end);
    this.root.addEventListener("pointercancel", end);
    this.root.addEventListener("wheel", (e) => {
      e.preventDefault();
      const box = this.root.getBoundingClientRect();
      this.zoom(Math.exp(-e.deltaY * 0.0015), e.clientX - box.left, e.clientY - box.top);
    }, { passive: false });
    this.root.addEventListener("dblclick", (e) => { if (!e.target.closest(".map-card")) this.fitNow(); });
    window.addEventListener("resize", () => this.fit());
  }
}
