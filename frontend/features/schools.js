import { api, session } from "../api.js";
import { el, dataEl, field, form, message } from "../ui.js";

export async function schools(root) {
  root.append(el("h1", "Schools"));
  const role = session.me.memberships.find(m => m.orgId === session.org)?.role;
  const { items } = await api("/tenant/schools");
  root.append(el("p", "Schools added here are available in the New student dropdown for this organization."));
  if (items.length) {
    const scroll = el("div", undefined, { class: "review-table-scroll", tabindex: "0", role: "region", "aria-label": "Organization schools" });
    const table = el("table", undefined, { class: "review-table" });
    table.append(el("caption", `${items.length} schools`));
    const head = el("thead"), row = el("tr");
    for (const label of ["School name", "Block", "District", "State"]) row.append(el("th", label, { scope: "col" }));
    head.append(row); table.append(head);
    const body = el("tbody");
    for (const school of items) {
      const row = el("tr");
      row.append(dataEl("th", school.name, { scope: "row" }));
      for (const key of ["block", "district", "state"]) row.append(dataEl("td", school[key]));
      body.append(row);
    }
    table.append(body); scroll.append(table); root.append(scroll);
  } else root.append(el("p", "No schools have been added yet."));
  if (role !== "ORG_ADMIN") return;
  const { f, finish } = form(root, "Add school", async data => {
    await api("/tenant/schools", "POST", data);
    root.replaceChildren(); await schools(root); message("School added. It is now available when adding students.");
  });
  f.append(el("h2", "Add school"));
  for (const [key, label] of [["name", "School name"], ["block", "Block"], ["district", "District"], ["state", "State"]])
    field(f, key, label).maxLength = 200;
  finish();
}
