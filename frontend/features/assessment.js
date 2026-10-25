import { assessmentCard } from "./assessment-card.js";
import { editStudent, deleteStudent } from "./student-management.js";
import { readingSession } from "./recorder.js";
import { allStudents, api, session } from "../api.js";
import { el, dataEl, field, select, form, action, message } from "../ui.js";
export async function students(root) {
  const processing = await api("/tenant/assessment-settings");
  session.assessmentProcessing = processing;
  const role = session.me.memberships.find(m => m.orgId === session.org)?.role;
  root.append(el("p", role === "ASSESSOR" ? "TEACHER WORKSPACE" : "STUDENT DIRECTORY", { class: "section-kicker" }),
    el("h1", role === "ASSESSOR" ? "Start with your student" : "Students"),
    el("p", role === "ASSESSOR" ? "Add a student or choose an existing student to record their reading." : "View students, filter by school name, and review their reading history."));
  const { items } = await allStudents();
  const filters = el("div", undefined, { class: "student-filters" });
  const search = field(filters, "studentSearch", "Find a student", "", "search", false);
  search.placeholder = "Search by name or school";
  const schoolNames = [...new Set(items.map(s => s.schoolName).filter(Boolean))].sort();
  const schoolFilter = select(filters, "studentSchool", "Filter by school", [["", "All schools"], ...schoolNames.map(name => [name, name, true])]);
  schoolFilter.setAttribute("aria-label", "Filter by school");
  root.append(filters);
  const count = el("p", undefined, { role: "status", class: "student-count" });
  const list = el("div", undefined, { class: "student-directory" });
  root.append(count, list);
  function display() {
    const query = search.value.trim().toLocaleLowerCase();
    const visible = items.filter(s => (!schoolFilter.value || s.schoolName === schoolFilter.value) && `${s.name} ${s.schoolName || ""}`.toLocaleLowerCase().includes(query));
    count.textContent = `${visible.length} of ${items.length} students`;
    list.querySelectorAll("audio").forEach(audio => audio.pause());
    list.replaceChildren();
    if (!visible.length) list.append(el("p", items.length ? "No students match your search." : role === "ASSESSOR" ? "No students yet. Add your first student below." : "No students yet. A teacher can add students.", { class: "notice" }));
    if (!visible.length) return;
    const scroll = el("div", undefined, { class: "review-table-scroll", tabindex: "0", role: "region", "aria-label": "Student directory" });
    const table = el("table", undefined, { class: "review-table student-table" });
    table.append(el("caption", "Students and assessment history"));
    const head = el("thead"), headings = el("tr");
    for (const label of ["Student name", "Age", "Class", "School", "Authorization", "Actions"])
      headings.append(el("th", label, { scope: "col" }));
    head.append(headings); table.append(head);
    const body = el("tbody");
    table.append(body); scroll.append(table); list.append(scroll);
    for (const [index, s] of visible.entries()) {
      const row = el("tr", undefined, { class: "student-row" });
      row.append(dataEl("th", s.name, { scope: "row" }), el("td", s.age ?? "—"), dataEl("td", s.className), s.schoolName ? dataEl("td", s.schoolName) : el("td", "School not recorded"));
      const authorized = s.consent?.status === "GRANTED";
      row.append(el("td", authorized ? "Ready for reading" : "Guardian authorization required", { class: authorized ? "student-ready" : "student-pending" }));
      const actions = el("td", undefined, { class: "student-actions" });
      row.append(actions);
      const detail = el("tr", undefined, { class: "student-history-row" });
      detail.hidden = true;
      const cell = el("td", undefined, { colspan: "6" });
      const panel = el("section", undefined, { id: `student-history-${index}`, "aria-label": `${s.name}: assessment history` });
      panel.append(el("h2", `${s.name}: assessment history`));
      const content = el("div");
      panel.append(content); cell.append(panel); detail.append(cell);
      let loaded = false;
      const past = action("Assessment history", async () => {
        detail.hidden = !detail.hidden;
        past.setAttribute("aria-expanded", String(!detail.hidden));
        past.textContent = detail.hidden ? "Assessment history" : "Hide assessment history";
        if (detail.hidden) content.querySelectorAll("audio").forEach(audio => audio.pause());
        else if (!loaded) loaded = await assessmentHistory(content, s);
      });
      past.className = "quiet-button";
      past.setAttribute("aria-expanded", "false");
      past.setAttribute("aria-controls", panel.id);
      actions.append(past);
      if (role === "ASSESSOR") {
        const back = async () => { root.replaceChildren(); await students(root); };
        const start = action("Start reading assessment", () => readingSession(root, s, back, () => history(root, s)));
        start.disabled = !authorized;
        const edit = action("Edit student", () => editStudent(root, s, back)); edit.className = "quiet-button";
        const remove = action("Delete student", () => deleteStudent(root, s, back)); remove.className = "danger-outline-button";
        actions.append(start, edit, remove);
      }
      body.append(row, detail);
    }
  }
  search.oninput = display; schoolFilter.onchange = display; display();
  if (role !== "ASSESSOR") return;
  const { items: schools } = await api("/tenant/schools");
  if (!schools.length) {
    root.append(el("p", "Ask your organization administrator to add a school before adding students."));
    return;
  }
  const newStudent = el("details", undefined, { class: "new-student-section" });
  const toggle = el("summary", "New student");
  newStudent.append(toggle);
  root.append(newStudent);
  const { f, finish } = form(
    newStudent,
    "Add student with authorization",
    async (d) => {
      await api("/tenant/students", "POST", {
        student: { formVersion: "student-v3", name: d.name, age: Number(d.age), className: d.className, schoolId: d.schoolId, email: d.email.trim(), mobile: d.mobile.trim() },
        consent: {
          guardianName: d.guardianName,
          relationship: d.relationship,
          noticeVersion: "2026-10-v1",
          purposes: ["reading_assessment"],
          guardianAdultVerified: f.elements.adult.checked,
          authorityVerified: f.elements.authority.checked,
        },
      });
      root.replaceChildren();
      await students(root);
      message("Student added.");
    },
  );
  f.setAttribute("aria-label", "New student details");
  field(f, "name", "Student name");
  field(f, "email", "Email address (optional)", "", "email", false).maxLength = 254;
  field(f, "mobile", "Mobile number (optional)", "", "tel", false).maxLength = 30;
  const age = select(f, "age", "Age", [["", "Select age"], ...Array.from({ length: 12 }, (_, i) => [String(i + 4), String(i + 4)])]);
  age.required = true; age.setAttribute("aria-label", "Age");
  const className = select(f, "className", "Class", [["", "Select class"], ...["2nd", "3rd", "4th", "5th", "6th", "7th", "8th", "9th", "10th"].map((label, i) => [String(i + 2), label])]);
  className.required = true; className.setAttribute("aria-label", "Class");
  const school = select(f, "schoolId", "School", [["", "Select school"], ...schools.map(s => [s.sk, `${s.name} — ${s.block}, ${s.district}, ${s.state}`, true])]);
  school.required = true; school.setAttribute("aria-label", "School");
  f.append(
    el(
      "p",
      "Purpose: record oral reading, obtain a transcript and calculate reading indicators for learning support. No provider model training is authorized. Guardian verification must be performed before recording this decision.",
    ),
  );
  field(f, "guardianName", "Guardian name");
  field(f, "relationship", "Relationship");
  field(
    f,
    "adult",
    "I verified that the guardian is an adult",
    false,
    "checkbox",
  );
  field(
    f,
    "authority",
    "I verified guardian authority and obtained consent to the stated purpose",
    false,
    "checkbox",
  );
  finish();
}
async function history(root, s) {
  root.replaceChildren(action("← All students", async () => { root.replaceChildren(); await students(root); }), el("h1", `${s.name}: assessment history`));
  const content = el("div");
  root.append(content);
  await assessmentHistory(content, s);
}
async function assessmentHistory(root, s) {
  root.querySelectorAll("audio").forEach(audio => audio.pause());
  root.replaceChildren(el("p", "Loading assessment history…", { role: "status" }));
  let items;
  try {
    ({ items } = await api(`/tenant/students/${s.sk}/assessments`));
  } catch (error) {
    root.replaceChildren(el("p", error.message || "Could not load assessment history.", { class: "error", role: "alert" }), action("Retry loading history", () => assessmentHistory(root, s)));
    return false;
  }
  root.replaceChildren();
  if (!items.length) root.append(el("p", "No assessments yet for this student.", { class: "notice" }));
  if (items.length) root.append(el("p", `${items.length} ${items.length === 1 ? "reading session" : "reading sessions"}`, { class: "assessment-history-count" }));
  for (const a of items) {
    const c = assessmentCard(a);
    root.append(c);
    if (a.error) c.append(el("p", a.error, { class: "error" }));
    if (a.state === "FAILED" && session.assessmentProcessing?.effectiveEnabled && session.me.memberships.find(m => m.orgId === session.org)?.role === "ASSESSOR")
      c.append(
        action("Retry processing", async () => {
          await api(`/tenant/assessments/${a.sk}/retry`, "POST", {});
          message("Retry queued.");
        }),
      );
    if (a.version) {
      let player;
      c.append(
        action("Play recording", async () => {
          if (player) { player.focus(); return; }
          const { url } = await api(`/tenant/assessments/${a.sk}/audio`);
          const audio = el("audio", undefined, { controls: "", "aria-label": "Assessment recording", tabindex: "0" });
          player = audio;
          audio.src = url;
          c.append(audio);
          audio.focus();
        }),
      );
    }
  }
  root.append(action("Refresh", () => assessmentHistory(root, s)));
  return true;
}

export { history as studentHistory };
