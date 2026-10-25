package platform

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"github.com/google/uuid"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"strings"
)

func (a *Service) CreateOrganization(ctx context.Context, u identity.User, input Doc) (Doc, error) {
	if u.TeamOrg != "" {
		return nil, fail(403, "Team accounts cannot register organizations")
	}
	id := uuid.NewString()
	o := Doc{"pk": "ORG", "sk": id, "id": id, "owner": u.Sub, "email": u.Email, "state": "DRAFT", "draft": normalizeApplication(input), "createdAt": now(), "gpk": "OWNER#" + u.Sub, "gsk": id, "retentionDays": 90}
	e := a.Repo.Commit(ctx, a.put(o), a.put(audit(id, u.Sub, "APPLICATION_CREATED", "")))
	return o, e
}
func (a *Service) EditOrganization(ctx context.Context, u identity.User, id string, input Doc) (Doc, error) {
	o, e := a.owned(ctx, u, id)
	if e != nil {
		return nil, e
	}
	if !contains(s(o, "state"), "DRAFT", "CHANGES_REQUESTED") {
		return nil, fail(409, "Application cannot be edited in this state")
	}
	o["draft"] = normalizeApplication(input)
	e = a.Repo.Commit(ctx, a.put(o), a.put(audit(id, u.Sub, "APPLICATION_EDITED", "")))
	return o, e
}
func (a *Service) Submit(ctx context.Context, u identity.User, id string) (Doc, error) {
	o, e := a.owned(ctx, u, id)
	if e != nil {
		return nil, e
	}
	if !contains(s(o, "state"), "DRAFT", "CHANGES_REQUESTED") {
		return nil, fail(409, "Application cannot be submitted in this state")
	}
	d := normalizeApplication(m(o, "draft"))
	o["draft"] = d
	if e = validateApplication(d); e != nil {
		return nil, e
	}
	docs, e := a.Repo.Query(ctx, a.Config.ControlTable, "DOCUMENT#"+id, false)
	if e != nil {
		return nil, e
	}
	for _, kind := range Policies[s(d, "type")].Documents {
		found := false
		for _, doc := range docs {
			if s(doc, "kind") == kind && s(doc, "state") == "UPLOADED" {
				found = true
			}
		}
		if !found {
			return nil, fail(422, "Required supporting document missing")
		}
	}
	o["state"] = "SUBMITTED"
	o["submittedAt"] = now()
	o["termsAcceptedAt"] = now()
	o["noticeVersion"] = Notice
	o["representativeEmail"] = u.Email
	e = a.Repo.Commit(ctx, a.put(o), a.put(audit(id, u.Sub, "SUBMITTED", "")))
	return o, e
}
func (a *Service) Document(ctx context.Context, u identity.User, id, docID string, input Doc) (Doc, error) {
	o, e := a.owned(ctx, u, id)
	if e != nil {
		return nil, e
	}
	if !contains(s(o, "state"), "DRAFT", "CHANGES_REQUESTED") {
		return nil, fail(409, "Documents cannot change in this state")
	}
	if docID != "" {
		d, e := a.Repo.Get(ctx, a.Config.ControlTable, "DOCUMENT#"+id, docID)
		if e != nil {
			return nil, e
		}
		if d == nil {
			return nil, fail(404, "Document not found")
		}
		if s(d, "state") == "UPLOADED" {
			return d, nil
		}
		obj, e := a.Objects.Inspect(ctx, a.Config.DocumentBucket, s(d, "key"), s(d, "type"), int64(n(d, "bytes")))
		if e != nil {
			return nil, fail(422, "Uploaded document does not match")
		}
		bytes, e := a.Objects.Read(ctx, a.Config.DocumentBucket, s(d, "key"), obj.Version)
		if e != nil {
			return nil, e
		}
		if len(bytes) < 5 || string(bytes[:5]) != "%PDF-" {
			return nil, fail(422, "Invalid PDF")
		}
		d["state"] = "UPLOADED"
		d["version"] = obj.Version
		d["etag"] = obj.ETag
		return d, a.Repo.Commit(ctx, a.put(d), store.Write{Table: a.Config.ControlTable, Item: o, Check: true})
	}
	if !contains(s(input, "kind"), "registration", "recognition") || s(input, "type") != "application/pdf" || n(input, "bytes") <= 0 || n(input, "bytes") > 5*1024*1024 {
		return nil, fail(422, "A PDF up to 5 MB is required")
	}
	docID = uuid.NewString()
	d := Doc{"pk": "DOCUMENT#" + id, "sk": docID, "orgId": id, "kind": input["kind"], "type": "application/pdf", "bytes": input["bytes"], "key": "organizations/" + id + "/" + docID, "state": "AWAITING_UPLOAD"}
	guard := store.Write{Table: a.Config.ControlTable, Item: o, Check: true}
	if b(o, "publicApplication") {
		if n(o, "publicDocumentCount") >= 8 {
			return nil, fail(422, "Document limit reached. Contact the service operator for assistance.")
		}
		o["publicDocumentCount"] = n(o, "publicDocumentCount") + 1
		guard = a.put(o)
	}
	if e = a.Repo.Commit(ctx, a.put(d), guard); e != nil {
		return nil, e
	}
	upload, e := a.Objects.Upload(ctx, a.Config.DocumentBucket, s(d, "key"), s(d, "type"), int64(n(d, "bytes")))
	d["upload"] = upload
	return d, e
}
func (a *Service) Review(ctx context.Context, u identity.User, id, action, reason string) (Doc, error) {
	if e := a.Company(ctx, u); e != nil {
		return nil, e
	}
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if e != nil {
		return nil, e
	}
	if o == nil {
		return nil, fail(404, "Organization not found")
	}
	if action == "approve" && contains(s(o, "state"), "APPROVED", "PROVISIONING", "ACTIVE") {
		return o, nil
	}
	transitions := map[string][2]string{"review": {"SUBMITTED", "UNDER_REVIEW"}, "changes": {"UNDER_REVIEW", "CHANGES_REQUESTED"}, "reject": {"UNDER_REVIEW", "REJECTED"}, "approve": {"UNDER_REVIEW", "APPROVED"}, "suspend": {"ACTIVE", "SUSPENDED"}, "reactivate": {"SUSPENDED", "ACTIVE"}, "retry": {"PROVISIONING_FAILED", "APPROVED"}}
	transition, ok := transitions[action]
	if !ok || s(o, "state") != transition[0] {
		return nil, fail(409, "Invalid state transition")
	}
	if !contains(action, "approve", "review") && strings.TrimSpace(reason) == "" {
		return nil, fail(422, "A reason is required")
	}
	if len(reason) > 1000 {
		return nil, fail(422, "Reason too long")
	}
	o["state"] = transition[1]
	o["reason"] = reason
	o["reviewer"] = u.Sub
	o["reviewedAt"] = now()
	writes := []store.Write{}
	if contains(action, "approve", "retry") {
		if len(m(o, "resources")) == 0 {
			o["resources"] = resourceNames(a.Config, id, s(m(o, "draft"), "displayName"))
		}
		writes = append(writes, a.put(job("PROVISION", id, Doc{})))
	}
	writes = append(writes, a.put(o), a.put(audit(id, u.Sub, strings.ToUpper(action), reason)))
	return o, a.Repo.Commit(ctx, writes...)
}
func (a *Service) Invite(ctx context.Context, u identity.User, t Tenant, input Doc) (Doc, error) {
	if !a.integrationsEnabled(t.Org) {
		return nil, fail(503, "Integrations are disabled for this organization")
	}
	available, err := a.integrationAvailability(ctx)
	if err != nil {
		return nil, err
	}
	if !b(available, "emailConfigured") {
		return nil, fail(503, "Email service is not configured or its worker is unavailable")
	}
	if s(t.Member, "role") != "ORG_ADMIN" {
		return nil, fail(403, "Administrator required")
	}
	email := strings.ToLower(strings.TrimSpace(s(input, "email")))
	role := s(input, "role")
	if !validEmail(email) || !contains(role, "ORG_ADMIN", "ASSESSOR", "VIEWER") {
		return nil, fail(422, "Invalid email or permitted role")
	}
	bytes := make([]byte, 32)
	if _, e := rand.Read(bytes); e != nil {
		return nil, e
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	inv := Doc{"pk": "INVITE", "sk": hash(token), "orgId": t.ID(), "email": email, "role": role, "state": "PENDING", "expiresAt": millis() + 48*3600000, "createdBy": u.Sub}
	writes := append(a.guards(t), a.put(inv), a.put(job("EMAIL", t.ID(), Doc{"email": email, "link": a.Config.Origin + "/#invite=" + token})), a.put(audit(t.ID(), u.Sub, "INVITED", role)))
	return Doc{"email": email, "expiresAt": inv["expiresAt"]}, a.Repo.Commit(ctx, writes...)
}
func (a *Service) Accept(ctx context.Context, u identity.User, token string) (Doc, error) {
	if u.TeamOrg != "" {
		return nil, fail(403, "Team accounts cannot accept invitations to other memberships")
	}
	if len(token) > 100 || token == "" {
		return nil, fail(403, "Invalid invitation")
	}
	inv, e := a.Repo.Get(ctx, a.Config.ControlTable, "INVITE", hash(token))
	if e != nil {
		return nil, e
	}
	if inv == nil || s(inv, "state") != "PENDING" || n(inv, "expiresAt") <= millis() || s(inv, "email") != u.Email {
		return nil, fail(403, "Invitation expired, used, or belongs to another email")
	}
	id := s(inv, "orgId")
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if e != nil {
		return nil, e
	}
	if s(o, "state") != "ACTIVE" {
		return nil, fail(403, "Organization inactive")
	}
	member, e := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+id, u.Sub)
	if e != nil {
		return nil, e
	}
	if b(member, "active") {
		return nil, fail(409, "Already a member")
	}
	next := Doc{"pk": "MEMBER#" + id, "sk": u.Sub, "orgId": id, "email": u.Email, "role": inv["role"], "active": true, "gpk": "USER#" + u.Sub, "gsk": id}
	if member != nil {
		next["rev"] = member["rev"]
	}
	inv["state"] = "ACCEPTED"
	inv["acceptedAt"] = now()
	e = a.Repo.Commit(ctx, store.Write{Table: a.Config.ControlTable, Item: o, Check: true}, a.put(inv), a.put(next), a.put(audit(id, u.Sub, "INVITATION_ACCEPTED", "")))
	return Doc{"orgId": id}, e
}
func (a *Service) Membership(ctx context.Context, u identity.User, t Tenant, subject string, input Doc) (Doc, error) {
	if s(t.Member, "role") != "ORG_ADMIN" {
		return nil, fail(403, "Administrator required")
	}
	if subject == u.Sub || subject == s(t.Org, "owner") {
		return nil, fail(409, "Founding administrator or own membership cannot be changed here")
	}
	role := s(input, "role")
	active, ok := input["active"].(bool)
	if !ok || !contains(role, "ORG_ADMIN", "ASSESSOR", "VIEWER") {
		return nil, fail(422, "Invalid membership update")
	}
	member, e := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+t.ID(), subject)
	if e != nil {
		return nil, e
	}
	if member == nil {
		return nil, fail(404, "Member not found")
	}
	if b(member, "managedTeam") && role == "ORG_ADMIN" {
		return nil, fail(422, "Team accounts can only be assessors or viewers")
	}
	member["role"] = role
	member["active"] = active
	writes := append(a.guards(t), a.put(member), a.put(audit(t.ID(), u.Sub, "MEMBERSHIP_UPDATED", "")))
	return member, a.Repo.Commit(ctx, writes...)
}
