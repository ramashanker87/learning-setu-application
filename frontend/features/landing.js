import { api, session, setToken, signIn } from "../api.js";
import { el, action, select } from "../ui.js";
import { getLanguage, setLanguage, translate, updatePageLanguage } from "../language.js";

export function landing(root, onSignIn) {
  const language = getLanguage();
  const t = (text) => translate(text, language);
  const selectedAccount = root.querySelector('select[name="account"]')?.value;
  updatePageLanguage(language);
  for (const button of document.querySelectorAll("[data-language]")) {
    button.onclick = () => {
      setLanguage(button.dataset.language);
      landing(root, onSignIn);
    };
  }
  // Only trusted translation strings enter this template; account data uses textContent.
  root.innerHTML = `
    <section class="landing-hero" aria-labelledby="hero-title">
      <div class="hero-copy">
        <p class="eyebrow"><span></span> ${t("YOUR SCHOOL’S READING WORKSPACE")}</p>
        <h1 id="hero-title">${t("A stronger start.<br>For every reader.")}</h1>
        <p class="hero-description">${t("Understand where your learners are today.<br class=\"desktop-break\"> Help them take their next step with confidence.")}</p>
        <p class="hero-detail">${t("Reading assessments, student records, and your teaching team — connected in one workspace.")}</p>
        <a class="button apply-primary" href="/#apply">${t("Apply to register your organization")} →</a><br><a class="hero-link" href="#how-it-works">${t("Explore how it works")} <span aria-hidden="true">↓</span></a>
        <div class="school-subjects" aria-label="${t("Reading assessment")}"><span>${t("Student records")}</span><span>${t("Reading sessions")}</span><span>${t("Learning insights")}</span></div>
        <div class="hero-note"><span class="note-line" aria-hidden="true"></span><p>${t("Built for educators.")}<br><strong>${t("Centered on learners.")}</strong></p></div>
      </div>
      <section class="signin-panel" aria-labelledby="signin-title">
        <div class="panel-topline"><span class="panel-icon" aria-hidden="true">▤</span><span>${t("TEACHER & SCHOOL SIGN-IN")}</span></div>
        <h2 id="signin-title">${t("Sign in to Learning Setu")}</h2>
        <p class="signin-description">${t("Sign in to access your organization and continue supporting your learners.")}</p>
        <div id="signin-controls"></div>
        <div class="signin-footnote"><span aria-hidden="true">◎</span><p>${t("New organization? Apply for registration without creating an account.")}<br><a href="/#apply">${t("Apply to register your organization")} →</a></p></div>
      </section>
    </section>
    <section class="workflow" id="how-it-works" aria-labelledby="workflow-title">
      <div class="workflow-heading"><div><p class="eyebrow">${t("From reading to understanding")}</p><h2 id="workflow-title">${t("A clear path for every assessment.")}</h2></div><p>${t("Less time managing the process.<br>More focus on the learner.")}</p></div>
      <div class="workflow-steps">
        <article><span class="step-number">${t("01 / Prepare")}</span><h3>${t("Bring your learners together")}</h3><p>${t("Manage student records and give your team the access they need in your organization’s workspace.")}</p></article>
        <article><span class="step-number">${t("02 / Listen")}</span><h3>${t("Make room for every voice")}</h3><p>${t("Start a reading session and record a learner’s reading for transcription and assessment.")}</p></article>
        <article><span class="step-number">${t("03 / Understand")}</span><h3>${t("Turn reading into insight")}</h3><p>${t("Review completed assessments and results to help inform your next conversation with each learner.")}</p></article>
      </div>
    </section>
    <footer class="landing-footer"><span>${t("Learning Setu")}</span><p>${t("Connecting learners, educators, and possibility.")}</p><span>${t("Reading assessment")}</span></footer>
  `;
  const controls = root.querySelector("#signin-controls");
  if (session.config.authMode === "local") {
    controls.append(el("p", t("Demo workspace"), { class: "demo-label" }));
    const account = select(
      controls,
      "account",
      t("Choose an account"),
      session.config.accounts.map((u) => [u.sub, u.email]),
      selectedAccount,
    );
    const button = action(t("Sign in to workspace →"), async () => {
      setToken((await api("/local/session", "POST", { account: account.value })).token);
      await onSignIn();
    });
    button.className = "signin-primary";
    if (!session.config.accounts.length) button.disabled = true;
    controls.append(button, el("p", t("Local demonstration · Synthetic accounts only"), { class: "demo-caption" }));
  } else {
    const organization = action(t("Organization sign-in →"), () => signIn());
    organization.className = "signin-primary";
    const company = action(t("Company administrator sign-in"), () => signIn(true));
    company.className = "signin-secondary";
    controls.append(organization, el("p", t("Continue to secure sign-in, then return to your school workspace."), { class: "signin-guidance" }), company);
  }
}
