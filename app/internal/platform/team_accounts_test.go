package platform

import (
	"context"
	"encoding/json"
	"learning-setu/app/internal/identity"
	"strings"
	"testing"
)

type fakeTeamCreator struct {
	calls   int
	hook    func()
	err     error
	request string
}

func (f *fakeTeamCreator) CreateTeam(_ context.Context, email, password, org, request string) (identity.TeamAccount, error) {
	f.calls++
	f.request = request
	if f.hook != nil {
		f.hook()
	}
	return identity.TeamAccount{Sub: "new-team-sub", Created: true}, f.err
}
func teamFixture(t *testing.T) (*Service, *memory, Tenant, identity.User, *fakeTeamCreator) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	id := "12345678-1234-1234-1234-123456789012"
	o := Doc{"pk": "ORG", "sk": id, "id": id, "state": "ACTIVE", "owner": "admin", "resources": Doc{"table": "tenant", "bucket": "audio"}}
	m := Doc{"pk": "MEMBER#" + id, "sk": "admin", "role": "ORG_ADMIN", "active": true}
	must(t, r.Commit(ctx, a.put(o), a.put(m)))
	u := identity.User{Sub: "admin", Email: "admin@example.test"}
	tenant, err := a.Tenant(ctx, u, id)
	must(t, err)
	f := &fakeTeamCreator{}
	a.TeamAccounts = f
	return a, r, tenant, u, f
}
func teamInput() Doc {
	return Doc{"username": "teacher@example.test", "temporaryPassword": "Secure-Temporary987!", "role": "ASSESSOR"}
}
func TestTeamCreationAndIsolation(t *testing.T) {
	ctx := context.Background()
	a, r, tenant, u, f := teamFixture(t)
	a.Config.IntegrationsDisabled = true
	result, err := a.CreateTeamMember(ctx, u, tenant, teamInput())
	must(t, err)
	if f.calls != 1 || s(result, "username") != "teacher@example.test" {
		t.Fatal(result)
	}
	encoded, _ := json.Marshal(r.data)
	if strings.Contains(string(encoded), "Secure-Temporary987!") {
		t.Fatal("password persisted")
	}
	encoded, _ = json.Marshal(result)
	if strings.Contains(string(encoded), "Secure-Temporary987!") {
		t.Fatal("password returned")
	}
	member := identity.User{Sub: "new-team-sub", Email: "teacher@example.test", TeamOrg: tenant.ID()}
	access, err := a.Tenant(ctx, member, tenant.ID())
	must(t, err)
	must(t, writer(access))
	// Even an erroneous second membership cannot bypass the immutable scope.
	other := "87654321-4321-4321-4321-210987654321"
	must(t, r.Commit(ctx, a.put(Doc{"pk": "ORG", "sk": other, "id": other, "state": "ACTIVE"}), a.put(Doc{"pk": "MEMBER#" + other, "sk": member.Sub, "active": true, "role": "VIEWER"})))
	if _, err = a.Tenant(ctx, member, other); status(err) != 403 {
		t.Fatal("cross-organization access allowed", err)
	}
	if _, err = a.CreateOrganization(ctx, member, Doc{}); status(err) != 403 {
		t.Fatal("team can create organizations")
	}
	if _, err = a.ClaimApplication(ctx, member, tenant.ID(), "anything"); status(err) != 403 {
		t.Fatal("team can claim admin")
	}
	if _, err = a.Accept(ctx, member, "anything"); status(err) != 403 {
		t.Fatal("team can accept another membership")
	}
	if _, err = a.Membership(ctx, u, tenant, member.Sub, Doc{"role": "ORG_ADMIN", "active": true}); status(err) != 422 {
		t.Fatal("team promoted to admin")
	}
	_, err = a.Membership(ctx, u, tenant, member.Sub, Doc{"role": "VIEWER", "active": true})
	must(t, err)
	access, err = a.Tenant(ctx, member, tenant.ID())
	must(t, err)
	if writer(access) == nil {
		t.Fatal("viewer can write")
	}
	_, err = a.Membership(ctx, u, tenant, member.Sub, Doc{"role": "VIEWER", "active": false})
	must(t, err)
	if _, err = a.Tenant(ctx, member, tenant.ID()); status(err) != 403 {
		t.Fatal("revoked member allowed")
	}
	if _, err = a.CreateTeamMember(ctx, u, tenant, teamInput()); status(err) != 409 || f.calls != 1 {
		t.Fatal("duplicate reset account")
	}
}
func TestTeamCreationAuthorizationAndValidation(t *testing.T) {
	for _, role := range []string{"VIEWER", "ASSESSOR"} {
		a, _, tenant, u, f := teamFixture(t)
		tenant.Member["role"] = role
		if _, err := a.CreateTeamMember(context.Background(), u, tenant, teamInput()); status(err) != 403 || f.calls != 0 {
			t.Fatal("nonadmin account creation")
		}
	}
	for _, change := range []Doc{{"role": "ORG_ADMIN"}, {"role": "UNKNOWN"}, {"username": "bad"}, {"temporaryPassword": "short"}, {"temporaryPassword": "NOLOWERCASE123!"}, {"temporaryPassword": "Password has spaces1!"}} {
		a, _, tenant, u, f := teamFixture(t)
		input := teamInput()
		for k, v := range change {
			input[k] = v
		}
		if _, err := a.CreateTeamMember(context.Background(), u, tenant, input); status(err) != 422 || f.calls != 0 {
			t.Fatal("invalid input accepted", change)
		}
	}
}
func TestTeamCreationRaceAndRetry(t *testing.T) {
	ctx := context.Background()
	a, r, tenant, u, f := teamFixture(t)
	f.hook = func() {
		o, _ := r.Get(ctx, a.Config.ControlTable, "ORG", tenant.ID())
		o["state"] = "SUSPENDED"
		must(t, r.Commit(ctx, a.put(o)))
	}
	if _, err := a.CreateTeamMember(ctx, u, tenant, teamInput()); err == nil {
		t.Fatal("stale organization accepted")
	}
	m, _ := r.Get(ctx, a.Config.ControlTable, "MEMBER#"+tenant.ID(), "new-team-sub")
	if m != nil {
		t.Fatal("membership granted after suspension")
	}
	request := f.request
	f.hook = nil
	o, _ := r.Get(ctx, a.Config.ControlTable, "ORG", tenant.ID())
	o["state"] = "ACTIVE"
	must(t, r.Commit(ctx, a.put(o)))
	tenant, err := a.Tenant(ctx, u, tenant.ID())
	must(t, err)
	_, err = a.CreateTeamMember(ctx, u, tenant, teamInput())
	must(t, err)
	if f.request != request {
		t.Fatal("retry lost account binding")
	}
}
