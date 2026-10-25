// Read-only hosted-login check. Does not submit credentials or create accounts.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "/tmp/setu-browser-check/node_modules/playwright-core/index.mjs");
const base = process.env.PLATFORM_URL;
assert(base, "Set PLATFORM_URL to the deployed HTTPS application URL");
const browser = await chromium.launch({ executablePath: "/usr/bin/google-chrome", headless: true, args: ["--no-sandbox"] });
const page = await browser.newPage();
try {
  if (process.env.TEST_WORKSPACE === "1") {
    const source = await readFile(new URL("../frontend/api.js", import.meta.url), "utf8");
    await page.route(base + "/api.js", route => route.fulfill({ contentType: "text/javascript", body: source }));
  }
  const config = await (await page.request.get(base + "/api/config")).json();
  for (let attempt = 0; attempt < 2; attempt++) {
    await page.goto(base);
    const outbound = page.waitForRequest(r => r.url().startsWith(config.cognitoDomain + "/logout?"));
    await page.getByRole("button", { name: "Organization sign-in", exact: false }).click();
    const request = new URL((await outbound).url());
    assert.equal(request.searchParams.get("client_id"), config.cognitoClient);
    assert.equal(request.searchParams.get("response_type"), "code");
    assert.equal(request.searchParams.get("redirect_uri"), base + "/");
    assert.equal(request.searchParams.get("code_challenge_method"), "S256");
    assert(request.searchParams.get("state"));
    assert(request.searchParams.get("code_challenge"));
    await page.waitForURL(url => url.origin === new URL(config.cognitoDomain).origin && url.pathname === "/login");
    const login = new URL(page.url());
    assert.equal(login.searchParams.get("state"), request.searchParams.get("state"));
    assert.equal(login.searchParams.get("code_challenge"), request.searchParams.get("code_challenge"));
    assert.equal(login.searchParams.get("code_challenge_method"), "S256");
    await page.locator('input[name="username"]:visible').waitFor();
    await page.locator('input[name="password"]:visible').waitFor();
  }
  console.log("PASS: repeated organization sign-in clears hosted session and displays username/password with state and PKCE preserved.");
} finally { await browser.close(); }
