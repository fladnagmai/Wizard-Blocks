// Screenshots the simple page states (see screens_test.go).
const { chromium } = require('playwright');
const cfg = JSON.parse(process.argv[2]);
(async () => {
  const browser = await chromium.launch({ executablePath: process.env.WB_CHROMIUM || undefined });
  const views = { desktop: { width: 1280, height: 900 }, phone: { width: 390, height: 844, isMobile: true, deviceScaleFactor: 2 } };
  for (const [name, url] of Object.entries(cfg.urls)) {
    for (const [vname, vp] of Object.entries(views)) {
      const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, isMobile: !!vp.isMobile, deviceScaleFactor: vp.deviceScaleFactor || 1, reducedMotion: 'reduce' });
      const page = await ctx.newPage();
      const errors = [];
      page.on('pageerror', (e) => errors.push(e.message));
      page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
      await page.goto(url);
      await page.waitForFunction(() => document.getElementById('status').textContent.startsWith('live'), null, { timeout: 10000 });
      await page.waitForFunction(() => document.getElementById('s-maxconn').value !== '', null, { timeout: 5000 });
      await page.waitForTimeout(400);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
      const file = `${cfg.out}/${name}-${vname}.png`;
      await page.screenshot({ path: file, fullPage: true });
      const banner = await page.evaluate(() => { const b = document.getElementById('banner'); return b.offsetParent === null ? '' : b.textContent; });
      const shown = await page.evaluate(() => Object.fromEntries(['chains', 'banner', 's-aux-doge', 's-aux-pep', 'diff'].map((id) => [id, !!document.getElementById(id) && document.getElementById(id).offsetParent !== null])));
      const oddsTxt = await page.evaluate(() => ['p-odds', 'cp-odds', 'ca-doge-odds', 'ca-pep-odds'].map((id) => (document.getElementById(id) || {}).textContent || '').join(' '));
      if (/%/.test(oddsTxt) || !/^(—|1:[\d,.E]+)( \/ (—|1:[\d,.E]+))*\s*/.test(oddsTxt.trim())) errors.push('block odds not shown as 1:N: ' + oddsTxt);
      // Per-coin accent, TEST note only for the test build, every settings control present.
      const color = await page.evaluate(() => getComputedStyle(document.body).color);
      const wantColor = { btc: 'rgb(247, 147, 26)', bch: 'rgb(10, 193, 142)', ltc: 'rgb(143, 180, 255)' }[name.split('-')[0]];
      if (color !== wantColor) errors.push('accent colour ' + color + ', want ' + wantColor);
      const note = await page.evaluate(() => document.getElementById('testnote').offsetParent !== null);
      if (note !== name.startsWith('ltc')) errors.push('TEST note shown=' + note);
      const title = await page.evaluate(() => document.getElementById('title').textContent);
      if (title !== 'WIZARD-BLOCKS-' + name.split('-')[0].toUpperCase() + (name.startsWith('ltc') ? ' + DOGE' : '') + (name.includes('pep') ? ' + PEP' : '')) errors.push('title ' + title);
      const ctl = await page.evaluate(() => ['d-mode', 'd-start', 'd-min', 'd-max', 'd-target', 'd-fixed', 'd-over', 'd-pw', 'd-sg', 'd-save', 'd-reset',
        's-addr', 's-save', 's-maxconn', 's-maxip', 's-lim-save', 's-lim-reset', 's-pw', 's-setpw'].filter((id) => { const e = document.getElementById(id); return !e || e.offsetParent === null; }));
      if (ctl.length) errors.push('settings controls missing or hidden: ' + ctl.join(','));
      const lim = await page.evaluate(() => [document.getElementById('s-maxconn').value, document.getElementById('s-maxip').value]);
      if (lim[0] !== '1024' || lim[1] !== '64') errors.push('connection limits ' + JSON.stringify(lim));
      const sfile = `${cfg.out}/${name}-settings-${vname}.png`;
      await page.locator('#settings').screenshot({ path: sfile });
      await page.locator('#diff').screenshot({ path: `${cfg.out}/${name}-difficulty-${vname}.png` });
      // Network difficulty in node units, as the nodes show it (the test
      // template is LTC 52M, DOGE 21M, PEP 3M node units; BTC/BCH 126T).
      const nd = await page.evaluate(() => ['p-netdiff', 'cp-diff', 'ca-doge-diff', 'ca-pep-diff'].map((id) => (document.getElementById(id) || {}).textContent || ''));
      const wantNd = name.startsWith('ltc') ? ['52.00M', '52.00M', '21.00M', name.includes('pep') ? '3.00M' : ''] : ['126.0T', '', '', ''];
      if (JSON.stringify(nd.map((x, i) => (wantNd[i] ? x : ''))) !== JSON.stringify(wantNd)) errors.push('network difficulty ' + JSON.stringify(nd));
      const wantAux = name.startsWith('ltc'), wantPep = name.includes('pep');
      const wantBanner = name.startsWith('ltc') && name !== 'ltc-doge-merged' && name !== 'ltc-doge-pep-merged';
      if (shown.chains !== wantAux || shown['s-aux-doge'] !== wantAux || shown['s-aux-pep'] !== wantPep || shown.banner !== wantBanner || !shown.diff) {
        errors.push('visibility wrong: ' + JSON.stringify(shown));
      }
      console.log(`${name} ${vname}: ${file}${banner ? ' | banner: ' + banner : ''}${overflow ? ' | HORIZONTAL OVERFLOW' : ''}${errors.length ? ' | JS ERRORS: ' + errors.join('; ') : ''}`);
      if (errors.length || overflow) process.exitCode = 1;
      await ctx.close();
    }
  }
  await browser.close();
})().catch((e) => { console.error(e); process.exit(1); });
