import { translate } from "../language.js";
import { api, upload, session, signIn } from "../api.js";
import { el, dataEl, field, select, form, card, action, message } from "../ui.js";
export async function onboarding(root) {
  root.append(el("h1", "Your organization applications"));
  const list = await api("/applications");
  for (const o of list.items) {
    const c = card(
      root,
      o.draft.displayName || "Draft application",
      `Status: ${o.state}${o.reason ? " — " + o.reason : ""}`,
    );
    if (o.draft.displayName) c.querySelector("h2").setAttribute("translate", "no");
    if (["DRAFT", "CHANGES_REQUESTED"].includes(o.state))
      c.append(action("Continue application", () => edit(root, o)));
  }
}
async function edit(root, o, isPublic = false) {
  const request = isPublic ? applicationAPI : api;
  root.replaceChildren(el("h1", "Apply to register your organization"));
  if (isPublic) root.append(action("← Application overview", () => publicApplication(root)));
  root.append(
    el(
      "p",
      "We use these details to review your organization and administer access. Supporting documents inform review; they do not establish legal compliance. Do not upload Aadhaar.",
    ),
  );
  const { f, finish } = form(root, "Save application", async (data) => {
    const draft = {
      ...data,
      formVersion: "organization-v2",
      identifiers: { pan: data.pan.trim().toUpperCase(), gstin: data.gstin.trim().toUpperCase() },
      termsAccepted: f.elements.termsAccepted.checked,
      authorityDeclared: f.elements.authorityDeclared.checked,
      noticeVersion: "2026-10-v1",
    };
    delete draft.pan;
    delete draft.gstin;
    if (isPublic && !o.id) {
      const created = await applicationAPI("/applications", "POST", draft);
      sessionStorage.setItem("setu-application", JSON.stringify({ id: created.application.id, accessKey: created.accessKey }));
      await publicApplication(root);
      message("Details saved. Keep your reference and access key, then upload the required documents.");
      return;
    }
    const updated = await request(`/applications/${o.id}`, "PUT", draft);
    await edit(root, updated, isPublic);
    message(
      "Application saved. Upload supporting documents before submission.",
    );
  });
  const d = o.draft;
  const fields = el("div", undefined, { class: "fields" });
  f.append(fields);
  const previousName = (d.representative?.name || "").trim().split(/\s+/);
  const values = {
    firstName: d.firstName || previousName[0] || "",
    lastName: d.lastName || previousName.slice(1).join(" "),
    address: d.address,
    contactEmail: d.contactEmail,
    contactPhone: d.contactPhone,
    pan: d.identifiers?.pan,
    gstin: d.identifiers?.gstin,
    orgNo: d.orgNo || d.registrationNumber,
  };
  for (const [key, label, type, required] of [
    ["firstName", "First Name", "text", true],
    ["lastName", "Last Name", "text", true],
    ["address", "Registered address", "textarea", true],
    ["contactEmail", "Email", "email", true],
    ["contactPhone", "Phone", "tel", true],
    ["pan", "PAN (optional)", "text", false],
    ["gstin", "GSTIN (optional)", "text", false],
    ["orgNo", "OrgNo", "text", true],
  ]) {
    const input = field(fields, key, label, values[key] || "", type, required);
    input.maxLength = key === "pan" ? 10 : key === "gstin" ? 15 : 1000;
    if (key === "pan" || key === "gstin") input.pattern = `[A-Za-z0-9]{${key === "pan" ? 10 : 15}}`;
  }
  f.append(el("p", "OrgNo is your organization's registration number. PAN and GSTIN can be left blank if not applicable."));
  if (isPublic) {
    f.append(el("p", "Use the email address of your future organization administrator. After approval, sign in with this email to activate access."));
    if (o.id) f.elements.contactEmail.readOnly = true;
  } else f.append(el("p", `Verified representative email: ${session.me.email}`));
  field(
    f,
    "termsAccepted",
    "I accept the service terms",
    d.termsAccepted,
    "checkbox",
  );
  field(
    f,
    "authorityDeclared",
    "I am authorized to represent this organization",
    d.authorityDeclared,
    "checkbox",
  );
  f.append(
    el(
      "p",
      "Privacy notice 2026-10-v1: application information is used for organization review, account administration and security. Contact the service operator for correction, deletion or grievances.",
    ),
  );
  finish();
  if (!o.id) return;
  const doc = card(
    root,
    "Supporting documents",
    "Upload the supporting document listed below. PDF, up to 5 MB.",
  );
  const requiredDocuments = session.config.policies[d.type]?.documents || ["registration"];
  const kind = select(doc, "kind", "Document type", requiredDocuments.map((kind) => [
    kind, kind === "recognition" ? "School recognition" : "Organization registration",
  ]));
  const file = field(doc, "document", "Document", "", "file");
  file.accept = "application/pdf";
  doc.append(
    action("Upload document", async () => {
      const value = file.files[0];
      if (!value) throw new Error("Choose a PDF");
      const init = await request(`/applications/${o.id}/documents`, "POST", {
        kind: kind.value,
        type: value.type,
        bytes: value.size,
      });
      await upload(init.upload, value);
      await request(
        `/applications/${o.id}/documents/${init.sk}/complete`,
        "POST",
        {},
      );
      message("Document uploaded.");
    }),
  );
  root.append(
    action("Submit for review", async () => {
      await request(`/applications/${o.id}/submit`, "POST", {});
      root.replaceChildren();
      await (isPublic ? publicApplication(root) : onboarding(root));
      message(
        "Application submitted. Assessment access starts after approval and setup.",
      );
    }),
  );
}
export async function company(root, view = { expanded: new Set(), query: "", state: "" }) {
  root.append(el("h1", "Company review"));
  const { items, integrationAvailability = {} } = await api("/company/organizations");
  const toolbar = el("div", undefined, { class: "review-toolbar" });
  root.append(toolbar);
  const search = field(toolbar, "organizationSearch", "Search organizations", view.query, "search", false);
  search.placeholder = "Name, email, OrgNo or ID";
  const status = select(toolbar, "organizationStatus", "Status", [["", "All statuses"], ...[...new Set(items.map(o => o.state))].sort().map(s => [s, s.replaceAll("_", " ")])], view.state);
  status.setAttribute("aria-label", "Status");
  const count = el("p", "", { role: "status", class: "review-count" });
  root.append(count);
  const scroll = el("div", undefined, { class: "review-table-scroll", tabindex: "0", role: "region", "aria-label": "Organization review table" });
  const table = el("table", undefined, { class: "review-table" });
  table.append(el("caption", "Organizations — expand a row to review details and take action."));
  const head = el("thead");
  const headings = el("tr");
  for (const label of ["Organization", "Contact email", "Status", "Details"]) headings.append(el("th", label, { scope: "col" }));
  head.append(headings); table.append(head);
  const body = el("tbody"); table.append(body); scroll.append(table); root.append(scroll);
  const rows = [];
  for (const o of items) {
    o.draft ||= {};
    const name = o.draft.displayName || o.draft.legalName || (o.state === "OFFBOARDED" ? "Offboarded organization" : "Draft application");
    const summary = el("tr", undefined, { class: "review-summary" });
    const organization = el("th", undefined, { scope: "row" });
    organization.append(dataEl("strong", name), el("small", o.draft.orgNo ? `OrgNo: ${o.draft.orgNo}` : o.id));
    summary.append(organization, dataEl("td", o.draft.contactEmail || o.email || "—"));
    const stateCell = el("td"); stateCell.append(el("span", o.state.replaceAll("_", " "), { class: "review-status", "data-state": o.state }));
    summary.append(stateCell);
    const detailRow = el("tr", undefined, { class: "review-detail", id: `review-${o.id}` });
    detailRow.hidden = !view.expanded.has(o.id);
    const toggle = action(detailRow.hidden ? "Expand" : "Collapse", () => {
      detailRow.hidden = !detailRow.hidden;
      if (detailRow.hidden) view.expanded.delete(o.id); else view.expanded.add(o.id);
      toggle.textContent = detailRow.hidden ? "Expand" : "Collapse";
      toggle.setAttribute("aria-expanded", String(!detailRow.hidden));
    });
    toggle.setAttribute("aria-expanded", String(!detailRow.hidden));
    toggle.setAttribute("aria-controls", detailRow.id);
    toggle.setAttribute("aria-label", `Organization details: ${name}`);
    const toggleCell = el("td"); toggleCell.append(toggle); summary.append(toggleCell);
    const detailCell = el("td", undefined, { colspan: "4" }); detailRow.append(detailCell);
    body.append(summary, detailRow);
    rows.push({ o, summary, detailRow, searchable: [name, o.id, o.email, ...Object.values(o.draft).filter(v => typeof v === "string")].join(" ").toLowerCase() });
    const c = card(
      detailCell,
      o.draft.displayName || (o.state === "OFFBOARDED" ? `Offboarded organization ${o.id}` : "Draft"),
      undefined,
    );
    c.append(dataEl("span", o.draft.legalName || ""), el("span", ` · ${o.state.replaceAll("_", " ")}`));
    if (o.draft.displayName) c.querySelector("h2").setAttribute("translate", "no");
    c.append(dataEl("p", o.reason || o.error || ""));
    const offboarding = ["OFFBOARDING", "OFFBOARDING_FAILED", "OFFBOARDED"].includes(o.state);
    if (o.offboarding) {
      c.append(el("p", `Cleanup: ${o.offboarding.phase.replaceAll("_", " ")}`));
      if (o.state === "OFFBOARDING" && o.offboarding.cleanupAfter)
        c.append(el("p", `Access is blocked. Cleanup can begin after ${new Date(o.offboarding.cleanupAfter).toLocaleString()}, once existing jobs and upload links have expired.`));
      if (o.state === "OFFBOARDED")
        c.append(el("p", "Live organization data and dedicated storage have been deleted. A minimal cleanup receipt remains. Backups, exported/downloaded copies, and shared sign-in accounts are not erased by this operation."));
      if (o.offboarding.providerFollowUpRequired)
        c.append(el("p", "Transcription-provider copies require separate deletion confirmation. Their references are retained for operator follow-up."));
    }
    if (o.draft.representative)
      c.append(
        el(
          "p",
          `Representative: ${o.draft.representative.name}${o.draft.representative.designation ? " · " + o.draft.representative.designation : ""} · ${o.representativeEmail || o.email}`,
        ),
      );
    for (const [key, value] of Object.entries(o.draft.identifiers || {}))
      if (value) c.append(el("p", `${key.toUpperCase()}: ${value}`));
    const labels = { firstName: "First Name", lastName: "Last Name", address: "Registered address", contactEmail: "Email", contactPhone: "Phone", orgNo: "OrgNo" };
    for (const [k, v] of Object.entries(o.draft))
      if (typeof v === "string" && !["formVersion", "noticeVersion"].includes(k)) c.append(el("p", `${labels[k] || k}: ${v}`));
    if (o.publicApplication) c.append(el("p", "Public application · Contact email is unverified until the applicant signs in to activate access."));
    if (!offboarding) c.append(
      action("Review documents", async () => {
        const docs = await api(`/company/organizations/${o.id}/documents`);
        for (const d of docs.items)
          c.append(
            el("a", `Open ${d.kind} document`, {
              href: d.url,
              target: "_blank",
              rel: "noreferrer noopener",
            }),
          );
      }),
    );
    if (o.state === "ACTIVE" && session.config.activationEmailsEnabled) {
      const emailState = o.activationEmail?.state;
      const descriptions = {
        QUEUED: "Login invitation is queued.",
        INVITATION_REQUESTED: "Cognito accepted the invitation request. The recipient should check Inbox and Spam. The temporary password expires in 7 days and must be changed at first sign-in.",
        EXISTING_ACCOUNT: "Activation notice sent. This user signs in with their existing password.",
        EXISTING_ACCOUNT_EMAIL_DISABLED: "Administrator access is ready for an existing account. Its password was not changed. A separate approval notice needs the general email sender to be configured.",
        FAILED: "Login invitation failed. Check account status and Cognito email availability, then retry.",
      };
      c.append(el("p", descriptions[emailState] || "Login invitation has not been requested yet."));
      c.append(action(emailState === "FAILED" ? "Retry login email" : "Send/resend login invitation", async () => {
        await api(`/company/organizations/${o.id}/activation-email`, "POST", {});
        root.replaceChildren(); await company(root, view);
        message("Login invitation queued. Existing passwords are preserved; temporary invitations may be renewed.");
      }));
    }
    if (["ACTIVE", "SUSPENDED"].includes(o.state)) {
      const integrationsEnabled = o.integrationsEnabled ?? !session.config.integrationsDisabled;
      const integrations = card(c, "Organization integrations", `IntegrationsEnabled: ${integrationsEnabled ? "true" : "false"}${o.integrationsEnabled == null ? " (inherited deployment default)" : ""}`);
      integrations.append(el("p", "This organization setting overrides the deployment default for assessment processing and general email invitations. It does not change other organizations, sign-in invitations or stored credentials."));
      integrations.append(el("p", `Assessment service: ${integrationAvailability.assessmentConfigured ? "configured" : "not configured or worker unavailable"}. Email service: ${integrationAvailability.emailConfigured ? "configured" : "not configured or worker unavailable"}.`));
      integrations.append(el("p", "Configured means credentials are present; it does not verify provider credit or successful delivery. Disabling pauses queued work; already-running requests may finish."));
      const integrationReason = field(integrations, "integrationReason", "Reason for integration change", "", "text", true);
      integrationReason.maxLength = 1000;
      integrations.append(action(integrationsEnabled ? "Disable organization integrations" : "Enable organization integrations", async () => {
        if (!integrationReason.reportValidity()) return;
        await api(`/company/organizations/${o.id}/integrations`, "POST", { enabled: !integrationsEnabled, reason: integrationReason.value });
        root.replaceChildren(); await company(root, view);
        message("Organization integrations setting saved.");
      }));
      const enabled = o.assessmentProcessingEnabled !== false;
      const processing = card(c, "Assessment processing", enabled ? "Enabled for this organization." : "Disabled for this organization.");
      if (!integrationsEnabled || !integrationAvailability.assessmentConfigured) processing.append(el("p", !integrationsEnabled ? "Enable Organization integrations above to allow submissions. This assessment-only setting cannot override disabled organization integrations." : "The assessment service is not configured or its worker is unavailable. Ask the service operator to configure the credentials and check the worker.", { class: "notice" }));
      if (o.state === "SUSPENDED") processing.append(el("p", "Organization access remains suspended regardless of this setting."));
      processing.append(el("p", "Disabling blocks new submissions and retries and pauses queued assessments. Work already sent to the provider may finish. Enabling resumes queued work when the platform service is available."));
      const settingReason = field(processing, "processingReason", "Reason for processing change", "", "text", true);
      settingReason.maxLength = 1000;
      processing.append(action(enabled ? "Disable assessment processing" : "Enable assessment processing", async () => {
        if (!settingReason.reportValidity()) return;
        await api(`/company/organizations/${o.id}/assessment-processing`, "POST", { enabled: !enabled, reason: settingReason.value });
        root.replaceChildren(); await company(root, view);
        message("Organization assessment setting saved.");
      }));
    }
    const reason = field(c, "reason", "Review reason", "", "text", false);
    const actions =
      {
        SUBMITTED: ["review"],
        UNDER_REVIEW: ["approve", "changes", "reject"],
        ACTIVE: ["suspend"],
        SUSPENDED: ["reactivate"],
        PROVISIONING_FAILED: ["retry"],
      }[o.state] || [];
    for (const a of actions)
      c.append(
        action(a, async () => {
          await api(`/company/organizations/${o.id}/${a}`, "POST", {
            reason: reason.value,
          });
          root.replaceChildren();
          await company(root, view);
        }),
      );
    if (!["OFFBOARDING", "OFFBOARDED"].includes(o.state)) {
      const cleanup = card(c, "Permanently offboard organization",
        "This cannot be undone. Access is blocked immediately; after an 11-minute drain period, cleanup deletes students, assessments, all audio versions and the audio bucket, the tenant table, registration documents, memberships, invitations and jobs. Backups/provider copies may require follow-up. Shared sign-in accounts remain.");
      const confirmation = field(cleanup, "confirmation", `Type organization ID to confirm: ${o.id}`);
      cleanup.append(action(o.state === "OFFBOARDING_FAILED" ? "Retry offboarding" : "Permanently offboard", async () => {
        if (confirmation.value !== o.id || !reason.value.trim())
          throw new Error("Enter the exact organization ID and a review reason.");
        if (!window.confirm(translate("Permanently delete this organization's live data and storage? This action cannot be undone."))) return;
        await api(`/company/organizations/${o.id}/offboard`, "POST", {
          confirmation: confirmation.value,
          reason: reason.value,
        });
        root.replaceChildren();
        await company(root, view);
        message("Offboarding queued. Organization access is blocked; use Refresh status to follow cleanup.");
      }));
    }
    c.append(
      action("Audit history", async () => {
        const events = await api(`/company/organizations/${o.id}/audit`);
        for (const e of events.items)
          c.append(dataEl("p", `${e.at} · ${e.actor} · ${e.action} ${e.reason}`));
      }),
    );
  }
  const empty = el("p", "", { class: "review-empty" }); root.append(empty);
  const filter = () => {
    view.query = search.value; view.state = status.value;
    let visible = 0;
    for (const row of rows) {
      const matches = (!view.state || row.o.state === view.state) && row.searchable.includes(view.query.trim().toLowerCase());
      row.summary.hidden = !matches;
      row.detailRow.hidden = !matches || !view.expanded.has(row.o.id);
      if (matches) visible++;
    }
    count.textContent = `${visible} of ${items.length} organizations`;
    empty.hidden = visible > 0;
    empty.textContent = items.length ? "No organizations match your search or status filter." : "No organization applications yet.";
  };
  search.addEventListener("input", filter); status.addEventListener("change", filter); filter();
  root.append(
    action("Refresh status", async () => {
      root.replaceChildren();
      await company(root, view);
    }),
  );
}

function savedApplication() {
  try { return JSON.parse(sessionStorage.getItem("setu-application") || "null"); }
  catch { sessionStorage.removeItem("setu-application"); return null; }
}
async function applicationAPI(path, method = "GET", body) {
  const saved = savedApplication();
  const response = await fetch(`/api/public${path}`, {
    method,
    headers: { "Content-Type": "application/json", ...(saved ? { "X-Application-Key": saved.accessKey } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "Application request failed");
  return data;
}
export async function publicApplication(root) {
  root.classList.add("application-page");
  root.replaceChildren(el("a", "← Back to Learning Setu", { href: "/" }),
    el("p", "ORGANIZATION APPLICATION", { class: "eyebrow" }),
    el("h1", "Bring Learning Setu to your organization"),
    el("p", "Apply as a school, NGO, or school group. No account or sign-in is needed to apply. Our team will review your details and supporting documents."));
  const saved = savedApplication();
  if (saved) {
    const security = card(root, "Keep your application details", "Keep this tab open to return to your application. Save the reference and private access key for your records. Treat the access key as confidential. " + (session.config.activationEmailsEnabled ? "After approval and setup, new users receive a login invitation by email. Existing users keep their current password." : "No application emails are sent while activation email is disabled."));
    const reference = field(security, "reference", "Application reference", saved.id); reference.readOnly = true;
    const key = field(security, "accessKey", "Private access key", saved.accessKey, "password"); key.readOnly = true;
    security.append(action("Show or hide access key", () => { key.type = key.type === "password" ? "text" : "password"; }));
    security.append(action("Forget application on this device", async () => {
      sessionStorage.removeItem("setu-application"); await publicApplication(root);
    }));
    let o;
    try { o = await applicationAPI(`/applications/${encodeURIComponent(saved.id)}`); }
    catch (e) { root.append(el("p", e.message, { class: "error" })); return; }
    const status = card(root, o.draft.displayName, `Status: ${o.state.replaceAll("_", " ")}`);
    status.querySelector("h2").setAttribute("translate", "no");
    if (o.reason) status.append(dataEl("p", o.reason));
    if (["DRAFT", "CHANGES_REQUESTED"].includes(o.state)) status.append(action("Continue application & upload documents", () => edit(root, o, true)));
    if (o.state === "ACTIVE") {
      if (o.activationAccessGranted) {
        status.append(el("p", `Administrator access is ready for ${o.draft.contactEmail}. New users should sign in with the emailed temporary password and choose a new password. Existing users should use their current password.`));
        status.append(action(session.token ? "Open organization workspace" : "Sign in to your organization", async () => {
          if (!session.token) { await signIn(); return; }
          sessionStorage.setItem("setu-org", o.id); location.href = "/";
        }));
      } else {
      status.append(el("p", `Your organization is approved and ready. ${session.config.activationEmailsEnabled ? "Your login invitation is being prepared. You can also" : "You can"} sign in or create an account with ${o.draft.contactEmail} to activate administrator access.`));
      status.append(action(session.token ? "Activate administrator access" : "Sign in to activate access", async () => {
        if (!session.token) { await signIn(); return; }
        await api(`/applications/${o.id}/claim`, "POST", { accessKey: saved.accessKey });
        sessionStorage.removeItem("setu-application"); sessionStorage.setItem("setu-org", o.id); location.href = "/";
      }));
      }
    } else if (!["DRAFT", "CHANGES_REQUESTED", "REJECTED"].includes(o.state)) status.append(el("p", "Your application is with our review team. Return here to check its progress."));
    root.append(action("Refresh application status", () => publicApplication(root)));
    return;
  }
  const steps = card(root, "How to apply", "1. Enter your organization details. 2. Save your reference and access key. 3. Upload supporting documents and submit for review.");
  steps.append(action("Start application", () => edit(root, { draft: {} }, true)));
}
