// The Map view: a force-directed graph of this machine, its processes,
// their listening ports and the hosts they talk to, with particles
// flowing along busy connections.

import { brand, clean, rate, cssVar, motion } from "./util.js";

const CAT_VARS = { dev: "--violet", database: "--mint", web: "--azure", container: "--cyan", system: null, app: "--amber" };
const MAX_REMOTES = 60;

function isLocal(addr) {
  return !addr || addr === "::1" || addr.startsWith("127.") || addr === "0.0.0.0" || addr === "::" || addr.startsWith("::ffff:127.");
}

export class MapView {
  constructor(canvas, tip, { onSelect }) {
    this.canvas = canvas;
    this.ctx = canvas.getContext("2d");
    this.tip = tip;
    this.onSelect = onSelect;
    this.nodes = new Map();
    this.edges = [];
    this.particles = [];
    this.view = { x: 0, y: 0, k: 1 };
    this.alpha = 1;
    this.running = false;
    this.hover = null;
    this.localAddrs = new Set();
    this.query = "";
    this.paused = false;
    this.bind();
  }

  // search dims every node that does not match q (process, project,
  // port or host), and their links.
  search(q) { this.query = q.trim().toLowerCase(); }

  matches(n) {
    if (!this.query) return true;
    const r = n.row || {};
    return [n.label, n.sub, r.process, r.project, r.framework, r.local_port, r.pid].some((v) => String(v ?? "").toLowerCase().includes(this.query));
  }

  setPaused(p) { this.paused = p; }

  // reset scatters the nodes again and refits the view.
  reset() {
    for (const n of this.nodes.values()) {
      if (n.id === "host") continue;
      const a = Math.random() * Math.PI * 2, d = 80 + Math.random() * 220;
      n.x = Math.cos(a) * d; n.y = Math.sin(a) * d; n.vx = n.vy = 0;
    }
    this.alpha = 1;
    this.userView = false;
  }

  zoom(f) {
    const w = this.canvas.clientWidth / 2, h = this.canvas.clientHeight / 2;
    const [wx, wy] = this.toWorld(w, h);
    this.userView = true;
    this.view.k = Math.min(3, Math.max(0.3, this.view.k * f));
    this.view.x = w - wx * this.view.k; this.view.y = h - wy * this.view.k;
  }

  fitNow() { this.userView = false; }

  stats() {
    let processes = 0, remotes = 0;
    for (const n of this.nodes.values()) {
      if (n.kind === "proc") processes++;
      if (n.kind === "remote") remotes++;
    }
    return { processes, remotes, links: this.edges.filter((e) => e.kind === "local" || e.kind === "remote").length };
  }

  colorFor(cat) {
    const v = CAT_VARS[cat];
    return v ? cssVar(v) : "#8a96b5";
  }

  update(rows) {
    if (this.paused) return;
    // Local addresses: any address a socket on this machine listens on.
    this.localAddrs = new Set(rows.filter((r) => r.listening).map((r) => r.local_address));
    const want = new Map();
    const edges = [];
    const add = (id, props) => {
      if (!want.has(id)) want.set(id, props);
      return id;
    };
    add("host", { kind: "host", label: "this machine", r: 22 });

    const listenerByPort = new Map();
    for (const r of rows) {
      if (!r.listening) continue;
      // Sockets whose owner portop cannot see share one node.
      const pid = r.pid ? "p:" + r.pid : "hidden";
      const b = brand(r);
      // Dev servers go by their project; services by their process.
      const name = r.category === "dev" ? r.project || r.process : r.process || r.project;
      if (r.pid) add(pid, { kind: "proc", label: clean(name || "PID " + r.pid), sub: r.framework || r.process || "", cat: r.category, pid: r.pid, row: r, color: r.category === "dev" ? b.color : null, conns: 0 });
      else add(pid, { kind: "proc", label: "other users", sub: "owners hidden without administrator rights", cat: "system", pid: 0, row: null, conns: 0 });
      const portId = "l:" + r.protocol + ":" + r.local_port + ":" + (r.pid || 0);
      // One node per port: the IPv4 and IPv6 sockets of a server share it,
      // with the widest exposure.
      const known = want.get(portId);
      if (known) {
        if (r.scope === "all" || (r.scope === "address" && known.scope === "loopback")) known.scope = r.scope;
        continue;
      }
      const port = add(portId, { kind: "port", label: ":" + r.local_port, proto: r.protocol, scope: r.scope, row: r, owner: pid });
      edges.push({ a: pid, b: port, len: 50, k: 0.5, kind: "own" });
      listenerByPort.set(r.local_port, port);
    }
    for (const id of [...want.keys()]) {
      if (id.startsWith("p:") || id === "hidden") edges.push({ a: "host", b: id, len: 210, k: 0.02, kind: "host" });
    }

    const remotes = new Map();
    for (const r of rows) {
      if (r.state !== "ESTABLISHED" || !r.remote_port) continue;
      const from = r.pid ? "p:" + r.pid : null;
      if (from && !want.has(from)) {
        add(from, { kind: "proc", label: clean((r.category === "dev" ? r.project : "") || r.process || "PID " + r.pid), sub: r.process || "", cat: r.category, pid: r.pid, row: r, conns: 0 });
        edges.push({ a: "host", b: from, len: 240, k: 0.02, kind: "host" });
      }
      const traffic = (r.rx_rate || 0) + (r.tx_rate || 0);
      const local = isLocal(r.remote_address) || this.localAddrs.has(r.remote_address);
      if (local && listenerByPort.has(r.remote_port) && from) {
        // A local client talking to a local server: link the client to
        // the server's port; skip the server's own end of the pair.
        edges.push({ a: from, b: listenerByPort.get(r.remote_port), len: 110, k: 0.08, kind: "local", rx: r.rx_rate || 0, tx: r.tx_rate || 0 });
        continue;
      }
      if (local && listenerByPort.has(r.local_port)) continue;
      if (!from) continue;
      const host = r.remote_host || r.remote_address;
      const rem = remotes.get(host) || { traffic: 0, links: [] };
      rem.traffic += traffic;
      rem.links.push({ from, r });
      remotes.set(host, rem);
    }
    const top = [...remotes.entries()].sort((a, b) => b[1].traffic - a[1].traffic || b[1].links.length - a[1].links.length).slice(0, MAX_REMOTES);
    for (const [host, rem] of top) {
      const id = add("r:" + host, { kind: "remote", label: clean(host), sub: rem.links.length + " connection" + (rem.links.length > 1 ? "s" : "") });
      const byProc = new Map();
      for (const l of rem.links) {
        const e = byProc.get(l.from) || { rx: 0, tx: 0 };
        e.rx += l.r.rx_rate || 0; e.tx += l.r.tx_rate || 0;
        byProc.set(l.from, e);
      }
      for (const [from, t] of byProc) edges.push({ a: from, b: id, len: 150, k: 0.05, kind: "remote", rx: t.rx, tx: t.tx });
    }

    // Keep positions of nodes that stay; place new ones near a neighbour.
    const next = new Map();
    const w = this.canvas.clientWidth || 800, hgt = this.canvas.clientHeight || 600;
    let added = false;
    for (const [id, props] of want) {
      const old = this.nodes.get(id);
      if (old) { Object.assign(old, props); next.set(id, old); continue; }
      added = true;
      const anchor = edges.find((e) => e.b === id && next.get(e.a)) ;
      const a = anchor ? next.get(anchor.a) : null;
      const ang = Math.random() * Math.PI * 2;
      const dist = a ? 40 : id === "host" ? 0 : 120 + Math.random() * 80;
      next.set(id, { id, ...props, x: (a ? a.x : 0) + Math.cos(ang) * dist, y: (a ? a.y : 0) + Math.sin(ang) * dist, vx: 0, vy: 0, born: performance.now() });
    }
    const sockets = new Map();
    for (const r of rows) {
      const id = r.pid ? "p:" + r.pid : r.listening ? "hidden" : null;
      if (id) sockets.set(id, (sockets.get(id) || 0) + 1);
    }
    for (const n of next.values()) {
      if (n.kind === "proc") {
        n.conns = edges.filter((e) => (e.a === n.id || e.b === n.id) && (e.kind === "local" || e.kind === "remote")).length;
        n.ports = edges.filter((e) => e.a === n.id && e.kind === "own").length;
        n.sockets = sockets.get(n.id) || 0;
        n.caption = [n.row?.framework || "", n.conns ? `${n.conns} link${n.conns > 1 ? "s" : ""}` : ""].filter(Boolean).join(" · ");
      }
      if (n.kind === "remote") n.caption = n.sub;
      n.r = n.kind === "host" ? 24 : n.kind === "proc" ? 11 + Math.min(14, Math.sqrt(n.conns) * 3) : n.kind === "port" ? 5 : 7;
    }
    this.nodes = next;
    this.edges = edges.filter((e) => next.has(e.a) && next.has(e.b));
    if (added) this.alpha = Math.max(this.alpha, 0.6);
    if (!this.view.init) { this.view = { x: w / 2, y: hgt / 2, k: 1, init: true }; }
  }

  start() {
    if (this.running) return;
    this.running = true;
    this.resize();
    const loop = (t) => {
      if (!this.running) return;
      if (!this.paused) this.tick();
      this.draw(t);
      requestAnimationFrame(loop);
    };
    requestAnimationFrame(loop);
  }

  stop() { this.running = false; }

  resize() {
    const dpr = window.devicePixelRatio || 1;
    const w = this.canvas.clientWidth, h = this.canvas.clientHeight;
    if (!w || !h) return;
    this.canvas.width = w * dpr; this.canvas.height = h * dpr;
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    if (!this.view.init) this.view = { x: w / 2, y: h / 2, k: 1, init: true };
  }

  tick() {
    // Ports are not simulated: they sit on an arc above their process, so
    // they never cover its name and caption below.
    const nodes = [...this.nodes.values()].filter((n) => n.kind !== "port");
    // Keep the graph gently alive, but settle fast after changes.
    this.alpha = Math.max(0.02, this.alpha * 0.985);
    const a = this.alpha;
    for (let i = 0; i < nodes.length; i++) {
      const p = nodes[i];
      for (let j = i + 1; j < nodes.length; j++) {
        const q = nodes[j];
        let dx = q.x - p.x, dy = q.y - p.y;
        let d2 = dx * dx + dy * dy;
        if (d2 > 250000) continue;
        if (d2 < 1) { dx = Math.random() - 0.5; dy = Math.random() - 0.5; d2 = 1; }
        const d = Math.sqrt(d2);
        const charge = (p.kind === "port" || q.kind === "port" ? 500 : 2600) * a / d2;
        // Leave room for the labels under the nodes.
        const both = p.kind === "proc" && q.kind === "proc";
        const portProc = (p.kind === "port" && q.kind === "proc") || (q.kind === "port" && p.kind === "proc");
        const min = p.r + q.r + (both ? 100 : portProc ? 36 : p.kind === "port" || q.kind === "port" ? 22 : 50);
        const push = d < min ? (min - d) * 0.25 : 0;
        const fx = (dx / d) * (charge + push), fy = (dy / d) * (charge + push);
        p.vx -= fx; p.vy -= fy; q.vx += fx; q.vy += fy;
      }
    }
    for (const e of this.edges) {
      if (e.kind === "own") continue;
      let p = this.nodes.get(e.a), q = this.nodes.get(e.b);
      // A link to a port pulls the port's process.
      if (p.kind === "port") p = this.nodes.get(p.owner) || p;
      if (q.kind === "port") q = this.nodes.get(q.owner) || q;
      if (p === q) continue;
      const dx = q.x - p.x, dy = q.y - p.y;
      const d = Math.sqrt(dx * dx + dy * dy) || 1;
      const f = (d - e.len) * e.k * a;
      const fx = (dx / d) * f, fy = (dy / d) * f;
      p.vx += fx; p.vy += fy; q.vx -= fx; q.vy -= fy;
    }
    for (const n of nodes) {
      if (n.id === "host") { n.x = 0; n.y = 0; n.vx = n.vy = 0; continue; }
      if (n.fixed) { n.vx = n.vy = 0; continue; }
      n.vx -= n.x * 0.004 * a; n.vy -= n.y * 0.004 * a;
      n.vx *= 0.82; n.vy *= 0.82;
      n.x += n.vx; n.y += n.vy;
    }
    this.placePorts();
  }

  placePorts() {
    const byOwner = new Map();
    for (const n of this.nodes.values()) {
      if (n.kind !== "port") continue;
      const list = byOwner.get(n.owner) || [];
      list.push(n);
      byOwner.set(n.owner, list);
    }
    for (const [owner, ports] of byOwner) {
      const o = this.nodes.get(owner);
      if (!o) continue;
      ports.sort((a, b) => (a.row?.local_port || 0) - (b.row?.local_port || 0));
      const n = ports.length, spread = Math.min(Math.PI * 0.9, 0.55 * (n - 1));
      ports.forEach((p, i) => {
        const ang = -Math.PI / 2 + (n === 1 ? 0 : -spread / 2 + (spread * i) / (n - 1));
        const dist = o.r + 22;
        p.x = o.x + Math.cos(ang) * dist; p.y = o.y + Math.sin(ang) * dist;
      });
    }
  }

  // fit eases the view so the whole graph is visible, until the user
  // pans or zooms.
  fit() {
    if (this.userView || !this.nodes.size) return;
    const w = this.canvas.clientWidth, h = this.canvas.clientHeight;
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (const n of this.nodes.values()) {
      x0 = Math.min(x0, n.x - n.r - 60); x1 = Math.max(x1, n.x + n.r + 60);
      y0 = Math.min(y0, n.y - n.r - 20); y1 = Math.max(y1, n.y + n.r + 34);
    }
    const k = Math.min(1.4, Math.max(0.3, Math.min(w / (x1 - x0), h / (y1 - y0))));
    const tx = w / 2 - ((x0 + x1) / 2) * k, ty = h / 2 - ((y0 + y1) / 2) * k;
    const ease = motion() ? 0.08 : 1;
    this.view.k += (k - this.view.k) * ease;
    this.view.x += (tx - this.view.x) * ease;
    this.view.y += (ty - this.view.y) * ease;
  }

  draw(t) {
    this.fit();
    const ctx = this.ctx, w = this.canvas.clientWidth, h = this.canvas.clientHeight;
    const light = document.documentElement.dataset.theme === "light";
    ctx.clearRect(0, 0, w, h);
    ctx.save();
    ctx.translate(this.view.x, this.view.y);
    ctx.scale(this.view.k, this.view.k);
    let hl = this.hover ? this.neighbours(this.hover) : null;
    if (!hl && this.query) {
      hl = new Set();
      for (const n of this.nodes.values()) if (this.matches(n)) for (const id of this.neighbours(n.id)) hl.add(id);
    }

    for (const e of this.edges) {
      const p = this.nodes.get(e.a), q = this.nodes.get(e.b);
      const dim = hl && !(hl.has(e.a) && hl.has(e.b));
      const traffic = (e.rx || 0) + (e.tx || 0);
      const base = e.kind === "host" ? 0.06 : e.kind === "own" ? 0.35 : e.kind === "local" ? 0.55 : 0.28;
      ctx.globalAlpha = dim ? 0.04 : Math.min(0.9, base + (traffic > 0 ? 0.2 : 0));
      ctx.strokeStyle = e.kind === "local" ? cssVar("--violet") : e.kind === "remote" ? cssVar("--azure") : light ? "#5a6a90" : "#9fb0d8";
      ctx.lineWidth = (e.kind === "own" ? 1 : 0.9) + Math.min(2, Math.log10(1 + traffic / 1000) * 0.7);
      ctx.setLineDash(e.kind === "host" ? [3, 6] : []);
      ctx.beginPath();
      const mx = (p.x + q.x) / 2 - (q.y - p.y) * 0.08, my = (p.y + q.y) / 2 + (q.x - p.x) * 0.08;
      ctx.moveTo(p.x, p.y);
      ctx.quadraticCurveTo(mx, my, q.x, q.y);
      ctx.stroke();
      if (traffic > 0 && motion() && !dim) this.flow(ctx, p, q, mx, my, e, t);
    }
    ctx.setLineDash([]);
    ctx.globalAlpha = 1;

    for (const n of this.nodes.values()) {
      const dim = hl && !hl.has(n.id);
      const grow = Math.min(1, (performance.now() - (n.born || 0)) / 500);
      const r = n.r * (0.4 + 0.6 * (1 - Math.pow(1 - grow, 3)));
      ctx.globalAlpha = dim ? 0.18 : 1;
      if (n.kind === "host") {
        const g = ctx.createRadialGradient(0, 0, 4, 0, 0, r * 2.6);
        g.addColorStop(0, "rgba(76,194,255,0.55)"); g.addColorStop(1, "rgba(76,194,255,0)");
        ctx.fillStyle = g; ctx.beginPath(); ctx.arc(0, 0, r * 2.6 + Math.sin(t / 700) * 3, 0, Math.PI * 2); ctx.fill();
        ctx.fillStyle = light ? "#fff" : "#0b1426"; ctx.strokeStyle = cssVar("--azure"); ctx.lineWidth = 2.5;
        ctx.beginPath(); ctx.arc(0, 0, r, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
        ctx.fillStyle = cssVar("--azure"); ctx.font = "700 11px ui-monospace, monospace"; ctx.textAlign = "center"; ctx.textBaseline = "middle";
        ctx.fillText("HOST", 0, 0);
      } else if (n.kind === "proc") {
        const c = n.color || this.colorFor(n.cat);
        const g = ctx.createRadialGradient(n.x - r / 3, n.y - r / 3, 1, n.x, n.y, r);
        g.addColorStop(0, c); g.addColorStop(1, shade(c));
        ctx.shadowColor = c; ctx.shadowBlur = this.hover === n.id ? 18 : 6;
        ctx.fillStyle = g; ctx.beginPath(); ctx.arc(n.x, n.y, r, 0, Math.PI * 2); ctx.fill();
        ctx.shadowBlur = 0;
        ctx.strokeStyle = "rgba(255,255,255,0.35)"; ctx.lineWidth = 1; ctx.stroke();
        if (n.sockets > 1 && r > 8) {
          // Socket count badge.
          const t = String(n.sockets), bx = n.x + r * 0.72, by = n.y + r * 0.62;
          ctx.font = "600 9px ui-monospace, monospace";
          const bw = ctx.measureText(t).width + 7;
          ctx.fillStyle = light ? "#fff" : "#0b1426"; ctx.strokeStyle = c; ctx.lineWidth = 1;
          ctx.beginPath(); ctx.roundRect(bx - bw / 2, by - 6.5, bw, 13, 4); ctx.fill(); ctx.stroke();
          ctx.fillStyle = cssVar("--text"); ctx.textAlign = "center"; ctx.textBaseline = "middle";
          ctx.fillText(t, bx, by + 0.5);
        }
      } else if (n.kind === "port") {
        ctx.fillStyle = n.scope === "all" ? cssVar("--amber") : n.scope === "loopback" ? cssVar("--mint") : cssVar("--azure");
        ctx.beginPath(); ctx.arc(n.x, n.y, r, 0, Math.PI * 2); ctx.fill();
      } else {
        ctx.fillStyle = light ? "#fff" : "#0e1830"; ctx.strokeStyle = cssVar("--azure"); ctx.lineWidth = 1.5;
        ctx.beginPath(); ctx.roundRect(n.x - r, n.y - r, r * 2, r * 2, 3); ctx.fill(); ctx.stroke();
      }
    }
    // Labels last, on top of everything.
    ctx.textAlign = "center"; ctx.textBaseline = "top";
    for (const n of this.nodes.values()) {
      if (n.kind === "host") continue;
      if (n.kind === "port") {
        const dim = hl && !hl.has(n.id);
        if (this.view.k < 0.8 && !hl) continue;
        ctx.globalAlpha = dim ? 0.15 : 0.85;
        ctx.font = "11px ui-monospace, monospace";
        ctx.fillStyle = cssVar("--muted");
        ctx.textAlign = "left"; ctx.textBaseline = "middle";
        ctx.fillText(n.label, n.x + n.r + 4, n.y);
        ctx.textAlign = "center"; ctx.textBaseline = "top";
        continue;
      }
      const dim = hl && !hl.has(n.id);
      if (n.kind === "port" && this.view.k < 0.8 && !hl) continue;
      if (n.kind === "remote" && this.view.k < 0.6 && !hl) continue;
      ctx.globalAlpha = dim ? 0.15 : n.kind === "proc" ? 1 : 0.8;
      ctx.font = n.kind === "proc" ? "650 12px system-ui, sans-serif" : "11px ui-monospace, monospace";
      ctx.fillStyle = n.kind === "proc" ? cssVar("--text") : cssVar("--muted");
      const label = n.label.length > 26 ? n.label.slice(0, 25) + "…" : n.label;
      ctx.fillText(label, n.x, n.y + n.r + 5);
      if (n.caption && (n.kind === "proc" || (n.kind === "remote" && this.view.k >= 0.8)) && (!hl || hl.has(n.id))) {
        ctx.font = "10.5px ui-monospace, monospace";
        ctx.fillStyle = cssVar("--faint");
        ctx.fillText(n.caption, n.x, n.y + n.r + 21);
      }
    }
    ctx.restore();
    ctx.globalAlpha = 1;
  }

  // flow draws particles moving along an edge, faster and denser with
  // more traffic: towards the process for received bytes, away for sent.
  flow(ctx, p, q, mx, my, e, t) {
    const dirs = [];
    if (e.rx > 0) dirs.push({ amount: e.rx, from: q, to: p, color: cssVar("--mint") });
    if (e.tx > 0) dirs.push({ amount: e.tx, from: p, to: q, color: cssVar("--azure") });
    for (const d of dirs) {
      const lvl = Math.min(1, Math.log10(1 + d.amount) / 7);
      const count = 1 + Math.round(lvl * 4);
      const speed = 0.00025 + lvl * 0.0006;
      for (let i = 0; i < count; i++) {
        const s = ((t * speed + i / count + (e.a.length % 7) / 7) % 1);
        const [ax, ay, bx, by] = d.from === p ? [p.x, p.y, q.x, q.y] : [q.x, q.y, p.x, p.y];
        const x = (1 - s) * (1 - s) * ax + 2 * (1 - s) * s * mx + s * s * bx;
        const y = (1 - s) * (1 - s) * ay + 2 * (1 - s) * s * my + s * s * by;
        ctx.globalAlpha = 0.9;
        ctx.fillStyle = d.color; ctx.shadowColor = d.color; ctx.shadowBlur = 8;
        ctx.beginPath(); ctx.arc(x, y, 2 + lvl * 1.5, 0, Math.PI * 2); ctx.fill();
        ctx.shadowBlur = 0;
      }
    }
  }

  neighbours(id) {
    const s = new Set([id]);
    for (const e of this.edges) {
      if (e.kind === "host") continue;
      if (e.a === id) s.add(e.b);
      if (e.b === id) s.add(e.a);
    }
    // A process's ports bring their own neighbours along.
    for (const e of this.edges) if (e.kind !== "host" && s.has(e.a) && this.nodes.get(e.a)?.kind === "port") s.add(e.b);
    return s;
  }

  toWorld(x, y) { return [(x - this.view.x) / this.view.k, (y - this.view.y) / this.view.k]; }

  hit(x, y) {
    const [wx, wy] = this.toWorld(x, y);
    let best = null, bestD = Infinity;
    for (const n of this.nodes.values()) {
      const d = Math.hypot(n.x - wx, n.y - wy);
      if (d < n.r + 6 && d < bestD) { best = n; bestD = d; }
    }
    return best;
  }

  bind() {
    const c = this.canvas;
    let drag = null;
    c.addEventListener("pointerdown", (ev) => {
      let n = this.hit(ev.offsetX, ev.offsetY);
      if (n?.kind === "port") n = this.nodes.get(n.owner) || n;
      drag = { n, x: ev.offsetX, y: ev.offsetY, vx: this.view.x, vy: this.view.y, moved: false };
      if (n && n.id !== "host") n.fixed = true;
      c.setPointerCapture(ev.pointerId);
      c.classList.add("dragging");
    });
    c.addEventListener("pointermove", (ev) => {
      if (drag) {
        const dx = ev.offsetX - drag.x, dy = ev.offsetY - drag.y;
        if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true;
        if (drag.n && drag.n.id !== "host") {
          const [wx, wy] = this.toWorld(ev.offsetX, ev.offsetY);
          drag.n.x = wx; drag.n.y = wy;
          this.placePorts();
          this.alpha = Math.max(this.alpha, 0.3);
        } else {
          this.userView = true;
          this.view.x = drag.vx + dx; this.view.y = drag.vy + dy;
        }
        return;
      }
      const n = this.hit(ev.offsetX, ev.offsetY);
      this.hover = n && n.kind !== "host" ? n.id : null;
      c.style.cursor = n ? "pointer" : "grab";
      this.showTip(n, ev.offsetX, ev.offsetY);
    });
    const end = () => {
      if (!drag) return;
      if (drag.n) drag.n.fixed = false;
      if (!drag.moved && drag.n?.row && drag.n.kind !== "remote" && drag.n.kind !== "host") this.onSelect(drag.n.row);
      drag = null;
      c.classList.remove("dragging");
    };
    c.addEventListener("pointerup", end);
    c.addEventListener("pointercancel", end);
    c.addEventListener("pointerleave", () => { this.hover = null; this.tip.hidden = true; });
    c.addEventListener("dblclick", () => { this.userView = false; });
    c.addEventListener("wheel", (ev) => {
      ev.preventDefault();
      this.userView = true;
      const k = Math.min(3, Math.max(0.3, this.view.k * Math.exp(-ev.deltaY * 0.0015)));
      const [wx, wy] = this.toWorld(ev.offsetX, ev.offsetY);
      this.view.k = k;
      this.view.x = ev.offsetX - wx * k; this.view.y = ev.offsetY - wy * k;
    }, { passive: false });
    window.addEventListener("resize", () => this.resize());
  }

  showTip(n, x, y) {
    if (!n || n.kind === "host") { this.tip.hidden = true; return; }
    const r = n.row;
    let title = n.label, sub = "";
    if (n.kind === "proc") sub = [n.sub, r?.pid ? "PID " + r.pid : "owner hidden", n.conns ? n.conns + " links" : ""].filter(Boolean).join(" · ");
    if (n.kind === "port") { title = `${n.proto} ${n.label}`; sub = { all: "all interfaces", loopback: "loopback only", address: "one address" }[n.scope] || ""; }
    if (n.kind === "remote") sub = n.sub;
    const edgeRate = this.edges.filter((e) => e.a === n.id || e.b === n.id).reduce((s, e) => s + (e.rx || 0) + (e.tx || 0), 0);
    if (edgeRate > 0) sub += (sub ? " · " : "") + rate(edgeRate);
    this.tip.replaceChildren();
    const b = document.createElement("b"); b.textContent = title;
    const s = document.createElement("small"); s.textContent = sub;
    this.tip.append(b, s);
    this.tip.hidden = false;
    const maxX = this.canvas.clientWidth - this.tip.offsetWidth - 8;
    this.tip.style.left = Math.min(maxX, x + 14) + "px";
    this.tip.style.top = y + 14 + "px";
  }
}

function shade(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
  if (!m) return hex;
  const n = parseInt(m[1], 16);
  const f = (v) => Math.round(v * 0.55).toString(16).padStart(2, "0");
  return "#" + f(n >> 16) + f((n >> 8) & 255) + f(n & 255);
}
