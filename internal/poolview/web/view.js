// Loaded before the pool page's app.js on the public view: the page shows
// stats only (app.js leaves out Settings and Difficulty), with a tab per pool.
'use strict';
window.WB_VIEW = true;
// The pool's colours from its tab name, before (or without) its state: a
// pool that is not running still shows in its own colour.
document.documentElement.dataset.coin = location.pathname.split('/').filter(Boolean)[0] || '';
document.addEventListener('DOMContentLoaded', async () => {
  const nav = document.createElement('nav');
  nav.className = 'tabs';
  nav.setAttribute('aria-label', 'Pools');
  const home = document.createElement('a');
  home.href = '/';
  home.textContent = '⟵ portal';
  home.className = 'home';
  nav.append(home);
  const here = location.pathname.split('/').filter(Boolean)[0];
  try {
    const pools = await (await fetch('/pools.json', { cache: 'no-store' })).json();
    for (const p of pools) {
      const a = document.createElement('a');
      a.href = `/${p.key}/`;
      a.textContent = p.label;
      if (p.key === here) a.setAttribute('aria-current', 'page');
      nav.append(a);
    }
  } catch (e) { /* tabs are optional */ }
  document.querySelector('main').prepend(nav);
});
