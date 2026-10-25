// Local synthetic data only; verifies teacher edits and durable deletion cleanup.
import assert from "node:assert/strict";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "/tmp/setu-browser-check/node_modules/playwright-core/index.mjs");
const browser = await chromium.launch({ executablePath: "/usr/bin/google-chrome", headless: true, args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const base = process.env.PLATFORM_URL || "http://localhost:3000";
const errors = []; page.on("pageerror", e => errors.push(e.message));
try {
  const login = async account => (await (await page.request.post(base + "/api/local/session", { data: { account } })).json()).token;
  const admin = await login("alpha-admin"), teacher = await login("teacher"), viewer = await login("viewer");
  const me = await (await page.request.get(base + "/api/me", { headers: { Authorization: "Bearer " + admin } })).json();
  const org = me.memberships[0].orgId;
  const headers = token => ({ Authorization: "Bearer " + token, "X-Organization-Id": org });
  async function request(path, token, data, expected = 201) {
    const r = await page.request.post(base + "/api" + path, { headers: headers(token), data });
    assert.equal(r.status(), expected, await r.text()); return r.json();
  }
  const school = await request("/tenant/schools", admin, { name: "Edit school " + Date.now(), block: "B", district: "D", state: "S" });
  const name = "Managed reader " + Date.now();
  const student = await request("/tenant/students", teacher, { student: { name, age: 8, className: "3", schoolId: school.sk }, consent: { guardianName: "Synthetic parent", relationship: "Parent", noticeVersion: "2026-10-v1", purposes: ["reading_assessment"], guardianAdultVerified: true, authorityVerified: true } });
  await page.goto(base);
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), teacher); await page.reload();
  await page.getByLabel("Find a student").fill(name);
  await page.getByRole("button", { name: "Edit student", exact: true }).click();
  await page.getByRole("heading", { name: "Edit student", exact: true }).waitFor();
  assert.equal(await page.getByLabel("Student name", { exact: true }).inputValue(), name);
  assert.equal(await page.getByLabel("School", { exact: true }).inputValue(), school.sk);
  await page.getByLabel("Student name", { exact: true }).fill("Cancelled edit");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByLabel("Find a student").fill(name);
  await page.getByRole("button", { name: "Edit student", exact: true }).click();
  await page.getByRole("heading", { name: "Edit student", exact: true }).waitFor();
  const updatedName = name + " updated";
  await page.getByLabel("Student name", { exact: true }).fill(updatedName);
  await page.getByLabel("Age", { exact: true }).selectOption("9");
  await page.getByLabel("Class", { exact: true }).selectOption("4");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await page.getByText("Student details updated.", { exact: true }).waitFor();
  const updated = await (await page.request.get(base + `/api/tenant/students/${student.sk}`, { headers: headers(teacher) })).json();
  assert.equal(updated.name, updatedName); assert.equal(updated.age, 9); assert.equal(updated.className, "4");
  assert.equal(updated.schoolId, school.sk); assert.equal(updated.consent.recordedBy, student.consent.recordedBy);
  // Create a synthetic recording so deletion also verifies assessment/audio cleanup.
  const audio = Buffer.alloc(32044); audio.write("RIFF"); audio.writeUInt32LE(audio.length - 8, 4); audio.write("WAVEfmt ", 8); audio.writeUInt32LE(16, 16); audio.writeUInt16LE(1, 20); audio.writeUInt16LE(1, 22); audio.writeUInt32LE(16000, 24); audio.writeUInt32LE(32000, 28); audio.writeUInt16LE(2, 32); audio.writeUInt16LE(16, 34); audio.write("data", 36); audio.writeUInt32LE(32000, 40);
  const assessment = await request("/tenant/assessments", teacher, { studentId: student.sk, passageId: "en-1", type: "audio/wav", bytes: audio.length, duration: 1 });
  assert((await page.request.put(assessment.upload.url, { headers: assessment.upload.headers, data: audio })).ok());
  await request(`/tenant/assessments/${assessment.sk}/complete`, teacher, {}, 202);
  const playback = await (await page.request.get(base + `/api/tenant/assessments/${assessment.sk}/audio`, { headers: headers(teacher) })).json();
  assert(playback.url);
  await page.getByLabel("Find a student").fill(updatedName);
  await page.getByRole("button", { name: "Delete student", exact: true }).click();
  assert(await page.getByRole("button", { name: "Delete student permanently", exact: true }).isDisabled());
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  assert.equal((await page.request.get(base + `/api/tenant/students/${student.sk}`, { headers: headers(teacher) })).status(), 200);
  await page.getByLabel("Find a student").fill(updatedName);
  await page.getByRole("button", { name: "Delete student", exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await page.getByLabel("Type the student's full name to confirm", { exact: true }).fill("Wrong name");
  assert(await page.getByRole("button", { name: "Delete student permanently", exact: true }).isDisabled());
  await page.getByLabel("Type the student's full name to confirm", { exact: true }).fill(updatedName);
  await page.getByRole("button", { name: "Delete student permanently", exact: true }).click();
  await page.getByText("Student removed from the list. Data deletion is queued.", { exact: true }).waitFor();
  assert.equal((await page.request.get(base + `/api/tenant/students/${student.sk}`, { headers: headers(teacher) })).status(), 404);
  assert.equal((await page.request.get(base + `/api/tenant/assessments/${assessment.sk}`, { headers: headers(teacher) })).status(), 404);
  let deleted = false;
  for (let i = 0; i < 60; i++) {
    const requests = await (await page.request.get(base + "/api/tenant/privacy", { headers: headers(admin) })).json();
    if (requests.items.some(r => r.studentId === student.sk && r.kind === "delete" && r.state === "LIVE_DATA_DELETED")) { deleted = true; break; }
    await new Promise(resolve => setTimeout(resolve, 1000));
  }
  assert(deleted, "Deletion worker did not complete");
  assert.equal((await page.request.get(playback.url)).status(), 404);
  await page.evaluate(token => sessionStorage.setItem("setu-token", token), viewer); await page.reload();
  await page.getByRole("button", { name: "Students", exact: true }).click();
  await page.getByRole("heading", { name: "Students", exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "Edit student", exact: true }).count(), 0);
  assert.equal(await page.getByRole("button", { name: "Delete student", exact: true }).count(), 0);
  assert.deepEqual(errors, []);
  console.log("PASS: teacher edit/cancel/persistence, school association and consent preservation, delete confirmation/cancel, immediate access removal, durable student/assessment/audio cleanup, mobile layout, viewer restrictions.");
} finally { await browser.close(); }
