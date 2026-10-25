// Read-only deployed frontend check with synthetic identity/data responses.
// No real login, student writes, audio uploads or Sarvam calls.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "/tmp/setu-browser-check/node_modules/playwright-core/index.mjs");
const base = process.env.PLATFORM_URL;
assert(base, "Set PLATFORM_URL to the deployed HTTPS application URL");
const browser = await chromium.launch({ executablePath: "/usr/bin/google-chrome", headless: true, args: ["--no-sandbox", "--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const errors = []; page.on("pageerror", e => errors.push(e.message));
let role = "ASSESSOR";
const writes = [];
let historyRequests = 0;
try {
  const passages = JSON.parse(await readFile(new URL("../app/internal/platform/passages.json", import.meta.url), "utf8"));
  const student = { sk: "synthetic-student", name: "Sample reader", age: 8, className: "3", schoolId: "synthetic-school", schoolName: "Sample School", consent: { status: "GRANTED" } };
  await page.addInitScript(() => sessionStorage.setItem("setu-token", "synthetic-browser-check"));
  await page.route("**/api/**", async route => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() !== "GET") { writes.push(path); return route.fulfill({ status: 403, json: { error: "Read-only test" } }); }
    if (path === "/api/config") return route.continue();
    if (path === "/api/me") return route.fulfill({ json: { sub: "synthetic-teacher", email: "teacher@example.test", teamAccount: role !== "ORG_ADMIN", memberships: [{ orgId: "synthetic-org", state: "ACTIVE", active: true, role, displayName: "Sample Organization" }] } });
    if (path === "/api/tenant/students/synthetic-student/assessments") {
      historyRequests++;
      if (historyRequests === 1) return route.fulfill({ status: 503, json: { error: "History temporarily unavailable" } });
      return route.fulfill({ json: { items: historyRequests === 2 ? [] : [{ sk: "sample-assessment", passage: { title: "Sample passage" }, state: "COMPLETED", createdAt: "2026-10-22", result: { score: { accuracy_score: 95, pronunciation_score: 90 }, wordsPerMinute: 80, transcript: "Sample transcript" } }] } });
    }
    if (path === "/api/tenant/students") return route.fulfill({ json: { items: [student, { ...student, sk: "second-student", name: "Other reader", schoolName: "Other School" }] } });
    if (path === "/api/tenant/passages") return route.fulfill({ json: { items: passages } });
    if (path === "/api/tenant/schools") return route.fulfill({ json: { items: [{ sk: "synthetic-school", name: "Sample School", block: "Sample Block", district: "Sample District", state: "Sample State" }] } });
    if (path === "/api/tenant/dashboard") return route.fulfill({ json: { role, organization: { displayName: "Sample Organization", retentionDays: 30 }, students: 1, assessments: 0 } });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto(base);
  await page.getByRole("heading", { name: "Start with your student" }).waitFor();
  await page.locator(".new-student-section > summary").waitFor();
  async function checkSchoolFilter() {
    await page.getByLabel("Filter by school", { exact: true }).selectOption("Other School");
    assert.equal(await page.locator(".student-row").count(), 1);
    assert.equal(await page.locator('.student-row th[scope="row"]').textContent(), "Other reader");
    await page.getByLabel("Find a student").fill("missing");
    assert.equal(await page.locator(".student-row").count(), 0);
    await page.getByLabel("Find a student").fill("");
    await page.getByLabel("Filter by school", { exact: true }).selectOption("");
    assert.equal(await page.locator(".student-row").count(), 2);
    await page.getByLabel("Filter by school", { exact: true }).selectOption("Sample School");
  }
  await checkSchoolFilter();
  assert.equal(historyRequests, 0);
  assert.equal(await page.locator(".student-history-row:visible").count(), 0);
  await page.getByRole("button", { name: "Assessment history", exact: true }).click();
  await page.getByRole("alert").filter({ hasText: "History temporarily unavailable" }).waitFor();
  await page.getByRole("button", { name: "Retry loading history", exact: true }).click();
  await page.getByText("No assessments yet for this student.", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await page.getByText("Sample transcript", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Hide assessment history", exact: true }).click();
  assert.equal(await page.locator(".student-history-row:visible").count(), 0);
  assert.equal(await page.getByRole("button", { name: "Assessment history", exact: true }).getAttribute("aria-expanded"), "false");
  await page.getByRole("button", { name: "Edit student", exact: true }).click();
  await page.getByRole("heading", { name: "Edit student", exact: true }).waitFor();
  assert.equal(await page.getByLabel("Student name", { exact: true }).inputValue(), "Sample reader");
  assert.equal(await page.getByLabel("School", { exact: true }).inputValue(), "synthetic-school");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByLabel("Filter by school", { exact: true }).selectOption("Sample School");
  await page.getByRole("button", { name: "Delete student", exact: true }).click();
  assert(await page.getByRole("button", { name: "Delete student permanently", exact: true }).isDisabled());
  await page.getByLabel("Type the student's full name to confirm", { exact: true }).fill("Sample reader");
  assert(!(await page.getByRole("button", { name: "Delete student permanently", exact: true }).isDisabled()));
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByLabel("Filter by school", { exact: true }).selectOption("Sample School");
  await page.getByRole("button", { name: "Start reading assessment", exact: true }).click();
  await page.getByLabel("Reading language").selectOption("hindi");
  assert.equal(await page.locator(".reading-text").getAttribute("lang"), "hi");
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByRole("button", { name: "Stop recording", exact: true }).waitFor();
  await page.waitForFunction(() => document.querySelector(".recording-timer").textContent >= "00:02");
  await page.getByRole("button", { name: "Stop recording", exact: true }).click();
  await page.getByLabel("Review your recording").waitFor();
  await page.screenshot({ path: "/tmp/setu-reading-live-desktop.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Record again", exact: true }).click();
  assert(await page.locator(".mobile-recording-dock").isVisible());
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  role = "ORG_ADMIN";
  await page.evaluate(() => sessionStorage.setItem("setu-org", "unrelated-organization"));
  await page.reload();
  await page.getByRole("region", { name: "Your organization", exact: true }).waitFor();
  assert.equal(await page.locator("#nav .organization-name").textContent(), "Sample Organization");
  assert.equal(await page.locator('#nav select[name="organization"]').count(), 0);
  assert.equal(await page.evaluate(() => sessionStorage.getItem("setu-org")), "synthetic-org");
  const identityBox = await page.locator(".organization-identity").boundingBox();
  const applicationsBox = await page.getByRole("button", { name: "Applications", exact: true }).boundingBox();
  assert(identityBox.y + identityBox.height <= applicationsBox.y);

  await page.getByRole("button", { name: "Students", exact: true }).click();
  await page.getByRole("rowheader", { name: "Sample reader", exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "Add student with authorization", includeHidden: true }).count(), 0);
  assert.equal(await page.getByRole("button", { name: "Edit student", exact: true }).count(), 0);
  assert.equal(await page.getByRole("button", { name: "Delete student", exact: true }).count(), 0);
  await checkSchoolFilter();
  await page.getByRole("button", { name: "Assessment history", exact: true }).click();
  await page.getByText("Sample transcript", { exact: true }).waitFor();
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await page.getByRole("button", { name: "Hide assessment history", exact: true }).click();
  assert.equal(await page.getByRole("button", { name: "Start reading assessment" }).count(), 0);
  role = "VIEWER"; await page.reload();
  await page.getByRole("button", { name: "Students", exact: true }).click();
  await page.getByRole("rowheader", { name: "Sample reader", exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "Start reading assessment" }).count(), 0);
  assert.equal(await page.getByRole("button", { name: "Add student with authorization", includeHidden: true }).count(), 0);
  assert.equal(await page.getByRole("button", { name: "Edit student", exact: true }).count(), 0);
  assert.equal(await page.getByRole("button", { name: "Delete student", exact: true }).count(), 0);
  assert.deepEqual(writes, []); assert.deepEqual(errors, []);
  console.log("PASS: deployed teacher edit/delete confirmation and landing/recording/playback, Hindi passage, mobile controls, student table, collapsed history/loading/error/retry/refresh, admin read-only student UI and school filtering for both roles, viewer restrictions; synthetic responses only, no data writes.");
} finally { await browser.close(); }
