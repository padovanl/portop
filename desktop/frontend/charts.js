// SVG charts: a two-series area chart for traffic and a sparkline.

const NS = "http://www.w3.org/2000/svg";
let gradientId = 0;

function smooth(points) {
  // Catmull-Rom to Bezier, for a soft line through every point.
  if (points.length < 2) return "";
  let d = `M${points[0][0]},${points[0][1]}`;
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i - 1] || points[i], p1 = points[i], p2 = points[i + 1], p3 = points[i + 2] || p2;
    const c1 = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6];
    const c2 = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6];
    d += ` C${c1[0].toFixed(1)},${c1[1].toFixed(1)} ${c2[0].toFixed(1)},${c2[1].toFixed(1)} ${p2[0].toFixed(1)},${p2[1].toFixed(1)}`;
  }
  return d;
}

function ensureGradient(svg, color) {
  svg.__grads = svg.__grads || {};
  if (svg.__grads[color]) return svg.__grads[color];
  let defs = svg.querySelector("defs");
  if (!defs) { defs = document.createElementNS(NS, "defs"); svg.prepend(defs); }
  const id = "g" + ++gradientId;
  const g = document.createElementNS(NS, "linearGradient");
  g.setAttribute("id", id); g.setAttribute("x1", "0"); g.setAttribute("x2", "0"); g.setAttribute("y1", "0"); g.setAttribute("y2", "1");
  for (const [off, op] of [["0", "0.42"], ["1", "0"]]) {
    const s = document.createElementNS(NS, "stop");
    s.setAttribute("offset", off); s.setAttribute("stop-color", color); s.setAttribute("stop-opacity", op);
    g.append(s);
  }
  defs.append(g);
  svg.__grads[color] = id;
  return id;
}

// area draws series ([{values, color}]) into svg, scaled to a shared max.
export function area(svg, series, { width = 300, height = 70, pad = 3, min = 1 } = {}) {
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);
  const n = Math.max(...series.map((s) => s.values.length), 2);
  const max = Math.max(min, ...series.flatMap((s) => s.values)) * 1.15;
  series.forEach((s, i) => {
    const pts = s.values.map((v, j) => [(width * (j + n - s.values.length)) / (n - 1), height - pad - (v / max) * (height - pad * 2)]);
    if (pts.length === 1) pts.unshift([0, pts[0][1]]);
    const line = smooth(pts);
    let fill = svg.querySelector(`path[data-fill="${i}"]`);
    let stroke = svg.querySelector(`path[data-line="${i}"]`);
    if (!fill) {
      fill = document.createElementNS(NS, "path"); fill.dataset.fill = i;
      stroke = document.createElementNS(NS, "path"); stroke.dataset.line = i;
      svg.append(fill, stroke);
    }
    const grad = ensureGradient(svg, s.color);
    fill.setAttribute("fill", `url(#${grad})`); fill.setAttribute("stroke", "none");
    stroke.setAttribute("fill", "none"); stroke.setAttribute("stroke", s.color);
    stroke.setAttribute("vector-effect", "non-scaling-stroke");
    if (!line) { fill.setAttribute("d", ""); stroke.setAttribute("d", ""); return; }
    stroke.setAttribute("d", line);
    fill.setAttribute("d", `${line} L${width},${height} L0,${height} Z`);
    if (!stroke.dataset.drawn) {
      stroke.dataset.drawn = "1";
      const len = stroke.getTotalLength?.() || 600;
      stroke.animate([{ strokeDasharray: `${len}`, strokeDashoffset: len }, { strokeDasharray: `${len}`, strokeDashoffset: 0 }], { duration: 1100, easing: "cubic-bezier(0.2,0.8,0.2,1)" });
    }
  });
}

// spark is a one-line chart sized to its element.
export function spark(values, color, cls = "spark") {
  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("class", cls);
  svg.setAttribute("preserveAspectRatio", "none");
  area(svg, [{ values: values.length ? values : [0, 0], color }], { width: 120, height: 26, pad: 2, min: 512 });
  return svg;
}

export function updateSpark(svg, values, color) {
  area(svg, [{ values: values.length ? values : [0, 0], color }], { width: 120, height: 26, pad: 2, min: 512 });
}
