# Delivery and verification

## Current release — 25 October 2026

JSO staging (`learning-setu-staging`, `ap-south-1`) runs `20261025-english-transcribe-1`, image digest `sha256:6fde39ab247fe130bd1bdd6a10ea9558d2128e4493fc00176689fe27a2a1a133`. CloudFormation completed successfully, replacement workers were confirmed running this image, and the HTTPS health endpoint returned `{"status":"ok"}`. Earlier entries below describe their release dates rather than the current image.

English jobs now send `en-IN` with `mode: transcribe`; Hindi retains `hi-IN` with `mode: verbatim`. The preceding `20261025-reading-language-1` release normalized language names/codes but did not resolve mixed-script output. A live comparison on the affected English recording returned English-script text with both standard transcription and codemix; standard transcription was selected. These were paid Sarvam calls using the existing provider. The comparison did not rewrite stored assessments or scores. This is evidence for the affected recording, not a guarantee of recognition quality across all speakers.

The full Go suite passed before the language-normalization deployment. Transcription and platform tests passed after the mode change, including outgoing English/Hindi language and mode assertions. New completed assessments are using S3 result storage; no historical-result backfill was performed during these releases.

## API capacity reduction — deployed 24 October 2026

Reduced only the JSO staging API task from 512 CPU units / 1,024 MiB to 256 CPU units / 512 MiB. CloudFormation completed successfully and ECS reached steady state with one running API task and zero failed tasks. At that deployment, the image remained `20261024-hide-password-reset-1`; worker allocations and all live parameters were preserved. The pending S3 assessment-storage code was not included in this infrastructure-only rollout.

Before deployment, the exact live image passed the full local synthetic HTTP workflow under enforced 0.25 CPU / 512 MiB limits, with swap disabled. The audio fixture was updated to the current 20-second minimum. A 45-second read-only load test with ten concurrent clients completed 9,941 requests (221 requests/second), zero errors, 100 ms p95 and 189 ms p99 latency. Container peak memory was 21,901,312 bytes (about 21 MiB), with zero restarts/OOM events. The test uses synthetic local authentication and emulated storage; it is not a guarantee of capacity for future production workloads. `tests/api-capacity.mjs` retains the test and refuses non-local hosts/authentication.

Nine deployment-parameter tests and AWS template validation passed. The change set contained only API CPU/memory and its service task-definition update. Post-rollout checks confirmed homepage 200, health `{"status":"ok"}`, and unauthenticated `/api/me` 401. Continue monitoring CPU, memory, latency and task failures as usage grows. Reverting the API task to 512 CPU units / 1,024 MiB restores its previous allocation without changing application data.

## Assessment result storage in S3 — deployed by 25 October 2026

Completed assessment details and passage snapshots now use version-pinned, checksum-verified JSON objects in the existing tenant bucket. DynamoDB keeps summary metrics and processing/index metadata. Detail/history/export responses hydrate the full data and preserve inline-result compatibility. The dry-run-first `archive-assessments --org ID [--execute]` command backfills completed inline results without transcription calls. Guarded commits and delayed orphan cleanup cover storage failures and late writers; student-prefix deletion includes all result versions.

Initial validation: all Go race tests and `go vet ./...` passed in the Docker build environment. A synthetic integration test against local DynamoDB and S3 verified a result over 400 KB, pinned-version reads, history reconstruction, Hindi legacy backfill, safe reruns and deletion of every result version. Unit tests also cover dry-run immutability, full export, tenant isolation, invalid references, missing/corrupt objects, lost upload responses, failed final commits, worker completion and cleanup fencing. Live checks on 25 October confirmed new assessment results in pinned S3 objects. Existing inline records remain readable; backfilling them requires the explicit archive command.

The active application is Go in `app/`, the existing feature frontend in `frontend/`, one CloudFormation template at `infrastructure/template.yml`, and AWS scripts under `scripts/aws/`. One production binary runs API, worker, synthetic seed and explicit-map migration commands. Sarvam is the only production transcription provider; local mock transcription is guarded by environment validation. The 20 original Hindi/English passages are retained in a versioned embedded JSON catalog, alongside two short synthetic test passages.

## Checks performed

- Go compile, 11 unit/service tests via `go test -race ./...`, and `go vet ./...`; the opt-in migration integration test was run separately.
- Service tests: tenant isolation, current membership revocation and suspension, company-only approval, duplicate approval, partial provisioning recovery with stable resource names, invitation email binding/expiry/single use, consent gates, retention limits and duplicate recording completion.
- Authentication tests: valid synthetic identity, forged signature and expired-token rejection; production configuration rejects development settings.
- Scoring tests: Hindi normalization, Unicode case folding, matches, omissions and insertions; unavailable timestamps remain unavailable.
- Sarvam protocol tests: batch creation/upload/start/status/download and no API-key forwarding to signed storage URLs. These are mocked HTTP tests, not paid provider integration calls.
- Real Docker HTTP workflow: onboarding → PDF upload → company review → dedicated resource provisioning → invitation via Mailpit → consent/student → audio upload → asynchronous mock score → playback → cross-tenant denial → revocation → suspension/reactivation → withdrawal → deletion including object versions.
- Migration integration against local DynamoDB/S3: wrong-school mapping rejection, dry-run creates no destination student, explicit execution, resumed execution, retained IDs, copied-byte verification and unchanged source.
- CloudFormation lint using the cached cfn-lint container; deployment shell syntax validation.
- Production runtime and Delve debug image builds, local Compose startup and seed; debug API health and JSON-RPC v2 connectivity. The first Docker builds encountered workstation DNS restrictions; subsequent standard-network build succeeded. No fixed network addresses were committed.

## Initial deployment history and external limits

On 2026-10-20, profile `jso` was used to deploy `learning-setu-staging-network` and launch the `learning-setu-staging` application in `ap-south-1`, using image `setu-staging-app:20261020-offboarding-1` at that time. The application is available at **https://d2n7v84ca43uhr.cloudfront.net**, through the stack's CloudFront distribution `E6TIN7RQU5H3J`. That initial deployment disabled email invitations and transcription with `DisableIntegrations=true`. Current per-organization overrides and managed Sarvam-secret setup are described in the AWS runbook; the global default alone does not indicate whether an organization can transcribe. The staging network includes a NAT gateway and incurs ongoing AWS charges. No production data was migrated.

Temporary HTTPS support uses CloudFront with a private ALB VPC origin. Both CloudFormation templates passed cfn-lint; seven deployment-parameter tests, Go race tests, and Go vet passed; and the runtime image built and pushed successfully. Live checks confirmed the Learning Setu landing page and frontend modules return HTTP 200, `/api/health` returns `{"status":"ok"}`, `/api/config` reports Cognito authentication with integrations disabled, unauthenticated `/api/me` returns 401, and both Cognito authorization flows reach sign-in pages without callback mismatch. All three ECS services started running tasks.

Actual user sign-in/MFA and recovery, initial company-admin bootstrap, Sarvam account/model access and output behavior, SMTP delivery, tenant provisioning and uploads, IAM separation, AWS quotas, PITR/restore, regional data flows and production presigned-URL enforcement still require staging verification. No user account was created during the smoke checks.

Organization offboarding was deployed on 2026-10-20; the application stack reached `UPDATE_COMPLETE`. Go race tests/vet and template validation passed. Regression tests cover company-only authorization, exact-ID confirmation, drain timing, stale-writer rejection, duplicate requests, retries, unsafe mappings, draft cleanup and other-tenant isolation. A synthetic local DynamoDB/S3 integration test verified table/bucket deletion, object versions/delete markers, incomplete uploads, ownership-tag checks, and preservation of another organization. A production-adapter test verifies ownership checks and disabling PITR before deleting a table. Live smoke checks confirmed the new UI, healthy API, unchanged integration settings, and rejection of an unauthenticated offboarding request for a nonexistent ID. No existing AWS organization was offboarded as part of deployment; authenticated destructive cleanup was tested only with local synthetic data. The deployment script defaults to profile `jso` and region `ap-south-1` and requires reviewed parameters.

The local provider deliberately returns the reference passage; it validates orchestration, not speech-recognition quality. Go scoring preserves word alignment and match metrics, not phoneme-level assessment. Sarvam word confidence/timing is not supplied by this adapter; pause count is null. Ambiguous Sarvam creation needs operator reconciliation to avoid duplicate paid jobs. Provider-copy deletion has no automatic API integration: restricted erasure records require manual provider confirmation.

Guardian verification is an auditable operator declaration/evidence reference, not an identity-verification integration. Organization documents do not establish legal compliance. Approved translated notices/contracts, educational-exemption review, legal roles, provider no-training terms, actual processing locations, incident contacts and notification duties remain legal/operational work documented in privacy.md.

Document retention, backup expiry/erasure replay, mailbox/expired-invitation purging, fleet-wide schema/policy migrations, provider reconciliation UI, on-call alarms and company-admin/bootstrap/ownership-transfer workflows need operational completion. List APIs currently query and return full matching partitions; add bounded pagination/load testing before large organizations. Original POC manual evaluations, MCP and school-wide export UI are not part of the new API; student-level privacy exports and legacy-data imports are available.

## Cleanup recovery

The superseded untracked POC/Node files were moved to `/tmp/learning-setu-before-go-20261015-160921` on the implementation workstation before cleanup. This is a private, temporary recovery copy, not a production backup or a repository dependency. Preserve it elsewhere if you need the old manual/MCP/export interfaces. No source AWS data or resources were changed.


## Public application and sign-in update — 2026-10-20

Deployed image `setu-staging-app:20261020-public-registration-1` (digest `sha256:ba911e10ae0b54a7604edd3b1e458cab178846022e765462dbfa27d9abc9d74e`) to `learning-setu-staging` using profile `jso`; CloudFormation completed successfully. Public registration is available at https://d2n7v84ca43uhr.cloudfront.net/#apply. The landing-page sign-in typography and both Cognito hosted-login styles were updated. Email and transcription remain disabled.

Validation passed: Go race tests and vet; seven deployment-parameter tests; CloudFormation lint; JavaScript syntax checks; existing local end-to-end assessment/team/privacy regression suite; and Chrome desktop/mobile public application flow (details, PDF upload, submit, resume, review, provisioning, verified-email activation). New backend tests cover wrong/missing/cross-application keys, pre-approval claim denial, wrong-email/company claim denial, no membership before claim, key revocation after claim/offboarding, upload/creation limits, foreign-origin rejection, and authenticated-route protection. The reusable local browser test is `tests/public-onboarding-browser.mjs` and requires playwright-core and Chrome.

Live smoke checks confirmed healthy API, unchanged integration settings, public form rendering without creating a draft, invalid public submission validation, protected company endpoints, and both branded hosted sign-in pages. No production application was created by these smoke checks; full approval/activation was tested with local synthetic data, not real user credentials.

The home-page header was subsequently changed from **How it works** to **Start application**, linking to `/#apply`, with English/Hindi labels. Image `20261020-onboarding-header-1` deployed successfully to the same stack/profile. Local browser checks verified both translations and navigation; live browser checks verified the header opens public onboarding without creating an application or requiring sign-in.

A follow-up fixed header visibility in authenticated workspaces and on mobile: the old base `.header-link { display: none }` and mobile span rule had hidden the renamed link outside the landing page. The link and label now remain visible in all views. HTML references refreshed entry/style URLs and static responses use `Cache-Control: no-cache, must-revalidate`. Image `20261020-visible-header-1` deployed successfully. Platform tests passed; local Chrome checks used actual synthetic sign-in at desktop/mobile sizes. Live checks verified refreshed assets/cache headers, landing and workspace CSS visibility (without real user credentials), and onboarding navigation.

## Simplified application form — 2026-10-20

Image `20261020-application-fields-1` deployed successfully to `learning-setu-staging` using profile `jso`. The form now collects First Name, Last Name, Registered address, Email, Phone, optional PAN/GSTIN, and OrgNo, with both required declarations and the unchanged 2026-10-v1 privacy notice. Backend normalization/validation and company review support the new version while retaining legacy API validation. The manual Continue an application reference/key entry block and Start application header link were removed as requested; the homepage Apply button remains.

Go race tests/vet and the existing local workflow regression suite passed. Local browser checks covered the new fields, edit/save, PDF upload, submission, review, activation and desktop/mobile layout. Live checks confirmed all eight fields, declaration requirements, exact notice, removed UI blocks, healthy API and disabled integrations without creating production applications.

A subsequent requested draft cleanup found zero DRAFT records in the deployed control table. Its three organization records were all OFFBOARDED receipts from earlier confirmed cleanups; no additional records were deleted.

## Organization activation invitations — 2026-10-20

Deployed image `20261020-activation-email-1` (digest `sha256:f56557f556aacf5799ea8bfaf5afdcd1f80517b6aac4715b143115ec5af5c087`) to `learning-setu-staging` using profile `jso`. CloudFormation completed successfully; both application and provisioning worker deployments reached steady state with no failed tasks. `EnableActivationEmails=true` enables Cognito-managed invitations after provisioning reaches ACTIVE; `DisableIntegrations=true` still disables general SMTP and transcription.

New tenant accounts receive a Cognito-generated temporary password, valid for seven days, with a mandatory first-sign-in change. Existing verified confirmed accounts retain their password; separate approval notices require a configured SMTP sender and are explicitly shown as disabled in the current deployment. Company review offers invitation status and resend/retry. Passwords are not stored in application records, jobs or API responses.

Validation passed: Go race tests and vet; eight deployment-parameter tests; CloudFormation lint; JavaScript syntax; local full workflow regression including Mailpit delivery; and desktop/mobile public onboarding browser tests. Unit tests cover managed account invitations, existing-password preservation, verification requirements, retries, company authorization, cooldown, independent integration flags, and stale activation writes after suspension. Live checks confirmed healthy API, enabled activation configuration, disabled general integrations, invitation UI, 401 protection of the resend endpoint, and the deployed Cognito template/expiry. No production account was created or email sent for verification; inbox delivery remains to be observed on the next genuine organization activation. No ACTIVE organizations existed when checked, so there was no activation backfill.

## Expandable company review table — 2026-10-20

Deployed image `20261020-company-table-1` (digest `sha256:d2238cbe257d914e521247ffe705910b1cce2dde1b3dfc0d507aa819ef8b42ab`) using profile `jso`; CloudFormation completed successfully. Company review now lists organization name/number, contact email and status in a compact table. Rows begin collapsed and expand independently to expose existing details and review, document, invitation, audit and offboarding actions. Search and status filters support larger lists; refresh and action reloads retain filters and expanded rows. Mobile tables scroll within the page.

JavaScript syntax and whitespace checks passed. Local and deployed-frontend browser checks used 30 synthetic organizations with intercepted API responses to verify collapsed rows, keyboard expansion, action visibility, search, status filters, empty results, refresh preservation and mobile overflow containment, without browser errors. No real organization records were changed. Entry/style asset versions were refreshed for the new layout.

## Signed-in registration link removal — 2026-10-20

Deployed `20261020-signed-in-registration-1` using profile `jso`; CloudFormation completed successfully. Removed “Apply to register an organization” from the signed-in Applications screen, preserving existing application listings and public landing-page registration. Entry/style versions were refreshed. JavaScript syntax and whitespace checks passed; local and live frontend browser checks with simulated account/application responses verified the removal and preserved public registration without changing real accounts or organization data.

## Organization-scoped team credentials — 2026-10-20

Deployed image `20261020-team-accounts-1` (digest `sha256:8aa0aca51976f6884298dac8bddf01ef763c8fa0035212ef999310afa4a6a981`) using profile `jso`; CloudFormation completed successfully. Organization administrators can create new email-username accounts with a temporary password and ASSESSOR or VIEWER role from Team. Email is suppressed; the administrator shares credentials directly. Existing accounts/passwords are never overwritten. Membership is granted only after a transaction checks the organization and administrator revisions. A password-free pending reservation supports retries after a partial external operation.

Immutable Cognito `team_org`/`team_request` attributes bind accounts to one organization/request. The public client cannot write either attribute. The team email is deliberately not marked verified; signed scope plus current membership authorizes tenant access. Team accounts cannot claim founder access, register organizations, accept invitations to other memberships, or be promoted to ORG_ADMIN. Offboarding removes team reservations and memberships while retained identities remain unusable outside their original organization.

Go race tests/vet and CloudFormation lint passed. Tests cover credential suppression, existing-identity protection, retries, password non-persistence, admin-only creation, allowed roles, read-only viewers, cross-organization denial, suspension races, revocation, signed-token issuer/audience checks, and offboarding isolation. Existing local end-to-end regression passed. Local and live frontend checks with simulated requests verified the Team form, restricted role editing, selected organization header, clearing the password field and desktop/mobile layout.

A disposable Cognito identity with email suppressed verified the actual hosted sign-in password-change challenge and successful scoped-token verification by the live API. Requests without membership and attempts to create organizations returned 403. The identity was deleted immediately afterward; no organization or real-user account was created or changed. Live client configuration confirmed scope readable but not writable. Temporary-password expiry remains seven days; recovery without a verified email requires operator assistance.

## New student form simplification — 2026-10-20

Deployed `20261020-student-fields-1` (digest `sha256:04a2bc8ba6e7c92febad1478d764f559c11bfecaa0ac220dea1e77804c965f19`) using profile `jso`; CloudFormation completed successfully. New student now provides age choices 4–15 and class labels 2nd–10th, requires school name, and omits Verification method and Reference to restricted verification evidence. The school name is persisted and shown with the student. Guardian declarations remain required; omitted method is recorded as `staff_declaration`, with no invented evidence reference. Legacy documentary methods still require a reference.

Go race tests/vet and focused validation/storage tests passed, covering boundary ages/classes, invalid ranges, missing school in the new form, consent declarations and legacy evidence requirements. The local full workflow regression passed. A local browser test submitted a synthetic student and verified persistence and mobile layout. Live frontend checks used simulated identity/list responses and verified the exact options and removed inputs without creating production student records.

## Organization school details — local verification, 2026-10-21

Organization administrators can list and add schools with required name, block, district, and state. New students select an organization-owned school; the server resolves its name and rejects missing or foreign school IDs. Assessors can select schools but cannot create them. Existing student records retain their recorded school names.

Go race tests and vet passed. The school browser test passed for creation/table display, student dropdown and persisted association, assessor restrictions, empty-list guidance, and mobile layout without browser errors. The full local workflow regression also passed. Tests used synthetic data against the Go-served frontend at port 3000 because another service occupied port 8080. The browser test now accepts `PLATFORM_URL`, and onboarding documentation includes adding schools before students. No AWS deployment was performed in this continuation.

## Organization school details — AWS deployment, 2026-10-21

At the user's request, deployed image `20261021-schools-2` (digest `sha256:3ec5260fd88a109c0ffee27e1da13ac757236f541d2375c514074272e04603ec`) to `learning-setu-staging` in `ap-south-1` using profile `jso`. The existing stack already referenced `20261021-schools-1`; this deployment used the verified workspace and preserved the stack parameters, including enabled activation emails and disabled general integrations.

CloudFormation completed successfully. All three ECS services completed rollout with one running task each, matching desired counts, and zero failed deployment tasks. Live checks at https://d2n7v84ca43uhr.cloudfront.net confirmed API health, exact workspace matches for the entry page, app module, schools module and student/assessment module, and HTTP 401 for unauthenticated school access. No real organization or student records were created for verification. All eight deployment-parameter tests and AWS template validation passed; local `cfn-lint` was unavailable.

## Teacher workflow verification — 2026-10-21

Extended `tests/schools-browser.mjs` and passed the local synthetic workflow as the teacher/ASSESSOR: select an administrator-created school, create a student, verify the saved school ID/name, open a reading passage, and start synthetic microphone capture. The same teacher then initialized an assessment, uploaded synthetic WAV audio via the API, queued it, and received completed mock results. A school created in another organization was absent from the dropdown, and submitting its ID directly returned 422. Existing checks also confirm that teachers cannot create schools, and cover empty-school guidance and mobile layout.

No application permission change was needed. Live staging parameters still have `DisableIntegrations=true`, with no `SarvamSecretArn`, `SmtpHost`, `SmtpSecretArn`, or `MailFrom` configured. Consequently, live recording and transcription remain disabled for every role. Local mock completion verifies the teacher workflow but does not establish live provider readiness. This verification changed tests and documentation only.

## Managed Sarvam API secret — 2026-10-21

Deployed `20261021-sarvam-secret-1` (digest `sha256:86e6f0b5cf76bb02338819d783ab761fc3f8e4374db859ff311fdd1bbbdd179a`) to `learning-setu-staging` using profile `jso`. CloudFormation created the retained `learning-setu/sarvam` secret with the initial JSON value `{"SARVAM_API_KEY":"dummy"}`. The application worker receives only the JSON key via ECS secret injection; the execution role can read the selected secret. An optional existing raw-key `SarvamSecretArn` remains supported. The client trims surrounding whitespace, rejects blank/dummy keys without contacting the provider, and continues authenticating batch transcription with `api-subscription-key` without forwarding that key to signed storage URLs.

Go race tests, Go vet, nine deployment-parameter tests, YAML/secret-reference checks, and AWS template validation passed. CloudFormation completed successfully, live API health passed, and the deployed application task definition selects `:SARVAM_API_KEY::` from the managed secret. The secret has an AWSCURRENT version; its value was not retrieved during verification. Live transcription was not attempted with the placeholder. Integrations remain disabled; after replacing the key, restart the application worker to reload it. Enabling the existing global integration switch still requires SMTP configuration.

## Always prompt for organization credentials — 2026-10-21

Deployed `20261021-organization-login-1` (digest `sha256:005ade52de7acca89661b3dacb05955c7b147ecddb0d440bf3357b0adf1da64e`) to staging using profile `jso`; CloudFormation completed successfully. Organization sign-in now uses Cognito's logout-to-login redirect to clear the previous hosted session before presenting username/email and password. This supports switching between organization administrators and team members on shared browsers. The company sign-in flow remains separate. Fresh OAuth state and PKCE are preserved; browser autofill can still populate fields.

JavaScript syntax and whitespace checks passed. A read-only browser test passed twice before deployment with the workspace module and twice against the deployed frontend, confirming the organization client, credential fields, redirect URI, state and S256 challenge survive the actual Cognito redirect. No credentials were submitted and no real accounts were changed. Live API health passed. `tests/organization-signin-browser.mjs` retains this regression check.

## Existing-student teacher reading workspace — 2026-10-21

Deployed `20261021-teacher-reading-1` (digest `sha256:9acc173f18b7f8ec60f197930cfc842642e72b0a55bea9982764ff68eb96783a`) to `learning-setu-staging` using profile `jso`; CloudFormation completed successfully. The role split is now administrators creating schools/students and teachers conducting assessments of existing students. Student creation requires ORG_ADMIN, while assessment initialization/completion/retry requires ASSESSOR. Administrators and viewers retain history/results access. Teachers land directly on a searchable student directory with a school filter, authorization guidance and an action to start a reading session.

The recorder takes inspiration from the reference `student-readability-app` passage/record/review workflow without its student-creation step. It shows existing student identity, class-matched English/Hindi passage choices, large reading text, microphone level, elapsed time and automatic recording limit. Teachers can stop, listen, discard/re-record, and submit separately. Passage selection is locked to the recorded text; failed submission retains the audio for retry. Processing progress is refreshed automatically. Navigation releases microphone tracks, timers, audio contexts and object URLs. Mobile start/stop controls remain available while reading the passage.

Go race tests/vet and the complete local workflow regression passed. Browser tests verified administrator/teacher restrictions, teacher landing, existing student selection, language changes, microphone denial, recording/playback/re-record, submission retry, actual browser audio upload and mock processing, navigation cleanup, mobile overflow/controls and automatic time limits. The local test proxies the emulator upload only to accommodate port-3000 versus seeded port-8080 CORS. A deployed frontend check used synthetic identity/student responses to verify recording/playback and role-specific UI without real data writes or paid provider calls. Live API health passed; service checks during rollout showed desired running counts and zero failed tasks.

The existing `DisableIntegrations=true` setting was preserved. Teachers can record and preview locally in this mode, but submission is disabled and unsaved audio is discarded on leaving. Live Sarvam processing still requires the integration configuration to be enabled. The API, role matrix, README, local tests and AWS runbook were updated to describe this behavior.

## Teacher student editing and deletion — 2026-10-21

Deployed `20261021-student-management-1` (digest `sha256:9ef6a9da78d5c8905502c732fefb1d468caa3188124c48aafc3112e02c7939af`) to staging using profile `jso`; CloudFormation completed successfully. Teachers and organization administrators now have Edit student and Delete student actions on student cards. Editing supports name, age, class and organization school, preserving guardian authorization and assessment history. Deletion requires entering the student's full name and uses the existing audited privacy deletion job; access is removed immediately and live student/assessment/audio data is cleaned up asynchronously.

API authorization now permits ASSESSOR for only the `correct` and `delete` privacy operations. Export, withdrawal, grievances, privacy-request listing and retention remain administrator-only. Student creation remains administrator-only. Tenant scoping, authoritative school lookup, input validation, membership revision checks and audit attribution remain enforced. These operations do not depend on the transcription integration flag.

Go race tests and vet passed. New service tests cover successful teacher corrections/deletion, unchanged consent, invalid ages, foreign school/student rejection, viewer and unrelated-privacy denial, duplicate deletion rejection, and stale revoked-member writes. A local browser test passed edit/cancel/persistence, confirmation/cancel, mobile layout, viewer restrictions, immediate access removal and completed worker cleanup including removal of synthetic audio. The deployed frontend check used synthetic identity/data responses and cancelled edits/deletion, verifying the controls without real data writes. Live API health passed; rollout checks showed matching running counts and no failed tasks. Documentation and the role matrix were updated.

## 22 October 2026 — student directory and organization navigation

Deployed `20261022-student-directory-1` (digest `sha256:270f6034b54b6825b810351b70a0a2daab0f5a261aa59a46a0b071d518fc7878`) to `learning-setu-staging` with AWS profile `jso` in `ap-south-1`. CloudFormation reached `UPDATE_COMPLETE`. Existing configuration and infrastructure were preserved. Previous image: `223885744552.dkr.ecr.ap-south-1.amazonaws.com/setu-staging-app:20261021-student-management-1`.

Teachers now create, edit and delete students; administrators view the student directory and history. The directory uses a table with school filtering and assessment history collapsed beneath each student. Organization administrators see their current organization name above Applications, without the organization dropdown.

Go race tests passed. The live health endpoint returned `{"status":"ok"}`. Browser regression against the deployed frontend passed using synthetic API responses, covering role controls, table filtering, history expansion/error/retry/refresh, organization heading and mobile layout. No live student data was changed and no paid transcription calls were made for verification.

## 25 October 2026 — visual reading progress status

Deployed `20261025-reading-status-1` (digest `sha256:af12d6848fe720e906f7dea5d22f169b579b3cde9c2fd12270d7149cd700e1e5`) to `learning-setu-staging` using profile `jso` in `ap-south-1`. CloudFormation completed successfully, and all three ECS service rollouts completed with zero failed tasks. Existing infrastructure and deployment parameters were preserved. Previous image: `223885744552.dkr.ecr.ap-south-1.amazonaws.com/setu-staging-app:20261024-hide-password-reset-1`.

Organization administrators now see assessment coverage, clickable reading status summaries, school coverage and status bars, and individual student status badges with next-step guidance. Status follows the selected metric and assessment scope.

Reading progress API tests and frontend model tests passed. The live health endpoint returned `{"status":"ok"}`, and the served dashboard JavaScript matched the local release. Browser regression against the deployed frontend passed using synthetic API responses, including status filtering, organization/school/student views, metrics, language/date/source filters, pagination, keyboard chart access, mobile layouts, and empty/error recovery. Verification did not modify live student data or call transcription providers.
