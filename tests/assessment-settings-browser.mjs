// Synthetic responses only; no external accounts or organizations are modified.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-browser-check/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', headless: true, args: ['--no-sandbox'] });
const page = await browser.newPage();
const base = process.env.PLATFORM_URL || 'http://127.0.0.1:4173';
let enabled = true, saves = 0, integrationsEnabled = false, integrationSaves = 0;
const errors = []; page.on('pageerror', e => errors.push(e.message));
await page.addInitScript(() => sessionStorage.setItem('setu-token', 'synthetic-company'));
await page.route('**/api/**', route => {
 const path = new URL(route.request().url()).pathname;
 let body = { items: [] };
 if (path === '/api/config') body = { authMode: 'local', integrationsDisabled: true };
 if (path === '/api/me') body = { company: true, email: 'company@example.test', memberships: [] };
 if (path === '/api/company/organizations') body = { items: [{ id: 'school', state: 'ACTIVE', draft: { displayName: 'Test School' }, assessmentProcessingEnabled: enabled, integrationsEnabled }] };
 if (path === '/api/tenant/assessment-settings') body = { enabled, platformAvailable: false, effectiveEnabled: false };
 if (path === '/api/tenant/passages') body = { items: [{ id: 'en-1', language: 'english', title: 'Reading', text: 'A short story.' }] };
 if (path.endsWith('/integrations')) { const input = route.request().postDataJSON(); assert.equal(input.reason, 'School launch'); integrationsEnabled = input.enabled; integrationSaves++; body = { enabled: integrationsEnabled }; }
 if (path.endsWith('/assessment-processing')) {
  const input = route.request().postDataJSON();
  assert.equal(input.reason, 'Term break');
  enabled = input.enabled; saves++;
  body = { enabled, effectiveEnabled: false, platformAvailable: false };
 }
 return route.fulfill({ json: body });
});
try {
 await page.goto(base);
 await page.getByRole('button', { name: "Organization details: Test School" }).first().click();
 await page.getByRole('heading', { name: 'Assessment processing', exact: true }).waitFor();
 await page.getByText(/Enable Organization integrations above/).waitFor();
 await page.getByLabel('Reason for integration change').fill('School launch');
 await page.getByRole('button', { name: 'Enable organization integrations' }).click();
 await page.getByRole('button', { name: 'Disable organization integrations' }).waitFor();
 assert.equal(integrationsEnabled, true);
 await page.getByLabel('Reason for integration change').fill('School launch');
 await page.getByRole('button', { name: 'Disable organization integrations' }).click();
 await page.getByRole('button', { name: 'Enable organization integrations' }).waitFor();
 assert.equal(integrationsEnabled, false); assert.equal(integrationSaves, 2);
 await page.getByRole('button', { name: 'Disable assessment processing' }).click();
 assert.equal(saves, 0);
 await page.getByLabel('Reason for processing change').fill('Term break');
 await page.getByRole('button', { name: 'Disable assessment processing' }).click();
 await page.getByRole('button', { name: 'Enable assessment processing' }).waitFor();
 assert.equal(enabled, false);
 await page.getByLabel('Reason for processing change').fill('Term break');
 await page.getByRole('button', { name: 'Enable assessment processing' }).click();
 await page.getByRole('button', { name: 'Disable assessment processing' }).waitFor();
 assert.equal(enabled, true); assert.equal(saves, 2);
 for (const organizationEnabled of [false, true]) {
  enabled = organizationEnabled;
  await page.evaluate(async () => {
   const { readingSession } = await import('/features/recorder.js');
   await readingSession(document.querySelector('#app'), { name: 'Reader', className: '3', consent: { status: 'GRANTED' } }, () => {}, () => {});
  });
  await page.getByText(organizationEnabled ? /The assessment service is not configured/ : /Your company administrator has disabled assessment processing/).waitFor();
  assert(await page.getByRole('button', { name: 'Submit assessment', exact: true, includeHidden: true }).isDisabled());
 }
 assert.deepEqual(errors, []);
 console.log('PASS: company organization processing controls, required reason, persistence and platform prerequisite notice.');
} finally { await browser.close(); }
