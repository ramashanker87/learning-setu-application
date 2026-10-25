import { readingDashboard } from "./features/reading-dashboard.js";
import { schools } from "./features/schools.js";
import { api, session, callback } from "./api.js";
import { el, dataEl, action, select, message } from "./ui.js";
import { onboarding, company, publicApplication } from "./features/onboarding.js";
import { students, studentHistory } from "./features/assessment.js";
import { team, privacy } from "./features/team.js";
import { landing } from "./features/landing.js";
import { getLanguage, setLanguage, updatePageLanguage, startInterfaceTranslation } from "./language.js";
startInterfaceTranslation();
const root = document.querySelector("#app"),
  nav = document.querySelector("#nav");
window.addEventListener("setu-language-change", () => updatePageLanguage(getLanguage()));
async function show(render) {
  for (const button of nav.querySelectorAll("button[data-view]")) {
    if (button.dataset.view === render.name) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  }
  message("");
  root.replaceChildren(el("p", "Opening your workspace…", { class: "workspace-loading", role: "status" }));
  try {
    const fragment = document.createElement("div");
    await render(fragment);
    root.replaceChildren(fragment);
  } catch (e) {
    const panel = el("section", undefined, { class: "card recovery-panel", role: "alert" });
    panel.append(el("h1", "We couldn’t open this page"), el("p", e.message), action("Try again", () => show(render)));
    root.replaceChildren(panel);
  }
}
async function boot() {
  session.config = await api("/config");
  try {
    await callback();
  } catch (error) {
    sessionStorage.removeItem("setu-token");
    session.token = null;
    document.body.classList.add("landing-page");
    nav.replaceChildren();
    landing(root, boot);
    message(error.message + " Please sign in again.", true);
    return;
  }
  document.body.classList.toggle("landing-page", !session.token);
  document.body.classList.toggle("application-mode", location.hash === "#apply");
  if (location.hash === "#apply") {
    updatePageLanguage(getLanguage());
    for (const button of document.querySelectorAll("[data-language]")) button.onclick = () => setLanguage(button.dataset.language);
    document.body.classList.add("landing-page");
    nav.replaceChildren();
    await show(publicApplication);
    return;
  }
  if (!session.token) {
    nav.replaceChildren();
    landing(root, boot);
    return;
  }
  updatePageLanguage(getLanguage());
  for (const button of document.querySelectorAll("[data-language]")) {
    button.onclick = () => setLanguage(button.dataset.language);
  }
  try {
    session.me = await api("/me");
  } catch (e) {
    sessionStorage.removeItem("setu-token");
    session.token = null;
    document.body.classList.add("landing-page");
    document.querySelector("#logout").hidden = true;
    nav.replaceChildren();
    landing(root, boot);
    message("We couldn’t verify your access. Please sign in again. " + e.message, true);
    return;
  }
  const logout = document.querySelector("#logout");
  logout.hidden = false;
  logout.onclick = () => {
    sessionStorage.clear();
    location.href = "/";
  };
  nav.replaceChildren();
  const invite = new URLSearchParams(location.hash.slice(1)).get("invite");
  if (invite) {
    root.replaceChildren(
      el("h1", "Accept team invitation"),
      el("p", `Signed in as ${session.me.email}`),
      action("Accept invitation", async () => {
        await api("/invitations/accept", "POST", { token: invite });
        history.replaceState(null, "", "/");
        await boot();
      }),
    );
    return;
  }
  const active = session.me.memberships.filter((m) => m.state === "ACTIVE" && m.active !== false);
  const current = active.find(m => m.orgId === session.org) || active[0];
  session.org = current?.orgId || null;
  if (session.org) sessionStorage.setItem("setu-org", session.org);
  else sessionStorage.removeItem("setu-org");
  const organizationAdmin = current?.role === "ORG_ADMIN" && !session.me.company;
  if (organizationAdmin) {
    const identity = el("section", undefined, { class: "organization-identity", "aria-label": "Your organization" });
    identity.append(
      el("p", "YOUR ORGANIZATION", { class: "organization-eyebrow" }),
      (current.displayName ? dataEl : el)("h2", current.displayName || "Organization workspace", { class: "organization-name" }),
      el("p", "Administrator workspace", { class: "organization-role" }),
    );
    nav.append(identity);
  }
  if (!session.me.teamAccount) nav.append(navigation("Applications", onboarding));
  if (session.me.company)
    nav.append(navigation("Company review", company));
  if (current) {
    if (!organizationAdmin) {
      const org = select(
        nav,
        "organization",
        "Organization",
        active.map((m) => [m.orgId, m.displayName, true]),
        session.org,
      );
      org.onchange = () => {
        session.org = org.value;
        sessionStorage.setItem("setu-org", session.org);
        boot().catch(bootError);
      };
    }
    nav.append(
      navigation("Dashboard", dashboard),
      navigation("Students", students),
      navigation("Privacy settings", privacy),
    );
    if (active.find((m) => m.orgId === session.org).role === "ORG_ADMIN")
      nav.append(navigation("Schools", schools), navigation("Team", team));
  }
  nav.append(
    action("Profile", () =>
      show(async (r) => {
        r.append(el("h1", "Profile"), dataEl("p", session.me.email));
        r.append(
          el(
            "p",
            "Production accounts use managed identity verification and MFA. Company accounts require MFA; organization administrators can enroll through the managed identity service.",
          ),
        );
      }),
    ),
    action("Refresh access", () => boot()),
  );
  await show(
    session.me.company ? company : active.find(m => m.orgId === session.org)?.role === "ASSESSOR" ? students : active.length ? dashboard : onboarding,
  );
}
function navigation(label, render) {
  const button = action(label, () => show(render));
  button.dataset.view = render.name;
  return button;
}
async function dashboard(r) {
  const d = await api("/tenant/dashboard");
  await readingDashboard(r, d.organization, () => show(students), student => show(root => studentHistory(root, student)));
}

function bootError(error) {
  const panel = el("section", undefined, { class: "card recovery-panel", role: "alert" });
  panel.append(el("h1", "Let’s get you back to your workspace"),
    el("p", "We couldn’t finish loading Learning Setu. Check your connection and try again."),
    el("p", error.message), action("Try again", () => boot().catch(bootError)));
  root.replaceChildren(panel);
}
window.addEventListener("hashchange", () => {
  if (["#apply", ""].includes(location.hash)) boot().catch(bootError);
});
boot().catch(bootError);
