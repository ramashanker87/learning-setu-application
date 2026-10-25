#!/usr/bin/env python3
"""Validate nonsecret deployment settings; emit one CLI argument per line."""
import json
import sys
from urllib.parse import urlsplit

path, environment, prefix, image = sys.argv[1:]
with open(path, encoding="utf-8") as stream:
    values = json.load(stream)
required = {"VpcId", "PublicSubnets", "PrivateSubnets"}
integration_fields = {"SmtpHost", "SmtpSecretArn", "SarvamSecretArn", "MailFrom"}
optional = integration_fields | {"EnableActivationEmails", "DisableIntegrations", "SarvamModel", "AccessMode", "CloudFrontPrefixListId", "CertificateArn", "AppOrigin"}
if not isinstance(values, dict):
    raise SystemExit("Deployment parameters must be a JSON object")
activation = values.get("EnableActivationEmails", "false")
if activation not in {"true", "false"}:
    raise SystemExit("EnableActivationEmails must be the string true or false")
disabled = values.get("DisableIntegrations", "false")
if disabled not in {"true", "false"}:
    raise SystemExit("DisableIntegrations must be the string true or false")
if disabled == "false":
    required |= integration_fields - {"SarvamSecretArn"}
mode = values.get("AccessMode", "custom-domain")
if mode not in {"custom-domain", "cloudfront"}:
    raise SystemExit("AccessMode must be custom-domain or cloudfront")
required |= {"CertificateArn", "AppOrigin"} if mode == "custom-domain" else {"CloudFrontPrefixListId"}
if required - values.keys():
    raise SystemExit("Missing deployment parameters: " + ", ".join(sorted(required - values.keys())))
if set(values) - required - optional:
    raise SystemExit("Unknown deployment parameter")
for key, value in values.items():
    if not isinstance(value, str) or (not value and key in required) or any(c in value for c in "\r\n") or "REPLACE" in value:
        raise SystemExit("Fill in deployment parameter " + key)
if mode == "custom-domain":
    origin = urlsplit(values["AppOrigin"])
    if origin.scheme != "https" or not origin.hostname or origin.username or origin.password or origin.path or origin.query or origin.fragment:
        raise SystemExit("AppOrigin must be an HTTPS origin without a path, query, or trailing slash")
else:
    if values.get("CertificateArn") or values.get("AppOrigin"):
        raise SystemExit("Omit CertificateArn and AppOrigin for cloudfront; the stack generates the HTTPS origin")
# Explicit defaults prevent stale settings when updating an existing stack.
values.setdefault("AccessMode", "custom-domain")
values.setdefault("DisableIntegrations", "false")
values.setdefault("EnableActivationEmails", "false")
for key in integration_fields:
    values.setdefault(key, "")
for key in ("CertificateArn", "AppOrigin", "CloudFrontPrefixListId"):
    values.setdefault(key, "")
values.update(Environment=environment, Prefix=prefix, ImageUri=image)
for key, value in values.items():
    print(f"{key}={value}")
