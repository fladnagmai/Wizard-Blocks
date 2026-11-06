// Wizard-Blocks simple UI: one stats page over /api/state, refreshed every
// 5 seconds, with a faint binary rain behind it. No dependencies.
'use strict';

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);

// ---------- formatting ----------
function fmtHash(h) {
  if (!(h > 0)) return '0 H/s';
  const u = ['H/s', 'kH/s', 'MH/s', 'GH/s', 'TH/s', 'PH/s', 'EH/s'];
  let i = 0;
  while (h >= 1000 && i < u.length - 1) { h /= 1000; i++; }
  return `${h >= 100 ? h.toFixed(1) : h.toFixed(2)} ${u[i]}`;
}
function fmtDiff(d) {
  if (!(d > 0)) return '—';
  if (d < 1) return d.toExponential(2); // regtest / tiny test difficulties
  const u = ['', 'K', 'M', 'G', 'T', 'P', 'E'];
  let i = 0;
  while (d >= 1000 && i < u.length - 1) { d /= 1000; i++; }
  return d.toFixed(d >= 100 ? 1 : 2) + u[i];
}
const fmtInt = (n) => (n ?? 0).toLocaleString('en-US');
function ago(iso) {
  const t = Date.parse(iso || '');
  if (!t) return '—';
  const s = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (s < 120) return `${s} s ago`;
  if (s < 7200) return `${Math.round(s / 60)} min ago`;
  if (s < 172800) return `${Math.round(s / 3600)} h ago`;
  return `${Math.round(s / 86400)} d ago`;
}
function set(id, text, bad = false) { const el = $(id); el.textContent = text; el.classList.toggle('bad', bad); }

// ---------- live state ----------
async function refresh() {
  try {
    const res = await fetch('api/state', { cache: 'no-store' });
    if (!res.ok) throw new Error('HTTP ' + res.status);
    render(await res.json());
    set('status', 'live · ' + new Date().toLocaleTimeString());
  } catch (e) {
    set('status', 'engine unreachable: ' + e.message, true);
  }
}

function fmtSecs(v) {
  if (!(v > 0)) return '—';
  if (v < 120) return `${v.toFixed(v < 10 ? 1 : 0)} s`;
  if (v < 7200) return `${(v / 60).toFixed(0)} min`;
  if (v < 172800) return `${(v / 3600).toFixed(1)} h`;
  if (v < 63072000) return `${(v / 86400).toFixed(1)} d`;
  return `${(v / 31536000).toFixed(1)} y`;
}
const pct = (v) => (v == null ? '—' : v < 0.01 ? (v * 100).toExponential(1) + ' %' : (v * 100).toFixed(2) + ' %');
// Chance of a block as odds "1:N" (v is a probability, 0..1).
const odds = (v) => {
  if (v == null || !isFinite(v) || v <= 0) return '—';
  const n = Math.max(1, Math.round(1 / v));
  return n >= 1e12 ? '1:' + (1 / v).toExponential(2).replace('e+', 'E') : '1:' + n.toLocaleString('en-US');
};

// Banner line for an aux chain (Dogecoin, Pepecoin) that is not being
// merge-mined right now; the parent and the other chains carry on.
function bannerText(name, ticker, reason) {
  switch (reason) {
    case 'no address': return `${name} is not merged: add a ${ticker} payout address in Settings to also mine ${name}.`;
    case 'node down': return `${name} is not merged: its node is down. ${name} mining resumes on its own.`;
    case 'node has no peers': return `${name} is not merged: its node has no peers (sync paused?). ${name} mining resumes when it reconnects.`;
    case 'node syncing': return `${name} is not merged: its node is syncing. ${name} mining resumes on its own.`;
    default: return `${name} is not merged: ${ticker} merged mining is unavailable (${reason || 'unknown'}).`;
  }
}

// Network difficulty as the nodes and explorers show it. The engine keeps
// Scrypt difficulty in share units (65536 x node units, the unit miners'
// share difficulty is in), so LTC/DOGE/PEP figures are divided back.
let diffScale = 1;
const netDiff = (d) => fmtDiff(d / diffScale);

function renderChain(prefix, title, v, isAux) {
  set(prefix + '-status', isAux ? (v.merged ? 'merged' : 'not merged: ' + (v.reason || '—')) : (!v.connected ? 'no node connection' : v.synced ? 'synced' : 'syncing'), isAux ? !v.merged : !v.connected || !v.synced);
  $(prefix + '-title').textContent = title;
  set(prefix + '-height', v.height ? fmtInt(v.height) : '—');
  set(prefix + '-diff', netDiff(v.network_difficulty));
  set(prefix + '-effort', v.effort_pct == null ? '—' : v.effort_pct.toFixed(4) + ' %');
  set(prefix + '-exp', fmtSecs(v.expected_block_s));
  set(prefix + '-odds-day', odds(v.odds_day));
  set(prefix + '-odds-week', odds(v.odds_week));
  set(prefix + '-blocks', `${fmtInt(v.blocks_found)}${v.blocks_pending ? ` (+${v.blocks_pending} pending)` : ''}`);
}

// The aux chains in a fixed order (DOGE before PEP), each with its panel
// and its payout field created once.
const AUX_ORDER = ['doge', 'pep'];
function auxNames(chains, parent) {
  return Object.keys(chains || {}).filter((k) => k !== parent).sort((a, b) => {
    const ia = AUX_ORDER.indexOf(a), ib = AUX_ORDER.indexOf(b);
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) || a.localeCompare(b);
  });
}
function ensureAux(name, c) {
  if ($('ca-' + name)) return;
  const dl = [['status', 'Status'], ['height', 'Height'], ['diff', 'Network diff'], ['effort', 'Effort'], ['exp', 'Expected block'],
    ['odds-day', 'Block chance / day'], ['odds-week', 'Block chance / week'], ['blocks', 'Blocks found']].map(([k, l]) => `<dt>${l}</dt><dd id="ca-${name}-${k}">—</dd>`).join('');
  $('chains').insertAdjacentHTML('beforeend', `<section class="panel aux" id="ca-${name}"><h2 id="ca-${name}-title"></h2><dl>${dl}</dl></section>`);
  const hint = esc(c.hint || '');
  $('s-aux').insertAdjacentHTML('beforeend', `<div class="aux-pay" id="s-aux-${name}">
    <label for="s-${name}">${esc(c.ticker)} payout address (optional)</label>
    <div class="row"><input id="s-${name}" spellcheck="false" autocomplete="off" placeholder="${hint}"><button id="s-${name}-save">Save</button></div>
    <p class="dim small">Set it to also mine ${esc(c.name)} with the same hashrate. Checked by your ${esc(c.name)} node. Leave empty to leave ${esc(c.name)} out.</p></div>`);
  const input = $('s-' + name);
  input.addEventListener('input', () => { input.dataset.dirty = '1'; });
  $('s-' + name + '-save').addEventListener('click', async () => {
    msg(`Checking with your ${c.name} node…`);
    try {
      const r = await api('api/admin/aux-payout', 'PUT', { chain: name, address: input.value.trim() });
      delete input.dataset.dirty;
      msg(r.address ? `${c.ticker} payout saved: ${r.address}` : `${c.ticker} payout cleared: ${c.name} is left out.`);
      refresh();
    } catch (e) { msg(e.message, true); }
  });
}

let lastState = null;
function render(st) {
  lastState = st;
  const n = st.node || {}, p = st.pool || {}, t = st.template || {}, d = st.derived || {};
  const coin = st.coin || {}, ticker = coin.ticker || '';
  diffScale = coin.coin === 'ltc' ? 65536 : 1;
  $('title').textContent = 'WIZARD-BLOCKS' + (ticker ? '-' + ticker : '');
  document.title = 'Wizard-Blocks' + (ticker ? '-' + ticker : '');
  document.documentElement.dataset.coin = coin.coin || '';
  const hint = (coin.address_hint || {})[st.chain] || '';
  $('s-addr').placeholder = hint;
  $('s-addr-label').textContent = `${ticker || 'Payout'} payout address${hint ? ' (' + hint + ')' : ''}`;

  const chains = st.chains || null;
  const aux = chains ? auxNames(chains, coin.coin) : [];
  const auxCoin = (name) => (st.aux_coins || {})[name] || { ticker: name.toUpperCase(), name: name.toUpperCase() };
  $('chains').hidden = !aux.length;
  document.querySelectorAll('.chaincol').forEach((el) => { el.hidden = !aux.length; });
  const banner = [];
  if (aux.length) {
    renderChain('cp', ticker, chains[coin.coin] || {}, false);
    for (const name of aux) {
      const c = auxCoin(name), v = chains[name];
      ensureAux(name, { ticker: c.ticker, name: c.name || c.ticker, hint: (c.address_hint || {})[st.chain] || '' });
      $('title').textContent += ' + ' + c.ticker;
      document.title += ' + ' + c.ticker;
      renderChain('ca-' + name, `${c.ticker} (MERGED)`, v, true);
      if (!v.merged) banner.push(bannerText(c.name || c.ticker, c.ticker, v.reason));
      const input = $('s-' + name);
      if (document.activeElement !== input && !input.dataset.dirty) input.value = v.payout_address || '';
    }
  }
  $('banner').hidden = !banner.length;
  $('banner').textContent = banner.join(' ');
  set('n-synced', !n.connected ? 'no node connection' : n.synced ? 'yes' : `syncing ${fmtInt(n.height)} / ${fmtInt(n.headers)}`, !n.connected || !n.synced);
  set('n-height', fmtInt(n.height));
  set('n-zmq', !n.zmq_enabled ? 'off (polling)' : n.zmq_connected ? 'connected' : 'not connected', n.zmq_enabled && !n.zmq_connected);
  set('n-chain', n.chain || st.chain || '—');
  const bc = t.block_check || '';
  set('n-check', bc ? `${bc} (height ${fmtInt(t.block_check_height)}, ${ago(t.block_check_at)})` : 'waiting for the first job', bc.startsWith('REJECTED'));

  set('p-hr1', fmtHash(p.hashrate_live));
  set('p-hr5', fmtHash(p.hashrate_5m));
  set('p-hr60', fmtHash(p.hashrate_1h));
  set('p-workers', `${p.workers_online ?? 0} online / ${(st.workers || []).length}`);
  set('p-acc', fmtInt(p.shares_accepted));
  const rej = Object.entries(p.rejects || {}).filter(([, v]) => v > 0).map(([k, v]) => `${k} ${v}`).join(', ');
  set('p-rej', fmtInt(p.shares_rejected) + (rej ? ` (${rej})` : '') + (p.shares_stale ? ` · ${fmtInt(p.shares_stale)} stale` : ''), p.shares_rejected > 0);
  set('p-bad', fmtInt(p.bad_messages));
  set('p-best', p.best_share_difficulty ? `${fmtDiff(p.best_share_difficulty)}${p.best_share_worker ? ' · ' + p.best_share_worker : ''}` : '—');
  set('p-netdiff', netDiff(t.network_difficulty));
  set('p-effort', d.luck_since_last_block_pct == null ? '—' : d.luck_since_last_block_pct.toFixed(4) + ' %');
  set('p-odds-day', odds(d.odds_day));
  set('p-odds-week', odds(d.odds_week));
  set('p-blocks', `${fmtInt(p.blocks_found)}${p.blocks_pending ? ` (+${p.blocks_pending} pending)` : ''}`);

  const sx = st.stratum || {};
  const host = location.hostname || 'umbrel.local';
  $('s-stratum').textContent = `Stratum: stratum+tcp://${host}:${sx.port || '—'} · user: any name · password: x`;
  if (!sx.payout_set) set('s-msg', 'No payout address yet: set it below. Miners get no work until then.', true);
  else if ($('s-msg').textContent.startsWith('No payout')) set('s-msg', '');
  if (document.activeElement !== $('s-addr') && !$('s-addr').dataset.dirty) $('s-addr').value = sx.payout_address || '';

  const workers = (st.workers || []).slice().sort((a, b) => (b.hashrate_live || 0) - (a.hashrate_live || 0));
  $('workers').innerHTML = workers.length ? workers.map((w) => `<tr class="wrow${w.name === graph.name ? ' open' : ''}" data-name="${esc(w.name)}" tabindex="0" title="Click for this worker's hashrate graph">
    <td>${graph.name === w.name ? '▾' : '▸'} ${esc(w.name)}${w.connections > 0 ? '' : ' <span class="dim">(offline)</span>'}</td>
    <td class="num">${esc(fmtHash(w.hashrate_live))}</td>
    <td class="num">${esc(fmtDiff(w.difficulty))}</td>
    <td class="num">${esc(fmtInt(w.shares_accepted))} / ${esc(fmtInt(w.shares_rejected))}</td>
    <td class="num">${esc(fmtInt(w.shares_stale))}</td>
    <td class="num">${esc(fmtDiff(w.best_share_difficulty))}</td>
    <td class="num">${esc(ago(w.last_share_at))}</td></tr>`).join('') : '<tr><td colspan="7" class="dim">No miners yet.</td></tr>';
  placeGraph();

  // Real blocks only (newest first): candidates that never made the chain
  // (stale, rejected) are left out.
  const real = (b) => b.chain_status !== 'stale' && b.status !== 'stale' && b.status !== 'rejected';
  let blocks = (st.blocks || []).filter(real).map((b) => ({ ...b, chainTicker: ticker }));
  if (aux.length) {
    blocks = blocks.concat((st.aux_blocks || []).filter(real).map((b) => {
      const c = auxCoin(b.chain || ''), tmpl = (c.explorer || {})[st.chain];
      return { ...b, chainTicker: c.ticker, explorer_url: tmpl ? tmpl.replace('{hash}', b.hash) : '', confirmations: null, chain_status: b.status };
    }));
    blocks.sort((a, b) => Date.parse(b.time) - Date.parse(a.time));
  }
  blocks = blocks.slice(0, 50);
  const cols = aux.length ? 7 : 6;
  $('blocks').innerHTML = blocks.length ? blocks.map((b) => {
    const hash = esc(b.hash || ''), short = hash.slice(0, 12) + '…' + hash.slice(-8);
    const link = b.explorer_url ? `<a href="${esc(b.explorer_url)}" target="_blank" rel="noopener noreferrer">${short}</a>` : short;
    return `<tr>${aux.length ? `<td>${esc(b.chainTicker)}</td>` : ''}<td class="num">${esc(fmtInt(b.height))}</td><td>${link}</td><td>${esc(b.worker)}</td>
      <td>${esc(new Date(b.time).toLocaleString())}</td><td class="num">${b.confirmations == null ? '—' : esc(fmtInt(b.confirmations))}</td><td>${esc(b.chain_status || b.status)}</td></tr>`;
  }).join('') : `<tr><td colspan="${cols}" class="dim">No blocks found yet.</td></tr>`;
  updateCalc();
}

// ---------- worker graph: click a worker for its hashrate over time ----------
// Ranges and averages: "raw" is the 10-second buckets (last hour only); the
// others are trailing averages of the per-minute history (last week). Any
// number of averages can be shown at once, each in its own neon colour.
const RANGES = { '1h': 60, '6h': 360, '24h': 1440, '7d': 10080 }; // minutes
const SMOOTH = { raw: 0, '1m': 1, '5m': 5, '15m': 15, '30m': 30, '1h': 60, '6h': 360, '12h': 720, '1d': 1440 }; // minutes
// Plain words, so the time shown and the averaging cannot be confused.
const RANGE_LABEL = { '1h': 'last hour', '6h': 'last 6 hours', '24h': 'last day', '7d': 'last week' };
const LINE_LABEL = { raw: 'raw', '1m': '1m', '5m': '5m', '15m': '15m', '30m': '30m', '1h': '1h', '6h': '6h', '12h': '12h', '1d': '1d' };
const LINE_TITLE = { raw: 'raw 10-second readings', '1m': '1-minute average', '5m': '5-minute average', '15m': '15-minute average', '30m': '30-minute average', '1h': '1-hour average', '6h': '6-hour average', '12h': '12-hour average', '1d': '1-day average' };
const NEON = { raw: '#f2f2ff', '1m': '#00e5ff', '5m': '#2f6bff', '15m': '#39ff14', '30m': '#ff8c00', '1h': '#ffea00', '6h': '#ff1f3d', '12h': '#a64dff', '1d': '#ff2bd6' };
const lineName = (k) => LINE_LABEL[k];
const graph = { name: '', range: '1h', shown: new Set(['15m']), data: null, at: 0, hoverT: null, series: [] };
const graphRow = document.createElement('tr');
graphRow.className = 'wgraph';
graphRow.innerHTML = `<td colspan="7"><div class="gctl">
  <span class="glabel" id="g-title"></span>
  <span class="gbar"><select id="g-range" aria-label="Time shown">${Object.keys(RANGES).map((k) => `<option value="${k}">${RANGE_LABEL[k]}</option>`).join('')}</select><span class="gbtns" id="g-smooth" role="group" aria-label="Lines">${Object.keys(SMOOTH).map((k) => `<button type="button" data-v="${k}" class="line-${k}" title="${LINE_TITLE[k]}">${lineName(k)}</button>`).join('')}</span></span>
  </div><div class="gwrap"><canvas id="g-canvas" height="300"></canvas><div class="gtip" id="g-tip" hidden></div></div>
  <p class="dim small" id="g-note"></p></td>`;

function placeGraph() {
  const row = graph.name && [...$('workers').querySelectorAll('tr.wrow')].find((r) => r.dataset.name === graph.name);
  if (!row) { graphRow.remove(); return; }
  row.after(graphRow);
  syncGraphButtons();
  drawGraph();
}
function syncGraphButtons() {
  graphRow.querySelector('#g-range').value = graph.range;
  graphRow.querySelectorAll('#g-smooth button').forEach((b) => {
    b.disabled = b.dataset.v === 'raw' && graph.range !== '1h'; // raw: last hour only
    b.hidden = b.disabled;
    b.setAttribute('aria-pressed', String(graph.shown.has(b.dataset.v) && !b.disabled));
  });
}
async function loadGraph() {
  const name = graph.name;
  if (!name) return;
  try {
    const r = await fetch('api/worker?name=' + encodeURIComponent(name), { cache: 'no-store' });
    if (!r.ok) throw new Error(r.status === 404 ? 'no history for this worker yet' : 'HTTP ' + r.status);
    const d = await r.json();
    if (graph.name === name) { graph.data = d; graph.at = Date.now(); drawGraph(); }
  } catch (e) {
    if (graph.name === name) { graph.data = null; set('g-note', 'Graph unavailable: ' + e.message, true); drawGraph(); }
  }
}
function openGraph(name) {
  graph.name = graph.name === name ? '' : name;
  graph.data = null; graph.hoverT = null;
  if (lastState) render(lastState);
  if (graph.name) loadGraph();
}
$('workers').addEventListener('click', (e) => {
  const row = e.target.closest('tr.wrow');
  if (row) openGraph(row.dataset.name);
});
$('workers').addEventListener('keydown', (e) => {
  const row = e.target.closest('tr.wrow');
  if (row && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); openGraph(row.dataset.name); }
});
graphRow.addEventListener('click', (e) => {
  const b = e.target.closest('button');
  if (!b || b.disabled) return;
  if (b.parentElement.id === 'g-smooth') {
    if (graph.shown.has(b.dataset.v)) { if (graph.shown.size > 1) graph.shown.delete(b.dataset.v); } else graph.shown.add(b.dataset.v);
  }
  // Keep at least one line that the range can show.
  if (graph.range !== '1h' && [...graph.shown].every((k) => k === 'raw')) graph.shown.add('1m');
  graph.hoverT = null;
  syncGraphButtons();
  drawGraph();
});
// The points [unix seconds, H/s] of one line for the chosen range.
function graphPoints(key) {
  const d = graph.data;
  if (!d) return [];
  const d1 = d.diff1_hashes;
  if (key === 'raw') {
    const n = d.raw.length;
    return d.raw.map((v, i) => [d.raw_end - (n - 1 - i) * d.raw_step_s, v * d1 / d.raw_step_s]);
  }
  const win = SMOOTH[key], span = RANGES[graph.range], m = d.minutes, n = m.length;
  // Long ranges are downsampled to at most ~720 points (each still the
  // trailing average over the line's window).
  const step = Math.max(1, Math.floor(span / 720));
  const pre = new Float64Array(n + 1);
  for (let i = 0; i < n; i++) pre[i + 1] = pre[i] + m[i];
  const pts = [];
  // Aligned so the newest minute is always the last point.
  for (let i = n - 1 - Math.floor((span - 1) / step) * step; i < n; i += step) {
    if (i < 0) continue;
    const lo = Math.max(0, i - win + 1), mins = i - lo + 1;
    pts.push([d.minute_end - (n - 1 - i) * 60, (pre[i + 1] - pre[lo]) * d1 / (mins * 60)]);
  }
  return pts;
}
function nearest(pts, t) {
  let best = 0;
  for (let i = 1; i < pts.length; i++) if (Math.abs(pts[i][0] - t) < Math.abs(pts[best][0] - t)) best = i;
  return pts[best];
}
function drawGraph() {
  const cv = document.getElementById('g-canvas');
  if (!cv || !graphRow.isConnected) return;
  const cs = getComputedStyle(document.documentElement);
  const dim = cs.getPropertyValue('--dim').trim() || '#080', line = cs.getPropertyValue('--line').trim() || '#030';
  const dpr = Math.min(2, window.devicePixelRatio || 1), W = cv.clientWidth || 600, H = 300;
  cv.width = W * dpr; cv.height = H * dpr;
  const g = cv.getContext('2d');
  g.setTransform(dpr, 0, 0, dpr, 0, 0);
  g.clearRect(0, 0, W, H);
  g.font = 'bold 14px ui-monospace, Menlo, Consolas, monospace';
  $('g-title').textContent = graph.name;
  const keys = Object.keys(SMOOTH).filter((k) => graph.shown.has(k) && !(k === 'raw' && graph.range !== '1h'));
  const series = keys.map((k) => ({ k, pts: graphPoints(k) })).filter((s) => s.pts.length);
  graph.series = series;
  if (graph.data) {
    const avg = (pts) => pts.reduce((a, p) => a + p[1], 0) / pts.length;
    const main = series.find((s) => s.k !== 'raw') || series[0];
    set('g-note', `${RANGE_LABEL[graph.range]}${main ? ' · average ' + fmtHash(avg(main.pts)) : ''} · tap or hover the graph for values · updates every 30 s`);
  }
  if (!series.length) {
    g.fillStyle = dim; g.fillText(graph.data ? 'no shares in this range' : 'loading…', 12, H / 2);
    $('g-tip').hidden = true;
    return;
  }
  const L = 110, R = 12, T = 12, B = 28, pw = W - L - R, ph = H - T - B;
  const t1 = Math.max(...series.map((s) => s.pts[s.pts.length - 1][0]));
  const t0 = Math.min(...series.map((s) => s.pts[0][0]));
  const ymax = Math.max(...series.flatMap((s) => s.pts.map((p) => p[1]))) * 1.1 || 1;
  const X = (t) => L + (t - t0) / Math.max(1, t1 - t0) * pw, Y = (v) => T + ph - v / ymax * ph;
  g.strokeStyle = line; g.fillStyle = dim; g.lineWidth = 1;
  g.textAlign = 'right'; g.textBaseline = 'middle';
  for (let k = 0; k <= 4; k++) {
    const v = ymax * k / 4, y = Math.round(Y(v)) + 0.5;
    g.beginPath(); g.moveTo(L, y); g.lineTo(W - R, y); g.stroke();
    g.fillText(fmtHash(v), L - 6, y);
  }
  g.textAlign = 'center'; g.textBaseline = 'top';
  const span = t1 - t0;
  for (let k = 0; k <= 4; k++) {
    const t = t0 + span * k / 4, d = new Date(t * 1000);
    const lbl = span > 2 * 86400 ? d.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString([], { hour: '2-digit' }) : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    g.fillText(lbl, Math.min(Math.max(X(t), L + 30), W - R - 30), H - B + 6);
  }
  // Each line: thick neon with a glow; raw first so the averages sit on top.
  for (const s of series) {
    g.save();
    g.strokeStyle = NEON[s.k]; g.lineWidth = s.k === 'raw' ? 3 : 5; g.lineJoin = 'round'; g.lineCap = 'round';
    g.shadowColor = NEON[s.k]; g.shadowBlur = 16;
    g.beginPath();
    s.pts.forEach((p, i) => (i ? g.lineTo(X(p[0]), Y(p[1])) : g.moveTo(X(p[0]), Y(p[1]))));
    g.stroke();
    g.restore();
  }
  // Hover / tap: a crosshair, a dot on every line and all their values.
  const tip = $('g-tip');
  if (graph.hoverT != null) {
    const t = Math.min(Math.max(graph.hoverT, t0), t1), x = X(t);
    g.strokeStyle = dim; g.lineWidth = 1;
    g.beginPath(); g.moveTo(Math.round(x) + 0.5, T); g.lineTo(Math.round(x) + 0.5, T + ph); g.stroke();
    tip.replaceChildren();
    const head = document.createElement('div');
    head.textContent = new Date(t * 1000).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    tip.append(head);
    for (const s of series) {
      const p = nearest(s.pts, t);
      g.fillStyle = '#000'; g.strokeStyle = NEON[s.k]; g.lineWidth = 3;
      g.beginPath(); g.arc(X(p[0]), Y(p[1]), 7, 0, Math.PI * 2); g.fill(); g.stroke();
      const row = document.createElement('div');
      row.style.color = NEON[s.k];
      row.textContent = `${LINE_TITLE[s.k]}: ${fmtHash(p[1])}`;
      tip.append(row);
    }
    tip.hidden = false;
    tip.style.left = Math.min(Math.max(x + 12, 0), W - tip.offsetWidth) + 'px';
    if (x + 12 + tip.offsetWidth > W) tip.style.left = Math.max(0, x - 12 - tip.offsetWidth) + 'px';
  } else tip.hidden = true;
}
const graphHover = (e) => {
  const cv = document.getElementById('g-canvas');
  if (e.target !== cv || !graph.series.length) return;
  const r = cv.getBoundingClientRect(), x = e.clientX - r.left, L = 110, pw = r.width - L - 12;
  const t1 = Math.max(...graph.series.map((s) => s.pts[s.pts.length - 1][0]));
  const t0 = Math.min(...graph.series.map((s) => s.pts[0][0]));
  graph.hoverT = t0 + (x - L) / pw * (t1 - t0);
  drawGraph();
};
graphRow.addEventListener('change', (e) => {
  if (e.target.id !== 'g-range') return;
  graph.range = e.target.value;
  if (graph.range !== '1h' && [...graph.shown].every((k) => k === 'raw')) graph.shown.add('1m');
  graph.hoverT = null;
  syncGraphButtons();
  drawGraph();
});
graphRow.addEventListener('pointermove', graphHover);
graphRow.addEventListener('pointerdown', graphHover); // a tap on a phone
graphRow.addEventListener('pointerleave', () => { graph.hoverT = null; drawGraph(); });
window.addEventListener('resize', drawGraph);
setInterval(() => { if (graph.name && !document.hidden && Date.now() - graph.at > 30000) loadGraph(); }, 5000);

// ---------- settings: payout address and optional password ----------
async function api(path, method, body) {
  const res = await fetch(path, {
    method, headers: { 'X-WB-Admin': '1', ...(body ? { 'Content-Type': 'application/json' } : {}) }, // X-WB-Admin: CSRF guard
    body: body ? JSON.stringify(body) : undefined, credentials: 'same-origin',
  });
  const j = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(j.error || 'HTTP ' + res.status);
  return j;
}
function msg(text, bad = false) { set('s-msg', text, bad); }

async function syncSession() {
  try {
    const s = await api('api/admin/session', 'GET');
    const locked = !s.enabled || (s.password_required && !s.authed);
    $('s-login').hidden = !(s.enabled && s.password_required && !s.authed);
    $('s-body').hidden = locked;
    $('diff').hidden = locked;
    if (!locked) loadDiff();
    $('s-rmpw').hidden = !s.password_set || s.env_password;
    $('s-logout').hidden = !(s.password_required && s.authed);
    if (!locked) loadLimits();
    $('s-setpw').textContent = s.password_set ? 'Change' : 'Set';
    if (!s.enabled) msg('Settings are disabled on this engine.', true);
  } catch (e) { msg(e.message, true); }
}

$('s-addr').addEventListener('input', () => { $('s-addr').dataset.dirty = '1'; });
$('s-save').addEventListener('click', async () => {
  msg('Checking with your node…');
  try {
    const r = await api('api/admin/payout', 'PUT', { address: $('s-addr').value.trim() });
    delete $('s-addr').dataset.dirty;
    msg('Saved: ' + ((r.payout && r.payout.address) || 'ok'));
    refresh();
  } catch (e) { msg(e.message, true); }
});
$('s-loginbtn').addEventListener('click', async () => {
  try { await api('api/admin/login', 'POST', { password: $('s-loginpw').value }); $('s-loginpw').value = ''; msg('Unlocked.'); syncSession(); } catch (e) { msg(e.message, true); }
});
$('s-setpw').addEventListener('click', async () => {
  try { await api('api/admin/password', 'POST', { password: $('s-pw').value }); $('s-pw').value = ''; msg('Settings password set.'); syncSession(); } catch (e) { msg(e.message, true); }
});
$('s-rmpw').addEventListener('click', async () => {
  try { await api('api/admin/password', 'DELETE'); msg('Settings password removed.'); syncSession(); } catch (e) { msg(e.message, true); }
});

$('s-logout').addEventListener('click', async () => {
  try { await api('api/admin/logout', 'POST'); msg('Logged out.'); syncSession(); } catch (e) { msg(e.message, true); }
});

// Connection limits (total and per IP). Hidden if the engine does not offer them.
function fillLimits(r) {
  $('s-maxconn').value = r.limits.max_connections;
  $('s-maxip').value = r.limits.max_connections_per_ip;
  $('s-maxconn').title = `default ${r.defaults.max_connections}, max ${r.max}`;
  $('s-maxip').title = `default ${r.defaults.max_connections_per_ip}`;
}
async function loadLimits() {
  try { fillLimits(await api('api/admin/limits', 'GET')); $('s-lim-wrap').hidden = false; } catch (e) { $('s-lim-wrap').hidden = true; }
}
$('s-lim-save').addEventListener('click', async () => {
  try {
    const r = await api('api/admin/limits', 'PUT', { max_connections: Number($('s-maxconn').value), max_connections_per_ip: Number($('s-maxip').value) });
    fillLimits(r);
    msg(`Connection limits saved: ${r.limits.max_connections} total, ${r.limits.max_connections_per_ip} per IP.`);
  } catch (e) { msg(e.message, true); }
});
$('s-lim-reset').addEventListener('click', async () => {
  try {
    const r = await api('api/admin/limits', 'DELETE');
    fillLimits(r);
    msg(`Connection limits back to the defaults: ${r.limits.max_connections} total, ${r.limits.max_connections_per_ip} per IP.`);
  } catch (e) { msg(e.message, true); }
});

// ---------- difficulty settings ----------
function dmsg(text, bad = false) { set('d-msg', text, bad); }
function fillDiff(d) {
  $('d-mode').value = d.fixed_diff > 0 ? 'fixed' : 'vardiff';
  $('d-start').value = d.start_diff || '';
  $('d-min').value = d.vardiff_min;
  $('d-max').value = d.vardiff_max;
  $('d-target').value = d.vardiff_target_seconds;
  $('d-fixed').value = d.fixed_diff || '';
  $('d-over').value = Object.entries(d.worker_overrides || {}).map(([k, v]) => `${k} = ${v}`).join('\n');
  $('d-pw').checked = !d.ignore_password_diff;
  $('d-sg').checked = !d.ignore_suggest_difficulty;
  updateCalc();
}
async function loadDiff() {
  try { const r = await api('api/admin/settings', 'GET'); fillDiff(r.settings); } catch (e) { dmsg(e.message, true); }
}
function readDiff() {
  const num = (id) => { const v = $(id).value.trim(); return v === '' ? 0 : Number(v); };
  const over = {};
  for (const line of $('d-over').value.split('\n')) {
    if (!line.trim()) continue;
    const i = line.lastIndexOf('=');
    if (i < 1) throw new Error(`override line "${line.trim()}" must look like name = difficulty`);
    over[line.slice(0, i).trim()] = Number(line.slice(i + 1).trim());
  }
  const fixed = $('d-mode').value === 'fixed' ? num('d-fixed') : 0;
  if ($('d-mode').value === 'fixed' && !(fixed > 0)) throw new Error('fixed mode needs a fixed difficulty');
  return {
    vardiff_min: num('d-min'), vardiff_max: num('d-max'), vardiff_target_seconds: num('d-target'),
    fixed_diff: fixed, worker_overrides: over, start_diff: num('d-start'),
    ignore_password_diff: !$('d-pw').checked, ignore_suggest_difficulty: !$('d-sg').checked,
  };
}
$('d-save').addEventListener('click', async () => {
  try { const r = await api('api/admin/settings', 'PUT', readDiff()); fillDiff(r.settings); dmsg('Saved. Connected miners get the new difficulty now.'); } catch (e) { dmsg(e.message, true); }
});
$('d-reset').addEventListener('click', async () => {
  try { await api('api/admin/settings/reset', 'POST'); await loadDiff(); dmsg('Back to the defaults.'); } catch (e) { dmsg(e.message, true); }
});
// Seconds per share at the start difficulty for a typed-in hashrate.
function updateCalc() {
  const start = Number($('d-start').value) || ((lastState && lastState.stratum && lastState.stratum.difficulty) || {}).start_diff || 0;
  const gh = Number($('d-hr').value);
  const diff1 = (lastState && lastState.coin && lastState.coin.coin === 'ltc') ? 65536 : 4294967296;
  set('d-calc', start > 0 && gh > 0 ? `one share every ${fmtSecs(start * diff1 / (gh * 1e9))} at difficulty ${fmtDiff(start)}` : '—');
}
['d-start', 'd-hr'].forEach((id) => $(id).addEventListener('input', updateCalc));

// ---------- binary rain: evenly spaced columns, alternating up/down ----------
const rain = (() => {
  const cv = $('rain'), ctx = cv.getContext('2d');
  const STEP = 1000 / 30; // ~30 fps cap
  const FONT = 14, LINE = 16, GAP = 22;
  let cols = [], raf = 0, last = 0, w = 0, h = 0;
  const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  function resize() {
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    w = window.innerWidth; h = window.innerHeight;
    cv.width = w * dpr; cv.height = h * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const n = Math.ceil(w / GAP), rows = Math.ceil(h / LINE) + 2;
    cols = Array.from({ length: n }, (_, i) => ({
      x: Math.round(i * GAP + GAP / 2),
      dir: i % 2 === 0 ? -1 : 1, // column 1 up, column 2 down, …
      speed: 18 + ((i * 37) % 23), // px per second
      off: Math.random() * LINE,
      bits: Array.from({ length: rows }, () => (Math.random() < 0.5 ? '0' : '1')),
    }));
    draw(0);
  }

  function draw(dt) {
    ctx.clearRect(0, 0, w, h);
    ctx.fillStyle = 'rgba(190, 190, 190, 0.10)';
    ctx.font = `${FONT}px ui-monospace, Menlo, Consolas, monospace`;
    ctx.textAlign = 'center';
    for (const c of cols) {
      c.off += c.speed * dt;
      // Each time a full line scrolls by, shift the column and add a new bit.
      while (c.off >= LINE) {
        c.off -= LINE;
        if (c.dir < 0) { c.bits.shift(); c.bits.push(Math.random() < 0.5 ? '0' : '1'); }
        else { c.bits.pop(); c.bits.unshift(Math.random() < 0.5 ? '0' : '1'); }
      }
      const shift = c.dir < 0 ? -c.off : c.off - LINE;
      for (let r = 0; r < c.bits.length; r++) ctx.fillText(c.bits[r], c.x, r * LINE + shift + FONT);
    }
  }

  function frame(ts) {
    raf = requestAnimationFrame(frame);
    if (ts - last < STEP) return;
    const dt = last ? Math.min(0.1, (ts - last) / 1000) : 0;
    last = ts;
    draw(dt);
  }
  function start() { if (!raf && !still) { last = 0; raf = requestAnimationFrame(frame); } }
  function stop() { cancelAnimationFrame(raf); raf = 0; }

  window.addEventListener('resize', resize);
  document.addEventListener('visibilitychange', () => (document.hidden ? stop() : start()));
  resize();
  if (!document.hidden) start();
  return { start, stop };
})();

// ---------- go ----------
// The public view (Wizard-Blocks View, view.js sets WB_VIEW) shows stats
// only: no Settings, no Difficulty, no admin requests.
if (window.WB_VIEW) {
  $('settings').hidden = true;
  $('diff').hidden = true;
} else {
  syncSession();
}
refresh();
setInterval(() => { if (!document.hidden) refresh(); }, 5000);
document.addEventListener('visibilitychange', () => { if (!document.hidden) refresh(); });
