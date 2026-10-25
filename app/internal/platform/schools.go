package platform

import (
	"context"
	"github.com/google/uuid"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"sort"
	"strings"
)

// Schools live in the organization's dedicated DynamoDB table.
func (a *Service) Schools(ctx context.Context, t Tenant) (Doc, error) {
	items, err := a.Repo.Query(ctx, t.Table, "SCHOOL", false)
	if err != nil {
		return nil, err
	}
	out := []Doc{}
	for _, school := range items {
		if s(school, "orgId") == t.ID() {
			out = append(out, school)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(s(out[i], "name")) < strings.ToLower(s(out[j], "name")) })
	return Doc{"items": out}, nil
}
func (a *Service) CreateSchool(ctx context.Context, u identity.User, t Tenant, input Doc) (Doc, error) {
	if err := admin(t); err != nil {
		return nil, err
	}
	school := Doc{"pk": "SCHOOL", "sk": uuid.NewString(), "orgId": t.ID(), "createdAt": now(), "createdBy": u.Sub}
	for _, key := range []string{"name", "block", "district", "state"} {
		value := strings.TrimSpace(s(input, key))
		if value == "" || len(value) > 200 {
			return nil, fail(422, "Enter "+key+" (up to 200 characters)")
		}
		school[key] = value
	}
	writes := append(a.guards(t), store.Put(t.Table, school), a.put(audit(t.ID(), u.Sub, "SCHOOL_CREATED", s(school, "sk"))))
	return school, a.Repo.Commit(ctx, writes...)
}
func (a *Service) school(ctx context.Context, t Tenant, id string) (Doc, error) {
	if id == "" || len(id) > 64 {
		return nil, fail(422, "Select a school added by your organization administrator")
	}
	school, err := a.Repo.Get(ctx, t.Table, "SCHOOL", id)
	if err != nil {
		return nil, err
	}
	if school == nil || s(school, "orgId") != t.ID() {
		return nil, fail(422, "Select a school belonging to this organization")
	}
	return school, nil
}
