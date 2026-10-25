// Synthetic organization and passages; no student records are changed.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-dashboard-browser/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox', '--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'] });
const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
page.setDefaultTimeout(15000);
const errors = []; page.on('pageerror', e => errors.push(e.message));
await page.addInitScript(() => sessionStorage.setItem('setu-token', 'synthetic'));
await page.route('**/api/**', route => {
 const path = new URL(route.request().url()).pathname;
 let body = { items: [] };
 if (path === '/api/config') body = { authMode: 'local', maxAudioSeconds: 60 };
 if (path === '/api/me') body = { email: 'teacher@example.test', teamAccount: true, memberships: [{ orgId: 'org', state: 'ACTIVE', active: true, role: 'ASSESSOR', displayName: 'School' }] };
 if (path.endsWith('/assessment-settings')) body = { enabled: true, effectiveEnabled: true };
 if (path.endsWith('/passages')) body = { items: [
  { id: 'en', language: 'english', title: 'English story', text: 'We go to school.', className: '3' },
  { id: 'hi', language: 'hindi', title: 'हिन्दी कहानी', text: 'हम विद्यालय जाते हैं।', className: '3' }
 ] };
 return route.fulfill({ json: body });
});
async function openReader() {
 await page.evaluate(async () => {
  const { readingSession } = await import('/features/recorder.js');
  await readingSession(document.querySelector('#app'), { name: 'Reader', className: '3', consent: { status: 'GRANTED' } }, () => document.querySelector('#app').replaceChildren(), () => {});
 });
}
try {
 await page.goto(process.env.PLATFORM_URL || 'http://127.0.0.1:4173');
 await page.getByRole('heading', { name: 'Start with your student' }).waitFor();
 const hindi = page.locator('[data-language=hi]'), english = page.locator('[data-language=en]');
 assert(await hindi.isVisible());
 await hindi.click(); await openReader();
 assert.equal(await page.locator('[name=readingLanguage]').inputValue(), 'hindi');
 assert.equal(await page.locator('.reading-text').textContent(), 'हम विद्यालय जाते हैं।');
 await english.click();
 assert.equal(await page.locator('.reading-text').textContent(), 'हम विद्यालय जाते हैं।');
 assert.equal(await page.locator('.reading-text').getAttribute('lang'), 'hi');
 await page.locator('[name=readingLanguage]').selectOption('english');
 assert.equal(await english.getAttribute('aria-pressed'), 'true');
 await hindi.click();
 assert.equal(await page.locator('[name=readingLanguage]').inputValue(), 'english');
 await page.reload(); await page.getByRole('heading', { name: 'अपने विद्यार्थी से शुरू करें' }).waitFor(); await openReader();
 assert.equal(await page.locator('[name=readingLanguage]').inputValue(), 'hindi');
 await page.locator('.mobile-recording-dock').getByRole('button', { name: 'रिकॉर्डिंग शुरू करें', exact: true }).click();
 await page.locator('.mobile-recording-dock').getByRole('button', { name: 'रिकॉर्डिंग रोकें', exact: true }).waitFor();
 assert(await english.isEnabled()); assert(await hindi.isEnabled());
 await english.click();
 assert.equal(await page.locator('.reading-workspace').getAttribute('data-recording'), 'true');
 await page.getByRole('button', { name: '← All students' }).click();
 assert(await english.isEnabled()); await english.click();
 await openReader(); assert.equal(await page.locator('[name=readingLanguage]').inputValue(), 'english');
 assert.deepEqual(errors, []);
 console.log('PASS: signed-in language switch, English/Hindi passages, independent passage selection, refresh persistence, interface switching during recording and cleanup');
} finally { await browser.close(); }
