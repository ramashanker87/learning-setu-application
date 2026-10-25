package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"testing"
)

func TestNewStudentFieldsAndDeclaration(t *testing.T) {
	a, r, _ := fixture(t)
	u := identity.Accounts[2]
	tenant := active(t, a, u)
	ctx := context.Background()
	c := consent()
	delete(c, "verificationMethod")
	delete(c, "evidenceReference")
	for _, fields := range []Doc{{"age": 4, "className": "2"}, {"age": 15, "className": "10"}} {
		fields["name"] = "Synthetic student"
		fields["formVersion"] = "student-v2"
		fields["schoolName"] = "Forged school name"
		fields["schoolId"] = "test-school"
		student, err := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, tenant), Doc{"student": fields, "consent": c})
		must(t, err)
		stored, err := r.Get(ctx, tenant.Table, "STUDENT", s(student, "sk"))
		must(t, err)
		if s(stored, "schoolName") != "Test School" || s(m(stored, "consent"), "verificationMethod") != "staff_declaration" {
			t.Fatal("school or declaration missing")
		}
		if _, exists := m(stored, "consent")["evidenceReference"]; exists {
			t.Fatal("fabricated evidence")
		}
	}
	for _, fields := range []Doc{{"age": 3, "className": "2"}, {"age": 16, "className": "10"}, {"age": 5.5, "className": "3"}, {"age": 9, "className": "1"}, {"age": 9, "className": "11"}} {
		fields["name"] = "Test"
		if validateStudent(fields) == nil {
			t.Fatal("invalid student accepted", fields)
		}
	}
	if validateStudent(Doc{"name": "Test", "age": 9, "className": "4", "formVersion": "student-v2"}) == nil {
		t.Fatal("new form without school accepted")
	}
	c["authorityVerified"] = false
	if validateConsent(c) == nil {
		t.Fatal("guardian declaration no longer required")
	}
	legacy := consent()
	delete(legacy, "evidenceReference")
	if validateConsent(legacy) == nil {
		t.Fatal("legacy documentary verification without reference accepted")
	}
}
