package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"testing"
)

func TestCompanyAssessmentSettingsAuthorizationAndIsolation(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	first := active(t, a, identity.Accounts[2])
	second := active(t, a, identity.Accounts[3])
	input := Doc{"enabled": false, "reason": "School holiday"}
	for _, user := range []identity.User{identity.Accounts[2], identity.Accounts[4]} {
		_, err := a.SetAssessmentProcessing(ctx, user, first.ID(), input)
		check(t, err, 403)
	}
	for _, invalid := range []Doc{{"enabled": "false", "reason": "test"}, {"enabled": false}, {"reason": "test"}} {
		_, err := a.SetAssessmentProcessing(ctx, identity.Accounts[0], first.ID(), invalid)
		check(t, err, 422)
	}
	settings, err := a.SetAssessmentProcessing(ctx, identity.Accounts[0], first.ID(), input)
	must(t, err)
	if b(settings, "enabled") || b(settings, "effectiveEnabled") {
		t.Fatal("disabled setting not returned")
	}
	org, err := r.Get(ctx, "control", "ORG", second.ID())
	must(t, err)
	if !assessmentEnabled(org) {
		t.Fatal("unrelated organization changed")
	}
	events, err := r.Query(ctx, "control", "AUDIT#"+first.ID(), false)
	must(t, err)
	found := false
	for _, event := range events {
		if s(event, "action") == "ASSESSMENT_PROCESSING_UPDATED" {
			found = true
			if s(event, "actor") != identity.Accounts[0].Sub || s(event, "reason") != "School holiday" || !b(event, "previousEnabled") || b(event, "enabled") {
				t.Fatal("incomplete audit event")
			}
		}
	}
	if !found {
		t.Fatal("missing audit event")
	}
	a.Config.IntegrationsDisabled = true
	settings, err = a.SetAssessmentProcessing(ctx, identity.Accounts[0], first.ID(), Doc{"enabled": true, "reason": "Ready"})
	must(t, err)
	if !b(settings, "enabled") || b(settings, "effectiveEnabled") || !b(settings, "platformAvailable") {
		t.Fatal("deployment default must remain enforced when no organization override is saved")
	}
}

func TestDisabledOrganizationBlocksSubmissionsAndPausesJobs(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	tenant := active(t, a, identity.Accounts[2])
	teacher := teacherTenant(t, a, tenant)
	student, err := a.CreateStudent(ctx, identity.Accounts[4], teacher, Doc{"student": Doc{"name": "Reader", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	must(t, err)
	input := Doc{"studentId": student["sk"], "passageId": "en-1", "type": "audio/wav", "bytes": 32044, "duration": 20}
	assessment, err := a.InitAssessment(ctx, identity.Accounts[4], teacher, input)
	must(t, err)
	_, err = a.QueueAssessment(ctx, identity.Accounts[4], teacher, s(assessment, "sk"), false)
	must(t, err)
	_, err = a.SetAssessmentProcessing(ctx, identity.Accounts[0], tenant.ID(), Doc{"enabled": false, "reason": "Pause"})
	must(t, err)
	teacher, err = a.Tenant(ctx, identity.Accounts[4], tenant.ID())
	must(t, err)
	_, err = a.InitAssessment(ctx, identity.Accounts[4], teacher, input)
	check(t, err, 403)
	for _, retry := range []bool{false, true} {
		_, err = a.QueueAssessment(ctx, identity.Accounts[4], teacher, s(assessment, "sk"), retry)
		check(t, err, 403)
	}
	must(t, NewWorker(a).Tick(ctx))
	jobs, err := r.Query(ctx, "control", "JOB", false)
	must(t, err)
	for _, job := range jobs {
		if s(job, "kind") == "ASSESS" && (n(job, "attempts") != 0 || s(job, "gpk") != "READY") {
			t.Fatal("paused work consumed or attempted")
		}
	}
	_, err = a.SetAssessmentProcessing(ctx, identity.Accounts[0], tenant.ID(), Doc{"enabled": true, "reason": "Resume"})
	must(t, err)
	must(t, NewWorker(a).Tick(ctx))
	jobs, err = r.Query(ctx, "control", "JOB", false)
	must(t, err)
	resumed := false
	for _, job := range jobs {
		if s(job, "kind") == "ASSESS" && n(job, "attempts") == 1 {
			resumed = true
		}
	}
	if !resumed {
		t.Fatal("queued work did not resume")
	}
}
