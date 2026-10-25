# POC migration: explicit selection, copy, validate, cut over

No migration ran against existing data. The tool only reads records explicitly listed in a mapping; it never scans for a school to infer tenant ownership, deletes a source record, changes a source reference or performs a cutover.

`app/internal/platform/migrate.go` supports the POC's `studentId` primary key with nested `assessments`. Export/review other historical schemas separately; no legacy migration utility is automatically invoked.

Create a restricted mapping file:

```json
{
  "organizationId":"APPROVED-ACTIVE-ORGANIZATION-UUID",
  "sourceTable":"EXPLICIT-POC-TABLE",
  "sourceBucket":"EXPLICIT-POC-AUDIO-BUCKET",
  "sourceSchoolId":"EXACT-POC-SCHOOL-ID",
  "studentIds":["EXPLICIT-STUDENT-UUID"]
}
```

The selected organization must already be ACTIVE and have different destination resources. Configure the platform environment and an operator migration role with explicit source-read and destination-write permissions. Do not widen the application runtime role for migration.

```sh
cd app
go run ./cmd/setu migrate --mapping mapping.json --manifest reviewed-plan.json
# Review source school, selected IDs, source hashes and destination mapping.
go run ./cmd/setu migrate --mapping mapping.json --manifest reviewed-plan.json --execute
```

The default dry run writes a mode-0600 local manifest with resource mapping, source hashes and selected IDs. Execution requires that the reviewed manifest still matches the source and mapping. A changed source record fails closed and needs a fresh review. Freeze POC writes during the final reviewed execution, or account for a later delta explicitly.

Student and assessment IDs are preserved if safe opaque identifiers. Existing assessment fields/results are preserved in `legacy`; they are marked `IMPORTED_REVIEW_REQUIRED`, not silently rescored or represented as a new algorithm's output. Student consent starts `PENDING_VERIFICATION`; an assessor/admin must record a current authorization before recording again. POC data does not prove guardian consent.

Audio is copied with source ETag/version protection to a deterministic opaque destination key, pinned to its new version, and checked by SHA-256. The destination object records its hash. Re-execution validates an existing copy before reusing it. Destination collisions fail instead of overwriting. Student/assessment migration markers make completed steps resumable; a crash after object copy can resume without creating a new recording key. The final student marker is VALIDATED only after its assessment copies complete.

Before cutover, independently compare source/destination student and assessment counts, audio hashes, preserved IDs, representative transcripts/scores, consent state and access-denial tests. The tool validates copied bytes but does not automatically replay the complete legacy UI or validate every provider-specific legacy field. Test on synthetic copies in staging first; migration has not been exercised against your production POC data.

Rollback is a routing/registry decision: keep the POC and source resources unchanged, retain the manifest, and return traffic to the old deployment if validation fails. Target copies remain isolated for investigation and may require later explicit deletion approval. Do not delete/reassign either side automatically. Update the application resource registry under revision control only after review; this migration script performs no traffic switch.
