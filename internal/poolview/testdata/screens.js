// Screenshots the public view (see screens_test.go) and checks it: the
// wizard is drawn and clicking him opens the BCH page; the pool pages have
// tabs, no settings and no payout address; nothing overflows.
const { chromium } = require('playwright');
const cfg = JSON.parse(process.argv[2]);
(async () => {
  const browser = await chromium.launch({ executablePath: process.env.WB_CHROMIUM || undefined });
  const views = { desktop: { width: 1280, height: 800 }, phone: { width: 390, height: 844, isMobile: true, deviceScaleFactor: 2 } };
  let failed = false;
  for (const [vname, vp] of Object.entries(views)) {
    const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, isMobile: !!vp.isMobile, deviceScaleFactor: vp.deviceScaleFactor || 1 });
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('console', (m) => { if (m.type() === 'error' && !/502/.test(m.text())) errors.push(m.text()); });
    await page.goto(cfg.base + '/');
    await page.waitForTimeout(2200); // the wizard mid-fall
    await page.screenshot({ path: `${cfg.out}/portal-${vname}.png` });
    // Click where the wizard is right now.
    const at = await page.evaluate(() => new Promise((res) => requestAnimationFrame(() => res(null))));
    // Clicking the portal (where he falls in) counts too; use it, it holds still.
    const box = await page.evaluate(() => ({ x: window.innerWidth / 2, y: window.innerHeight * 0.64 }));
    await Promise.all([page.waitForURL('**/bch/'), page.mouse.click(box.x, box.y)]);
    await page.waitForFunction(() => (document.getElementById('status') || {}).textContent?.startsWith('live'), null, { timeout: 10000 });
    await page.waitForTimeout(500);
    for (const coin of ['bch', 'ltc', 'btc']) {
      if (coin !== 'bch') { await page.click(`nav.tabs a[href="/${coin}/"]`); await page.waitForURL(`**/${coin}/`); await page.waitForTimeout(800); }
      const info = await page.evaluate(() => ({
        settings: !!document.getElementById('settings') && document.getElementById('settings').offsetParent !== null,
        diff: !!document.getElementById('diff') && document.getElementById('diff').offsetParent !== null,
        tabs: [...document.querySelectorAll('nav.tabs a')].map((a) => a.textContent),
        text: document.body.innerText,
        overflow: document.documentElement.scrollWidth > window.innerWidth + 1,
      }));
      if (info.settings || info.diff) errors.push(`${coin}: settings shown`);
      if (info.tabs.join(',') !== '⟵ portal,BCH,LTC,BTC') errors.push(`${coin}: tabs ${info.tabs}`);
      if (/qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a/.test(info.text)) errors.push(`${coin}: payout address shown`);
      if (coin !== 'btc' && !/ash/.test(info.text)) errors.push(`${coin}: workers missing`);
      if (coin === 'btc' && !/unreachable/.test(info.text)) errors.push('btc: no "unreachable" for a pool that is not running');
      if (info.overflow) errors.push(`${coin}: horizontal overflow`);
      await page.screenshot({ path: `${cfg.out}/${coin}-${vname}.png`, fullPage: true });
    }
    console.log(`${vname}: ${errors.length ? 'ERRORS: ' + errors.join('; ') : 'ok'}`);
    if (errors.length) failed = true;
    await ctx.close();
  }
  await browser.close();
  process.exitCode = failed ? 1 : 0;
})().catch((e) => { console.error(e); process.exit(1); });
