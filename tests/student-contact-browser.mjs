// Synthetic responses only; does not create real student records.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-browser-check/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox'] });
const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
let saved;
await page.route('**/api/**', async route => {
 const path = new URL(route.request().url()).pathname;
 if (route.request().method() === 'POST') { saved = route.request().postDataJSON(); return route.fulfill({ json: {} }); }
 const items = path.endsWith('/schools') ? [{ sk: 'school', name: 'School', block: 'B', district: 'D', state: 'S' }] : [];
 return route.fulfill({ json: { items } });
});
await page.route('**/contact-test', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><link rel=stylesheet href=/styles.css><div id=message></div><main id=app></main>' }));
try {
 await page.goto((process.env.PLATFORM_URL || 'http://127.0.0.1:4173') + '/contact-test');
 for (const populated of [false, true]) {
  await page.evaluate(async () => {
   const { session } = await import('/api.js'); session.org = 'org'; session.me = { memberships: [{ orgId: 'org', role: 'ASSESSOR' }] };
   const { students } = await import('/features/assessment.js'); const root = document.querySelector('#app'); root.replaceChildren(); await students(root);
  });
  assert.equal(await page.getByLabel('Student name', { exact: true }).isVisible(), false);
  await page.locator('.new-student-section > summary').click();
  for (const label of ['Email address (optional)', 'Mobile number (optional)']) assert.equal(await page.getByLabel(label).evaluate(el => el.required), false);
  await page.getByLabel('Student name', { exact: true }).fill('Reader');
  await page.locator('.new-student-section > summary').click();
  assert.equal(await page.getByLabel('Student name', { exact: true }).isVisible(), false);
  await page.locator('.new-student-section > summary').click();
  assert.equal(await page.getByLabel('Student name', { exact: true }).inputValue(), 'Reader');
  await page.getByLabel('Age', { exact: true }).selectOption('8');
  await page.getByLabel('Class', { exact: true }).selectOption('3');
  await page.getByLabel('School', { exact: true }).selectOption('school');
  await page.getByLabel('Guardian name').fill('Guardian');
  await page.getByLabel('Relationship').fill('Parent');
  await page.locator('input[type=checkbox]').evaluateAll(inputs => inputs.forEach(input => input.checked = true));
  if (populated) { await page.getByLabel('Email address (optional)').fill('reader@example.test'); await page.getByLabel('Mobile number (optional)').fill('+91 98765 43210'); }
  saved = undefined;
  await page.getByRole('button', { name: 'Add student with authorization', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('input[name=name]')?.value === '');
  assert.equal(saved.student.email, populated ? 'reader@example.test' : '');
  assert.equal(saved.student.mobile, populated ? '+91 98765 43210' : '');
 }
 await page.evaluate(async () => {
  const { editStudent } = await import('/features/student-management.js');
  await editStudent(document.querySelector('#app'), { sk: 'test', name: 'Reader', age: 8, className: '3', schoolId: 'school', email: 'reader@example.test', mobile: '+91 98765 43210' }, () => {});
 });
 assert.equal(await page.getByLabel('Email address (optional)').inputValue(), 'reader@example.test');
 await page.getByLabel('Email address (optional)').fill(''); await page.getByLabel('Mobile number (optional)').fill('');
 await page.getByRole('button', { name: 'Save changes' }).click();
 await page.getByText('Student details updated.', { exact: true }).waitFor();
 assert.equal(saved.correction.email, ''); assert.equal(saved.correction.mobile, '');
 assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
 console.log('PASS: optional contact fields, empty/populated create payloads, edit prefill/clearing, mobile layout');
} finally { await browser.close(); }
