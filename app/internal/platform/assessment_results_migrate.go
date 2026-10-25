package platform

import (
	"context"
	"flag"
	"fmt"
	"time"

	"learning-setu/app/internal/store"
)

// MigrateAssessmentResults is dry-run by default and never calls transcription.
// Re-running it skips records already moved; inline data is removed only after
// the versioned object has been read back and the guarded transaction succeeds.
func MigrateAssessmentResults(ctx context.Context, a *Service, args []string) error {
	flags := flag.NewFlagSet("archive-assessments", flag.ContinueOnError)
	orgID := flags.String("org", "", "organization ID to migrate")
	execute := flags.Bool("execute", false, "move completed inline results to S3")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !resultID.MatchString(*orgID) || flags.NArg() != 0 {
		return fmt.Errorf("usage: setu archive-assessments --org ORGANIZATION_ID [--execute]")
	}
	count, err := a.archiveAssessments(ctx, *orgID, *execute)
	if err != nil {
		return fmt.Errorf("assessment archive stopped after %d records: %w", count, err)
	}
	if *execute {
		fmt.Printf("Archived %d assessment results. Already archived and ineligible records were skipped.\n", count)
	} else {
		fmt.Printf("Dry run: %d completed inline assessment results eligible. No data changed. Add --execute to migrate.\n", count)
	}
	return nil
}

func (a *Service) archiveAssessments(ctx context.Context, orgID string, execute bool) (int, error) {
	o, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", orgID)
	if err != nil {
		return 0, err
	}
	if s(o, "state") != "ACTIVE" || s(m(o, "resources"), "table") == "" || s(m(o, "resources"), "bucket") == "" {
		return 0, fmt.Errorf("active organization with provisioned storage required")
	}
	table := s(m(o, "resources"), "table")
	items, err := a.Repo.Query(ctx, table, "ASSESSMENT", false)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range items {
		moved, err := func() (bool, error) {
			itemCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			current, err := a.Repo.Get(itemCtx, a.Config.ControlTable, "ORG", orgID)
			if err != nil {
				return false, err
			}
			if s(current, "state") != "ACTIVE" || digest(m(current, "resources")) != digest(m(o, "resources")) {
				return false, fmt.Errorf("organization changed during migration")
			}
			d, err := a.Repo.Get(itemCtx, table, "ASSESSMENT", s(item, "sk"))
			if err != nil {
				return false, err
			}
			if d == nil || s(d, "state") != "COMPLETED" || len(m(d, "resultStorage")) != 0 || len(m(d, "result")) == 0 {
				return false, nil
			}
			if s(d, "orgId") != orgID {
				return false, fmt.Errorf("assessment ownership mismatch")
			}
			tenant := Tenant{Org: current, Table: table, Bucket: s(m(current, "resources"), "bucket")}
			student, err := a.Student(itemCtx, tenant, s(d, "studentId"))
			if err != nil {
				if e, ok := err.(*Error); ok && e.Status == 404 {
					return false, nil // Retention/deletion owns these records.
				}
				return false, err
			}
			if _, err = resultPrefix(d); err != nil {
				return false, err
			}
			if !execute {
				return true, nil
			}
			guards := []store.Write{{Table: a.Config.ControlTable, Item: current, Check: true}, {Table: table, Item: student, Check: true}}
			archived, err := a.archiveResult(itemCtx, tenant, d, guards...)
			if err != nil {
				return false, err
			}
			writes := append(guards, a.resultCleanupWrite(archived), store.Put(table, archived), store.Put(table, audit(s(d, "studentId"), "archive-assessments", "ASSESSMENT_RESULT_ARCHIVED", s(d, "sk"))))
			return true, a.Repo.Commit(itemCtx, writes...)
		}()
		if err != nil {
			return count, err
		}
		if moved {
			count++
		}
	}
	return count, nil
}
