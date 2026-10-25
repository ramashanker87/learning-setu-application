import { api } from "../api.js";
import { el, field, select, form, action, message } from "../ui.js";

export async function editStudent(root, student, back) {
  const { items: schools } = await api("/tenant/schools");
  root.replaceChildren(action("← All students", back), el("p", "STUDENT DETAILS", { class: "section-kicker" }), el("h1", "Edit student"));
  root.append(el("p", "Update the student's details. Their existing assessments and guardian authorization will be kept."));
  const { f, finish } = form(root, "Save changes", async data => {
    const correction = { name: data.name.trim(), age: Number(data.age), className: data.className, email: data.email.trim(), mobile: data.mobile.trim() };
    if (data.schoolId) correction.schoolId = data.schoolId;
    await api("/tenant/privacy", "POST", { kind: "correct", studentId: student.sk, correction });
    await back(); message("Student details updated.");
  });
  field(f, "name", "Student name", student.name).maxLength = 200;
  field(f, "email", "Email address (optional)", student.email || "", "email", false).maxLength = 254;
  field(f, "mobile", "Mobile number (optional)", student.mobile || "", "tel", false).maxLength = 30;
  const age = select(f, "age", "Age", Array.from({ length: 12 }, (_, i) => [String(i + 4), String(i + 4)]), String(student.age));
  age.required = true; age.setAttribute("aria-label", "Age");
  const className = select(f, "className", "Class", ["2nd", "3rd", "4th", "5th", "6th", "7th", "8th", "9th", "10th"].map((label, i) => [String(i + 2), label]), student.className);
  className.required = true; className.setAttribute("aria-label", "Class");
  const options = schools.map(s => [s.sk, `${s.name} — ${s.block}, ${s.district}, ${s.state}`, true]);
  if (!student.schoolId) options.unshift(["", `Keep existing school${student.schoolName ? ": " + student.schoolName : ""}`]);
  else if (!schools.some(s => s.sk === student.schoolId)) options.unshift(["", "Select an organization school"]);
  const school = select(f, "schoolId", "School", options, student.schoolId);
  school.setAttribute("aria-label", "School");
  school.required = !!student.schoolId;
  finish();
  const cancel = action("Cancel", back); cancel.className = "quiet-button"; f.append(cancel);
}

export function deleteStudent(root, student, back) {
  root.replaceChildren(action("← All students", back), el("p", "DELETE STUDENT", { class: "section-kicker" }), el("h1", `Delete ${student.name}?`));
  root.append(el("p", `Class ${student.className} · ${student.schoolName || "School not recorded"}`));
  root.append(el("p", "This immediately removes access to the student and queues deletion of their student record, assessments and recordings. This action cannot be undone.", { class: "notice" }));
  const { f, finish } = form(root, "Delete student permanently", async data => {
    if (data.confirmName !== student.name) throw new Error("Enter the student's full name exactly to confirm deletion.");
    await api("/tenant/privacy", "POST", { kind: "delete", studentId: student.sk });
    await back(); message("Student removed from the list. Data deletion is queued.");
  });
  const confirmation = field(f, "confirmName", "Type the student's full name to confirm");
  confirmation.autocomplete = "off";
  finish();
  const remove = f.querySelector('[type="submit"]'); remove.className = "danger-button"; remove.disabled = true;
  confirmation.oninput = () => { remove.disabled = confirmation.value !== student.name; };
  const cancel = action("Cancel", back); cancel.className = "quiet-button"; f.append(cancel);
}
