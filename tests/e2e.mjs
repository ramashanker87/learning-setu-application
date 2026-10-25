import assert from "node:assert/strict";
import { application, consent } from "./fixtures.mjs";
const base = process.env.PLATFORM_URL || "http://localhost:8080";
async function req(
  path,
  { token, org, method = "GET", body, status = 200 } = {},
) {
  const r = await fetch(`${base}/api${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(org ? { "X-Organization-Id": org } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await r.json();
  assert.equal(r.status, status, `${method} ${path}: ${JSON.stringify(data)}`);
  return data;
}
const login = async (account) =>
  (await req("/local/session", { method: "POST", body: { account } })).token;
async function wait(fn, predicate) {
  for (let i = 0; i < 90; i++) {
    const r = await fn();
    if (predicate(r)) return r;
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error("Workflow timed out");
}
async function put(upload, bytes) {
  const r = await fetch(upload.url, {
    method: "PUT",
    headers: upload.headers,
    body: bytes,
  });
  assert.equal(r.status, 200, await r.text());
}
await req("/tenant/students", {status:401});
const token = await login("applicant"),
  company = await login("company");
const o = await req("/applications", {
  token,
  method: "POST",
  body: { ...application, displayName: `E2E School ${Date.now()}` },
  status: 201,
});
await req("/tenant/students", { token, org: o.id, status: 403 });
const pdf = Buffer.from("%PDF-1.4\nSynthetic end-to-end document\n%%EOF");
const d = await req(`/applications/${o.id}/documents`, {
  token,
  method: "POST",
  body: { kind: "recognition", type: "application/pdf", bytes: pdf.length },
  status: 201,
});
await put(d.upload, pdf);
await req(`/applications/${o.id}/documents/${d.sk}/complete`, {
  token,
  method: "POST",
  body: {},
});
await req(`/applications/${o.id}/submit`, { token, method: "POST", body: {} });
await req(`/company/organizations/${o.id}/review`, {
  token,
  method: "POST",
  body: {},
  status: 403,
});
await req(`/company/organizations/${o.id}/review`, {
  token: company,
  method: "POST",
  body: {},
});
await req(`/company/organizations/${o.id}/approve`, {
  token: company,
  method: "POST",
  body: {},
});
await req(`/company/organizations/${o.id}/approve`, {
  token: company,
  method: "POST",
  body: {},
});
await wait(
  () => req("/applications", { token }),
  (r) => r.items.find((x) => x.id === o.id)?.state === "ACTIVE",
);
const ctx = { token, org: o.id };
assert.equal((await req("/tenant/passages",ctx)).items.length,22);
const priorMail = new Set(
  (
    await fetch("http://localhost:8025/api/v1/messages").then((r) => r.json())
  ).messages.map((m) => m.ID),
);
await req("/tenant/invitations", {
  ...ctx,
  method: "POST",
  body: { email: "invitee@example.test", role: "ASSESSOR" },
  status: 201,
});
const inbox = await wait(
  async () =>
    fetch("http://localhost:8025/api/v1/messages").then((r) => r.json()),
  (r) =>
    r.messages.some(
      (m) =>
        !priorMail.has(m.ID) &&
        m.To?.some((t) => t.Address === "invitee@example.test"),
    ),
);
const mail = inbox.messages.find(
  (m) =>
    !priorMail.has(m.ID) &&
    m.To?.some((t) => t.Address === "invitee@example.test"),
);
const body = await fetch(
  `http://localhost:8025/api/v1/message/${mail.ID}`,
).then((r) => r.json());
const invitation = body.Text.match(/#invite=([A-Za-z0-9_-]+)/)[1];
const invited = await login("invitee");
await req("/invitations/accept", {
  token: invited,
  method: "POST",
  body: { token: invitation },
});
await req("/invitations/accept", {
  token: invited,
  method: "POST",
  body: { token: invitation },
  status: 403,
});
const school = await req("/tenant/schools", { token, org: o.id, method: "POST", status: 201, body: {name: "E2E School", block: "Test Block", district: "Test District", state: "Test State"} });
const student = await req("/tenant/students", {
  token: invited, org: o.id,
  method: "POST",
  body: {
    student: { name: "Synthetic E2E learner", age: 9, className: "4", schoolId: school.sk },
    consent,
  },
  status: 201,
});
const audioBytes = 32000 * 20;
const bytes = Buffer.alloc(audioBytes + 44);
bytes.write("RIFF");
bytes.writeUInt32LE(bytes.length - 8, 4);
bytes.write("WAVEfmt ", 8);
bytes.writeUInt32LE(16, 16);
bytes.writeUInt16LE(1, 20);
bytes.writeUInt16LE(1, 22);
bytes.writeUInt32LE(16000, 24);
bytes.writeUInt32LE(32000, 28);
bytes.writeUInt16LE(2, 32);
bytes.writeUInt16LE(16, 34);
bytes.write("data", 36);
bytes.writeUInt32LE(audioBytes, 40);
const teacherContext = { token: invited, org: o.id };
const a = await req("/tenant/assessments", {
  ...teacherContext,
  method: "POST",
  body: {
    studentId: student.sk,
    passageId: "en-1",
    type: "audio/wav",
    bytes: bytes.length,
    duration: 20,
  },
  status: 201,
});
await put(a.upload, bytes);
await req(`/tenant/assessments/${a.sk}/complete`, {
  ...teacherContext,
  method: "POST",
  body: {},
  status: 202,
});
await req(`/tenant/assessments/${a.sk}/complete`, {
  ...teacherContext,
  method: "POST",
  body: {},
  status: 202,
});
const result = await wait(
  () => req(`/tenant/assessments/${a.sk}`, ctx),
  (r) => r.state === "COMPLETED",
);
assert.equal(result.result.score.accuracy_score, 100);
const playback = await req(`/tenant/assessments/${a.sk}/audio`, ctx);
assert.equal((await fetch(playback.url)).status, 200);
const beta = await login("beta-admin");
const bm = await req("/me", { token: beta });
const bo = bm.memberships[0].orgId;
await req("/tenant/students", { token: beta, org: o.id, status: 403 });
for (const path of [
  `/tenant/students/${student.sk}`,
  `/tenant/assessments/${a.sk}`,
  `/tenant/assessments/${a.sk}/audio`,
])
  await req(path, { token: beta, org: bo, status: 404 });
await req("/tenant/privacy", {
  token: beta,
  org: bo,
  method: "POST",
  body: { kind: "export", studentId: student.sk },
  status: 404,
});
await req("/tenant/members/invitee", {
  ...ctx,
  method: "PATCH",
  body: { role: "ASSESSOR", active: false },
});
await req("/tenant/students", { token: invited, org: o.id, status: 403 });
await req(`/company/organizations/${o.id}/suspend`, {
  token: company,
  method: "POST",
  body: { reason: "Synthetic test" },
});
await req("/tenant/students", { ...ctx, status: 403 });
await req(`/company/organizations/${o.id}/reactivate`, {
  token: company,
  method: "POST",
  body: { reason: "Test complete" },
});
await req("/tenant/privacy", {
  ...ctx,
  method: "POST",
  body: { kind: "withdraw", studentId: student.sk },
  status: 201,
});
await req(`/tenant/assessments/${a.sk}/audio`, { ...ctx, status: 409 });
await req("/tenant/members/invitee", {
  ...ctx, method: "PATCH", body: { role: "ASSESSOR", active: true },
});
await req("/tenant/privacy", {
  ...ctx,
  method: "POST",
  body: { kind: "delete", studentId: student.sk },
  token: invited,
  status: 201,
});
await wait(
  () => req("/tenant/students", ctx),
  (r) => !r.items.some((s) => s.sk === student.sk),
);
await wait(
  () => req("/tenant/privacy", ctx),
  (r) =>
    r.items.some(
      (p) =>
        p.kind === "delete" &&
        p.studentId === student.sk &&
        p.state === "LIVE_DATA_DELETED",
    ),
);
assert.equal((await fetch(playback.url)).status, 404);
console.log(
  "PASS: onboarding → document → review → provisioning → invitation/email → consent → upload → scoring → playback → isolation → revocation → suspension → withdrawal → deletion",
);
