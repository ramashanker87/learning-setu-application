package platform

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
)

// Public applications use a random capability, never an email address as proof
// of ownership. Only its hash is persisted; it grants no workspace access.
func (a *Service) CreatePublicApplication(ctx context.Context, d Doc) (Doc, error) {
	d = normalizeApplication(d)
	if err := validateApplication(d); err != nil {
		return nil, err
	}
	// One bounded, shared counter works across API replicas without storing IPs.
	for attempt := 0; ; attempt++ {
		limit, err := a.Repo.Get(ctx, a.Config.ControlTable, "PUBLIC_RATE", "APPLICATIONS")
		if err != nil {
			return nil, err
		}
		if limit == nil {
			limit = Doc{"pk": "PUBLIC_RATE", "sk": "APPLICATIONS"}
		}
		if n(limit, "until") <= millis() {
			limit["until"] = millis() + 60000
			limit["count"] = 0
		}
		if n(limit, "count") >= 10 {
			return nil, fail(429, "Too many applications. Please try again in a minute.")
		}
		limit["count"] = n(limit, "count") + 1
		err = a.Repo.Commit(ctx, a.put(limit))
		if errors.Is(err, store.ErrConflict) && attempt < 3 {
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	id := uuid.NewString()
	email := strings.ToLower(strings.TrimSpace(s(d, "contactEmail")))
	d["contactEmail"] = email
	o := Doc{"pk": "ORG", "sk": id, "id": id, "owner": "public#" + id, "email": email, "state": "DRAFT", "draft": d, "createdAt": now(), "retentionDays": 90, "publicApplication": true, "applicationKeyHash": hash(token)}
	if err := a.Repo.Commit(ctx, a.put(o), a.put(audit(id, "public-applicant", "APPLICATION_CREATED", ""))); err != nil {
		return nil, err
	}
	return Doc{"application": publicView(o), "accessKey": token}, nil
}
func publicView(o Doc) Doc {
	out := Doc{}
	for _, k := range []string{"id", "state", "draft", "reason", "submittedAt", "activationAccessGranted"} {
		if v, ok := o[k]; ok {
			out[k] = v
		}
	}
	return out
}
func (a *Service) publicApplicant(ctx context.Context, id, key string) (identity.User, Doc, error) {
	var u identity.User
	if len(key) != 43 {
		return u, nil, fail(403, "Invalid application reference or access key")
	}
	o, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if err != nil {
		return u, nil, err
	}
	if !b(o, "publicApplication") || offboardingState(s(o, "state")) || subtle.ConstantTimeCompare([]byte(s(o, "applicationKeyHash")), []byte(hash(key))) != 1 {
		return u, nil, fail(403, "Invalid application reference or access key")
	}
	return identity.User{Sub: s(o, "owner"), Email: s(o, "email")}, o, nil
}
func (a *Service) ClaimApplication(ctx context.Context, u identity.User, id, key string) (Doc, error) {
	if u.TeamOrg != "" {
		return nil, fail(403, "Team accounts cannot claim administrator access")
	}
	_, o, err := a.publicApplicant(ctx, id, key)
	if err != nil {
		return nil, err
	}
	if u.Company || !strings.EqualFold(u.Email, s(o, "email")) {
		return nil, fail(403, "Sign in with the application contact email using organization sign-in")
	}
	if s(o, "state") != "ACTIVE" {
		return nil, fail(409, "Administrator access is available after approval and setup")
	}
	o["owner"] = u.Sub
	o["gpk"] = "OWNER#" + u.Sub
	o["gsk"] = id
	delete(o, "applicationKeyHash")
	delete(o, "publicApplication")
	oldMember, err := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+id, u.Sub)
	if err != nil {
		return nil, err
	}
	member := Doc{"pk": "MEMBER#" + id, "sk": u.Sub, "orgId": id, "email": u.Email, "role": "ORG_ADMIN", "active": true, "gpk": "USER#" + u.Sub, "gsk": id}
	if oldMember != nil {
		member["rev"] = oldMember["rev"]
	}
	err = a.Repo.Commit(ctx, a.put(o), a.put(member), a.put(audit(id, u.Sub, "APPLICATION_CLAIMED", "")))
	return Doc{"orgId": id}, err
}
