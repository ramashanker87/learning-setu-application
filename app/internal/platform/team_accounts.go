package platform

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"learning-setu/app/internal/identity"
	"strings"
	"unicode"
)

func teamPasswordValid(password string) bool {
	if len(password) < 14 || len(password) > 128 {
		return false
	}
	var lower, upper, digit, symbol bool
	for _, r := range password {
		if unicode.IsSpace(r) {
			return false
		}
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			symbol = true
		}
	}
	return lower && upper && digit && symbol
}

func (a *Service) CreateTeamMember(ctx context.Context, u identity.User, t Tenant, input Doc) (Doc, error) {
	if err := admin(t); err != nil {
		return nil, err
	}
	if u.TeamOrg != "" {
		return nil, fail(403, "Team accounts cannot administer members")
	}
	email := strings.ToLower(strings.TrimSpace(s(input, "username")))
	role, password := s(input, "role"), s(input, "temporaryPassword")
	if len(email) > 254 || !validEmail(email) || !contains(role, "ASSESSOR", "VIEWER") {
		return nil, fail(422, "Enter an email username and select Assessor / teacher or Viewer")
	}
	if !teamPasswordValid(password) {
		return nil, fail(422, "Temporary password must contain 14–128 characters, uppercase and lowercase letters, a number and a symbol, with no spaces")
	}
	creator := a.TeamAccounts
	if creator == nil {
		if a.Config.Local() {
			return nil, fail(503, "Managed team credentials require Cognito; local mode uses demo accounts")
		}
		var err error
		creator, err = identity.NewInviter(ctx, a.Config)
		if err != nil {
			return nil, fail(503, "Team account service is unavailable")
		}
	}
	// Reserve the username and role before calling Cognito. Retries can complete
	// a pending membership without taking over another identity or saving a password.
	reservation, err := a.Repo.Get(ctx, a.Config.ControlTable, "TEAM_ACCOUNT", hash(email))
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		reservation = Doc{"pk": "TEAM_ACCOUNT", "sk": hash(email), "orgId": t.ID(), "role": role, "requestId": uuid.NewString(), "state": "CREATING"}
		if err = a.Repo.Commit(ctx, append(a.guards(t), a.put(reservation))...); err != nil {
			return nil, err
		}
		reservation, err = a.Repo.Get(ctx, a.Config.ControlTable, "TEAM_ACCOUNT", hash(email))
		if err != nil {
			return nil, err
		}
	}
	if s(reservation, "orgId") != t.ID() || s(reservation, "role") != role || s(reservation, "state") != "CREATING" {
		return nil, fail(409, "Username is already assigned or has a pending request with another role")
	}
	account, err := creator.CreateTeam(ctx, email, password, t.ID(), s(reservation, "requestId"))
	if errors.Is(err, identity.ErrAccountUnavailable) {
		return nil, fail(409, "Username is already registered. Existing accounts and passwords cannot be replaced here")
	}
	if err != nil {
		return nil, fail(503, "Account creation could not finish. Retry with the same username, role and original temporary password")
	}
	if account.Sub == "" {
		return nil, fail(503, "Account creation did not return an identity")
	}
	member := Doc{"pk": "MEMBER#" + t.ID(), "sk": account.Sub, "orgId": t.ID(), "email": email, "role": role, "active": true, "managedTeam": true, "gpk": "USER#" + account.Sub, "gsk": t.ID()}
	reservation["state"] = "CREATED"
	reservation["subject"] = account.Sub
	// Guards reject suspension, offboarding or revoked admin authority during the
	// external request. No membership exists until this transaction succeeds.
	if err = a.Repo.Commit(ctx, append(a.guards(t), a.put(reservation), a.put(member), a.put(audit(t.ID(), u.Sub, "TEAM_ACCOUNT_CREATED", role)))...); err != nil {
		return nil, err
	}
	return Doc{"member": member, "username": email, "created": account.Created, "temporaryPasswordDays": 7}, nil
}
