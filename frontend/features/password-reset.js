import { session } from "../api.js";
import { el, field, form, action } from "../ui.js";

// Cognito verifies the email code; the tenant pool trigger restricts recovery to
// active organization administrators even when its API is called directly.
export function organizationPasswordReset(root, back) {
  root.replaceChildren(el("h3", "Reset organization administrator password"),
    el("p", "For existing organization administrators only. Teachers and other staff must contact their organization administrator."));
  const notice = el("p", "Enter your registered administrator email address.", { role: "status", "aria-live": "polite" });
  const container = el("div");
  root.append(notice, container);
  const request = async (operation, input) => {
    const issuer = new URL(session.config.cognitoIssuer);
    const response = await fetch(issuer.origin, {
      method: "POST",
      headers: { "Content-Type": "application/x-amz-json-1.1", "X-Amz-Target": `AWSCognitoIdentityProviderService.${operation}` },
      body: JSON.stringify({ ClientId: session.config.cognitoClient, ...input }),
      signal: AbortSignal.timeout(20000),
    });
    const result = await response.json();
    if (!response.ok) { const error = new Error("Password reset could not be completed."); error.code = (result.__type || "").split("#").pop(); throw error; }
  };
  function begin() {
    container.replaceChildren();
    const { f, finish } = form(container, "Send reset code", async data => {
      const email = data.email.trim();
      try { await request("ForgotPassword", { Username: email }); }
      catch (error) {
        if (!['UserNotFoundException', 'UserLambdaValidationException', 'InvalidParameterException', 'NotAuthorizedException'].includes(error.code)) {
          notice.textContent = "Unable to request a reset code right now. Please try again later."; return;
        }
      }
      notice.textContent = "If this email belongs to an eligible organization administrator, a verification code will be sent. Check your inbox.";
      confirm(email);
    });
    field(f, "email", "Registered administrator email", "", "email").autocomplete = "username";
    finish();
  }
  function confirm(email) {
    container.replaceChildren();
    const { f, finish } = form(container, "Reset password", async data => {
      if (data.password !== data.confirmPassword) { notice.textContent = "Passwords do not match."; return; }
      try {
        await request("ConfirmForgotPassword", { Username: email, ConfirmationCode: data.code.trim(), Password: data.password });
        container.replaceChildren(); notice.textContent = "Password reset complete. Return to sign-in and use your new password.";
      } catch (error) {
        notice.textContent = error.code === "InvalidPasswordException" ? "Use at least 14 characters with uppercase and lowercase letters, a number, and a symbol." : "Unable to reset the password. Check your code and password, or request a new code.";
      }
    });
    field(f, "code", "Email verification code").autocomplete = "one-time-code";
    f.append(el("p", "Use at least 14 characters, including uppercase and lowercase letters, a number, and a symbol."));
    for (const [name, label] of [["password", "New password"], ["confirmPassword", "Confirm new password"]]) {
      const input = field(f, name, label, "", "password"); input.minLength = 14; input.autocomplete = "new-password";
    }
    finish();
    container.append(action("Request a new code", begin));
  }
  begin();
  root.append(action("Back to sign-in", back));
}
