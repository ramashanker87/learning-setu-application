// Read-only synthetic load test. Run against local emulators, never live tenants.
import assert from 'node:assert/strict';
import { performance } from 'node:perf_hooks';

const base = process.env.PLATFORM_URL || 'http://127.0.0.1:3000';
assert(['localhost', '127.0.0.1'].includes(new URL(base).hostname), 'Local API required');
const config = await fetch(`${base}/api/config`).then(r => r.json());
assert.equal(config.authMode, 'local', 'Synthetic local authentication required');
const login = await fetch(`${base}/api/local/session`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ account: 'alpha-admin' }),
}).then(r => r.json());
assert(login.token, 'Seeded alpha-admin account required');
const headers = { Authorization: `Bearer ${login.token}` };
const me = await fetch(`${base}/api/me`, { headers }).then(r => r.json());
assert(me.memberships?.length, 'Seeded organization required');
headers['X-Organization-Id'] = me.memberships[0].orgId;
const paths = ['/api/me', '/api/tenant/students', '/api/tenant/schools', '/api/tenant/passages', '/api/tenant/dashboard', '/api/tenant/assessment-settings', '/api/config', '/'];
const concurrency = Number(process.env.LOAD_CONCURRENCY || 10);
const seconds = Number(process.env.LOAD_SECONDS || 45);
assert(Number.isInteger(concurrency) && concurrency > 0 && concurrency <= 50);
assert(seconds > 0 && seconds <= 300);
const timings = [], errors = [];
const start = performance.now(), deadline = start + seconds * 1000;
await Promise.all(Array.from({ length: concurrency }, async (_, client) => {
  let i = client;
  while (performance.now() < deadline) {
    const path = paths[i++ % paths.length], began = performance.now();
    try {
      const response = await fetch(`${base}${path}`, { headers, signal: AbortSignal.timeout(5000) });
      await response.arrayBuffer();
      if (response.status !== 200) errors.push(`${path}: ${response.status}`);
    } catch (error) { errors.push(`${path}: ${error.message}`); }
    timings.push(performance.now() - began);
  }
}));
const elapsed = (performance.now() - start) / 1000;
timings.sort((a, b) => a - b);
const percentile = p => Math.round(timings[Math.min(timings.length - 1, Math.floor(timings.length * p))]);
console.log(JSON.stringify({ concurrency, seconds: Math.round(elapsed), requests: timings.length, requestsPerSecond: Math.round(timings.length / elapsed), p50Ms: percentile(.5), p95Ms: percentile(.95), p99Ms: percentile(.99), errors: errors.length, examples: errors.slice(0, 3) }, null, 2));
assert.equal(errors.length, 0, 'All requests must succeed');
assert(percentile(.95) < 2000, 'p95 latency must stay below two seconds');
