package platform

import (
	"context"
	"errors"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"strings"
	"time"
)

// The deployment flag is a default. Explicit organization settings take precedence.
func (a *Service) integrationsEnabled(org Doc) bool {
	if value, exists := org["integrationsEnabled"]; exists {
		return value == true
	}
	return !a.Config.IntegrationsDisabled
}

func (a *Service) configuredServices() Doc {
	c := a.Config
	key := strings.TrimSpace(c.SarvamKey)
	assessment := c.Local() && c.Provider == "mock" || c.Provider == "sarvam" && key != "" && key != "dummy" && !strings.HasPrefix(key, "{")
	email := c.Local() || c.SMTPHost != "" && c.SMTPUser != "" && c.SMTPPassword != "" && validEmail(c.MailFrom)
	return Doc{"assessmentConfigured": assessment, "emailConfigured": email}
}

// Only the application worker has credentials. Publish booleans, never secret values.
func (w *Worker) publishIntegrationStatus(ctx context.Context) error {
	a := w.Service
	if a.Config.Mode == "provisioning" || time.Since(w.lastIntegrationStatus) < 30*time.Second {
		return nil
	}
	old, err := a.Repo.Get(ctx, a.Config.ControlTable, "PLATFORM", "INTEGRATIONS")
	if err != nil {
		return err
	}
	status := a.configuredServices()
	status["pk"], status["sk"], status["rev"] = "PLATFORM", "INTEGRATIONS", n(old, "rev")
	status["validUntil"] = millis() + 300000
	err = a.Repo.Commit(ctx, a.put(status))
	if errors.Is(err, store.ErrConflict) {
		return nil
	}
	if err == nil {
		w.lastIntegrationStatus = time.Now()
	}
	return err
}

func (a *Service) integrationAvailability(ctx context.Context) (Doc, error) {
	if a.Config.Local() {
		return a.configuredServices(), nil
	}
	status, err := a.Repo.Get(ctx, a.Config.ControlTable, "PLATFORM", "INTEGRATIONS")
	if err != nil {
		return nil, err
	}
	if n(status, "validUntil") <= millis() {
		return Doc{"assessmentConfigured": false, "emailConfigured": false}, nil
	}
	return Doc{"assessmentConfigured": b(status, "assessmentConfigured"), "emailConfigured": b(status, "emailConfigured")}, nil
}

func (a *Service) integrationSettings(ctx context.Context, org Doc) (Doc, error) {
	availability, err := a.integrationAvailability(ctx)
	if err != nil {
		return nil, err
	}
	enabled := a.integrationsEnabled(org)
	return Doc{"enabled": enabled, "inherited": org["integrationsEnabled"] == nil,
		"assessmentConfigured": b(availability, "assessmentConfigured"), "emailConfigured": b(availability, "emailConfigured"),
		"assessmentEnabled": enabled && assessmentEnabled(org) && b(availability, "assessmentConfigured"),
		"emailEnabled":      enabled && b(availability, "emailConfigured")}, nil
}

func (a *Service) SetIntegrations(ctx context.Context, u identity.User, id string, input Doc) (Doc, error) {
	if err := a.Company(ctx, u); err != nil {
		return nil, err
	}
	enabled, ok := input["enabled"].(bool)
	if !ok {
		return nil, fail(422, "enabled must be true or false")
	}
	reason := strings.TrimSpace(s(input, "reason"))
	if reason == "" || len(reason) > 1000 {
		return nil, fail(422, "Enter a reason of 1 to 1000 characters")
	}
	org, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, fail(404, "Organization not found")
	}
	if !contains(s(org, "state"), "ACTIVE", "SUSPENDED") {
		return nil, fail(409, "Integration settings require an active or suspended organization")
	}
	event := audit(id, u.Sub, "INTEGRATIONS_UPDATED", reason)
	event["previousEnabled"], event["enabled"] = a.integrationsEnabled(org), enabled
	org["integrationsEnabled"], org["integrationsUpdatedBy"], org["integrationsUpdatedAt"] = enabled, u.Sub, now()
	// Read availability before committing so a failed response never hides a successful save.
	settings, err := a.integrationSettings(ctx, org)
	if err != nil {
		return nil, err
	}
	if err = a.Repo.Commit(ctx, a.put(org), a.put(event)); err != nil {
		return nil, err
	}
	return settings, nil
}
