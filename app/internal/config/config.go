// Package config provides a single fail-closed configuration boundary.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ActivationEmailsEnabled, IntegrationsDisabled                                              bool
	Env, Mode, Region, Account, Prefix, ControlTable, DocumentBucket                           string
	DynamoEndpoint, S3Endpoint, S3PublicEndpoint, Origin, FrontendDir                          string
	AuthMode, LocalSecret, Issuer, Client, CompanyIssuer, CompanyClient, Domain, CompanyDomain string
	Provider, SarvamKey, SarvamURL, SarvamModel                                                string
	SMTPHost, SMTPPort, SMTPUser, SMTPPassword, MailFrom                                       string
	Port, MaxAudioBytes, MaxAudioSeconds, RetentionMaxDays, TableQuota, BucketQuota            int
}

func Load() (Config, error) { return From(os.Getenv) }
func From(get func(string) string) (Config, error) {
	v := func(k, d string) string {
		if x := get(k); x != "" {
			return x
		}
		return d
	}
	var invalid error
	n := func(k string, d int) int {
		x, e := strconv.Atoi(v(k, strconv.Itoa(d)))
		if e != nil || x <= 0 {
			invalid = fmt.Errorf("invalid %s", k)
		}
		return x
	}
	c := Config{Env: v("APP_ENV", "production"), Mode: v("WORKER_MODE", "application"), Region: v("AWS_REGION", "ap-south-1"), Account: v("AWS_ACCOUNT_ID", "000000000000"), Prefix: v("RESOURCE_PREFIX", "setu-local"), ControlTable: v("CONTROL_TABLE", "setu-control-local"), DocumentBucket: v("DOCUMENT_BUCKET", "setu-documents-local"), DynamoEndpoint: get("DYNAMODB_ENDPOINT"), S3Endpoint: get("S3_ENDPOINT"), S3PublicEndpoint: get("S3_PUBLIC_ENDPOINT"), Origin: v("APP_ORIGIN", "http://localhost:8080"), FrontendDir: v("FRONTEND_DIR", "../frontend"), AuthMode: v("AUTH_MODE", "cognito"), LocalSecret: get("LOCAL_AUTH_SECRET"), Issuer: get("COGNITO_ISSUER"), Client: get("COGNITO_CLIENT_ID"), CompanyIssuer: get("COMPANY_COGNITO_ISSUER"), CompanyClient: get("COMPANY_COGNITO_CLIENT_ID"), Domain: get("COGNITO_DOMAIN"), CompanyDomain: get("COMPANY_COGNITO_DOMAIN"), Provider: v("TRANSCRIPTION_PROVIDER", "sarvam"), SarvamKey: get("SARVAM_API_KEY"), SarvamURL: v("SARVAM_API_URL", "https://api.sarvam.ai"), SarvamModel: v("SARVAM_MODEL", "saaras:v3"), SMTPHost: get("SMTP_HOST"), SMTPPort: v("SMTP_PORT", "465"), SMTPUser: get("SMTP_USER"), SMTPPassword: get("SMTP_PASSWORD"), MailFrom: v("MAIL_FROM", "noreply@example.test"), Port: n("PORT", 3000), MaxAudioBytes: n("MAX_AUDIO_BYTES", 10*1024*1024), MaxAudioSeconds: n("MAX_AUDIO_SECONDS", 300), RetentionMaxDays: n("RETENTION_MAX_DAYS", 365), TableQuota: n("TABLE_QUOTA", 2500), BucketQuota: n("BUCKET_QUOTA", 10000)}
	if !one(v("DISABLE_INTEGRATIONS", "false"), "true", "false") {
		return c, fmt.Errorf("DISABLE_INTEGRATIONS must be true or false")
	}
	c.IntegrationsDisabled = v("DISABLE_INTEGRATIONS", "false") == "true"
	if !one(v("ENABLE_ACTIVATION_EMAILS", "false"), "true", "false") {
		return c, fmt.Errorf("ENABLE_ACTIVATION_EMAILS must be true or false")
	}
	c.ActivationEmailsEnabled = v("ENABLE_ACTIVATION_EMAILS", "false") == "true"

	if invalid != nil {
		return c, invalid
	}
	if !one(c.Env, "local", "test", "staging", "production") || !one(c.Provider, "sarvam", "mock") || !one(c.AuthMode, "local", "cognito") || !one(c.Mode, "all", "application", "provisioning") {
		return c, fmt.Errorf("invalid environment, provider, authentication or worker mode")
	}
	if c.MaxAudioBytes > 25*1024*1024 || c.MaxAudioSeconds > 1800 || len(c.Prefix) > 16 || strings.Trim(c.Prefix, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" || c.Prefix == "" {
		return c, fmt.Errorf("invalid resource prefix or media limits")
	}
	if !c.Local() && (c.AuthMode == "local" || c.Provider == "mock" || c.DynamoEndpoint != "" || c.S3Endpoint != "" || c.S3PublicEndpoint != "" || c.Mode == "all") {
		return c, fmt.Errorf("development configuration forbidden outside local/test")
	}
	if !c.Local() && (!strings.HasPrefix(c.Origin, "https://") || c.SarvamURL != "https://api.sarvam.ai") {
		return c, fmt.Errorf("production requires HTTPS origin and the official Sarvam endpoint")
	}
	if c.AuthMode == "local" && len(c.LocalSecret) < 32 {
		return c, fmt.Errorf("local authentication requires a 32-character secret")
	}
	if c.AuthMode == "cognito" && (c.Issuer == "" || c.Client == "" || c.CompanyIssuer == "" || c.CompanyClient == "" || c.Issuer == c.CompanyIssuer) {
		return c, fmt.Errorf("configure separate tenant and MFA-required company Cognito pools")
	}
	return c, nil
}
func (c Config) Local() bool { return c.Env == "local" || c.Env == "test" }
func one(v string, values ...string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
