package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
)

func activationJob(o Doc) Doc {
	j := job("ACTIVATION", s(o, "id"), Doc{})
	o["activationEmail"] = Doc{"state": "QUEUED", "jobId": j["sk"], "requestedAt": now()}
	return j
}
func (a *Service) RetryActivationEmail(ctx context.Context, u identity.User, id string) (Doc, error) {
	if err := a.Company(ctx, u); err != nil {
		return nil, err
	}
	if !a.Config.ActivationEmailsEnabled {
		return nil, fail(503, "Activation emails are disabled")
	}
	o, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if err != nil {
		return nil, err
	}
	if s(o, "state") != "ACTIVE" {
		return nil, fail(409, "Organization must be ACTIVE before sending login details")
	}
	old := m(o, "activationEmail")
	if s(old, "state") == "QUEUED" {
		prior, e := a.Repo.Get(ctx, a.Config.ControlTable, "JOB", s(old, "jobId"))
		if e != nil {
			return nil, e
		}
		if contains(s(prior, "state"), "PENDING", "RUNNING") {
			return o, nil
		}
	}
	if millis()-n(old, "lastRequestedAt") < 60000 {
		return nil, fail(429, "Wait one minute before sending another invitation")
	}
	j := activationJob(o)
	m(o, "activationEmail")["lastRequestedAt"] = millis()
	err = a.Repo.Commit(ctx, a.put(o), a.put(j), a.put(audit(id, u.Sub, "ACTIVATION_EMAIL_REQUESTED", "")))
	return o, err
}
func (w *Worker) activateAccount(ctx context.Context, j Doc) ([]store.Write, error) {
	a := w.Service
	if !a.Config.ActivationEmailsEnabled {
		return nil, fail(503, "Activation emails are disabled")
	}
	o, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
	if err != nil {
		return nil, err
	}
	if s(o, "state") != "ACTIVE" {
		return nil, fail(409, "Organization is not active")
	}
	progress := m(o, "activationEmail")
	if s(progress, "jobId") != s(j, "sk") {
		return nil, fail(409, "Activation request is no longer current")
	}
	if s(progress, "state") != "QUEUED" {
		return nil, nil
	}
	if w.Accounts == nil {
		w.Accounts, err = identity.NewInviter(ctx, a.Config)
		if err != nil {
			return nil, err
		}
	}
	account, err := w.Accounts.Invite(ctx, s(o, "email"))
	if err != nil {
		return nil, err
	}
	// Re-read after the external call. Offboarding/suspension/manual claim must
	// never be overwritten by a stale activation write.
	current, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
	if err != nil {
		return nil, err
	}
	if n(current, "rev") != n(o, "rev") || s(current, "state") != "ACTIVE" {
		return nil, store.ErrConflict
	}
	if account.Sub == "" || (!b(o, "publicApplication") && s(o, "owner") != account.Sub) {
		return nil, fail(409, "Account does not match the organization owner")
	}
	member, err := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+s(o, "id"), account.Sub)
	if err != nil {
		return nil, err
	}
	writes := []store.Write{}
	if member == nil {
		member = Doc{"pk": "MEMBER#" + s(o, "id"), "sk": account.Sub, "orgId": o["id"], "email": o["email"], "role": "ORG_ADMIN", "active": true, "gpk": "USER#" + account.Sub, "gsk": o["id"]}
		writes = append(writes, a.put(member))
	} else if !b(member, "active") || s(member, "role") != "ORG_ADMIN" {
		return nil, fail(409, "Existing membership requires administrator review")
	}
	o["owner"] = account.Sub
	o["gpk"] = "OWNER#" + account.Sub
	o["gsk"] = o["id"]
	o["activationAccessGranted"] = true
	progress["state"] = "INVITATION_REQUESTED"
	progress["processedAt"] = now()
	if !account.InvitationRequested {
		progress["state"] = "EXISTING_ACCOUNT"
		if !a.integrationsEnabled(o) || !b(a.configuredServices(), "emailConfigured") || a.Config.SMTPHost == "" {
			progress["state"] = "EXISTING_ACCOUNT_EMAIL_DISABLED"
		} else {
			if err = w.sendActivationNotice(ctx, s(o, "email")); err != nil {
				return nil, err
			}
		}
	}
	o["activationEmail"] = progress
	return append(writes, a.put(o), a.put(audit(s(o, "id"), "worker", s(progress, "state"), ""))), nil
}
