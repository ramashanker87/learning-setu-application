// Mock Cognito only: never sends mail or changes a real password.
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || '/tmp/setu-browser-check/node_modules/playwright-core/index.mjs');
const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', args: ['--no-sandbox'] });
const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
let confirms = 0, deliveryFailure = false;
await page.route('**/api/config', route => route.fulfill({ json: { authMode: 'cognito', cognitoIssuer: 'https://cognito-idp.ap-south-1.amazonaws.com/test', cognitoClient: 'synthetic', cognitoDomain: 'https://synthetic.auth.ap-south-1.amazoncognito.com' } }));
await page.route('https://cognito-idp.ap-south-1.amazonaws.com/**', route => {
 const request = route.request();
 if (request.method() === 'OPTIONS') return route.fulfill({ status: 204, headers: { 'Access-Control-Allow-Origin': '*', 'Access-Control-Allow-Headers': '*' } });
 const data = request.postDataJSON();
 const operation = request.headers()['x-amz-target'];
 let status = 200, json = {};
 assert.equal(data.ClientId, 'synthetic');
 if (operation.endsWith('.ForgotPassword')) {
  if (deliveryFailure) { status = 400; json = { __type: 'TooManyRequestsException' }; }
  else if (data.Username !== 'admin@example.test') { status = 400; json = { __type: 'UserLambdaValidationException' }; }
 } else { assert(operation.endsWith('.ConfirmForgotPassword')); confirms++; assert.equal(data.Username, 'admin@example.test'); assert.equal(data.ConfirmationCode, '123456'); }
 return route.fulfill({ status, json, headers: { 'Access-Control-Allow-Origin': '*' } });
});
try {
 await page.goto(process.env.PLATFORM_URL || 'http://127.0.0.1:4173');
 await page.getByRole('button', { name: 'Organization sign-in →', exact: true }).waitFor();
 assert.equal(await page.getByRole('button', { name: /Forgot password/ }).count(), 0);
 // The reset UI is temporarily disconnected from sign-in; exercise it directly.
 const openReset = () => page.evaluate(async () => {
  const { organizationPasswordReset } = await import('/features/password-reset.js');
  const { landing } = await import('/features/landing.js');
  const root = document.querySelector('#app');
  organizationPasswordReset(document.querySelector('#signin-controls'), () => landing(root, () => {}));
 });
 for (const email of ['teacher@example.test', 'missing@example.test', 'admin@example.test']) {
  await openReset();
  await page.getByLabel('Registered administrator email').fill(email);
  await page.getByRole('button', { name: 'Send reset code' }).click();
  await page.getByText('If this email belongs to an eligible organization administrator, a verification code will be sent. Check your inbox.', { exact: true }).waitFor();
  if (email === 'admin@example.test') {
   await page.getByLabel('Email verification code').fill('123456');
   await page.getByLabel('New password', { exact: true }).fill('SyntheticPassword123!');
   await page.getByLabel('Confirm new password', { exact: true }).fill('DifferentPassword123!');
   await page.getByRole('button', { name: 'Reset password', exact: true }).click();
   await page.getByText('Passwords do not match.', { exact: true }).waitFor(); assert.equal(confirms, 0);
   await page.getByLabel('Confirm new password', { exact: true }).fill('SyntheticPassword123!');
   await page.getByRole('button', { name: 'Reset password', exact: true }).click();
   await page.getByText(/Password reset complete/).waitFor(); assert.equal(confirms, 1);
  }
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await page.getByRole('button', { name: 'Back to sign-in' }).click();
 }
 deliveryFailure = true;
 await openReset();
 await page.getByLabel('Registered administrator email').fill('admin@example.test');
 await page.getByRole('button', { name: 'Send reset code' }).click();
 await page.getByText('Unable to request a reset code right now. Please try again later.', { exact: true }).waitFor();
 console.log('PASS: admin reset UI, generic eligibility response, code/password confirmation, mismatch and service errors, mobile layout');
} finally { await browser.close(); }
