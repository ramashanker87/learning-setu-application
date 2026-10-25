package config

import (
	"strings"
	"testing"
)

func TestProductionRejectsDevelopmentSettings(t *testing.T) {
	base := map[string]string{"APP_ENV": "production", "APP_ORIGIN": "https://setu.example.org", "COGNITO_ISSUER": "https://pool.example/tenant", "COGNITO_CLIENT_ID": "tenant", "COMPANY_COGNITO_ISSUER": "https://pool.example/company", "COMPANY_COGNITO_CLIENT_ID": "company"}
	for k, v := range map[string]string{"AUTH_MODE": "local", "TRANSCRIPTION_PROVIDER": "mock", "S3_ENDPOINT": "http://localhost:9000", "DYNAMODB_ENDPOINT": "http://localhost:8100", "WORKER_MODE": "all"} {
		x := map[string]string{}
		for a, b := range base {
			x[a] = b
		}
		x[k] = v
		x["LOCAL_AUTH_SECRET"] = strings.Repeat("x", 32)
		if _, e := From(func(k string) string { return x[k] }); e == nil {
			t.Errorf("accepted %s", k)
		}
	}
}

func TestDisableIntegrations(t *testing.T) {
	for _, value := range []string{"", "false", "true", "invalid"} {
		values := map[string]string{"APP_ENV": "test", "AUTH_MODE": "local", "LOCAL_AUTH_SECRET": strings.Repeat("x", 32), "DISABLE_INTEGRATIONS": value}
		c, err := From(func(k string) string { return values[k] })
		if value == "invalid" {
			if err == nil {
				t.Fatal("accepted an invalid integration switch")
			}
			continue
		}
		if err != nil || c.IntegrationsDisabled != (value == "true") {
			t.Fatalf("switch %q: config=%v err=%v", value, c.IntegrationsDisabled, err)
		}
	}
}

func TestActivationEmailSwitch(t *testing.T) {
	for _, value := range []string{"", "false", "true", "invalid"} {
		values := map[string]string{"APP_ENV": "test", "AUTH_MODE": "local", "LOCAL_AUTH_SECRET": strings.Repeat("x", 32), "ENABLE_ACTIVATION_EMAILS": value, "DISABLE_INTEGRATIONS": "true"}
		c, err := From(func(k string) string { return values[k] })
		if value == "invalid" {
			if err == nil {
				t.Fatal("invalid switch accepted")
			}
			continue
		}
		if err != nil || c.ActivationEmailsEnabled != (value == "true") || !c.IntegrationsDisabled {
			t.Fatalf("%s %v", value, err)
		}
	}
}
