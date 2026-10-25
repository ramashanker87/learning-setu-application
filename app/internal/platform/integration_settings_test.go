package platform

import (
	"context"
	"encoding/json"
	"learning-setu/app/internal/identity"
	"strings"
	"testing"
)

func TestOrganizationIntegrationsOverrideDefaultAndIsolateTenants(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	first := active(t, a, identity.Accounts[2])
	second := active(t, a, identity.Accounts[3])
	a.Config.IntegrationsDisabled = true
	teacher := teacherTenant(t, a, first)
	_, err := a.InitAssessment(ctx, identity.Accounts[4], teacher, Doc{})
	check(t, err, 503)
	for _, user := range []identity.User{identity.Accounts[2], identity.Accounts[4]} {
		_, err = a.SetIntegrations(ctx, user, first.ID(), Doc{"enabled": true, "reason": "Start term"})
		check(t, err, 403)
	}
	for _, input := range []Doc{{"enabled": "true", "reason": "Test"}, {"enabled": true}, {"reason": "Test"}} {
		_, err = a.SetIntegrations(ctx, identity.Accounts[0], first.ID(), input)
		check(t, err, 422)
	}
	settings, err := a.SetIntegrations(ctx, identity.Accounts[0], first.ID(), Doc{"enabled": true, "reason": "Start term"})
	must(t, err)
	if !b(settings, "assessmentEnabled") || !b(settings, "emailEnabled") || b(settings, "inherited") {
		t.Fatal(settings)
	}
	first, err = a.Tenant(ctx, identity.Accounts[2], first.ID())
	must(t, err)
	if !a.integrationsEnabled(first.Org) || a.integrationsEnabled(second.Org) {
		t.Fatal("override crossed tenant boundary")
	}
	// SMTP invitations and assessments both honor the organization override.
	_, err = a.Invite(ctx, identity.Accounts[2], first, Doc{"email": "new@example.test", "role": "VIEWER"})
	must(t, err)
	teacher, err = a.Tenant(ctx, identity.Accounts[4], first.ID())
	must(t, err)
	student, err := a.CreateStudent(ctx, identity.Accounts[4], teacher, Doc{"student": Doc{"name": "Reader", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	must(t, err)
	input := Doc{"studentId": student["sk"], "passageId": "en-1", "type": "audio/wav", "bytes": 32044, "duration": 20}
	assessment, err := a.InitAssessment(ctx, identity.Accounts[4], teacher, input)
	must(t, err)
	_, err = a.QueueAssessment(ctx, identity.Accounts[4], teacher, s(assessment, "sk"), false)
	must(t, err)
	_, err = a.SetIntegrations(ctx, identity.Accounts[0], first.ID(), Doc{"enabled": false, "reason": "Pause"})
	must(t, err)
	first, err = a.Tenant(ctx, identity.Accounts[2], first.ID())
	must(t, err)
	_, err = a.Invite(ctx, identity.Accounts[2], first, Doc{})
	check(t, err, 503)
	teacher, err = a.Tenant(ctx, identity.Accounts[4], first.ID())
	must(t, err)
	_, err = a.InitAssessment(ctx, identity.Accounts[4], teacher, input)
	check(t, err, 503)
	for _, retry := range []bool{false, true} {
		_, err = a.QueueAssessment(ctx, identity.Accounts[4], teacher, s(assessment, "sk"), retry)
		check(t, err, 503)
	}
	worker := NewWorker(a)
	must(t, worker.Tick(ctx))
	jobs, err := r.Query(ctx, "control", "JOB", false)
	must(t, err)
	for _, j := range jobs {
		if contains(s(j, "kind"), "ASSESS", "EMAIL") && n(j, "attempts") != 0 {
			t.Fatal("paused job attempted")
		}
	}
	_, err = a.SetIntegrations(ctx, identity.Accounts[0], first.ID(), Doc{"enabled": true, "reason": "Resume"})
	must(t, err)
	must(t, worker.Tick(ctx))
	jobs, err = r.Query(ctx, "control", "JOB", false)
	must(t, err)
	resumed := false
	for _, j := range jobs {
		if s(j, "kind") == "ASSESS" && n(j, "attempts") == 1 {
			resumed = true
		}
	}
	if !resumed {
		t.Fatal("assessment did not resume")
	}
	events, err := r.Query(ctx, "control", "AUDIT#"+first.ID(), false)
	must(t, err)
	changes := 0
	for _, event := range events {
		if s(event, "action") == "INTEGRATIONS_UPDATED" {
			changes++
			if s(event, "actor") != identity.Accounts[0].Sub || s(event, "reason") == "" {
				t.Fatal("missing audit details")
			}
		}
	}
	if changes != 3 {
		t.Fatal("missing integration change audits")
	}
}

func TestIntegrationReadinessNeverExposesCredentialsAndExpires(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	a.Config.Env = "staging"
	a.Config.Provider = "sarvam"
	a.Config.SarvamKey = "dummy"
	if b(a.configuredServices(), "assessmentConfigured") {
		t.Fatal("placeholder key accepted")
	}
	a.Config.SarvamKey = "synthetic-private-key"
	a.Config.SMTPHost = "smtp.example.test"
	a.Config.SMTPUser = "private-user"
	a.Config.SMTPPassword = "private-password"
	a.Config.MailFrom = "sender@example.test"
	must(t, NewWorker(a).publishIntegrationStatus(ctx))
	availability, err := a.integrationAvailability(ctx)
	must(t, err)
	if !b(availability, "assessmentConfigured") || !b(availability, "emailConfigured") {
		t.Fatal(availability)
	}
	stored, err := r.Get(ctx, "control", "PLATFORM", "INTEGRATIONS")
	must(t, err)
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "smtp.example") {
		t.Fatal("credential metadata leaked")
	}
	stored["validUntil"] = 0
	must(t, r.Commit(ctx, a.put(stored)))
	availability, err = a.integrationAvailability(ctx)
	must(t, err)
	if b(availability, "assessmentConfigured") || b(availability, "emailConfigured") {
		t.Fatal("stale worker status accepted")
	}
	if err = a.requireAssessmentProcessing(ctx, Doc{"integrationsEnabled": true}); status(err) != 503 {
		t.Fatal("missing service readiness accepted")
	}
}
