// Synthetic API responses only; run against a static frontend server.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-browser-check/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', headless: true, args: ['--no-sandbox'] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const base = process.env.PLATFORM_URL || 'http://127.0.0.1:4173';
const errors = [];
page.on('pageerror', e => errors.push(e.message));
let failConfig = false, failIdentity = false, failDashboard = false;
await page.route('**/api/**', route => {
  const path = new URL(route.request().url()).pathname;
  let status = 200, body;
  if (path === '/api/config') {
    status = failConfig ? 503 : 200;
    body = failConfig ? { error: 'Service unavailable' } : { authMode: 'local', accounts: [{ sub: 'teacher', email: 'teacher@school.example' }] };
  } else if (path === '/api/local/session') body = { token: 'synthetic-session' };
  else if (path === '/api/me') {
    status = failIdentity ? 401 : 200;
    body = failIdentity ? { error: 'Session expired' } : { email: 'teacher@school.example', memberships: [{ orgId: 'school', displayName: 'Greenfield School', state: 'ACTIVE', role: 'ORG_ADMIN' }] };
  } else if (path === '/api/tenant/dashboard') {
    status = failDashboard ? 503 : 200;
    body = failDashboard ? { error: 'Temporarily unavailable' } : { role: 'ORG_ADMIN', organization: { displayName: 'Greenfield School', retentionDays: 365 } };
  } else if (path === '/api/tenant/reading-progress') body = { schools: [], students: [], assessments: [], days: 90, from: '2026-07-27', through: '2026-10-24', generatedAt: '2026-10-24T12:00:00Z' };
  else body = { items: [] };
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
});
try {
  await page.goto(base);
  await page.getByRole('heading', { name: 'Sign in to Learning Setu' }).waitFor();
  await page.screenshot({ path: '/tmp/setu-login-desktop.png', fullPage: true });
  await page.getByRole('button', { name: 'हिन्दी' }).click();
  await page.getByRole('heading', { name: 'लर्निंग सेतु में साइन इन करें' }).waitFor();
  await page.getByRole('button', { name: 'English', exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await page.screenshot({ path: '/tmp/setu-login-mobile.png', fullPage: true });
  await page.getByRole('button', { name: 'Sign in to workspace' }).click();
  await page.getByRole('heading', { name: 'Reading progress' }).waitFor();
  assert.equal(await page.locator('#nav [aria-current="page"]').textContent(), 'Dashboard');
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({ path: '/tmp/setu-dashboard-desktop.png', fullPage: true });
  failDashboard = true;
  await page.getByRole('button', { name: 'Dashboard', exact: true }).click();
  await page.getByRole('heading', { name: 'We couldn’t open this page' }).waitFor();
  failDashboard = false;
  await page.getByRole('button', { name: 'Try again', exact: true }).click();
  await page.getByRole('heading', { name: 'Reading progress' }).waitFor();
  failIdentity = true;
  await page.reload();
  await page.getByRole('heading', { name: 'Sign in to Learning Setu' }).waitFor();
  assert.match(await page.locator('#message').textContent(), /verify your access/);
  failIdentity = false;
  await page.goto(base + '/?error=access_denied');
  await page.getByRole('heading', { name: 'Sign in to Learning Setu' }).waitFor();
  assert.match(await page.locator('#message').textContent(), /cancelled/);
  assert.equal(new URL(page.url()).search, '');
  await page.goto(base + '/?code=invalid&state=wrong');
  await page.getByRole('heading', { name: 'Sign in to Learning Setu' }).waitFor();
  assert.match(await page.locator('#message').textContent(), /Invalid sign-in response/);
  failConfig = true;
  await page.goto(base);
  await page.getByRole('heading', { name: 'Let’s get you back to your workspace' }).waitFor();
  failConfig = false;
  await page.getByRole('button', { name: 'Try again', exact: true }).click();
  await page.getByRole('heading', { name: 'Sign in to Learning Setu' }).waitFor();
  assert.deepEqual(errors, []);
  console.log('PASS: responsive sign-in/dashboard, Hindi, navigation, expired session, OAuth errors and retry recovery.');
} finally { await browser.close(); }
