package platform

import (
	"context"
	"errors"
	"learning-setu/app/internal/identity"
	"testing"
)

type fakeInviter struct {
	calls    int
	result   identity.InvitedAccount
	err      error
	callback func()
}

func (f *fakeInviter) Invite(context.Context, string) (identity.InvitedAccount, error) {
	f.calls++
	if f.callback != nil {
		f.callback()
	}
	return f.result, f.err
}
func activatedPublic(t *testing.T, a *Service) (string, string) {
	t.Helper()
	ctx := context.Background()
	created, err := a.CreatePublicApplication(ctx, publicDraft())
	must(t, err)
	id, key := s(m(created, "application"), "id"), s(created, "accessKey")
	u, _, err := a.publicApplicant(ctx, id, key)
	must(t, err)
	doc, err := a.Document(ctx, u, id, "", Doc{"kind": "registration", "type": "application/pdf", "bytes": 8})
	must(t, err)
	_, err = a.Document(ctx, u, id, s(doc, "sk"), Doc{})
	must(t, err)
	_, err = a.Submit(ctx, u, id)
	must(t, err)
	_, err = a.Review(ctx, identity.Accounts[0], id, "review", "")
	must(t, err)
	_, err = a.Review(ctx, identity.Accounts[0], id, "approve", "")
	must(t, err)
	worker := NewWorker(a)
	a.Config.Mode = "provisioning"
	must(t, worker.Tick(ctx))
	a.Config.Mode = "all"
	return id, key
}
func TestActivationInvitationAfterActiveAndIndependentOfTranscription(t *testing.T) {
	a, repo, _ := fixture(t)
	a.Config.ActivationEmailsEnabled = true
	a.Config.IntegrationsDisabled = true
	id, key := activatedPublic(t, a)
	ctx := context.Background()
	o, err := repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	must(t, err)
	if s(o, "state") != "ACTIVE" || s(m(o, "activationEmail"), "state") != "QUEUED" {
		t.Fatal(o)
	}
	fake := &fakeInviter{result: identity.InvitedAccount{Sub: identity.Accounts[2].Sub, InvitationRequested: true}}
	worker := NewWorker(a)
	worker.Accounts = fake
	must(t, worker.Tick(ctx))
	if fake.calls != 1 {
		t.Fatal("invitation not requested")
	}
	tenant, err := a.Tenant(ctx, identity.Accounts[2], id)
	must(t, err)
	if s(tenant.Member, "role") != "ORG_ADMIN" {
		t.Fatal("founding access missing")
	}
	o, err = repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	must(t, err)
	if s(m(o, "activationEmail"), "state") != "INVITATION_REQUESTED" || !b(o, "activationAccessGranted") {
		t.Fatal(o)
	}
	must(t, worker.Tick(ctx))
	if fake.calls != 1 {
		t.Fatal("duplicate invitation")
	}
	// A saved application can still be opened and securely claimed after automatic access.
	_, _, err = a.publicApplicant(ctx, id, key)
	must(t, err)
	_, err = a.ClaimApplication(ctx, identity.Accounts[2], id, key)
	must(t, err)
}
func TestActivationExistingAccountAndCompanyOnlyResend(t *testing.T) {
	a, repo, _ := fixture(t)
	a.Config.ActivationEmailsEnabled = true
	a.Config.IntegrationsDisabled = true
	id, _ := activatedPublic(t, a)
	ctx := context.Background()
	fake := &fakeInviter{result: identity.InvitedAccount{Sub: identity.Accounts[2].Sub}}
	worker := NewWorker(a)
	worker.Accounts = fake
	must(t, worker.Tick(ctx))
	o, _ := repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if s(m(o, "activationEmail"), "state") != "EXISTING_ACCOUNT_EMAIL_DISABLED" {
		t.Fatal(o)
	}
	_, err := a.RetryActivationEmail(ctx, identity.Accounts[2], id)
	check(t, err, 403)
	_, err = a.RetryActivationEmail(ctx, identity.Accounts[0], id)
	check(t, err, 429)
	m(o, "activationEmail")["lastRequestedAt"] = 0
	must(t, repo.Commit(ctx, a.put(o)))
	_, err = a.RetryActivationEmail(ctx, identity.Accounts[0], id)
	must(t, err)
	_, err = a.RetryActivationEmail(ctx, identity.Accounts[0], id)
	must(t, err)
	must(t, worker.Tick(ctx))
	if fake.calls != 2 {
		t.Fatal(fake.calls)
	}
}
func TestActivationFailureRetryAndSuspensionGuard(t *testing.T) {
	a, repo, _ := fixture(t)
	a.Config.ActivationEmailsEnabled = true
	id, _ := activatedPublic(t, a)
	ctx := context.Background()
	fake := &fakeInviter{err: errors.New("provider failure")}
	worker := NewWorker(a)
	worker.Accounts = fake
	for i := 0; i < 5; i++ {
		o, _ := repo.Get(ctx, a.Config.ControlTable, "ORG", id)
		j, _ := repo.Get(ctx, a.Config.ControlTable, "JOB", s(m(o, "activationEmail"), "jobId"))
		j["availableAt"] = 0
		must(t, repo.Commit(ctx, a.put(j)))
		must(t, worker.Tick(ctx))
	}
	o, _ := repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if s(m(o, "activationEmail"), "state") != "FAILED" || s(o, "state") != "ACTIVE" {
		t.Fatal(o)
	}
	m(o, "activationEmail")["lastRequestedAt"] = 0
	must(t, repo.Commit(ctx, a.put(o)))
	_, err := a.RetryActivationEmail(ctx, identity.Accounts[0], id)
	must(t, err)
	fake.err = nil
	fake.result = identity.InvitedAccount{Sub: identity.Accounts[2].Sub, InvitationRequested: true}
	fake.callback = func() {
		current, _ := repo.Get(ctx, a.Config.ControlTable, "ORG", id)
		current["state"] = "SUSPENDED"
		must(t, repo.Commit(ctx, a.put(current)))
	}
	must(t, worker.Tick(ctx))
	members, _ := repo.Query(ctx, a.Config.ControlTable, "MEMBER#"+id, false)
	if len(members) != 0 {
		t.Fatal("stale activation granted membership")
	}
	_, err = a.RetryActivationEmail(ctx, identity.Accounts[0], id)
	check(t, err, 409)
}
