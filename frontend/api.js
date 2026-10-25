/** @typedef {import('./contracts').Identity} Identity */
export const session = {
  token: sessionStorage.getItem("setu-token"),
  org: sessionStorage.getItem("setu-org"),
  me: null,
  config: null,
};
export async function api(path, method = "GET", body) {
  const r = await fetch(`/api${path}`, {
    method,
    signal: AbortSignal.timeout(20000),
    headers: {
      "Content-Type": "application/json",
      ...(session.token ? { Authorization: `Bearer ${session.token}` } : {}),
      ...(session.org ? { "X-Organization-Id": session.org } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await r.json();
  if (!r.ok) throw new Error(data.error || "Request failed");
  return data;
}
export async function upload(upload, file) {
  const r = await fetch(upload.url, {
    method: upload.method,
    headers: upload.headers,
    body: file,
  });
  if (!r.ok)
    throw new Error(
      "Upload failed. Check your connection, then create a new recording.",
    );
}
export function setToken(token) {
  session.token = token;
  sessionStorage.setItem("setu-token", token);
}
export async function signIn(company = false) {
  const c = session.config;
  const domain = company ? c.companyDomain : c.cognitoDomain,
    client = company ? c.companyClient : c.cognitoClient;
  if (!domain || !client) throw new Error("Sign-in is not configured");
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  const b64 = (b) =>
    btoa(String.fromCharCode(...b))
      .replaceAll("+", "-")
      .replaceAll("/", "_")
      .replaceAll("=", "");
  const verifier = b64(bytes),
    state = crypto.randomUUID();
  sessionStorage.setItem(
    "setu-oauth",
    JSON.stringify({ verifier, state, domain, client, returnTo: location.hash === "#apply" ? "#apply" : "" }),
  );
  const challenge = b64(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier)),
    ),
  );
  // Classic Cognito hosted UI ignores prompt=login. Clear its session before
  // organization sign-in so a shared browser can choose an admin or team account.
  const endpoint = company ? "/oauth2/authorize" : "/logout";
  location.href = `${domain}${endpoint}?${new URLSearchParams({ response_type: "code", client_id: client, redirect_uri: location.origin + "/", scope: "openid email profile", state, code_challenge: challenge, code_challenge_method: "S256" })}`;
}
export async function callback() {
  const query = new URLSearchParams(location.search);
  if (query.has("error")) {
    sessionStorage.removeItem("setu-oauth");
    history.replaceState(null, "", "/");
    throw new Error("Sign-in was cancelled or could not be completed.");
  }
  if (!query.has("code")) return;
  // Consume the response URL once, including unsuccessful callbacks.
  history.replaceState(null, "", "/");
  const saved = JSON.parse(sessionStorage.getItem("setu-oauth") || "null");
  sessionStorage.removeItem("setu-oauth");
  if (!saved || saved.state !== query.get("state"))
    throw new Error("Invalid sign-in response");
  const r = await fetch(saved.domain + "/oauth2/token", {
    method: "POST",
    signal: AbortSignal.timeout(20000),
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "authorization_code",
      client_id: saved.client,
      redirect_uri: location.origin + "/",
      code: query.get("code"),
      code_verifier: saved.verifier,
    }),
  });
  if (!r.ok) throw new Error("Sign-in failed");
  const result = await r.json();
  if (!result.id_token) throw new Error("Sign-in did not return a valid session");
  setToken(result.id_token);
  history.replaceState(null, "", "/" + (saved.returnTo === "#apply" ? "#apply" : ""));
}

export async function allStudents() {
  const items = [];
  let cursor;
  do {
    const r = await api(
      "/tenant/students" +
        (cursor ? "?cursor=" + encodeURIComponent(cursor) : ""),
    );
    items.push(...r.items);
    cursor = r.cursor;
  } while (cursor);
  return { items };
}
