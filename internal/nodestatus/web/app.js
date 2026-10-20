// Litecoin + Dogecoin Node page: each node's live state (Litecoin, Dogecoin,
// Pepecoin), refreshed every 5 s. Plain text only.
'use strict';

const int = (n) => (n || 0).toLocaleString('en-US');

function bytes(b) {
  if (!(b > 0)) return '0 B';
  const u = ['B', 'kB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (b >= 1000 && i < u.length - 1) { b /= 1000; i++; }
  return `${b >= 100 ? b.toFixed(0) : b.toFixed(1)} ${u[i]}`;
}

function si(d) {
  if (!(d > 0)) return '-';
  if (d < 1) return d.toExponential(2); // regtest/testnet
  const u = ['', 'k', 'M', 'G', 'T', 'P'];
  let i = 0;
  while (d >= 1000 && i < u.length - 1) { d /= 1000; i++; }
  return d.toFixed(d >= 100 ? 1 : 2) + u[i];
}

function ago(secs) {
  if (!(secs >= 0)) return '-';
  if (secs < 60) return `${Math.round(secs)}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ${Math.round(secs % 60)}s ago`;
  if (secs < 86400 * 2) return `${Math.floor(secs / 3600)}h ${Math.floor((secs % 3600) / 60)}m ago`;
  return `${Math.floor(secs / 86400)} days ago`;
}

function state(n) {
  const p = n.paused ? (n.network_active === false ? ' · PAUSED (sync stopped)' : ' · pausing…') : '';
  return base(n) + p;
}

function base(n) {
  switch (n.state) {
    case 'synced': return 'synced';
    // The node's own estimate (verificationprogress) can read ~100% early in
    // a sync when its chain data is old (Pepecoin), so blocks / headers caps it.
    case 'syncing': return n.headers > 0 ? `syncing ${(Math.min(n.progress, n.blocks / n.headers) * 100).toFixed(2)}%` : 'syncing (finding peers)';
    case 'starting': return `starting: ${n.error || ''}`.trim();
    default: return `down: ${n.error || 'not reachable'}`;
  }
}

// el creates an element with text (never HTML).
function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

function render(st) {
  const box = document.getElementById('nodes');
  // Do not rebuild under the user's hands (an open menu would close).
  if (box.contains(document.activeElement) && document.activeElement.tagName === 'SELECT') return;
  const cards = [];
  for (const n of st.nodes || []) {
    const card = el('section', 'node');
    card.append(el('h2', '', n.name));
    card.append(el('p', 'state' + (n.state === 'down' ? ' bad' : ''), state(n)));
    const rows = [];
    if (n.mem_bytes) rows.push(['memory', bytes(n.mem_bytes)]);
    if (n.state === 'synced' || n.state === 'syncing') {
      rows.push(['height', `${int(n.blocks)} / ${int(n.headers)}`]);
      rows.push(['last block', n.tip_time ? ago(st.now - n.tip_time) : '-']);
      rows.push(['peers', n.peers === undefined ? '-' : int(n.peers)]);
      rows.push(['mempool', n.mempool_tx === undefined ? '-' : `${int(n.mempool_tx)} tx (${bytes(n.mempool_bytes)})`]);
      rows.push(['difficulty', si(n.difficulty)]);
      rows.push(['disk', `${n.size_on_disk ? bytes(n.size_on_disk) : '-'}${n.pruned ? ' (pruned)' : ''}`]);
      if (n.prune_set_mib || n.prune_running_mib) {
        const pend = n.prune_set_mib && n.prune_running_mib && n.prune_set_mib !== n.prune_running_mib ? ` (${int(n.prune_set_mib)} on restart)` : '';
        rows.push(['prune', `${int(n.prune_running_mib || n.prune_set_mib)} MiB${pend}`]);
      }
      if (n.dbcache_mib) rows.push(['cache', `${int(n.dbcache_mib)} MiB`]);
      if (n.chain && n.chain !== 'main') rows.push(['chain', n.chain]);
      rows.push(['version', n.version || '-']);
    }
    const dl = el('dl');
    for (const [k, v] of rows) dl.append(el('dt', '', k), el('dd', '', v));
    card.append(dl);
    card.append(settings(n));
    cards.push(card);
  }
  box.replaceChildren(...cards);
  const mem = (st.nodes || []).reduce((a, n) => a + (n.mem_bytes || 0), 0);
  document.getElementById('mem').textContent = mem
    ? `Memory, all nodes: ${bytes(mem)}${st.host_mem_total ? ` of ${bytes(st.host_mem_total)} on this machine (${bytes(st.host_mem_available)} available)` : ''}`
    : '';
  document.getElementById('checked').textContent = `checked ${ago(st.now - Math.min(...(st.nodes || []).map((n) => n.checked || st.now)))}`;
}

// The settings at the bottom of a node's card: its prune target and cache
// (each applied by restarting that node) and, for Dogecoin and Pepecoin,
// pausing the sync so the Litecoin node gets the bandwidth while it syncs.
let busy = false;
const picked = { prune: {}, cache: {} }; // chosen in a menu but not applied yet
function settings(n) {
  const box = el('div', 'settings');
  if (n.prune_options && n.prune_options.length) box.append(choice(n, 'prune', n.prune_options, n.prune_set_mib, setPrune));
  if (n.dbcache_options && n.dbcache_options.length) box.append(choice(n, 'cache', n.dbcache_options, n.dbcache_mib, setCache));
  if (n.pausable) {
    const coin = n.name.replace(' Node', '');
    const b = el('button', 'pause', n.paused ? `Resume ${coin} sync` : `Pause ${coin} sync`);
    b.type = 'button';
    b.disabled = busy;
    b.addEventListener('click', () => setPaused(n, !n.paused));
    box.append(b);
  }
  return box;
}

// choice builds one "<what> [menu] [Apply]" row.
function choice(n, what, options, now, apply) {
  const row = el('div', 'row');
  const label = el('label', '', what);
  const sel = document.createElement('select');
  sel.id = `${what}-${n.key}`;
  label.htmlFor = sel.id;
  const cur = picked[what][n.key] || now;
  for (const mib of options) {
    const o = el('option', '', `${int(mib)} MiB${mib === now ? ' (now)' : ''}`);
    o.value = String(mib);
    o.selected = mib === cur;
    sel.append(o);
  }
  sel.disabled = busy;
  sel.addEventListener('change', () => { picked[what][n.key] = Number(sel.value); });
  const b = el('button', '', 'Apply');
  b.type = 'button';
  b.title = 'restarts this node';
  b.disabled = busy;
  b.addEventListener('click', () => apply(n, Number(sel.value)));
  row.append(label, sel, b);
  return row;
}

async function setCache(n, mib) {
  const msg = document.getElementById('msg');
  if (mib === n.dbcache_mib) { msg.textContent = `${n.name} cache is already ${int(mib)} MiB.`; return; }
  if (!window.confirm(`Set ${n.name} database cache to ${int(mib)} MiB and restart it now?`)) return;
  busy = true;
  msg.textContent = `setting ${n.name} cache to ${int(mib)} MiB…`;
  try {
    await post(`api/nodes/${encodeURIComponent(n.key)}/cache`, { mib });
    delete picked.cache[n.key];
    msg.textContent = `${n.name} cache set to ${int(mib)} MiB; the node is restarting to apply it (a minute or two). Set it back once synced to free the memory.`;
  } catch (e) {
    msg.textContent = `could not set the cache size: ${e.message}`;
  } finally {
    busy = false;
    refresh();
  }
}

async function post(path, body) {
  const r = await fetch(path, {
    method: 'POST', credentials: 'same-origin',
    headers: { 'X-NS-Action': '1', 'Content-Type': 'application/json' }, // CSRF guard
    body: JSON.stringify(body),
  });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || `HTTP ${r.status}`);
  return j;
}

async function setPrune(n, mib) {
  const msg = document.getElementById('msg');
  if (mib === n.prune_set_mib) { msg.textContent = `${n.name} is already set to ${int(mib)} MiB.`; return; }
  if (!window.confirm(`Set ${n.name} prune target to ${int(mib)} MiB and restart it now?`)) return;
  busy = true;
  msg.textContent = `setting ${n.name} prune target to ${int(mib)} MiB…`;
  try {
    const j = await post(`api/nodes/${encodeURIComponent(n.key)}/prune`, { mib });
    delete picked.prune[n.key];
    msg.textContent = j.restarting
      ? `${n.name} prune target set to ${int(mib)} MiB; the node is restarting to apply it (a minute or two).`
      : `${n.name} prune target saved (${int(mib)} MiB). ${j.warning || ''}`;
  } catch (e) {
    msg.textContent = `could not set the prune target: ${e.message}`;
  } finally {
    busy = false;
    refresh();
  }
}

async function setPaused(n, paused) {
  busy = true;
  const msg = document.getElementById('msg');
  msg.textContent = `${paused ? 'pausing' : 'resuming'} ${n.name}…`;
  try {
    const j = await post(`api/nodes/${encodeURIComponent(n.key)}/sync`, { paused });
    msg.textContent = paused
      ? `${n.name} sync paused (kept across restarts). Merged mining of this coin is off until you resume.${j.warning ? ' ' + j.warning : ''}`
      : `${n.name} sync resumed.${j.warning ? ' ' + j.warning : ''}`;
  } catch (e) {
    msg.textContent = `could not ${paused ? 'pause' : 'resume'}: ${e.message}`;
  } finally {
    busy = false;
    refresh();
  }
}

async function refresh() {
  try {
    const r = await fetch('api/status', { cache: 'no-store' });
    if (!r.ok) throw new Error(r.status);
    render(await r.json());
  } catch (e) {
    document.getElementById('checked').textContent = 'status page not reachable';
  }
}

refresh();
setInterval(() => { if (!document.hidden) refresh(); }, 5000);
