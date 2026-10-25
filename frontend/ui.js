export function el(tag, text, attrs = {}) {
  const n = document.createElement(tag);
  if (text !== undefined) n.textContent = text;
  for (const [k, v] of Object.entries(attrs))
    if (k === "class") n.className = v;
    else n.setAttribute(k, v);
  return n;
}
// API content must never be interpreted as interface copy, even if it happens
// to equal a label (for example a student named "Student").
export function dataEl(tag, text, attrs = {}) {
  return el(tag, text, { ...attrs, translate: "no" });
}
export function message(text, error = false) {
  const n = document.querySelector("#message");
  n.textContent = text;
  n.className = error ? "error" : "notice";
}
export function action(label, fn) {
  const b = el("button", label, { type: "button" });
  b.addEventListener("click", async () => {
    b.disabled = true;
    try {
      await fn();
    } catch (e) {
      message(e.message, true);
    } finally {
      b.disabled = false;
    }
  });
  return b;
}
export function field(
  parent,
  name,
  label,
  value = "",
  type = "text",
  required = true,
) {
  const l = el("label", label);
  const n = el(type === "textarea" ? "textarea" : "input", undefined, {
    name,
    ...(type === "textarea" ? {} : { type }),
  });
  if (type === "checkbox") n.checked = !!value;
  else n.value = value;
  n.required = required;
  l.append(n);
  parent.append(l);
  return n;
}
export function select(parent, name, label, options, value) {
  const l = el("label", label);
  const n = el("select", undefined, { name, "aria-label": label });
  for (const [v, t, verbatim] of options) {
    const o = el("option", t, { value: v, ...(verbatim ? { translate: "no" } : {}) });
    n.append(o);
  }
  if (value) n.value = value;
  l.append(n);
  parent.append(l);
  return n;
}
export function form(parent, submit, callback) {
  const f = el("form");
  f.addEventListener("submit", async (e) => {
    e.preventDefault();
    const b = f.querySelector("[type=submit]");
    b.disabled = true;
    try {
      await callback(Object.fromEntries(new FormData(f)), f);
    } catch (err) {
      message(err.message, true);
    } finally {
      b.disabled = false;
    }
  });
  const b = el("button", submit, { type: "submit" });
  parent.append(f);
  return { f, finish: () => f.append(b) };
}
export function card(parent, title, text) {
  const c = el("section", undefined, { class: "card" });
  c.append(el("h2", title));
  if (text) c.append(el("p", text));
  parent.append(c);
  return c;
}
export const showJSON = (parent, value) =>
  parent.append(dataEl("pre", JSON.stringify(value, null, 2)));
