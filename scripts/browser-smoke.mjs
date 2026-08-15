#!/usr/bin/env node
// Real-browser smoke test for the official Immich Web UI (HARD REQUIREMENT).
//
// Verifies Web UI login correctness the ONLY way that matters: a real headless
// Chromium driving the official web SPA through the reverse-proxied public URL.
// `curl` is NOT sufficient — see AGENTS.md rule 8 / STATUS.md §P.
//
// One-time setup:
//   npm install playwright-core
//   npx playwright install chromium        # or set CHROMIUM_BIN to an existing binary
//   # on Debian/Ubuntu the chromium libs may be needed:
//   #   apt-get install -y libglib2.0-0 libnss3 libnspr4 libatk1.0-0 \
//   #     libatk-bridge2.0-0 libcups2 libdrm2 libxkbcommon0 libxcomposite1 \
//   #     libxdamage1 libxfixes3 libxrandr2 libgbm1 libpango-1.0-0 libcairo2 \
//   #     libasound2 libatspi2.0-0 libxshmfence1
//
// Env:
//   BASE_URL     public URL the browser hits (default https://s9rqfcgtmc-8081.cnb.run)
//   CHROMIUM_BIN absolute path to chrome/chromium (default: a common Playwright cache path)
//   TEST_EMAIL / TEST_PASSWORD  login creds (default admin@immich.app / password)
//
import { chromium } from 'playwright-core';

const BASE = process.env.BASE_URL || 'https://s9rqfcgtmc-8081.cnb.run';
const EXE = process.env.CHROMIUM_BIN
  || '/root/.cache/ms-playwright/chromium-1237/chrome-linux64/chrome';
const EMAIL = process.env.TEST_EMAIL || 'admin@immich.app';
const PASS = process.env.TEST_PASSWORD || 'password';

const fail = (m) => { console.error('FAIL:', m); };
let ok = true;
const expect = (cond, m) => { if (!cond) { ok = false; fail(m); } else console.log('PASS:', m); };

const browser = await chromium.launch({
  executablePath: EXE,
  headless: true,
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--enable-unsafe-swiftshader'],
});
const ctx = await browser.newContext({ ignoreHTTPSErrors: true });
const page = await browser.newPage();
const pageErrors = [];
page.on('pageerror', (e) => pageErrors.push(e.message));

try {
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForSelector('input[type="email"]', { timeout: 30000 });
  await page.fill('input[type="email"]', EMAIL);
  await page.fill('input[type="password"]', PASS);
  await page.click('button:has-text("登录"), button:has-text("Sign in"), button:has-text("Log in")');
  await page.waitForURL('**/photos', { timeout: 15000 });

  await page.waitForTimeout(4000);

  const url = page.url();
  expect(url.endsWith('/photos'), `reached authenticated home: ${url}`);

  const loginVisible = await page.evaluate(() => !!document.querySelector('input[type="email"]'));
  expect(!loginVisible, 'login form is NOT visible (photos page rendered)');

  const cookie = await page.evaluate(() => document.cookie);
  expect(/immich_is_authenticated=true/.test(cookie), 'immich_is_authenticated cookie present');

  for (const p of ['/api/users/me', '/api/users/me/preferences', '/api/notifications?unread=true']) {
    const st = await page.evaluate(async (u) => {
      try { return (await fetch(u, { credentials: 'same-origin' })).status; } catch (e) { return 'ERR:' + e; }
    }, p);
    expect(st === 200, `GET ${p} -> ${st}`);
  }

  expect(pageErrors.length === 0, `no client-side pageerror (got: ${JSON.stringify(pageErrors)})`);
} catch (e) {
  ok = false;
  fail('exception: ' + e.message);
  console.error('pageErrors so far:', pageErrors);
} finally {
  await browser.close();
}

console.log(ok ? '\nSMOKE TEST PASSED' : '\nSMOKE TEST FAILED');
process.exit(ok ? 0 : 1);
