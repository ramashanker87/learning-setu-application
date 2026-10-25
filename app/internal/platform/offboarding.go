package platform

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
)

func offboardingState(state string) bool {
	return contains(state, "OFFBOARDING", "OFFBOARDING_FAILED", "OFFBOARDED")
}

// Offboarding is irreversible. The confirmation binds the request to the exact
// organization, and the state change and durable job are committed together.
func (a *Service) Offboard(ctx context.Context, u identity.User, id string, input Doc) (Doc, error) {
	if err := a.Company(ctx, u); err != nil {
		return nil, err
	}
	if s(input, "confirmation") != id || strings.TrimSpace(s(input, "reason")) == "" || len(s(input, "reason")) > 1000 {
		return nil, fail(422, "Enter the organization ID and a reason to confirm permanent offboarding")
	}
	o, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fail(404, "Organization not found")
	}
	if contains(s(o, "state"), "OFFBOARDING", "OFFBOARDED") {
		return o, nil
	}
	if !contains(s(o, "state"), "DRAFT", "SUBMITTED", "UNDER_REVIEW", "CHANGES_REQUESTED", "REJECTED", "APPROVED", "PROVISIONING", "PROVISIONING_FAILED", "ACTIVE", "SUSPENDED", "OFFBOARDING_FAILED") {
		return nil, fail(409, "Organization cannot be offboarded in this state")
	}
	if !offboardingState(s(o, "state")) {
		// Allow existing eight-minute worker calls, ten-minute leases, and issued
		// upload URLs to expire before destructive cleanup begins.
		o["offboarding"] = Doc{"requestedAt": now(), "requestedBy": u.Sub, "cleanupAfter": millis() + 11*60*1000, "phase": "WAITING_FOR_IN_FLIGHT_WORK"}
	}
	o["state"] = "OFFBOARDING"
	delete(o, "error")
	j := job("OFFBOARD", id, Doc{})
	j["availableAt"] = m(o, "offboarding")["cleanupAfter"]
	o["offboardingJobId"] = j["sk"]
	err = a.Repo.Commit(ctx, a.put(o), a.put(j), a.put(audit(id, u.Sub, "OFFBOARDING_REQUESTED", s(input, "reason"))))
	return o, err
}

func (w *Worker) offboard(ctx context.Context, j Doc) ([]store.Write, bool, error) {
	a := w.Service
	id := s(j, "orgId")
	o, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if err != nil {
		return nil, false, err
	}
	if s(o, "state") == "OFFBOARDED" {
		return nil, false, nil
	}
	if s(o, "state") != "OFFBOARDING" || s(o, "offboardingJobId") != s(j, "sk") {
		return nil, false, fail(409, "Offboarding job is no longer current")
	}
	progress := m(o, "offboarding")
	if millis() < n(progress, "cleanupAfter") {
		return nil, true, nil
	}
	// The stable mapping must name only this organization's resources. Ownership
	// tags are checked again by the adapters before any table or bucket deletion.
	if !regexp.MustCompile(`^[a-f0-9-]{36}$`).MatchString(id) {
		return nil, false, fmt.Errorf("invalid organization identifier")
	}
	res := m(o, "resources")
	table, bucket := s(res, "table"), s(res, "bucket")
	pattern := regexp.MustCompile("^" + regexp.QuoteMeta(a.Config.Prefix) + `-[a-z0-9-]{1,4}-` + strings.ReplaceAll(id, "-", "") + `-[a-f0-9]{6}$`)
	if (table != "" && (!pattern.MatchString(table) || table == a.Config.ControlTable)) || (bucket != "" && (!pattern.MatchString(bucket) || bucket == a.Config.DocumentBucket)) {
		return nil, false, fmt.Errorf("unsafe organization resource mapping")
	}
	checkpoint := func(phase string) error {
		progress["phase"] = phase
		o["offboarding"] = progress
		if err := a.Repo.Commit(ctx, a.put(o), store.Write{Table: a.Config.ControlTable, Item: j, Check: true}); err != nil {
			return err
		}
		var err error
		o, err = a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
		progress = m(o, "offboarding")
		return err
	}
	if !b(progress, "providerReferencesCaptured") {
		if table != "" {
			exists, err := a.Repo.OwnedTableExists(ctx, table, id)
			if err != nil {
				return nil, false, err
			}
			if exists {
				assessments, err := a.Repo.Query(ctx, table, "ASSESSMENT", false)
				if err != nil {
					return nil, false, err
				}
				for _, d := range assessments {
					if s(d, "providerJobId") == "" && s(d, "providerStep") == "" {
						continue
					}
					reference := s(d, "providerJobId")
					if reference == "" {
						reference = "ambiguous-" + id + "-" + s(d, "sk")
					}
					old, err := a.Repo.Get(ctx, a.Config.ControlTable, "PROVIDER_ERASURE", reference)
					if err != nil {
						return nil, false, err
					}
					if old == nil {
						record := Doc{"pk": "PROVIDER_ERASURE", "sk": reference, "orgId": id, "provider": "sarvam", "state": "MANUAL_CONFIRMATION_REQUIRED", "createdAt": now()}
						if err = a.Repo.Commit(ctx, a.put(record)); err != nil {
							return nil, false, err
						}
					}
				}
			}
		}
		references, err := a.Repo.Query(ctx, a.Config.ControlTable, "PROVIDER_ERASURE", false)
		if err != nil {
			return nil, false, err
		}
		for _, ref := range references {
			if s(ref, "orgId") == id {
				progress["providerFollowUpRequired"] = true
			}
		}
		progress["providerReferencesCaptured"] = true
		if err := checkpoint("DELETING_AUDIO"); err != nil {
			return nil, false, err
		}
	}
	if !b(progress, "audioDeleted") {
		if bucket != "" {
			if err := a.Objects.DeleteTenantBucket(ctx, bucket, id); err != nil {
				return nil, false, err
			}
		}
		progress["audioDeleted"] = true
		if err := checkpoint("DELETING_DOCUMENTS"); err != nil {
			return nil, false, err
		}
	}
	if !b(progress, "documentsDeleted") {
		if err := a.Objects.DeletePrefix(ctx, a.Config.DocumentBucket, "organizations/"+id+"/"); err != nil {
			return nil, false, err
		}
		progress["documentsDeleted"] = true
		if err := checkpoint("DELETING_TENANT_TABLE"); err != nil {
			return nil, false, err
		}
	}
	if !b(progress, "tableDeleted") {
		if table != "" {
			if err := a.Repo.DeleteTenantTable(ctx, table, id); err != nil {
				return nil, false, err
			}
		}
		progress["tableDeleted"] = true
		if err := checkpoint("DELETING_CONTROL_RECORDS"); err != nil {
			return nil, false, err
		}
	}
	// Base-partition queries are strongly consistent, including legacy records
	// without organization lookup indexes. Never scan or erase unrelated tenants.
	for _, pk := range []string{"MEMBER#" + id, "DOCUMENT#" + id, "AUDIT#" + id, "INVITE", "JOB", "TEAM_ACCOUNT"} {
		records, err := a.Repo.Query(ctx, a.Config.ControlTable, pk, false)
		if err != nil {
			return nil, false, err
		}
		for _, record := range records {
			if (pk == "INVITE" || pk == "JOB" || pk == "TEAM_ACCOUNT") && s(record, "orgId") != id {
				continue
			}
			if pk == "JOB" && s(record, "sk") == s(j, "sk") {
				continue
			}
			if err := a.Repo.Commit(ctx, store.Write{Table: a.Config.ControlTable, Item: record, Delete: true}, store.Write{Table: a.Config.ControlTable, Item: j, Check: true}); err != nil {
				return nil, false, err
			}
		}
	}
	receipt := Doc{"pk": "ORG", "sk": id, "id": id, "rev": o["rev"], "state": "OFFBOARDED", "draft": Doc{}, "offboardedAt": now(), "offboarding": Doc{"phase": "LIVE_DATA_DELETED", "providerFollowUpRequired": b(progress, "providerFollowUpRequired"), "backupStatus": "RETENTION_OR_OPERATOR_CLEANUP_REQUIRED"}}
	// Keep only an ID/status receipt and minimal completion audit. Application
	// details, owner lookup, emails, reasons and physical mappings are removed.
	return []store.Write{a.put(receipt), a.put(audit(id, s(progress, "requestedBy"), "ORGANIZATION_LIVE_DATA_DELETED", ""))}, false, nil
}
