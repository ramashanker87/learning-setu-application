import { allStudents, api, session } from "../api.js";
import {
  el,
  field,
  select,
  form,
  card,
  action,
  message,
  showJSON,
} from "../ui.js";
export async function team(root) {
  root.append(el("h1", "Team access"));
  const integrations = await api("/tenant/integration-settings");
  const roles = [
    ["ASSESSOR", "Assessor / teacher"],
    ["VIEWER", "Read-only viewer"],
    ["ORG_ADMIN", "Organization administrator"],
  ];
  if (session.config.authMode === "cognito") {
    const create = card(root, "Create team member", "Create an account for this organization only. Share the username, temporary password and application link directly with the member; no email is sent. They must change the password at first sign-in within 7 days.");
    const { f, finish } = form(create, "Create member", async (d) => {
      const result = await api("/tenant/members", "POST", d);
      password.value = "";
      root.replaceChildren(); await team(root);
      message(`Team member ${result.username} is ready. Use Organization sign-in with ${result.created ? "the temporary password you entered" : "the original temporary password (the pending request was completed without resetting it)"}.`);
    });
    field(f, "username", "Username (email address)", "", "email").autocomplete = "off";
    const password = field(f, "temporaryPassword", "Temporary password", "", "password");
    password.autocomplete = "new-password"; password.minLength = 14; password.maxLength = 128;
    f.append(el("p", "Use 14–128 characters with uppercase and lowercase letters, a number and a symbol, and no spaces. Existing accounts cannot be overwritten."));
    select(f, "role", "Member role", roles.filter(([value]) => value !== "ORG_ADMIN"));
    finish();
  } else {
    root.append(el("p", "Local demo uses predefined accounts. Creating members with passwords is available in the deployed application."));
  }
  for (const m of (await api("/tenant/members")).items) {
    const c = card(root, m.email, m.active ? "Active" : "Revoked");
    c.querySelector("h2").setAttribute("translate", "no");
    const role = select(c, "role", "Role", m.managedTeam ? roles.filter(([value]) => value !== "ORG_ADMIN") : roles, m.role);
    c.append(
      action("Save role", async () => {
        await api(`/tenant/members/${m.sk}`, "PATCH", {
          role: role.value,
          active: m.active,
        });
        message("Role saved.");
      }),
      action("Revoke access", async () => {
        await api(`/tenant/members/${m.sk}`, "PATCH", {
          role: role.value,
          active: false,
        });
        message("Access revoked.");
      }),
    );
  }
  if (!integrations.emailEnabled) {
    root.append(el("p", "Email invitations are unavailable for this organization. Ask the company administrator to check Organization integrations and email service availability. Members created above can sign in using the credentials you share directly."));
    return;
  }
  const { f, finish } = form(root, "Send invitation", async (d) => {
    await api("/tenant/invitations", "POST", d);
    message("Invitation queued for email delivery. It expires in 48 hours.");
  });
  field(f, "email", "Email", "", "email");
  select(f, "role", "Role", roles);
  finish();
}
export async function privacy(root) {
  root.append(el("h1", "Privacy and retention"));
  root.append(
    el(
      "p",
      "Privacy notice 2026-10-v1. Reading data supports learning assessment. Contact your organization administrator to withdraw authorization, request correction, export, deletion or raise a grievance.",
    ),
  );
  const info = await api("/tenant/dashboard");
  if (info.role !== "ORG_ADMIN") return;
  const retention = form(root, "Update retention", async (d) => {
    await api("/tenant/retention", "PUT", { days: Number(d.days) });
    message(
      "Retention saved. Shorter periods also apply to existing student records; extending retention does not revive expired records.",
    );
  });
  field(
    retention.f,
    "days",
    `Retention days (1–${session.config.retentionMaxDays})`,
    info.organization.retentionDays,
    "number",
  );
  retention.finish();
  const students = (await allStudents()).items;
  const { f, finish } = form(root, "Submit privacy request", async (d) => {
    const data = { kind: d.kind, studentId: d.studentId };
    const result = await api("/tenant/privacy", "POST", data);
    if (d.kind === "export") {
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(result, null, 2)], {
          type: "application/json",
        }),
      );
      const a = el("a", "Download export", {
        href: url,
        download: "student-export.json",
      });
      root.append(a);
    }
    message("Privacy request recorded.");
  });
  select(
    f,
    "studentId",
    "Student",
    students.map((s) => [s.sk, s.name, true]),
  );
  select(f, "kind", "Request", [
    ["withdraw", "Withdraw consent"],
    ["export", "Export student data"],
    ["grievance", "Raise a grievance"],
  ]);
  finish();
  for (const p of (await api("/tenant/privacy")).items) {
    const c = card(root, p.kind, `${p.state} · ${p.createdAt}`);
    if (p.kind === "grievance" && p.state === "OPEN") {
      const reason = field(c, "resolution", "Resolution");
      c.append(
        action("Resolve grievance", async () => {
          await api(`/tenant/privacy/${p.sk}/resolve`, "POST", {
            reason: reason.value,
          });
          message("Grievance resolved.");
        }),
      );
    }
  }
}
