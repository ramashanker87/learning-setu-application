package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"testing"
)

func TestSchoolsAuthorizationAndIsolation(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	u := identity.Accounts[2]
	tenant := active(t, a, u)
	other := active(t, a, identity.Accounts[3])
	input := Doc{"name": " New School ", "block": " Block A ", "district": "District A", "state": "State A", "orgId": other.ID()}
	school, err := a.CreateSchool(ctx, u, tenant, input)
	must(t, err)
	if s(school, "name") != "New School" || s(school, "block") != "Block A" || s(school, "orgId") != tenant.ID() {
		t.Fatal("incorrect school scope or normalization")
	}
	for _, role := range []string{"ASSESSOR", "VIEWER"} {
		restricted := tenant
		restricted.Member = store.Clone(tenant.Member)
		restricted.Member["role"] = role
		if _, err = a.CreateSchool(ctx, u, restricted, input); status(err) != 403 {
			t.Fatal("nonadmin created school")
		}
		if _, err = a.Schools(ctx, restricted); err != nil {
			t.Fatal("member cannot read schools")
		}
	}
	for _, key := range []string{"name", "block", "district", "state"} {
		invalid := store.Clone(input)
		invalid[key] = " "
		if _, err = a.CreateSchool(ctx, u, tenant, invalid); status(err) != 422 {
			t.Fatal("missing school field accepted", key)
		}
	}
	foreign, err := a.CreateSchool(ctx, identity.Accounts[3], other, input)
	must(t, err)
	student := Doc{"name": "Learner", "age": 8, "className": "3", "schoolId": foreign["sk"]}
	if _, err = a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, tenant), Doc{"student": student, "consent": consent()}); status(err) != 422 {
		t.Fatal("foreign school accepted")
	}
	delete(student, "schoolId")
	if _, err = a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, tenant), Doc{"student": student, "consent": consent()}); status(err) != 422 {
		t.Fatal("missing school accepted")
	}
	student["schoolId"] = school["sk"]
	student["schoolName"] = "Forged name"
	created, err := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, tenant), Doc{"student": student, "consent": consent()})
	must(t, err)
	if s(created, "schoolName") != "New School" || s(created, "schoolId") != s(school, "sk") {
		t.Fatal("student did not use authoritative school")
	}
	list, err := a.Schools(ctx, tenant)
	must(t, err)
	for _, item := range list["items"].([]Doc) {
		if s(item, "orgId") != tenant.ID() {
			t.Fatal("school list crossed tenants")
		}
	}
	org, err := r.Get(ctx, a.Config.ControlTable, "ORG", tenant.ID())
	must(t, err)
	org["state"] = "SUSPENDED"
	must(t, r.Commit(ctx, a.put(org)))
	if _, err = a.CreateSchool(ctx, u, tenant, input); err != store.ErrConflict {
		t.Fatal("stale administrator write accepted", err)
	}
}
