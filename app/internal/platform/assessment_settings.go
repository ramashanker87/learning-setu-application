package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"strings"
)

// Missing settings preserve existing organizations' assessment access.
func assessmentEnabled(org Doc) bool {
	enabled, exists := org["assessmentProcessingEnabled"]
	return !exists || enabled == true
}

func (a *Service) assessmentSettings(ctx context.Context, org Doc) (Doc, error) {
	settings, err := a.integrationSettings(ctx, org)
	if err != nil {
		return nil, err
	}
	return Doc{"enabled": assessmentEnabled(org), "integrationsEnabled": settings["enabled"], "platformAvailable": settings["assessmentConfigured"], "effectiveEnabled": settings["assessmentEnabled"]}, nil
}

func (a *Service) requireAssessmentProcessing(ctx context.Context, org Doc) error {
	if !a.integrationsEnabled(org) {
		return fail(503, "Integrations are disabled for this organization. Contact your company administrator.")
	}
	if !assessmentEnabled(org) {
		return fail(403, "Assessment processing is disabled for this organization. Contact your company administrator.")
	}
	available, err := a.integrationAvailability(ctx)
	if err != nil {
		return err
	}
	if !b(available, "assessmentConfigured") {
		return fail(503, "The assessment service is not configured or its worker is unavailable. Contact the service operator.")
	}
	return nil
}

func (a *Service) SetAssessmentProcessing(ctx context.Context, u identity.User, id string, input Doc) (Doc, error) {
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
		return nil, fail(409, "Assessment settings are available only for active or suspended organizations")
	}
	previous := assessmentEnabled(org)
	org["assessmentProcessingEnabled"] = enabled
	org["assessmentProcessingUpdatedAt"] = now()
	org["assessmentProcessingUpdatedBy"] = u.Sub
	event := audit(id, u.Sub, "ASSESSMENT_PROCESSING_UPDATED", reason)
	event["previousEnabled"] = previous
	event["enabled"] = enabled
	settings, err := a.assessmentSettings(ctx, org)
	if err != nil {
		return nil, err
	}
	if err = a.Repo.Commit(ctx, a.put(org), a.put(event)); err != nil {
		return nil, err
	}
	return settings, nil
}
