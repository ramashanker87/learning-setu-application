package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"testing"
)

func TestOnlyTeachersManageStudentsAndConductAssessments(t *testing.T) {
	a, _, _ := fixture(t)
	ctx := context.Background()
	adminUser := identity.Accounts[2]
	tenant := active(t, a, adminUser)
	teacher := teacherTenant(t, a, tenant)
	input := Doc{"student": Doc{"name": "Reader", "age": 8, "className": "3", "schoolId": "test-school"}, "consent": consent()}
	_, err := a.CreateStudent(ctx, adminUser, tenant, input)
	check(t, err, 403)
	student, err := a.CreateStudent(ctx, identity.Accounts[4], teacher, input)
	must(t, err)
	recording := Doc{"studentId": student["sk"], "passageId": "en-1", "type": "audio/wav", "bytes": 32044, "duration": 20}
	for _, role := range []string{"ORG_ADMIN", "VIEWER"} {
		restricted := tenant
		restricted.Member = store.Clone(tenant.Member)
		restricted.Member["role"] = role
		_, err = a.CreateStudent(ctx, adminUser, restricted, input)
		check(t, err, 403)
		_, err = a.Student(ctx, restricted, s(student, "sk"))
		must(t, err)
		for _, kind := range []string{"correct", "delete"} {
			_, err = a.Privacy(ctx, adminUser, restricted, Doc{"kind": kind, "studentId": student["sk"]})
			check(t, err, 403)
		}
		_, err = a.Consent(ctx, adminUser, restricted, s(student, "sk"), consent())
		check(t, err, 403)
		_, err = a.InitAssessment(ctx, adminUser, restricted, recording)
		check(t, err, 403)
		_, err = a.QueueAssessment(ctx, adminUser, restricted, "any", false)
		check(t, err, 403)
		_, err = a.QueueAssessment(ctx, adminUser, restricted, "any", true)
		check(t, err, 403)
	}
	assessment, err := a.InitAssessment(ctx, identity.Accounts[4], teacher, recording)
	must(t, err)
	if s(assessment, "studentId") != s(student, "sk") {
		t.Fatal("assessment lost existing student")
	}
	foreign := active(t, a, identity.Accounts[3])
	other, err := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, foreign), input)
	must(t, err)
	recording["studentId"] = other["sk"]
	_, err = a.InitAssessment(ctx, identity.Accounts[4], teacher, recording)
	check(t, err, 404)
}
