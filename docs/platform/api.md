# Platform API

The new entry point is `app/cmd/setu` (`setu api`). JSON requests and responses use `/api`. Authenticated requests send `Authorization: Bearer <ID token>`. Tenant routes additionally send `X-Organization-Id`; the server checks the selected organization against live membership. Neither role nor organization claims grant access. Errors are `{ "error": "message" }`, with 401/403/404/409/422/500 status codes. Request bodies are limited to 64 KB.

| Method/path | Purpose |
|---|---|
| GET `/config`, `/health` | Public UI configuration, liveness |
| POST `/local/session` | Synthetic local account selector; absent with production auth |
| POST `/public/applications` | No sign-in; validated details → `{application, accessKey}` (201). Save the key; only its hash is stored. Global limit: 10 creates/minute (429). |
| GET/PUT `/public/applications/:id` | Requires `X-Application-Key`; retrieve public application fields or edit DRAFT/CHANGES_REQUESTED. Contact email is immutable. |
| POST `/public/applications/:id/submit` | Requires application key; same document/policy validation as authenticated applications. |
| POST `/public/applications/:id/documents` | Requires application key; PDF up to 5 MB, max eight uploads per application. |
| POST `/public/applications/:id/documents/:doc/complete` | Requires application key; verify uploaded PDF and pin version. |
| POST `/applications/:id/claim` | Verified tenant sign-in plus `{accessKey}`; ACTIVE application, matching contact email; atomically grants founding membership and invalidates key. |
| GET `/me` | Verified identity, current memberships, company role |
| GET/POST `/applications` | List own applications/create draft |
| PUT `/applications/:id` | Save own DRAFT or CHANGES_REQUESTED application |
| POST `/applications/:id/submit` | Validate policy, declarations and completed documents |
| POST `/applications/:id/documents` | Register PDF; returns short-lived upload URL |
| POST `/applications/:id/documents/:doc/complete` | Verify PDF size, type, signature and version |
| POST `/invitations/accept` | `{token}`; verified invited email, expiry and atomic single-use check |
| GET `/company/organizations` | Company review queue |
| POST `/company/organizations/:id/:action` | review, changes, reject, approve, suspend, reactivate, retry; `{reason}` |
| POST `/company/organizations/:id/offboard` | Company administrator only; `{confirmation: "<exact organization UUID>", reason}`; 202 queues irreversible cleanup or returns existing progress. Repeat after `OFFBOARDING_FAILED` to retry. |
| GET `/company/organizations/:id/documents` | Restricted document playback URLs |
| GET `/company/organizations/:id/audit` | Review/action history |
| GET `/company/jobs` | Job status without email-link payloads |
| GET `/tenant/dashboard`, `/tenant/passages` | Tenant summary and versioned passages |
| PUT `/tenant/organization` | `{displayName}`; resource names stay unchanged |
| GET `/tenant/students` | List students for current organization members |
| POST `/tenant/students` | ASSESSOR only; create a student linked to an organization school |
| GET `/tenant/students/:id` | Authorized student lookup |
| POST `/tenant/students/:id/consent` | Replace current authorization with audited verification decision |
| GET `/tenant/students/:id/assessments` | Student history |
| POST `/tenant/assessments` | ASSESSOR only; Register recording and return upload URL |
| GET `/tenant/assessments/:id` | Processing state/result |
| POST `/tenant/assessments/:id/complete` | ASSESSOR only; Validate uploaded object and durably enqueue processing |
| POST `/tenant/assessments/:id/retry` | ASSESSOR only; Retry a failed assessment without changing recording identity |
| GET `/tenant/assessments/:id/audio` | Version-pinned, 60-second playback URL |
| GET `/tenant/schools` | Current organization schools, readable by members |
| POST `/tenant/schools` | ORG_ADMIN only; required `{name, block, district, state}`, each up to 200 characters |
| GET `/tenant/members` | Organization team |
| POST `/tenant/members` | ORG_ADMIN only; `{username, temporaryPassword, role}`. Username is a new email address; role ASSESSOR or VIEWER. Creates a Cognito identity scoped to this organization without sending email. |
| POST `/tenant/invitations` | `{email, role}`; email delivered by worker |
| PATCH `/tenant/members/:subject` | `{role, active}`; no company role permitted |
| GET `/tenant/privacy` | ORG_ADMIN only; list privacy requests |
| POST `/tenant/privacy` | ORG_ADMIN or ASSESSOR for `correct`/`delete`; ORG_ADMIN only for `withdraw`/`export`/`grievance`. All targets are scoped to the current organization. |
| POST `/tenant/privacy/:id/resolve` | Resolve an open grievance with `{reason}` |
| PUT `/tenant/retention` | `{days}` within company limits; shorter periods apply to existing records |

Application schema and configurable identifiers are in `app/internal/platform/service.go` and `onboarding-policies.json`. A student's creation body is:

```json
{
  "student": {"formVersion":"student-v3","name":"Synthetic learner","age":9,"className":"4","schoolId":"<school sk from /tenant/schools>"},
  "consent": {
    "guardianName":"Synthetic guardian","relationship":"parent",
    "noticeVersion":"2026-10-v1","purposes":["reading_assessment"],
    "guardianAdultVerified":true,"authorityVerified":true
  }
}
```

These booleans record an assessor's completed verification decision; they do not themselves verify a guardian. Adult participants currently also use this conservative authorization form; a distinct adult self-consent experience is not implemented.

Assessment creation body: `{studentId, passageId, type, bytes, duration}`. Supported content types: audio/wav, audio/webm, audio/ogg, audio/mp4. Use the returned upload method and headers exactly, including `If-None-Match: *`. Call completion only after a successful upload. Server-side ffprobe measures actual duration independently of the browser's value. Poll assessment GET for QUEUED → PROCESSING → COMPLETED or FAILED. Provider/scoring versions and the passage snapshot are persisted.

Completed assessment details and passage text are stored in private S3 JSON objects, with a version-pinned `resultStorage` reference and summary metrics in DynamoDB. Assessment GET, student history and student export still return the full `result` and `passage` fields, including older inline results. Missing, corrupted or mismatched result objects return 503 instead of displaying an incomplete summary as a full assessment. Playback and processing-state mutations do not load result JSON. Existing UI clients need no change.

Student ages must be whole numbers from 4 to 15; class values are strings `"2"` through `"10"`. New student creation requires `schoolId` belonging to the current organization. The server saves the school ID and authoritative school name; existing student records retain their historical names. Omitted verification method is recorded as `staff_declaration`, with the guardian declarations still required. Legacy documentary methods still require an evidence reference.

Privacy bodies use `{kind, studentId}`; correction additionally supplies `{correction:{name,age,className}}`, optionally with `schoolId` to select another school in the same organization. Linked students retain an authoritative school name; arbitrary school-name overrides are ignored. Export returns JSON. Deletion returns a queued request and eventually `LIVE_DATA_DELETED`; that state explicitly does not assert deletion from all backups. Withdrawing authorization prevents subsequent uploads, processing and new playback URLs; a separate erasure request or retention sweep removes retained records.

Lists return repository records including opaque IDs/revisions. The Go repository follows DynamoDB pages internally; this version returns complete matching partitions. Incremental client pagination and a generated OpenAPI schema remain scale/contract improvements. `frontend/contracts.d.ts` and the route table above document the current contract.

Offboarding immediately changes the organization to `OFFBOARDING`, blocking tenant access, invitation acceptance, new background jobs and document playback URLs. Cleanup starts after an 11-minute drain period. Company organization responses include `offboarding.phase` and `cleanupAfter`; use the company list to poll. Cleanup removes the tenant table, audio bucket including every version/delete marker and incomplete multipart upload, registration-document versions, memberships, invitations, old jobs, and old organization audits. It leaves an `OFFBOARDED` receipt, a completed cleanup job, and a minimal completion audit. No reactivation is supported. After retry exhaustion, state is `OFFBOARDING_FAILED`, access stays blocked, and the same endpoint resumes from checkpoints.

`LIVE_DATA_DELETED` does not mean all external copies are gone. The worker disables tenant PITR before table deletion, but separate backups, shared-control-table recovery points, exports/downloads and provider copies require separate retention or cleanup. Provider job references remain in `PROVIDER_ERASURE` when follow-up is required. Shared Cognito identities and their memberships in other organizations are preserved.


The current application form payload uses `formVersion: "organization-v2"`, `firstName`, `lastName`, `address`, `contactEmail`, `contactPhone`, `orgNo`, `identifiers: {pan, gstin}`, `termsAccepted`, `authorityDeclared`, and `noticeVersion: "2026-10-v1"`. PAN/GSTIN are optional strings; nonempty values must be 10/15 alphanumeric characters. The server trims fields, uppercases identifiers, derives `type: "ORGANIZATION"`, `displayName: "Organization <orgNo>"`, and representative name/phone. The generic policy requires a registration PDF. Legacy payloads without the new form version retain their original type-specific validation. Tax identifiers are format-checked, not independently verified.


`GET /config` exposes `activationEmailsEnabled`. Company-only `POST /company/organizations/:id/activation-email` queues/retries a login invitation for an ACTIVE organization (200), is idempotent while the current job is pending/running, and rate-limits new requests to one per minute (429). It returns 503 when managed invitations are disabled. The organization response includes `activationEmail.state`: QUEUED, INVITATION_REQUESTED (Cognito request accepted), EXISTING_ACCOUNT (SMTP notice sent), EXISTING_ACCOUNT_EMAIL_DISABLED (access ready, notice not sent), or FAILED. No response includes a password. Public application status includes only `activationAccessGranted` for the new automatic access path, not internal notification job details.


Teachers update an existing student using `POST /tenant/privacy` with `{"kind":"correct","studentId":"<id>","correction":{"name":"Updated name","age":9,"className":"4","schoolId":"<organization school id>"}}`. The server validates the age/class and resolves the authoritative school name. Guardian authorization and assessment history are preserved. For deletion, send `{"kind":"delete","studentId":"<id>"}`; the response has state `QUEUED`, access is blocked immediately, and the worker performs existing live-data cleanup. Both actions record the actor and audit event and enforce membership revision checks. Student creation remains ORG_ADMIN only.

## Organization integration controls

Company administrators use `POST /api/company/organizations/{id}/integrations` with `{"enabled":true,"reason":"Start school assessments"}`. The Boolean is stored as `integrationsEnabled` on that organization and audited with its prior value. Only active or suspended organizations support changes. An explicit setting overrides the deployment's `DisableIntegrations` default; organizations without a setting keep inheriting that default. Enabling does not reactivate a suspended organization or configure credentials.

`GET /api/tenant/integration-settings` returns `enabled`, `inherited`, `assessmentConfigured`, `emailConfigured`, `assessmentEnabled` and `emailEnabled` for the caller's tenant. The company organization list also includes `integrationAvailability` with the two configuration booleans. Credential values never enter these responses. The application worker reports credential presence, with a five-minute status expiry; presence is not provider authentication or credit verification.

The existing `POST /api/company/organizations/{id}/assessment-processing` and `GET /api/tenant/assessment-settings` remain supported for the assessment-only preference. Assessments require organization integrations, that preference and assessment service configuration. General SMTP invitations require organization integrations and email service configuration. Disabling pauses queued work without consuming job attempts; requests already in progress may finish. Cognito sign-in invitations remain separately controlled by `EnableActivationEmails`.
