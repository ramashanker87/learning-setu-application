// Local synthetic accounts, microphone audio and mock transcription only.
import assert from "node:assert/strict";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "/tmp/setu-browser-check/node_modules/playwright-core/index.mjs");
const browser = await chromium.launch({ executablePath: "/usr/bin/google-chrome", headless: true, args: ["--no-sandbox", "--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const errors = []; page.on("pageerror", e => errors.push(e.message));
const base = process.env.PLATFORM_URL || "http://localhost:3000";
try {
  await page.addInitScript(() => {
    window.testStreams = [];
    const original = navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
    navigator.mediaDevices.getUserMedia = async (...args) => {
      if (window.testDenyMicrophone) throw new DOMException("Denied", "NotAllowedError");
      const stream = await original(...args); window.testStreams.push(stream); return stream;
    };
  });
  await page.goto(base);
  const login = async account => (await (await page.request.post(base + "/api/local/session", { data: { account } })).json()).token;
  const admin = await login("alpha-admin"), teacher = await login("teacher");
  const me = await (await page.request.get(base + "/api/me", { headers: { Authorization: "Bearer " + admin } })).json();
  const org = me.memberships[0].orgId;
  const headers = { Authorization: "Bearer " + admin, "X-Organization-Id": org };
  const school = (await (await page.request.get(base + "/api/tenant/schools", { headers })).json()).items[0];
  const name = "Recording reader " + Date.now();
  const created = await page.request.post(base + "/api/tenant/students", { headers: { ...headers, Authorization: "Bearer " + teacher }, data: {
    student: { name, age: 8, className: "3", schoolId: school.sk },
    consent: { guardianName: "Synthetic guardian", relationship: "Parent", noticeVersion: "2026-10-v1", purposes: ["reading_assessment"], guardianAdultVerified: true, authorityVerified: true },
  } });
  assert.equal(created.status(), 201); const student = await created.json();
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), admin); await page.reload();
  await page.getByRole("button", { name: "Students", exact: true }).click();
  assert.equal(await page.getByRole("button", { name: "Start reading assessment" }).count(), 0);
  const forbidden = await page.request.post(base + "/api/tenant/assessments", { headers, data: { studentId: student.sk, passageId: "en-1", bytes: 1000, duration: 2, type: "audio/webm" } });
  assert.equal(forbidden.status(), 403);
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), teacher); await page.reload();
  await page.getByRole("heading", { name: "Start with your student" }).waitFor();
  assert.equal(await page.getByRole("button", { name: "Add student with authorization", includeHidden: true }).count(), 1);
  async function openReader() {
    await page.getByLabel("Find a student").fill(name);
    await page.getByRole("button", { name: "Start reading assessment", exact: true }).click();
    await page.getByLabel("Reading passage").waitFor();
  }
  await openReader();
  assert.equal(await page.locator('input[name="name"]').count(), 0);
  await page.getByLabel("Reading language").selectOption("hindi");
  assert.equal(await page.locator(".reading-text").getAttribute("lang"), "hi");
  await page.getByLabel("Reading language").selectOption("english");
  await page.getByLabel("Reading passage").selectOption("en-1");
  await page.evaluate(() => { window.testDenyMicrophone = true; });
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByText("Microphone unavailable. Allow access, check your microphone, and try again.", { exact: true }).waitFor();
  await page.evaluate(() => { window.testDenyMicrophone = false; });
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByRole("button", { name: "Stop recording", exact: true }).waitFor();
  assert(await page.getByLabel("Reading passage").isDisabled());
  await page.waitForFunction(() => document.querySelector(".recording-timer").textContent >= "00:20");
  await page.getByRole("button", { name: "Stop recording", exact: true }).click();
  await page.getByLabel("Review your recording").waitFor();
  assert(await page.evaluate(() => window.testStreams.every(s => s.getTracks().every(t => t.readyState === "ended"))));
  await page.getByRole("button", { name: "Record again", exact: true }).click();
  assert(!(await page.getByLabel("Reading passage").isDisabled()));
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByRole("button", { name: "Stop recording", exact: true }).waitFor();
  await page.waitForFunction(() => document.querySelector(".recording-timer").textContent >= "00:20");
  await page.getByRole("button", { name: "Stop recording", exact: true }).click();
  await page.getByLabel("Review your recording").waitFor();
  // Local emulator was seeded for port 8080. Proxy uploads in this check when using
  // port 3000; the actual signed PUT and media processing still run against S3.
  await page.route("http://localhost:9000/**", async route => {
    if (route.request().method() === "OPTIONS") { await route.fulfill({ status: 204, headers: { "Access-Control-Allow-Origin": base, "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "PUT,GET" } }); return; }
    const response = await route.fetch();
    await route.fulfill({ response, headers: { ...response.headers(), "Access-Control-Allow-Origin": base } });
  });
  let failOnce = true;
  await page.route("**/api/tenant/assessments", async route => {
    if (route.request().method() === "POST" && failOnce) { failOnce = false; await route.fulfill({ status: 503, json: { error: "Synthetic temporary failure" } }); }
    else await route.continue();
  });
  await page.getByRole("button", { name: "Submit assessment", exact: true }).click();
  await page.getByText(/Synthetic temporary failure.*recording is still available/).waitFor();
  assert(await page.getByLabel("Review your recording").isVisible());
  const submitted = page.waitForResponse(r => r.url().endsWith("/api/tenant/assessments") && r.request().method() === "POST" && r.status() === 201);
  await page.getByRole("button", { name: "Submit assessment", exact: true }).click();
  const assessment = await (await submitted).json();
  assert.equal(assessment.studentId, student.sk);
  assert.equal(assessment.passage.id, "en-1");
  await page.getByText("Assessment complete. Your results are ready.", { exact: true }).waitFor({ timeout: 90000 });
  await page.screenshot({ path: "/tmp/setu-reading-desktop.png", fullPage: true });
  await page.getByRole("button", { name: "← All students", exact: true }).click();
  await openReader();
  await page.setViewportSize({ width: 390, height: 844 });
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await page.screenshot({ path: "/tmp/setu-reading-mobile.png", fullPage: true });
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByRole("button", { name: "Stop recording", exact: true }).waitFor();
  await page.getByRole("button", { name: "← All students", exact: true }).click();
  await page.waitForFunction(() => window.testStreams.every(s => s.getTracks().every(t => t.readyState === "ended")));
  // Disabled processing must not prevent microphone recording and local playback.
  await page.route("**/api/config", async route => { const r = await route.fetch(); await route.fulfill({ json: { ...await r.json(), integrationsDisabled: true } }); });
  await page.reload(); await openReader();
  await page.getByText(/Submission is unavailable until/).waitFor();
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByRole("button", { name: "Stop recording", exact: true }).waitFor();
  await page.waitForFunction(() => document.querySelector(".recording-timer").textContent >= "00:20");
  await page.getByRole("button", { name: "Stop recording", exact: true }).click();
  await page.getByLabel("Review your recording").waitFor();
  assert(await page.getByRole("button", { name: "Submit assessment", exact: true }).isDisabled());
  await page.unroute("**/api/config");
  await page.route("**/api/config", async route => { const r = await route.fetch(); await route.fulfill({ json: { ...await r.json(), integrationsDisabled: true, maxAudioSeconds: 3 } }); });
  await page.reload(); await openReader();
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByLabel("Review your recording").waitFor({ timeout: 10000 });
  assert(await page.evaluate(() => window.testStreams.every(s => s.getTracks().every(t => t.readyState === "ended"))));
  assert.deepEqual(errors, []);
  console.log("PASS: admin/teacher permissions, teacher landing, existing-student selection, Hindi/English passages, microphone errors, recording/preview/re-record, submission retry, real local audio processing, navigation cleanup, mobile layout and persistent controls, automatic time limit, disabled-processing preview.");
} finally { await browser.close(); }
