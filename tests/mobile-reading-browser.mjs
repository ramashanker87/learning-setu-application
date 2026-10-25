// Synthetic APIs and Chromium's fake microphone; no student data or provider calls.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-browser-check/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox', '--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'] });
const page = await browser.newPage({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' });
const errors = []; page.on('pageerror', error => errors.push(error.message));
let enabled = true, submissions = 0;
await page.route('**/api/**', route => {
 const path = new URL(route.request().url()).pathname;
 let body = { items: [] };
 if (path === '/api/config') body = { authMode: 'local' };
 if (path === '/api/tenant/assessment-settings') body = { enabled, integrationsEnabled: enabled, effectiveEnabled: enabled };
 if (path === '/api/tenant/passages') body = { items: [
  { id: 'en', title: 'A day at school', language: 'english', text: 'We walked to school together. '.repeat(40) },
  { id: 'hi', title: 'हमारा विद्यालय', language: 'hindi', text: 'हम हर दिन विद्यालय जाते हैं। '.repeat(40) }
 ] };
 if (path === '/api/tenant/assessments') { submissions++; return route.fulfill({ status: 503, json: { error: 'Please try again' } }); }
 return route.fulfill({ json: body });
});
async function openReader() {
 await page.evaluate(async () => {
  const { readingSession } = await import('/features/recorder.js');
  const { session } = await import('/api.js'); session.config = {};
  document.body.classList.remove('landing-page');
  await readingSession(document.querySelector('#app'), { sk: 'test', name: 'Sample reader', className: '3', consent: { status: 'GRANTED' } }, () => {}, () => {});
 });
}
const dock = page.locator('.mobile-recording-dock');
try {
 await page.route('**/mobile-reading-test', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><link rel=stylesheet href=/styles.css><main id=app></main>' }));
 await page.goto((process.env.PLATFORM_URL || 'http://127.0.0.1:4173') + '/mobile-reading-test');

 await openReader();
 for (const width of [320, 390, 720, 1440]) {
  await page.setViewportSize({ width, height: 844 });
  assert.equal(await dock.isVisible(), width <= 720);
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `Overflow at ${width}`);
 }
 await page.setViewportSize({ width: 390, height: 844 });
 await page.getByLabel('Reading language').selectOption('hindi');
 assert.equal(await page.locator('.reading-text').getAttribute('lang'), 'hi');
 // A short recording must be discarded locally, before any assessment request.
 await dock.getByRole('button', { name: 'Start recording', exact: true }).click();
 await dock.getByRole('button', { name: 'Stop recording', exact: true }).waitFor();
 await page.waitForTimeout(1200);
 await dock.getByRole('button', { name: 'Stop recording', exact: true }).click();
 await page.getByText(/Recording discarded/).first().waitFor();
 assert.equal(submissions, 0);
 assert.equal(await page.getByLabel('Review your recording').isVisible(), false);
 await dock.getByRole('button', { name: 'Start recording', exact: true }).click();
 await dock.getByRole('button', { name: 'Stop recording', exact: true }).waitFor();
 await page.waitForTimeout(20500);
 assert(await page.getByLabel('Reading language').isDisabled());
 assert(await page.locator('.passage-panel h2').evaluate(el => el.getBoundingClientRect().top >= 0 && el.getBoundingClientRect().top < 100));
 await page.screenshot({ path: '/tmp/setu-mobile-reading.png' });
 await dock.getByRole('button', { name: 'Stop recording', exact: true }).click();
 await dock.getByRole('button', { name: 'Submit assessment', exact: true }).waitFor();
 assert(await page.getByLabel('Review your recording').isVisible());
 await page.getByLabel('Review your recording').evaluate(async audio => { await audio.play(); audio.pause(); });
 await dock.getByRole('button', { name: 'Submit assessment', exact: true }).click();
 await page.getByText(/Your recording is still available here/).first().waitFor();
 assert.equal(submissions, 1);
 assert(await page.getByLabel('Review your recording').getAttribute('src'));
 await dock.getByRole('button', { name: 'Record again', exact: true }).click();
 assert(await page.getByLabel('Reading language').isEnabled());
 enabled = false; await openReader();
 await dock.getByRole('button', { name: 'Start recording', exact: true }).click();
 await dock.getByRole('button', { name: 'Stop recording', exact: true }).waitFor();
 await page.waitForTimeout(20500);
 await dock.getByRole('button', { name: 'Stop recording', exact: true }).click();
 await dock.getByRole('button', { name: 'Submit assessment', exact: true }).waitFor();
 assert(await dock.getByRole('button', { name: 'Submit assessment', exact: true }).isDisabled());
 assert(await dock.getByRole('button', { name: 'Record again', exact: true }).isEnabled());
 await page.screenshot({ path: '/tmp/setu-mobile-review.png' });
 enabled = true;
 await page.clock.install();
 await openReader();
 await dock.getByRole('button', { name: 'Start recording', exact: true }).click();
 await dock.getByRole('button', { name: 'Stop recording', exact: true }).waitFor();
 await page.clock.fastForward(61000);
 await page.getByText(/Recording exceeded 60 seconds/).first().waitFor();
 assert(await dock.getByRole('button', { name: 'Submit assessment', exact: true }).isDisabled());
 assert(await page.locator('.recording-review').getByRole('button', { name: 'Submit assessment', exact: true }).isDisabled());
 assert.equal(await page.getByLabel('Review your recording').isVisible(), false);
 assert.equal(submissions, 1, 'Overlong recording must not make a submission request');
 await page.setViewportSize({ width: 1440, height: 1000 });
 await page.locator('.recording-review').getByRole('button', { name: 'Record again', exact: true }).click();
 assert(await page.getByLabel('Reading language').isEnabled());
 assert(await page.locator('.recorder-panel').getByRole('button', { name: 'Start recording', exact: true }).isEnabled());
 assert.deepEqual(errors, []);
 console.log('PASS: mobile/desktop layout, Hindi passage, recording, review, retry, and organization submission restrictions');
} finally { await browser.close(); }
