"""Deployment parameter checks; run with python3 -m unittest discover -s tests -p '*_test.py'."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class DeploymentParametersTest(unittest.TestCase):
    def run_parameters(self, values):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "parameters.json"
            path.write_text(json.dumps(values))
            return subprocess.run(
                ["python3", str(ROOT / "scripts/aws/parameters.py"), str(path),
                 "staging", "setu-staging", "example/image:tag"],
                capture_output=True, text=True,
            )

    def parameters(self, mode="cloudfront"):
        values = {key: "configured" for key in (
            "VpcId", "PublicSubnets", "PrivateSubnets", "SmtpHost",
            "SmtpSecretArn", "SarvamSecretArn", "MailFrom",
        )}
        values["AccessMode"] = mode
        if mode == "cloudfront":
            values["CloudFrontPrefixListId"] = "pl-12345678"
        else:
            values.update(AppOrigin="https://setu.example.org", CertificateArn="arn:certificate")
        return values

    def test_temporary_access_needs_no_domain_or_certificate(self):
        result = self.run_parameters(self.parameters())
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("AppOrigin=\n", result.stdout)
        self.assertIn("CertificateArn=\n", result.stdout)

    def test_managed_sarvam_secret_is_default(self):
        values = self.parameters()
        del values["SarvamSecretArn"]
        result = self.run_parameters(values)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("SarvamSecretArn=\n", result.stdout)

    def test_temporary_access_requires_prefix_list(self):
        values = self.parameters()
        del values["CloudFrontPrefixListId"]
        self.assertNotEqual(self.run_parameters(values).returncode, 0)

    def test_disabled_integrations_need_no_secrets(self):
        values = self.parameters()
        for key in ("SmtpHost", "SmtpSecretArn", "SarvamSecretArn", "MailFrom"):
            del values[key]
        self.assertNotEqual(self.run_parameters(values).returncode, 0)
        values["DisableIntegrations"] = "true"
        result = self.run_parameters(values)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("DisableIntegrations=true", result.stdout)
        values["DisableIntegrations"] = "invalid"
        self.assertNotEqual(self.run_parameters(values).returncode, 0)

    def test_activation_email_is_independent_of_disabled_integrations(self):
        values = self.parameters()
        values["DisableIntegrations"] = "true"
        values["EnableActivationEmails"] = "true"
        for key in ("SmtpHost", "SmtpSecretArn", "SarvamSecretArn", "MailFrom"):
            del values[key]
        result = self.run_parameters(values)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("EnableActivationEmails=true", result.stdout)
        values["EnableActivationEmails"] = "invalid"
        self.assertNotEqual(self.run_parameters(values).returncode, 0)

    def test_temporary_access_rejects_conflicting_origin(self):
        values = self.parameters()
        values["AppOrigin"] = "https://setu.example.org"
        self.assertNotEqual(self.run_parameters(values).returncode, 0)

    def test_existing_domain_configuration_remains_supported(self):
        values = self.parameters("custom-domain")
        del values["AccessMode"]
        result = self.run_parameters(values)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("AccessMode=custom-domain", result.stdout)

    def test_domain_requires_valid_https_origin_and_certificate(self):
        for origin in ("http://example.org", "https://example.org/", "https://example.org/path",
                       "https://user@example.org", "https://example.org?query"):
            with self.subTest(origin=origin):
                values = self.parameters("custom-domain")
                values["AppOrigin"] = origin
                self.assertNotEqual(self.run_parameters(values).returncode, 0)
        values = self.parameters("custom-domain")
        del values["CertificateArn"]
        self.assertNotEqual(self.run_parameters(values).returncode, 0)

    def test_invalid_input_fails_without_emitting_overrides(self):
        for values in ([], {"AccessMode": "http"}, {**self.parameters(), "VpcId": "vpc-REPLACE"}):
            with self.subTest(values=values):
                result = self.run_parameters(values)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, "")
