// Synthetic history and signed-audio response only; no real student/provider calls.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-browser-check/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox'] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } });
const errors = []; page.on('pageerror', error => errors.push(error.message));
const assessment = {
 sk: 'test-assessment', state: 'COMPLETED', createdAt: '2026-10-24T09:30:00Z', version: 'test',
 passage: { title: 'हमारा विद्यालय', language: 'hindi', text: 'हम हर दिन विद्यालय जाते हैं।' },
 result: { duration: 42, wordsPerMinute: 86, transcript: 'हम हर रोज स्कूल जाते हैं फिर', score: { accuracy_score: 82.5, pronunciation_score: 88, pause_count: null, errors: [
  { error_type: 'skipped', expected_word: 'दिन', spoken_word: null, position: 2 },
  { error_type: 'substituted', expected_word: 'विद्यालय', spoken_word: 'स्कूल', position: 3 },
  { error_type: 'inserted', expected_word: null, spoken_word: 'फिर', position: 6 }
 ] } }
};
let playbackRequests = 0, historyFails = true;
await page.route('**/api/**', route => {
 const path = new URL(route.request().url()).pathname;
 let body = { items: [] };
 if (path.endsWith('/assessment-settings')) body = { effectiveEnabled: true };
 if (path.endsWith('/students')) body = { items: [{ sk: 'reader', name: 'Sample reader', age: 8, className: '3', consent: { status: 'GRANTED' } }] };
 if (path.endsWith('/assessments')) {
  if (historyFails) { historyFails = false; return route.fulfill({ status: 503, json: { error: 'Temporary history failure' } }); }
  body = { items: [assessment] };
 }
 if (path.endsWith('/audio')) { playbackRequests++; body = { url: 'data:audio/wav;base64,UklGRg==' }; }
 return route.fulfill({ json: body });
});
await page.route('**/history-test', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><link rel=stylesheet href=/styles.css><div id=message></div><main id=app></main>' }));
try {
 await page.goto((process.env.PLATFORM_URL || 'http://127.0.0.1:4173') + '/history-test');
 await page.evaluate(async a => {
  const { assessmentCard } = await import('/features/assessment-card.js');
  document.querySelector('#app').append(assessmentCard(a));
 }, assessment);
 for (const value of ['0:42', '86', '82.5%', '88%']) await page.getByText(value, { exact: true }).waitFor();
 assert.equal(await page.locator('.assessment-breakdown .assessment-metric').first().locator('dd').textContent(), 'Unavailable');
 await page.getByText('Word review · 3 differences', { exact: true }).click();
 assert.equal(await page.locator('.word-review-item').count(), 3);
 assert.equal(await page.locator('.assessment-transcript p').getAttribute('lang'), 'hi');
 for (const width of [1440, 390, 320]) {
  await page.setViewportSize({ width, height: 1100 });
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  if (width !== 320) await page.screenshot({ path: `/tmp/setu-history-${width}.png`, fullPage: true });
 }
 await page.evaluate(async () => {
  const { assessmentCard } = await import('/features/assessment-card.js');
  const root = document.querySelector('#app');
  root.replaceChildren(assessmentCard({ state: 'COMPLETED', result: { duration: 59.9, score: { accuracy_score: 0, pronunciation_score: 0, pause_count: 0, errors: [] } } }));
 });
 await page.getByText('1:00', { exact: true }).waitFor();
 assert.equal(await page.getByText('0%', { exact: true }).count(), 2);
 await page.getByText('Word review · 0 differences').click();
 await page.getByText('No word differences detected in the recognized speech.').waitFor();
 await page.evaluate(async () => {
  const { assessmentCard } = await import('/features/assessment-card.js');
  document.querySelector('#app').replaceChildren(assessmentCard({ state: 'COMPLETED', result: {} }));
 });
 await page.getByText('Word review', { exact: true }).click();
 await page.getByText('Word-level review is unavailable for this assessment.').waitFor();
 await page.evaluate(async () => {
  const { session } = await import('/api.js'); session.org = 'org'; session.me = { memberships: [{ orgId: 'org', role: 'VIEWER' }] };
  const { students } = await import('/features/assessment.js'); const root = document.querySelector('#app'); root.replaceChildren(); await students(root);
 });
 await page.getByRole('button', { name: 'Assessment history', exact: true }).click();
 await page.getByRole('button', { name: 'Retry loading history' }).click();
 await page.getByText('82.5%', { exact: true }).waitFor();
 assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
 await page.getByRole('button', { name: 'Play recording' }).click();
 await page.getByRole('button', { name: 'Play recording' }).click();
 assert.equal(playbackRequests, 1); assert.equal(await page.locator('audio').count(), 1);
 await page.getByRole('button', { name: 'Hide assessment history' }).click();
 assert.equal(await page.locator('.student-history-row:visible').count(), 0);
 assert.deepEqual(errors, []);
 console.log('PASS: metric values, Hindi word differences, unavailable/zero data, responsive cards, history retry and single playback control');
} finally { await browser.close(); }
