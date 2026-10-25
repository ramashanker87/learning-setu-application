// Synthetic identities and aggregate metrics only. No real student/API/provider calls.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-dashboard-browser/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox'] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } });
page.setDefaultTimeout(15000);
const errors = []; page.on('pageerror', error => errors.push(error.message));
let failure = false, empty = false, sourceRequests = 0;
const schools = [{ id: 'sunrise', name: 'Sunrise Primary School', district: 'Patna' }, { id: 'riverdale', name: 'Riverdale Public School', district: 'Patna' }, { id: 'greenvalley', name: 'Green Valley School', district: 'Gaya' }];
const names = ['Aarav Kumar', 'Ananya Singh', 'Diya Sharma', 'Ishaan Verma', 'Kabir Patel', 'Meera Kumari', 'Neha Yadav', 'Rohan Das', 'Sana Khan', 'Vihaan Raj', 'Aditi Sinha', 'Arjun Kumar', 'Kavya Singh', 'Reyansh Gupta', 'Riya Kumari', 'Shreya Das', 'Tanvi Mehta', 'Vivaan Kumar'];
function fixture(language = 'english', days = 90) {
  const students = names.map((name, i) => ({ id: `s${i}`, name, className: String(3 + i % 3), schoolId: schools[i % 3].id, schoolName: schools[i % 3].name }));
  const assessments = [];
  for (let i = 0; i < 16; i++) for (let j = 0; j < (i === 15 ? 1 : 4); j++) {
    const at = ['2026-08-06', '2026-08-28', '2026-09-19', '2026-10-18'][j] + 'T10:00:00Z';
    assessments.push({ id: `a${i}-${j}`, studentId: `s${i}`, at, passageId: `${language}-school`, passageVersion: 1, passageTitle: language === 'hindi' ? 'हमारा विद्यालय' : 'A day at school', language, provider: 'sarvam', providerVersion: 'saaras:v3', scoringVersion: 'poc-v1', accuracy: Math.min(98, 48 + i % 6 * 5 + j * (i % 7 === 0 ? -3 : i % 7 === 1 ? 0 : 7)), wpm: 47 + i % 8 * 4 + j * 7 });
  }
  assessments.push({ ...assessments[0], id: 'demo', accuracy: 100, provider: 'mock', at: '2026-10-23T10:00:00Z' });
  const through = '2026-10-24', from = new Date(new Date(through).getTime() - (days - 1) * 86400000).toISOString().slice(0, 10);
  return { students: empty ? [] : students, schools, assessments: empty ? [] : assessments.filter(a => a.at >= from), language, days, from, through, generatedAt: '2026-10-24T14:32:00Z' };
}
await page.addInitScript(() => { sessionStorage.setItem('setu-token', 'synthetic'); sessionStorage.setItem('setu-org', 'org'); });
await page.route('**/api/**', route => {
  const url = new URL(route.request().url()), path = url.pathname;
  let body = { items: [] };
  if (path === '/api/config') body = { authMode: 'cognito' };
  if (path === '/api/me') body = { email: 'admin@example.test', memberships: [{ orgId: 'org', displayName: 'Learning Together Foundation', state: 'ACTIVE', active: true, role: 'ORG_ADMIN' }] };
  if (path === '/api/tenant/dashboard') body = { role: 'ORG_ADMIN', organization: { displayName: 'Learning Together Foundation', retentionDays: 365 } };
  if (path === '/api/tenant/reading-progress') {
    sourceRequests++;
    if (failure) return route.fulfill({ status: 503, json: { error: 'Temporary reporting outage' } });
    body = fixture(url.searchParams.get('language'), Number(url.searchParams.get('days')));
  }
  return route.fulfill({ json: body });
});
try {
  await page.goto(process.env.PLATFORM_URL || 'http://127.0.0.1:4173');
  await page.getByRole('heading', { name: 'Reading progress', exact: true }).waitFor();
  await page.getByRole('heading', { name: 'School comparison', exact: true }).waitFor();
  assert.equal(await page.locator('.progress-kpi').count(), 4);
  assert.equal(await page.locator('.reading-status-tile').count(), 5);
  assert.match(await page.locator('.reading-status-overview .reading-coverage').textContent(), /89% assessed · 16 of 18 learners/);
  await page.getByRole('button', { name: 'Not assessed yet: 2 learners. Filter learner progress', exact: true }).click();
  assert.equal(await page.locator('.progress-table tbody tr').count(), 2);
  assert.equal(await page.locator('.progress-table .status-unassessed').count(), 2);
  await page.getByRole('button', { name: 'Not assessed yet: 2 learners. Filter learner progress', exact: true }).click();
  assert.equal(await page.locator('.school-comparison-row').count(), 3);
  assert.equal(await page.locator('.progress-table tbody tr').count(), 10);
  assert.equal(await page.locator('.progress-demo').count(), 0);
  await page.screenshot({ path: '/tmp/setu-reading-dashboard-desktop.png', fullPage: true });
  await page.locator('[data-language=hi]').click();
  await page.getByRole('heading', { name: 'पठन प्रगति', exact: true }).waitFor();
  assert.equal(await page.locator('.kpi-blue .kpi-description').textContent(), 'इस दृश्य के 18 विद्यार्थियों में से');
  assert.equal(await page.locator('.reading-status-tile').first().locator('span').textContent(), 'सुधार हुआ');
  assert.equal(await page.locator('.ring-label').textContent(), 'सुधार हुआ');
  await page.screenshot({ path: '/tmp/setu-reading-dashboard-hindi.png', fullPage: true });
  await page.locator('[data-language=en]').click();
  await page.getByRole('heading', { name: 'Reading progress', exact: true }).waitFor();
  await page.getByRole('button', { name: 'Next →', exact: true }).click();
  assert.equal(await page.locator('.progress-table tbody tr').count(), 8);
  await page.getByLabel('Find a learner', { exact: true }).fill('Aarav');
  assert.equal(await page.locator('.progress-table tbody tr').count(), 1);
  await page.getByRole('button', { name: 'View progress for Aarav Kumar', exact: true }).click();
  await page.getByRole('heading', { name: 'Aarav Kumar’s reading journey', exact: true }).waitFor();
  assert.equal(await page.locator('.school-comparison').count(), 0);
  assert.equal(await page.locator('.reading-status-overview .status-lower').textContent(), 'Lower latest score');
  await page.getByLabel('Passage comparison').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Reading speed', exact: true }).click();
  assert.equal(await page.getByRole('button', { name: 'Reading speed', exact: true }).getAttribute('aria-pressed'), 'true');
  await page.locator('.chart-point').first().focus();
  assert.match(await page.locator('.chart-detail').textContent(), /wpm/);
  await page.getByText('View chart data', { exact: true }).click();
  assert.equal(await page.locator('.chart-data tbody tr').count(), 4);
  await page.screenshot({ path: '/tmp/setu-reading-dashboard-student.png', fullPage: true });
  await page.getByLabel('Student', { exact: true }).selectOption('');
  await page.getByLabel('School', { exact: true }).selectOption('sunrise');
  assert.equal(await page.locator('.school-comparison-row').count(), 1);
  assert.match(await page.locator('.reading-status-overview .reading-coverage').textContent(), /100% assessed · 6 of 6 learners/);
  const before = sourceRequests;
  await page.getByLabel('Reading language', { exact: true }).selectOption('hindi');
  await page.waitForFunction(() => document.querySelector('.progress-content')?.getAttribute('aria-busy') !== 'true');
  assert(sourceRequests > before);
  await page.getByLabel('Date range', { exact: true }).selectOption('30');
  await page.waitForFunction(() => document.querySelector('.progress-content')?.getAttribute('aria-busy') !== 'true');
  assert.match(await page.locator('.kpi-teal .kpi-description').textContent(), /^0 learners/);
  await page.getByLabel('Assessment data', { exact: true }).selectOption('mock');
  await page.locator('.progress-demo').waitFor();
  assert.match(await page.locator('.kpi-blue .kpi-footnote').textContent(), /^1 completed/);
  await page.getByLabel('Assessment data', { exact: true }).selectOption('real');
  await page.getByLabel('School', { exact: true }).selectOption('');
  await page.getByLabel('Date range', { exact: true }).selectOption('90');
  await page.getByLabel('Reading language', { exact: true }).selectOption('english');
  await page.waitForFunction(() => document.querySelector('.progress-content')?.getAttribute('aria-busy') !== 'true');
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `overflow at ${width}px`);
    if (width === 390) await page.screenshot({ path: '/tmp/setu-reading-dashboard-mobile.png', fullPage: true });
  }
  failure = true;
  await page.getByRole('button', { name: '↻ Refresh', exact: true }).click();
  await page.getByRole('heading', { name: 'We couldn’t load reading progress' }).waitFor();
  failure = false;
  await page.getByRole('button', { name: 'Try loading again' }).click();
  await page.getByRole('heading', { name: 'School comparison' }).waitFor();
  empty = true;
  await page.getByRole('button', { name: '↻ Refresh', exact: true }).click();
  await page.getByRole('heading', { name: 'Your reading story starts here' }).waitFor();
  assert.equal(await page.locator('.kpi-value').count(), 0);
  assert.deepEqual(errors, []);
  console.log('PASS: organization/school/student drill-down, metrics, chart keyboard access, language/date/source filters, pagination, mobile layouts, empty and error recovery.');
} catch (error) {
  console.error(await page.locator('body').innerText());
  throw error;
} finally { await browser.close(); }
