// Local synthetic data only.
import assert from "node:assert/strict";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "/tmp/setu-browser-check/node_modules/playwright-core/index.mjs");
const browser = await chromium.launch({ executablePath: "/usr/bin/google-chrome", headless: true, args: ["--no-sandbox", "--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const errors = []; page.on("pageerror", e => errors.push(e.message));
try {
  const base = process.env.PLATFORM_URL || "http://localhost:8080";
  await page.goto(base);
  const login = async account => (await (await page.request.post(base + "/api/local/session", { data: { account } })).json()).token;
  const admin = await login("alpha-admin");
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), admin);
  await page.reload(); await page.getByRole("button", { name: "Schools", exact: true }).click();
  const name = "Browser School " + Date.now();
  for (const [key, value] of Object.entries({ name, block: "Sample Block", district: "Sample District", state: "Sample State" }))
    await page.locator('form [name="' + key + '"]').fill(value);
  const added = page.waitForResponse(r => r.url().endsWith("/api/tenant/schools") && r.request().method() === "POST");
  await page.getByRole("button", { name: "Add school", exact: true }).click();
  const response = await added; assert.equal(response.status(), 201); const school = await response.json();
  await page.getByRole("rowheader", { name, exact: true }).waitFor();
  await page.getByRole("button", { name: "Students", exact: true }).click();
  assert.equal(await page.getByRole("button", { name: "Add student with authorization", includeHidden: true }).count(), 0);
  const teacher = await login("teacher");
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), teacher); await page.reload();
  await page.locator(".new-student-section > summary").click();
  await page.getByLabel("School", { exact: true }).selectOption(school.sk);
  await page.locator('[name=name]').fill("School-linked learner");
  await page.getByLabel("Age", { exact: true }).selectOption("8");
  await page.getByLabel("Class", { exact: true }).selectOption("3");
  await page.getByLabel("Guardian name", { exact: true }).fill("Synthetic Guardian");
  await page.getByLabel("Relationship", { exact: true }).fill("Parent");
  await page.locator('[name=adult]').check(); await page.locator('[name=authority]').check();
  const saved = page.waitForResponse(r => r.url().endsWith("/api/tenant/students") && r.request().method() === "POST");
  await page.getByRole("button", { name: "Add student with authorization", exact: true }).click();
  const studentResponse = await saved; assert.equal(studentResponse.status(), 201);
  const student = await studentResponse.json(); assert.equal(student.schoolId, school.sk); assert.equal(student.schoolName, name);
  await page.getByText("Student added.", { exact: true }).waitFor();
  const org = await page.locator("select[name=organization]").inputValue();
  const denied = await page.request.post(base + "/api/tenant/schools", { headers: { Authorization: "Bearer " + teacher, "X-Organization-Id": org }, data: { name: "Denied", block: "B", district: "D", state: "S" } });
  assert.equal(denied.status(), 403);
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), teacher); await page.reload();
  assert.equal(await page.getByRole("button", { name: "Schools", exact: true }).count(), 0);
  await page.getByRole("button", { name: "Students", exact: true }).click();
  assert.equal(await page.getByLabel("School", { exact: true }).count(), 1);
  assert.equal(await page.getByRole("button", { name: "Add student with authorization", includeHidden: true }).count(), 1);
  const teacherHeaders = { Authorization: "Bearer " + teacher, "X-Organization-Id": org };
  const deniedStudent = await page.request.post(base + "/api/tenant/students", { headers: { ...teacherHeaders, Authorization: "Bearer " + admin }, data: {
    student: { name: "Denied student", age: 8, className: "3", schoolId: school.sk }, consent: {},
  } });
  assert.equal(deniedStudent.status(), 403);
  await page.getByLabel("Find a student").fill(name);
  const reader = page.locator("tr.student-row").filter({ hasText: name });
  await reader.getByRole("button", { name: "Start reading assessment", exact: true }).click();
  await page.getByLabel("Reading passage").selectOption("en-1");
  await page.getByRole("button", { name: "Start recording", exact: true }).click();
  await page.getByText("Recording… Read the passage aloud, then stop.", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Students", exact: true }).click();
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), admin); await page.reload();
  await page.getByRole("button", { name: "Schools", exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  assert.deepEqual(errors, []);
  console.log("PASS: admin school creation/table, student dropdown and association, teacher student creation and recording, denied admin student creation and teacher school creation, mobile layout.");
} finally { await browser.close(); }
