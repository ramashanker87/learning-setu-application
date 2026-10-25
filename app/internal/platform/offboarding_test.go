package platform

import (
	"context"
	"strings"
	"testing"

	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
)

func readyOffboard(t *testing.T, a *Service, id string) {
	t.Helper()
	ctx := context.Background()
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	must(t, e)
	m(o, "offboarding")["cleanupAfter"] = 0
	j, e := a.Repo.Get(ctx, a.Config.ControlTable, "JOB", s(o, "offboardingJobId"))
	must(t, e)
	j["availableAt"] = 0
	must(t, a.Repo.Commit(ctx, a.put(o), a.put(j)))
}
func requestOffboard(t *testing.T, a *Service, id string) Doc {
	t.Helper()
	o, e := a.Offboard(context.Background(), identity.Accounts[0], id, Doc{"confirmation": id, "reason": "Synthetic offboarding test"})
	must(t, e)
	return o
}
func TestOffboardingAuthorizationAndDrain(t *testing.T) {
	a, r, objects := fixture(t)
	ctx := context.Background()
	u := identity.Accounts[2]
	tenant := active(t, a, u)
	teacher := teacherTenant(t, a, tenant)
	_, e := a.Offboard(ctx, u, tenant.ID(), Doc{"confirmation": tenant.ID(), "reason": "test"})
	check(t, e, 403)
	_, e = a.Offboard(ctx, identity.Accounts[0], tenant.ID(), Doc{"confirmation": "wrong", "reason": "test"})
	check(t, e, 422)
	_, e = a.Offboard(ctx, identity.Accounts[0], tenant.ID(), Doc{"confirmation": tenant.ID()})
	check(t, e, 422)
	first := requestOffboard(t, a, tenant.ID())
	second := requestOffboard(t, a, tenant.ID())
	if s(first, "offboardingJobId") != s(second, "offboardingJobId") {
		t.Fatal("duplicate cleanup job")
	}
	_, e = a.Tenant(ctx, u, tenant.ID())
	check(t, e, 403)
	_, e = a.Review(ctx, identity.Accounts[0], tenant.ID(), "reactivate", "test")
	check(t, e, 409)
	must(t, NewWorker(a).Tick(ctx))
	if !r.tables[tenant.Table] || !objects.buckets[tenant.Bucket] {
		t.Fatal("cleanup started before drain period")
	}
	// Even an old tenant snapshot cannot write after offboarding changed the org revision.
	_, e = a.CreateStudent(ctx, identity.Accounts[4], teacher, Doc{"student": Doc{"name": "Late write", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	if e != store.ErrConflict {
		t.Fatalf("late writer was not rejected: %v", e)
	}
}
func TestOffboardingCleansOnlyTargetAndLeavesReceipt(t *testing.T) {
	a, r, objects := fixture(t)
	ctx := context.Background()
	u := identity.Accounts[2]
	target := active(t, a, u)
	other := active(t, a, identity.Accounts[3])
	must(t, r.Commit(ctx,
		store.Put(target.Table, Doc{"pk": "STUDENT", "sk": "one", "name": "Sensitive name"}),
		store.Put(target.Table, Doc{"pk": "ASSESSMENT", "sk": "assessment", "providerJobId": "provider-test", "providerStep": "POLLING"}),
		store.Put(other.Table, Doc{"pk": "STUDENT", "sk": "keep", "name": "Other learner"}),
		a.put(Doc{"pk": "DOCUMENT#" + target.ID(), "sk": "doc", "orgId": target.ID()}),
		a.put(Doc{"pk": "INVITE", "sk": "remove", "orgId": target.ID(), "email": "private@example.test"}),
		a.put(Doc{"pk": "INVITE", "sk": "keep", "orgId": other.ID()}),
		a.put(Doc{"pk": "TEAM_ACCOUNT", "sk": "remove-team", "orgId": target.ID()}),
		a.put(Doc{"pk": "TEAM_ACCOUNT", "sk": "keep-team", "orgId": other.ID()}),
		a.put(job("EMAIL", target.ID(), Doc{"email": "private@example.test"}))))
	o := requestOffboard(t, a, target.ID())
	readyOffboard(t, a, target.ID())
	a.Config.IntegrationsDisabled = true
	must(t, NewWorker(a).Tick(ctx))
	o, e := r.Get(ctx, a.Config.ControlTable, "ORG", target.ID())
	must(t, e)
	if s(o, "state") != "OFFBOARDED" || s(m(o, "offboarding"), "phase") != "LIVE_DATA_DELETED" {
		t.Fatalf("incomplete: %v", o)
	}
	for _, field := range []string{"owner", "email", "gpk", "resources", "reason", "representativeEmail"} {
		if _, ok := o[field]; ok {
			t.Fatalf("retained %s", field)
		}
	}
	if len(m(o, "draft")) != 0 || !b(m(o, "offboarding"), "providerFollowUpRequired") {
		t.Fatal("incorrect receipt")
	}
	if r.tables[target.Table] || objects.buckets[target.Bucket] || !r.tables[other.Table] || !objects.buckets[other.Bucket] {
		t.Fatal("incorrect resource deletion")
	}
	for _, pk := range []string{"MEMBER#" + target.ID(), "DOCUMENT#" + target.ID()} {
		items, e := r.Query(ctx, a.Config.ControlTable, pk, false)
		must(t, e)
		if len(items) != 0 {
			t.Fatal("control records remain")
		}
	}
	removedTeam, e := r.Get(ctx, a.Config.ControlTable, "TEAM_ACCOUNT", "remove-team")
	must(t, e)
	keptTeam, e := r.Get(ctx, a.Config.ControlTable, "TEAM_ACCOUNT", "keep-team")
	must(t, e)
	if removedTeam != nil || keptTeam == nil {
		t.Fatal("team reservations not isolated during cleanup")
	}
	invite, e := r.Get(ctx, a.Config.ControlTable, "INVITE", "keep")
	must(t, e)
	if invite == nil {
		t.Fatal("other invitation deleted")
	}
	for key, d := range r.data {
		if strings.HasPrefix(key, target.Table+"|") {
			t.Fatal("tenant records remain")
		}
		if s(d, "pk") == "JOB" && s(d, "orgId") == target.ID() && (s(d, "kind") != "OFFBOARD" || s(d, "state") != "COMPLETED" || len(m(d, "data")) != 0) {
			t.Fatalf("sensitive/unfinished job retained: %v", d)
		}
	}
	if len(objects.deletedPrefixes) != 1 || objects.deletedPrefixes[0] != "documents/organizations/"+target.ID()+"/" {
		t.Fatal("wrong document prefix")
	}
	requestOffboard(t, a, target.ID()) // completed request is idempotent, cannot reactivate
}
func TestOffboardingFailureAndRetry(t *testing.T) {
	a, r, objects := fixture(t)
	ctx := context.Background()
	tenant := active(t, a, identity.Accounts[2])
	requestOffboard(t, a, tenant.ID())
	readyOffboard(t, a, tenant.ID())
	objects.failDelete = true
	o, e := r.Get(ctx, a.Config.ControlTable, "ORG", tenant.ID())
	must(t, e)
	j, e := r.Get(ctx, a.Config.ControlTable, "JOB", s(o, "offboardingJobId"))
	must(t, e)
	j["attempts"] = 4
	must(t, r.Commit(ctx, a.put(j)))
	must(t, NewWorker(a).Tick(ctx))
	o, e = r.Get(ctx, a.Config.ControlTable, "ORG", tenant.ID())
	must(t, e)
	if s(o, "state") != "OFFBOARDING_FAILED" || !r.tables[tenant.Table] {
		t.Fatal("failed cleanup incorrectly completed")
	}
	objects.failDelete = false
	requestOffboard(t, a, tenant.ID())
	must(t, NewWorker(a).Tick(ctx))
	o, e = r.Get(ctx, a.Config.ControlTable, "ORG", tenant.ID())
	must(t, e)
	if s(o, "state") != "OFFBOARDED" {
		t.Fatalf("retry did not resume: %v", o)
	}
}
func TestOffboardingRejectsCrossTenantMapping(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	target := active(t, a, identity.Accounts[2])
	other := active(t, a, identity.Accounts[3])
	o, e := r.Get(ctx, a.Config.ControlTable, "ORG", target.ID())
	must(t, e)
	o["resources"] = other.Org["resources"]
	must(t, r.Commit(ctx, a.put(o)))
	requestOffboard(t, a, target.ID())
	readyOffboard(t, a, target.ID())
	must(t, NewWorker(a).Tick(ctx))
	if !r.tables[other.Table] {
		t.Fatal("other tenant table deleted")
	}
	o, e = r.Get(ctx, a.Config.ControlTable, "ORG", target.ID())
	must(t, e)
	if s(o, "state") == "OFFBOARDED" {
		t.Fatal("unsafe mapping accepted")
	}
}
func TestOffboardingUnprovisionedDraft(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	o, e := a.CreateOrganization(ctx, identity.Accounts[1], Doc{"displayName": "Draft"})
	must(t, e)
	requestOffboard(t, a, s(o, "id"))
	readyOffboard(t, a, s(o, "id"))
	must(t, NewWorker(a).Tick(ctx))
	o, e = r.Get(ctx, a.Config.ControlTable, "ORG", s(o, "id"))
	must(t, e)
	if s(o, "state") != "OFFBOARDED" {
		t.Fatal("draft cleanup failed")
	}
}
