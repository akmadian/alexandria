/* Alexandria design library.
   Reads ../tokens.json, generates --alx-* CSS variables per theme, renders every section.
   ponytail: client-side var generation is the interim generator — replaced by the real
   compiler when the pipeline lands (constitution §22); this file stays a pure renderer. */

const $ = (sel) => document.querySelector(sel);
const el = (tag, cls, html) => { const n = document.createElement(tag); if (cls) n.className = cls; if (html !== undefined) n.innerHTML = html; return n; };

let T; // tokens

/* ---------- color math: any CSS color -> sRGB via canvas, then APCA-W3 Lc ---------- */
const canvas = document.createElement("canvas");
canvas.width = canvas.height = 1;
const cx2d = canvas.getContext("2d", { willReadFrequently: true });
function toRGB(css) {
  cx2d.clearRect(0, 0, 1, 1);
  cx2d.fillStyle = "#000"; cx2d.fillStyle = css; // second assignment wins only if valid
  cx2d.fillRect(0, 0, 1, 1);
  const d = cx2d.getImageData(0, 0, 1, 1).data;
  return [d[0] / 255, d[1] / 255, d[2] / 255];
}
function screenY([r, g, b]) {
  const c = (x) => Math.pow(x, 2.4);
  let y = 0.2126729 * c(r) + 0.7151522 * c(g) + 0.0721750 * c(b);
  return y < 0.022 ? y + Math.pow(0.022 - y, 1.414) : y;
}
function apcaLc(txtCss, bgCss) {
  const Yt = screenY(toRGB(txtCss)), Yb = screenY(toRGB(bgCss));
  const s = Yb > Yt ? (Math.pow(Yb, 0.56) - Math.pow(Yt, 0.57)) * 1.14
                    : (Math.pow(Yb, 0.65) - Math.pow(Yt, 0.62)) * 1.14;
  if (Math.abs(s) < 0.035) return 0;
  return Math.abs((s > 0 ? s - 0.027 : s + 0.027) * 100);
}

/* ---------- token access ---------- */
const val = (node) => (node && typeof node === "object" && "$value" in node) ? node.$value : node;
const get = (path, themed = true) => {
  const root = themed ? T.theme[currentTheme()] : T;
  return path.split(".").reduce((n, k) => (n ? n[k] : undefined), root);
};
const currentTheme = () => document.documentElement.dataset.theme;
const pinOf = (node) => node?.$extensions?.alx?.pin ? " <span class='pin-flag' title='" + node.$extensions.alx.pin + "'>PIN</span>" : "";

/* ---------- CSS variable generation (the interim generator) ---------- */
function cssVarBlock(themeName) {
  const th = T.theme[themeName];
  const lines = [];
  for (const [g, group] of Object.entries(th)) {
    if (g.startsWith("$")) continue;
    for (const [k, tok] of Object.entries(group)) {
      if (k.startsWith("$")) continue;
      lines.push(`--alx-${g === "ink" ? "ink" : g}-${k}: ${val(tok)};`);
    }
  }
  return lines.join("");
}
function buildStyles() {
  const s = [];
  for (const t of Object.keys(T.theme)) s.push(`[data-theme="${t}"]{${cssVarBlock(t)}}`);
  const r = [];
  r.push(`--alx-stage: ${val(T.stage.default)};`);
  r.push(`--alx-accent: ${val(T.hue.accent)};`);
  r.push(`--alx-attention: ${val(T.hue.attention)};`);
  r.push(`--alx-error: ${val(T.hue.error)};`);
  for (const [k, v] of Object.entries(T.hue.label)) if (!k.startsWith("$")) r.push(`--alx-label-${k}: ${val(v)};`);
  r.push(`--alx-light-gradient: ${val(T.hue["light-register"].gradient)};`);
  r.push(`--alx-light-flow-duration: ${val(T.hue["light-register"]["flow-duration"])};`);
  r.push(`--alx-light-glass-blur: ${val(T.hue["light-register"]["glass-blur"])};`);
  for (const [k, f] of Object.entries(T.type.family)) if (!k.startsWith("$")) r.push(`--alx-font-${k === "sans" ? "sans" : k === "mono" ? "mono" : k}: ${val(f)};`);
  for (const [k, s2] of Object.entries(T.type.scale)) if (!k.startsWith("$")) {
    r.push(`--alx-type-${k}-size: ${s2.size}; --alx-type-${k}-lh: ${s2.lineHeight}; --alx-type-${k}-tracking: ${s2.tracking};`);
  }
  for (const [k, w] of Object.entries(T.type.weight)) r.push(`--alx-weight-${k}: ${w};`);
  for (const [k, d] of Object.entries(T.layout)) if (!k.startsWith("$") && val(d) !== undefined && typeof val(d) !== "object") r.push(`--alx-${k.replace(/^thumb-hairline-alpha$/, "thumb-hairline-alpha")}: ${val(d)};`);
  for (const [k, d] of Object.entries(T.radius)) if (!k.startsWith("$")) r.push(`--alx-r-${k === "docked" ? "docked" : k}: ${val(d)};`);
  for (const [k, d] of Object.entries(T.shadow)) if (!k.startsWith("$")) r.push(`--alx-shadow-${k}: ${val(d)};`);
  for (const [k, d] of Object.entries(T.motion.duration)) r.push(`--alx-dur-${k}: ${val(d)};`);
  for (const [k, d] of Object.entries(T.motion.easing)) r.push(`--alx-ease-${k}: ${val(d)};`);
  r.push(`--alx-focus-ring-width: ${val(T.focus["ring-width"])}; --alx-focus-ring-offset: ${val(T.focus["ring-offset"])};`);
  s.push(`:root{${r.join("")}}`);
  const tag = el("style"); tag.textContent = s.join("\n");
  document.head.appendChild(tag);
}

/* ---------- clipboard ---------- */
let toastTimer;
function copyVar(name) {
  navigator.clipboard?.writeText(`var(${name})`);
  const t = $("#toast"); t.hidden = false; t.textContent = `copied var(${name})`;
  clearTimeout(toastTimer); toastTimer = setTimeout(() => (t.hidden = true), 1400);
}

/* ---------- renderers ---------- */
function swatchRow(varName, cssValue, extra = "") {
  const row = el("div", "swatch");
  row.append(el("span", "chip"), el("span", "name", varName + extra), el("span", "val", cssValue));
  row.querySelector(".chip").style.background = `var(${varName})`;
  row.onclick = () => copyVar(varName);
  return row;
}

function renderSurfaces() {
  const wrap = $("#surface-ladders"); wrap.innerHTML = "";
  const th = T.theme[currentTheme()];
  const dirs = { chrome: T.family.chrome.direction[currentTheme()], cell: T.family.cell.direction[currentTheme()] };
  const groups = [
    ["chrome family — surfaces", "surface", Object.keys(th.surface).filter(k => !k.startsWith("$")), dirs.chrome],
    ["cell family", "cell", Object.keys(th.cell).filter(k => !k.startsWith("$")), dirs.cell],
    ["ink", "ink", Object.keys(th.ink).filter(k => !k.startsWith("$")), null],
  ];
  for (const [title, g, keys, dir] of groups) {
    const lad = el("div", "ladder");
    lad.append(el("h3", null, title + (dir ? ` <span class="pin-flag">direction: ${dir}</span>` : "")));
    for (const k of keys) lad.append(swatchRow(`--alx-${g}-${k}`, val(th[g][k]), pinOf(th[g][k])));
    wrap.append(lad);
  }
  const stage = el("div", "ladder");
  stage.append(el("h3", null, "stage (theme-independent)"));
  stage.append(swatchRow("--alx-stage", val(T.stage.default), pinOf(T.stage.default)));
  wrap.append(stage);
}

function renderInk() {
  const th = T.theme[currentTheme()];
  const rows = [];
  for (const c of T.contracts.text) {
    const inkKey = c.ink.split(".")[1];
    const inkVal = val(th.ink[inkKey]);
    for (const on of c.on) {
      const [g, k] = on.split(".");
      const bg = val(th[g]?.[k]); if (!bg) continue;
      const lc = apcaLc(inkVal, bg);
      const ok = lc >= c.minLc;
      rows.push(`<tr>
        <td class="mono">--alx-ink-${inkKey}</td><td class="mono">--alx-${g}-${k}</td>
        <td><span class="ink-sample" style="color:${inkVal};background:${bg}">metadata 1/250 sec ƒ/3.2</span></td>
        <td class="mono">≥ ${c.minLc}</td>
        <td class="${ok ? "lc-pass" : "lc-fail"}">Lc ${lc.toFixed(1)} ${ok ? "✓" : "✗ FAIL"}</td></tr>`);
    }
  }
  const oklchL = (css) => { const m = /oklch\(\s*([\d.]+)/.exec(css); return m ? parseFloat(m[1]) : NaN; };
  for (const s of T.contracts.separation) {
    const [aPath, bPath] = s.pair.map(p => p.split("."));
    const a = val(aPath[0] === "ink" ? th.ink[aPath[1]] : th[aPath[0]]?.[aPath[1]]);
    const b = val(th[bPath[0]]?.[bPath[1]]);
    if (!a || !b) continue;
    const dL = Math.abs(oklchL(a) - oklchL(b));
    const ok = dL >= s.deltaL[0] && dL <= s.deltaL[1];
    rows.push(`<tr><td class="mono">${s.pair[0]}</td><td class="mono">${s.pair[1]}</td>
      <td><span class="ink-sample" style="background:${b};box-shadow:inset 0 0 0 1px ${a}">separation</span></td>
      <td class="mono">ΔL ${s.deltaL[0]}–${s.deltaL[1]}</td>
      <td class="${ok ? "lc-pass" : "lc-fail"}">ΔL ${dL.toFixed(3)} ${ok ? "✓" : "✗ FAIL"}</td></tr>`);
  }
  $("#ink-table").innerHTML = `<table><thead><tr><th>ink</th><th>on surface</th><th>sample</th><th>contract</th><th>measured (APCA)</th></tr></thead><tbody>${rows.join("")}</tbody></table>`;
}

function renderCells() {
  const strip = $("#cell-demo"); strip.innerHTML = "";
  const states = [
    ["well", null, "the family base"],
    ["rest", "--alx-cell-rest", ""],
    ["hover", "--alx-cell-hover", ""],
    ["selected", "--alx-cell-selected", ""],
    ["active", "--alx-cell-active", "ceiling + ink frame"],
    ["focus", "--alx-cell-selected", "accent ring (keyboard)"],
    ["drop-target", null, "accent tint"],
    ["attention", "--alx-cell-rest", "glyph + hue"],
  ];
  for (const [name, varName, note] of states) {
    if (name === "well") continue;
    const c = el("div", "cell");
    if (varName) c.style.background = `var(${varName})`;
    if (name === "active") c.style.boxShadow = "inset 0 0 0 1px var(--alx-ink-2)";
    if (name === "focus") { c.style.outline = "var(--alx-focus-ring-width) solid var(--alx-accent)"; c.style.outlineOffset = "var(--alx-focus-ring-offset)"; }
    if (name === "drop-target") c.style.background = "color-mix(in oklch, var(--alx-accent) 14%, var(--alx-cell-rest))";
    const thumb = el("div", "thumb");
    const cap = el("div", "cap", `<span>150</span><span>★★★★</span>`);
    if (name === "attention") cap.innerHTML = `<span style="color:var(--alx-attention)">⚑ missing</span><span>150</span>`;
    c.append(thumb, cap, el("div", "statename", `<b>${name}</b> ${note}`));
    c.tabIndex = 0;
    strip.append(c);
  }
}

function renderLedger() {
  const rows = [];
  const sw = (css) => `<span class="ledger-swatch" style="background:${css}"></span>`;
  rows.push(`<tr><td>${sw("var(--alx-accent)")}accent</td><td class="mono">--alx-accent</td><td>focus ring · drop-target · toggles-on · links</td><td>outline-first; not user-swappable ${pinOf(T.hue.accent)}</td></tr>`);
  rows.push(`<tr><td>${sw("var(--alx-attention)")}attention</td><td class="mono">--alx-attention</td><td>missing · pending review · failed · offline · conflict</td><td>one hue; severity=rung; identity=glyph+word; user-picked ${pinOf(T.hue.attention)}</td></tr>`);
  rows.push(`<tr><td>${sw("var(--alx-error)")}error</td><td class="mono">--alx-error</td><td>invalid input</td><td>conventional; independent of attention</td></tr>`);
  const labels = Object.keys(T.hue.label).filter(k => !k.startsWith("$"));
  rows.push(`<tr><td>${labels.map(k => sw(`var(--alx-label-${k})`)).join("")}labels ×5</td><td class="mono">--alx-label-*</td><td>user judgment (LrC-compatible)</td><td>solid swatch style; user-toggleable off</td></tr>`);
  const tg = T.hue.tag;
  const chips = tg.hues.map(h =>
    `<span class="tag-chip" style="color:oklch(${tg.light.lightness} ${tg.light.chroma} ${h});background:oklch(${tg.light.lightness} ${tg.light.chroma} ${h} / ${tg.tintAlpha})">●&nbsp;${h}°</span>`).join("");
  rows.push(`<tr><td colspan="2">tag palette ×12 ${pinOf(tg)}<br>${chips}</td><td>user meaning (tags); attention picker source</td><td>fixed L/C per world; tinted-chip style</td></tr>`);
  rows.push(`<tr><td><button class="hero-btn"><span class="face">Import 1,204 assets</span></button></td><td class="mono">--alx-light-*</td><td>hero register: primary CTA · marquee progress</td><td>one per view; never adjacent to assets; static under reduced motion</td></tr>`);
  $("#ledger").innerHTML = `<table><thead><tr><th>row</th><th>tokens</th><th>meaning</th><th>constraint</th></tr></thead><tbody>${rows.join("")}</tbody></table>`;
}

function renderType() {
  const wrap = $("#type-specimens"); wrap.innerHTML = "";
  for (const [k, s] of Object.entries(T.type.scale)) {
    if (k.startsWith("$")) continue;
    for (const [w, wv] of Object.entries(T.type.weight)) {
      const row = el("div", "spec-row");
      row.append(
        el("div", "spec-meta", `--alx-type-${k} · ${w} ${wv}<br>${s.size}/${s.lineHeight} ${s.tracking}`),
        el("div", null, `<span style="font-size:${s.size};line-height:${s.lineHeight};letter-spacing:${s.tracking};font-weight:${wv}">Adams 2026 — 596 assets · 1/250 sec at ƒ/8.0, ISO 400, 121.8 mm</span>`)
      );
      wrap.append(row);
    }
  }
  const pair = el("div", "spec-row");
  pair.append(el("div", "spec-meta", "sans + mono pairing<br>(matched metrics — no jitter)"),
    el("div", null, `<span>Focal length</span>&nbsp;&nbsp;<code>28.3 mm</code>&nbsp;&nbsp;<span>Dimensions</span>&nbsp;&nbsp;<code>7728 × 5152</code>`));
  wrap.append(pair);
  const joy = el("div", "spec-row");
  joy.append(el("div", "spec-meta", "joy voices — §16 registers only"),
    el("div", null, `<span style="font-family:var(--alx-font-joy-serif);font-style:italic;font-size:28px">Nothing here yet — the library awaits.</span>&nbsp;&nbsp;<span style="font-family:var(--alx-font-joy-pixel);font-size:18px">DOT VOICE</span>`));
  wrap.append(joy);
}

function renderDensity() {
  const wrap = $("#density-demo"); wrap.innerHTML = "";
  const q = parseInt(val(T.layout.quantum));
  const bars = el("div", "quantum-bars");
  for (const m of [1, 2, 3, 4, 5, 6, 8]) { const b = el("div", null, `${m * q}`); b.style.height = `${m * q}px`; bars.append(b); }
  wrap.append(el("p", "note", `quantum ${val(T.layout.quantum)} — every spacing value a multiple; controls ${val(T.layout["control-sm"])}/${val(T.layout["control-md"])}; row ${val(T.layout.row)}; side inset ${val(T.layout["row-inset"])}`), bars);
  const tree = el("div", "tree-demo");
  const data = [["▸", "Adams 2026", "596"], ["▸", "Camp Muir — May 9 2026", "270"], ["▾", "Discovery Park w Dad", "297"], ["", "Selected row (register: selected)", "164"]];
  data.forEach(([g, n, c], i) => {
    const r = el("div", "tree-row", `<span class="glyph">${g}</span><span>${n}</span><span class="count">${c}</span>`);
    if (i === 3) r.dataset.selected = "";
    tree.append(r);
  });
  wrap.append(tree);
}

function renderScales() {
  const wrap = $("#scales-tables"); wrap.innerHTML = "";
  const radii = el("div", "ladder", `<h3>radius by detachment</h3>`);
  for (const k of ["docked", "control", "transient", "round"]) {
    radii.append(swatchRow(`--alx-r-${k}`, val(T.radius[k]), pinOf(T.radius[k])));
  }
  const z = el("div", "ladder", `<h3>z-order registry</h3><table>${Object.entries(T.z).filter(([k]) => !k.startsWith("$")).map(([k, v]) => `<tr><td class="mono">z.${k}</td><td class="mono">${v}</td></tr>`).join("")}</table>`);
  const shadows = el("div", "ladder", `<h3>the two shadows</h3>`);
  shadows.append(el("div", "demo-transient", "occlusion — a transient over the page"));
  const tun = el("div", "tunnel-demo");
  tun.append(el("div", "shade"));
  for (let i = 1; i <= 12; i++) tun.append(el("div", "tree-row", `<span>scrolled content passes under the chrome — the tunnel</span>`));
  shadows.append(el("div", null, "&nbsp;"), tun);
  const motion = el("div", "ladder", `<h3>motion tokens</h3>`);
  for (const [k, d] of Object.entries(T.motion.duration)) motion.append(el("span", "motion-chip mono", `${k} ${val(d)}`));
  motion.append(el("p", "note", `easing out: ${val(T.motion.easing.out)} — hover these chips`));
  wrap.append(radii, z, shadows, motion);
}

async function renderMachinery() {
  let pulse = "";
  try { pulse = await (await fetch("assets/pulse-rings.svg")).text(); } catch {}
  const rows = T.machinery.map.map(m =>
    `<tr><td class="mono">${m.state}</td><td>${m.category}</td><td class="mono">${m.loader}</td>
     <td>${m.loader === "pulse-rings" ? `<div class="loader">${pulse}</div>` : ""}</td></tr>`).join("");
  $("#machinery-table").innerHTML = `<table><thead><tr><th>domain state</th><th>loader category</th><th>loader</th><th>live</th></tr></thead><tbody>${rows}</tbody></table>
    <p class="note">Source: dot-matrix-animations.vercel.app — 60 loaders, 5×5 grid, reduced-motion built in. PIN = selection pending.</p>`;
}

function renderConventions() {
  $("#conventions-demo").innerHTML = `<table>
    <tr><td>filename (middle-truncate)</td><td class="mono" title="_DSF4926-Enhanced-NR-Edit-final.RAF">_DSF4926-En…-final.RAF</td></tr>
    <tr><td>name (end-truncate)</td><td title="Darrington — Boulder River Wilderness — Boulder Falls">Darrington — Boulder River Wild…</td></tr>
    <tr><td>counts</td><td class="mono">164 · 2,344 · 20.3k <span class="pin-flag">(exact ≤ 4 digits; hover exact)</span></td></tr>
    <tr><td>the readout</td><td class="mono">1,204 · 3 selected · _DSF4926.RAF</td></tr>
    <tr><td>mixed value</td><td class="mono">Rating&nbsp;&nbsp;—&nbsp;&nbsp;<span class="pin-flag">(varies across selection)</span></td></tr>
  </table>`;
}

function renderAll() {
  renderSurfaces(); renderInk(); renderCells(); renderLedger();
  renderType(); renderDensity(); renderScales(); renderMachinery(); renderConventions();
}

/* ---------- boot ---------- */
(async function boot() {
  T = await (await fetch("../tokens.json")).json();
  buildStyles();
  const accents = T.hue.accent.$extensions.alx.candidates;
  const ap = $("#accent-picker");
  accents.forEach((c) => { const o = el("option", null, c); o.value = c; ap.append(o); });
  ap.onchange = () => { document.documentElement.style.setProperty("--alx-accent", ap.value); };
  $("#theme-picker").onchange = (e) => { document.documentElement.dataset.theme = e.target.value; renderAll(); };
  renderAll();
})();
