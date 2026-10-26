# Learning Setu

A Go reading-assessment application with organization onboarding, company approval, dedicated tenant storage, team roles, and asynchronous Sarvam transcription. The existing feature-based browser frontend is retained.

```text
app/                    Go API, worker, seed/migration commands, unit tests, Dockerfile
  cmd/setu/             Single executable: api | worker | seed | migrate | archive-assessments | health
  internal/             Configuration, identity, repositories, storage, use cases, scoring, Sarvam
frontend/               Browser UI, feature modules, shared API client and contracts
infrastructure/         template.yml (CloudFormation), local nginx configuration
scripts/aws/            Deployment script and example parameters; default profile jso
tests/                 HTTP end-to-end workflow and synthetic fixtures
docs/platform/         API, data model, roles, operations, migration and privacy readiness
compose.yml             Local frontend, Go API/worker, DynamoDB, S3 emulator and Mailpit
compose.debug.yml       GoLand/Delve debugging override
```

## Roles and access

Learning Setu separates company-level organization review from access to an organization's students and assessments.

| Role | Who it is for | Permissions |
|---|---|---|
| Applicant | An authorized representative registering an organization | Create and view their own applications, save drafts, upload supporting documents, submit for review, and correct applications when changes are requested. Applying alone does not grant assessment access. |
| Company administrator | The Learning Setu service operator's review team | Review organization details and documents; approve, request changes, or reject applications; retry failed provisioning; suspend/reactivate or permanently offboard organizations; control per-organization integrations and assessment permission; and view organization audit history. Production company identities require MFA. This role does not automatically grant access to tenant student data. |
| Organization administrator (`ORG_ADMIN`) | The founding representative and additional administrators | View students, results, and recordings; add schools; review assessments conducted by teachers; invite team members; manage other eligible members' roles and access; and manage retention and student privacy requests. |
| Assessor / teacher (`ASSESSOR`) | Staff conducting reading assessments | Add students with guardian authorization; select, update, and delete existing students; record their reading, review audio, and submit/retry assessments. Cannot create schools, manage team access, export student data, withdraw consent, or change retention. |
| Read-only viewer (`VIEWER`) | Staff who need to review learning outcomes | View students, assessment history, results, and authorized audio playback. Cannot add students, conduct assessments, or administer the organization. |

Applicant is an application-owner relationship, not an assignable team role. An existing team member can also apply for another organization. Team roles are assigned per organization, so a person may have different roles in different organizations. Students are records managed by staff; there is currently no student or guardian sign-in role.

Organization administrators can invite only `ORG_ADMIN`, `ASSESSOR`, or `VIEWER`; company privileges cannot be granted through team management. Invitations are single-use, expire after **48 hours**, and must be accepted while signed in with the invited verified email address. Administrators cannot change their own membership or the founding administrator's membership through ordinary team management.

Tenant access requires both active membership and an `ACTIVE` organization. Suspension or membership revocation is enforced on subsequent API requests; previously issued audio links remain usable until their short expiry.

## Organization onboarding

For the complete staff and company workflow, use the [onboarding and operations guide](docs/platform/onboarding.md). It covers platform setup, registration, approval, schools, staff permissions, students and assessments.

### Application form fields

The public form collects **Contact Person Name, Registered address, Email, Phone, GSTIN, and OrgNo**. Contact Person Name identifies the authorized representative; OrgNo is the organization registration number and supplies the initial review/workspace label (`Organization <OrgNo>`). GSTIN is optional; supplied values are normalized to uppercase and checked for 15 alphanumeric characters. These format checks do not verify registration or tax status.

Both declarations are required:

- I accept the service terms
- I am authorized to represent this organization

The form displays: “Privacy notice 2026-10-v1: application information is used for organization review, account administration and security. Contact the service operator for correction, deletion or grievances.”

The simplified form uses `formVersion: organization-v2` and the generic `ORGANIZATION` policy, with an organization registration PDF of up to **5 MB** required before submission. Existing school, NGO, and school-group API applications retain their original validation/document policies until edited into the new form. Application email remains immutable once a public draft has been created and must match the verified sign-in used to activate access after approval. Do not upload Aadhaar.

### Application, review, and activation

1. **Start without signing in:** choose **Apply to register your organization** on the home page (`/#apply`). Opening the form creates no database record.
2. **Complete and save the application:** enter the details above. Save the application reference and private access key shown after saving. Keep the browser tab open to upload documents, check status, and make requested corrections without an account. The browser retains the application details for the current session; keep your own secure copy. The registration page no longer shows a manual reference/key entry block. The contact email cannot be changed after creation.
3. **Upload supporting documents:** select the appropriate document type and upload its PDF, then choose **Submit for review**. Submission checks required fields, declarations, identifiers, and completed document uploads.
4. **Company review:** a company administrator starts the review, examines the application and documents, and approves it, requests corrections, or rejects it. Applicants can see their application status and the review reason.
5. **Corrections, if requested:** open **Continue application**, update and save the details, upload any required documents, and resubmit. Editing is available only in `DRAFT` and `CHANGES_REQUESTED`; a rejected application cannot be edited or resubmitted through this flow.
6. **Provisioning after approval:** a background worker creates a dedicated organization data table and audio bucket. Approval alone does not unlock assessments.
7. **Activation:** after provisioning succeeds, the organization becomes `ACTIVE`. With managed activation emails enabled, new tenant accounts receive a Cognito temporary-password invitation and founding `ORG_ADMIN` access; the first sign-in requires a password change. Existing verified accounts keep their password and receive access. The saved application also retains the manual **Sign in to activate access** fallback, which checks a verified matching email and consumes the private key. Existing authenticated applications continue to activate their owner automatically.

Public applications create no tenant membership before manual verified activation or managed Cognito account invitation after approval. Application creation is limited to 10 per minute across API replicas; each public application allows up to eight document uploads of 5 MB each. Cognito activation invitations use the separate `EnableActivationEmails` switch; general SMTP notices remain disabled with `DisableIntegrations=true`. Lost access keys require operator assistance; there is no public email-based lookup or reset.

The normal status sequence is:

```text
DRAFT → SUBMITTED → UNDER_REVIEW → APPROVED → PROVISIONING → ACTIVE
```

Review can also lead to `CHANGES_REQUESTED` (then resubmission) or `REJECTED`. If setup exhausts its retries, the organization becomes `PROVISIONING_FAILED`; a company administrator can retry provisioning. An active organization can be `SUSPENDED` and later reactivated. Requests for changes, rejection, provisioning retries, suspension, and reactivation require a reason and are recorded in audit history.

### What is available after activation

| Area | What it includes |
|---|---|
| Organization dashboard | Organization display name, the current user's role, and the configured data retention period. Users with multiple active memberships can switch organizations. |
| Team management | Organization administrators can invite administrators, assessors/teachers, and viewers, change eligible members' roles, and revoke access. |
| Student management | Student name, age (4–15), class (2nd–10th), school name, and guardian authorization information, including confirmation of adult status and authority. The simplified form records a staff declaration without method or evidence-reference inputs. Authorization must be verified before recording. |
| Reading assessments | Passage selection, browser microphone recording, audio upload, asynchronous transcription and scoring, processing status, and retries for failed assessments. Production transcription uses Sarvam; local development uses deterministic mock transcription. |
| Results and history | Per-student assessment history, transcript, reading accuracy, ASR-derived pronunciation indicator, words per minute, and authorized recording playback. |
| Privacy and retention | Administrators can configure retention, withdraw consent, correct student details, export a student's structured data, request deletion, and record/resolve grievances. New organizations start with a **90-day** retention period. |
| Organization storage | A dedicated DynamoDB table for tenant records and a dedicated S3 audio bucket. Application documents are stored separately with restricted access. |
| Profile and access refresh | Account email, identity/MFA information, and an option to refresh access after membership or organization status changes. |

For a new organization, the initial administrator should invite the team, review retention settings, add schools from **Schools**, and have teachers add students with verified authorization before starting assessments. New organizations start without students or assessment history; the example schools and results described below belong to the local synthetic seed data.

The current implementation does not include student/guardian portals, school-wide exports, or manual assessment evaluation. See the [architecture and detailed role matrix](docs/platform/architecture.md) and [API reference](docs/platform/api.md) for implementation details.

### Permanently offboard an organization

A company administrator can open **Company review**, find the organization, enter a review reason, and type its organization ID under **Permanently offboard organization**. Confirming this action blocks access immediately and queues irreversible cleanup after an 11-minute drain period.

Cleanup removes students, assessments, the dedicated DynamoDB table, all audio versions and the S3 bucket, registration documents, memberships, invitations and old jobs. **Refresh status** shows progress; **Retry offboarding** resumes failed cleanup. Other organizations and shared sign-in accounts are preserved.

A minimal offboarding receipt remains. Separate backups, provider copies and already downloaded/exported data require separate follow-up; the UI distinguishes live-data deletion from those remaining obligations. See [offboarding operations](docs/platform/operations.md#offboarding).

## Run locally

Install Docker with Compose v2, then run from this directory:

```sh
make up
```

Open **http://localhost:8080**. Choose a synthetic account on the sign-in screen. Seed data includes a company administrator, Alpha and Beta schools, administrators, a teacher, a viewer, students and completed assessments. Invitation email appears at **http://localhost:8025**. Local transcription is deterministic and free; no AWS profile, AWS credentials or Sarvam key is needed.

`make logs` follows API/worker logs. `make down` stops the environment. Both cloud emulators are intentionally ephemeral; `down` followed by `up` starts a new synthetic dataset. Use this environment only for synthetic data. The first build downloads Go modules and Debian audio tools and needs working Docker internet/DNS.

## Debug and verify

```sh
make debug       # API under Delve at localhost:2345; UI remains on port 8080
make test        # Go 1.26+ installed locally; runs tests with race detection
make e2e         # Node 22+ test client against the running Compose stack
make lint        # Go vet and cfn-lint
```

In GoLand, open `app/`, use a Go Remote run configuration at `localhost:2345`, and map the local `app` directory to `/workspace/app`. [Local/debug guide](docs/platform/local.md) also describes running Go directly with only dependencies in Docker. Node is a test-client convenience; the backend/runtime does not use Node or Python.

### Local files and commits

Keep credentials in the AWS profile or Secrets Manager. `.gitignore` excludes local `.env` files, AWS configuration, private-key files, `*.local.json` deployment parameters, IDE/agent settings, build output, test reports and caches. Commit `.env.example`, deployment parameter examples, `app/go.mod` and `app/go.sum`. Docker builds also exclude local credentials and generated reports. Before committing, review `git status --short` and `git diff --cached --check`; ignore rules do not remove files already tracked by Git.

## Deploy to ECS Fargate

### Launch before your domain is ready

Use [parameters.cloudfront.example.json](scripts/aws/parameters.cloudfront.example.json) as your local parameter file and select `AccessMode: cloudfront`. This creates an AWS-provided **HTTPS CloudFront address** in front of a private ALB, so sign-in and microphone recording work without a custom domain or ACM certificate. Omit `AppOrigin` and `CertificateArn`; CloudFormation sets the application origin, Cognito callbacks, and document CORS from the generated address. Open the stack's **ApplicationUrl** output.

The bare ALB address is private in this mode. CloudFront reaches it through a VPC origin; browser traffic uses HTTPS. Application responses are not cached, and authentication and organization headers are forwarded. See [temporary HTTPS deployment and domain migration](docs/platform/operations.md#temporary-https-access-without-a-domain) for network setup, parameters, and the later domain switch.

If email/transcription credentials are not ready, set `"DisableIntegrations": "true"` in the deployment parameters. Registration, organization provisioning, and student management remain available. Company administrators can override this deployment default per organization using **Company review → Organization integrations**. Assessments require configured Sarvam credentials and the assessment-only setting; general email invitations require SMTP credentials.

### Launch with your own domain

Copy `scripts/aws/parameters.example.json` to an ignored local file and fill in the VPC/subnets, HTTPS certificate, domain, SMTP and Sarvam secret ARNs. Then explicitly run:

```sh
./scripts/aws/deploy.sh staging scripts/aws/parameters.local.json
```

The script uses **AWS profile `jso`** and **region `ap-south-1`** by default, builds/pushes the Go image to ECR, and deploys **`infrastructure/template.yml`**. Override `AWS_PROFILE`, `AWS_REGION`, `RESOURCE_PREFIX` or `STACK_NAME` when needed. It does not run automatically. The template runs API, application worker and provisioning worker as separate Fargate services with separate task permissions. The production API serves the frontend from the same image.

Sarvam is the only production transcription adapter. Amazon Transcribe and Bodhan code, worker definitions and provider choices have been removed. Production rejects local authentication, mock transcription and emulator endpoints. Cognito supplies managed authentication, with mandatory MFA for company identities.

The selected reading passage determines Sarvam's language: English sends `en-IN` with `mode: transcribe`; Hindi sends `hi-IN` with `mode: verbatim`. Missing or unsupported languages fail before a provider request. English uses standard transcription because Saaras v3 verbatim mode was observed writing English words in Devanagari despite `en-IN`. Standard transcription may normalize punctuation and numbers. This setting applies to new provider jobs; deploying it does not rewrite existing transcripts or scores. See [transcription troubleshooting](docs/platform/aws-runbook.md#english-speech-appears-in-hindi-script).

For step-by-step AWS setup, secret updates, deployment checks and troubleshooting, see the [AWS runbook](docs/platform/aws-runbook.md). For approximate monthly transcription spending and AWS budgeting inputs, see [monthly costs](docs/platform/monthly-costs.md).

See [architecture and role matrix](docs/platform/architecture.md), [API](docs/platform/api.md), [deployment/operations](docs/platform/operations.md), [migration](docs/platform/migration.md), [India privacy readiness](docs/platform/privacy.md), and [verification and remaining dependencies](docs/platform/delivery.md). AWS deployment and real Sarvam calls require your external configuration and staging verification; local tests do not certify those integrations or legal compliance.


## Organization activation emails

`EnableActivationEmails=true` enables Cognito-managed login invitations independently of `DisableIntegrations`. When approval and storage provisioning finish, the organization becomes ACTIVE and a durable activation job runs. For a new tenant identity, Cognito generates and emails a temporary password with the Learning Setu sign-in link. The recipient signs in using their email, must choose a new password on first login, and receives founding administrator membership automatically. Temporary passwords expire after seven days. Passwords are never stored in application records/jobs or returned by the API.

For an existing confirmed, verified tenant account, the worker preserves the password and grants the matching administrator access. A normal approval notice is sent through configured SMTP when available; while the general email integration is disabled, Company review explicitly shows that this existing-account notice was not sent. Unverified, disabled or recovery-required accounts need account recovery/verification; the worker does not silently confirm or reset them. An existing temporary-password account can receive a renewed Cognito invitation.

Company review shows the invitation status and **Send/resend login invitation** or **Retry login email** for ACTIVE organizations. Resends are rate-limited and never reset a confirmed account's password. Provider acceptance is shown as `INVITATION_REQUESTED`, not proof of inbox delivery. Failed attempts leave the organization ACTIVE and allow a company administrator to retry. The existing key-based manual activation path remains available. No Cognito calls are made by local demo workers unless a test injects a fake adapter.

## Create organization team accounts

An organization administrator can open **Team → Create team member**, enter a new email address as the username and a temporary password, and choose **Assessor / teacher** or **Read-only viewer**. Share those credentials directly with the member and direct them to **Organization sign-in**. No invitation email is required. The temporary password must have 14–128 characters, uppercase/lowercase letters, a number and a symbol, with no spaces. Cognito requires a new password at first sign-in within seven days.

These accounts are permanently scoped to the creating organization. They cannot claim administrator access, register another organization, accept other memberships, or be promoted to organization administrator. Assessors retain assessment permissions; viewers retain read-only permissions. Revoking membership or suspending/offboarding the organization blocks access immediately. Existing usernames/passwords cannot be overwritten by this form.

Team credentials are available only with managed Cognito authentication; local mode retains predefined demo accounts. Passwords pass directly to Cognito over TLS and are never saved in application records/jobs or returned in API responses. If account creation is interrupted, retry with the same username, role and original temporary password; the pending request can resume without resetting credentials. Expired temporary credentials or password recovery without a verified email require operator assistance. Offboarding removes membership and pending account records; the Cognito identity remains bound to the former organization and cannot access another one.


## Organization schools

Organization administrators can open **Schools**, view the school table, and add **School name, Block, District, and State**. Each field is required. Schools belong to the current organization and are stored in its dedicated DynamoDB table under the `SCHOOL` partition; offboarding deletes them with that table.

The New student form now requires a school from this organization's dropdown. Only teachers create students and select their school. Organization administrators view students and filter by school name. Teachers also filter students by school name and conduct assessments; viewers cannot add students or schools. If no schools exist, an administrator must add one first. New students store the school ID and the server-resolved school name. Existing student records keep their original school names; no school locations or associations are invented for historical records.


## Teacher reading workspace

Completed assessment transcripts, word differences and passage snapshots are stored in the organization's private S3 bucket. DynamoDB keeps summary scores, student/date indexes, processing state and a verified object reference. History and export retain their existing format. Existing inline results can be moved with the dry-run-first `archive-assessments` command described in the [operations guide](docs/platform/operations.md#moving-existing-assessment-results-to-s3).

Teachers land directly on the student directory after sign-in. Search by student name or school, then choose **Start reading assessment**. The session shows the existing student's details beside an English/Hindi passage and audio recorder; it does not create another student profile. Choose a passage, press **Start recording**, then **Stop recording**. Listen to the playback, use **Record again** if needed, and press **Submit assessment** to upload and process it. Progress updates automatically, and history remains available to administrators and viewers.

The header's **English / हिन्दी** switch changes the interface language across signed-in screens and remembers the choice on this browser. It updates the current screen without clearing form values, filters or a recording. Reading language is selected independently in the reading session: changing the interface does not replace the selected passage or filter results to a different language. Transcripts, original passages, word-level result text, saved names and entered values remain in their source language; only interface labels and messages are translated.

Passage selection is locked until a recording is discarded so the submitted text matches the audio. Microphone capture ends when leaving the session. If transcription integrations are disabled, teachers can still record and preview locally, but submission is disabled and unsaved audio is discarded on leaving. Organization administrators add schools and manage the team; teachers add students; assessment creation and processing retries require ASSESSOR on both the frontend and API.


The student directory uses a table with name, age, class, school, authorization and actions. **Assessment history** expands beneath the selected student and starts collapsed; history loads when opened, with refresh and retry controls. Search and school filtering apply to the table for both administrators and teachers. On small screens, the table scrolls horizontally within the directory.

Teachers can choose **Edit student** or **Delete student** in a student table row. Editing updates name, age, class and the organization school without replacing existing assessment history or guardian authorization. Deletion requires entering the student's full name; it immediately removes the student from normal access and queues the existing audited cleanup of student records, assessments and recordings. Organization administrators and viewers cannot add, edit or delete students. These actions work even when transcription integrations are disabled.
