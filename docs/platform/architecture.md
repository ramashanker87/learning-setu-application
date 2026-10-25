# Architecture and implementation

## Repository findings

The inspected POC used a root-level JavaScript/HTML frontend, Node Express/Lambda APIs, Cognito, S3, DynamoDB student records with embedded assessments, and Python transcription/scoring workers. A second Node platform implementation duplicated deployment entry points. The requested cleanup replaces both backend entry points with Go and retains the feature frontend. Original Unicode normalization, stable word alignment, accuracy and ASR-derived pronunciation calculations are ported into Go. Sarvam is the sole production provider. Legacy manual evaluation, school-wide exports and MCP interfaces are outside the new application API; legacy records can be imported for review.

## Modules

`app/cmd/setu` starts the API, worker, seed or migration command. `internal/config` validates centralized environment settings and rejects development adapters outside local/test. `internal/identity` verifies JWT signature, issuer, audience, expiry, token type and verified email for ordinary accounts. Managed team accounts instead require an immutable organization scope set by the administrator-only creation API; their email is a username, not proof of mailbox control. Separate Cognito pools enforce company MFA and support tenant MFA.

`internal/platform/http.go` maps HTTP requests to application use cases. `organizations.go` owns onboarding, review and invitations; `assessments.go` owns students, recordings and privacy; `worker.go` owns durable job execution. `onboarding-policies.json` configures applicable identifiers and documents by type; edit the reviewed policy and rebuild. No Aadhaar field is offered. `internal/store` and `internal/storage` isolate AWS SDK v2 repositories/adapters. `internal/transcription` implements Sarvam batch calls and ffprobe media validation; `internal/scoring` implements deterministic scoring. Local/production use the same business services.

Frontend feature modules use a shared API client, loading/error states and native forms. `contracts.d.ts` documents response types; the UI remains JavaScript, not a full TypeScript conversion. No frontend bundler is required.

## State and concurrency

Applications follow DRAFT → SUBMITTED → UNDER_REVIEW → CHANGES_REQUESTED / REJECTED / APPROVED. Corrected applications resubmit. Approval writes the immutable physical resource mapping, APPROVED state, audit and provisioning job in one DynamoDB transaction. Worker progress moves through PROVISIONING to ACTIVE, or PROVISIONING_FAILED after retry exhaustion. Company administrators can retry failed provisioning, suspend ACTIVE organizations and reactivate SUSPENDED ones with reasons.

Company administrators can permanently offboard any nonterminal organization with its exact UUID confirmation and a reason. The transaction sets `OFFBOARDING` and queues an `OFFBOARD` infrastructure job, blocking access immediately. After an 11-minute drain period, checkpointed cleanup deletes tenant resources and related control records, then leaves an `OFFBOARDED` live-data deletion receipt. `OFFBOARDING_FAILED` supports explicit retry and never restores access. Minimal completion audit and external provider-erasure references remain; shared identities and separate backups are outside live-data cleanup.

Worker job claims use revisions and a ten-minute lease. Final results and completion are committed together using the claimed revision. Crashed claims can be reclaimed. Sarvam job IDs and upload/start steps are persisted on the assessment. A lost creation response is marked ambiguous and requires operator reconciliation instead of creating another paid job. Duplicate completion calls cannot enqueue multiple assessment jobs; retrying an already complete worker job cannot overwrite its result. Jobs have bounded retry delays, a five-attempt failure threshold and a thirty-minute assessment deadline. Pending provider polls retain the same durable job. Failed provisioning never deletes data.

Resources use a sanitized name prefix, immutable organization UUID without hyphens, and account/region hash. The name is assigned once; display-name updates only update organization metadata. Existing bucket/table ownership is checked before adoption. A bucket created immediately before a tagging failure requires operator ownership review rather than unsafe adoption.

## Data model and access patterns

All new tables use `pk`/`sk`, numeric `rev`, and optional `gpk`/`gsk` for the `lookup` index. A transaction compares each old revision or requires a new key not to exist. No normal application operation uses DynamoDB Scan.

| Store | Primary keys | Queries |
|---|---|---|
| Shared control | `ORG / organization UUID` | Company list queries ORG; applicant index `OWNER#subject` |
| Shared control | `MEMBER#org / subject` | Consistent membership authorization; `USER#subject` index for navigation |
| Shared control | `COMPANY / subject` | Current company-admin allowlist |
| Shared control | `INVITE / SHA256(token)` | Single-use acceptance, email binding, explicit expiry |
| Shared control | `DOCUMENT#org / opaque document ID` | Restricted application documents; versioned object reference |
| Shared control | `JOB / UUID` | `READY` index for polling; leases and progress; completed/failed jobs retained |
| Shared control | `AUDIT#org / timestamp#UUID` | Reviewer/action/reason history |
| Dedicated tenant table | `STUDENT / preserved or generated UUID` | Student listing and exact lookup; current consent and expiry |
| Dedicated tenant table | `ASSESSMENT / UUID` | Exact lookup; `STUDENT#id` index for history |
| Dedicated tenant table | `PRIVACY / UUID` | Privacy request tracking |
| Dedicated tenant table | `AUDIT#student / timestamp#UUID` | Consent/privacy audit history |

Students and assessment results never go in the shared control plane. Assessment items contain recording metadata, pinned audio version, passage metadata, provider/scoring versions, summary scores and processing state. When processing completes, the full result (including transcript and word differences) and passage snapshot are stored as JSON in the organization's private, versioned S3 bucket. The DynamoDB item retains `resultStorage` with the object key, version, SHA-256 and schema version; its `result` contains only summary metrics. In-progress records retain the passage text needed for scoring. Existing inline results remain readable until explicitly archived.

Audio keys are `students/{studentId}/assessments/{assessmentId}/{recordingId}`. Result keys are `students/{studentId}/assessments/{assessmentId}/results/{attemptUUID}/result.json`. The API resolves result objects through the authorized tenant bucket, validates their ownership and checksum, and reconstructs the existing detail/history/export response. No result object is made public. History currently reads one object per archived assessment; moving details to S3 reduces DynamoDB item and index size but adds S3 requests and is not a guarantee of net savings.

Before uploading, the worker registers a delayed `RESULT_CLEANUP` job. It reads back the pinned object version before committing the result summary, object reference, assessment completion, and assessment-job completion. That transaction also conditionally deletes the cleanup job. A failed or ambiguous upload/commit leaves cleanup scheduled for 15 minutes later. Cleanup removes only its attempt's directory if it is unreferenced; claiming cleanup advances its revision and prevents a late writer from publishing a deleted object. Student erasure deletes the entire student prefix, including result versions. Organization offboarding removes the tenant bucket. Failed cleanup jobs require operator review/retry. Verification documents use a separate bucket and opaque identifiers. Resource names and object keys contain no student names. Export returns the student's full structured data; authorized playback provides audio separately.

The simple READY job partition and organization-wide student partition suit the initial modular monolith. Before high-volume deployment, load-test and introduce time/status shards and incremental frontend list rendering. Worker erasure queries the base assessment partition to avoid missing recent writes due to GSI lag; this is a query, but costs grow with tenant size.

## Role matrix

| Operation | Applicant | Company admin | Org admin | Assessor | Viewer |
|---|---:|---:|---:|---:|---:|
| Own application and documents | Yes | Through review | Own application | Own application | Own application |
| Review/approve/reject/suspend/retry provisioning | No | Yes, MFA identity | No | No | No |
| Permanently offboard/retry cleanup | No | Yes, explicit UUID confirmation | No | No | No |
| Student/assessment reads and playback | No | No implicit tenant access | Yes | Yes | Yes |
| Add students | No | No implicit tenant access | No | Yes | No |
| Record authorization | No | No implicit tenant access | Yes | Yes | No |
| Conduct/retry assessments | No | No implicit tenant access | No | Yes | No |
| Invite, change permitted roles, revoke membership | No | No implicit tenant access | Yes | No | No |
| Correct/delete existing students | No | No implicit tenant access | No | Yes | No |
| Retention, export, consent withdrawal and grievances | No | No implicit tenant access | Yes | No | No |
| Grant company privileges | No | No public API | No | No | No |

Organization selection is a request hint, always checked against current membership and ACTIVE state. Suspension/revocation applies on the next request even with an older token. Already issued direct S3 URLs remain usable until their short expiry (upload 120 seconds, playback 60 seconds); immediate revocation of a previously issued URL is not promised. Founding administrators cannot be revoked via ordinary team management; transfer/offboarding requires an audited operator process.


Public application routes use a 256-bit random access key, stored as a SHA-256 hash on the organization. The key travels only in `X-Application-Key`, never URLs. Public responses whitelist application fields and cannot return resource mappings or the hash. Public applicants have a synthetic owner reference with no identity-service account or tenant membership. On approval, provisioning creates storage but skips founding membership for public applications. Claim requires an ACTIVE organization, the key, and a verified tenant identity with the exact contact email; one optimistic transaction replaces the synthetic owner, grants founding membership and removes the key. Offboarding invalidates public access immediately and removes key/contact data with the organization receipt.


When managed activation emails are enabled, provisioning also queues an `ACTIVATION` job in the transaction that marks the organization ACTIVE. The application worker uses tenant-pool-scoped Cognito permissions to request an invitation, then commits owner/membership and notification status with optimistic organization/job checks. Cognito generates, delivers and expires temporary passwords; none are represented in application storage. Existing verified accounts keep credentials. A managed activation keeps the public application's read-only status capability until manual claim or offboarding; it does not enable public edits of ACTIVE records. The alternate manual claim path supports already-created membership without duplicate-transaction failures.
