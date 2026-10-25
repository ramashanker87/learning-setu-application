# India privacy readiness — reviewed 15 October 2026

This is a technical readiness record, not a legal-compliance certification. Registration documents support human organization review; collecting them does not prove compliance. Indian hosting alone does not settle legal duties or international data flows.

## Official sources and commencement

The commencement notification activates different parts of the DPDP Act in stages: specified institutional/definition provisions at publication, section 6(9) and section 27(1)(d) after one year, and the main processing obligations, rights and enforcement provisions after eighteen months. The notification is dated 13 November 2025; its electronic Gazette identifier bears 14 November. Counsel should confirm the operative publication date and calendar deadlines before relying on a day-specific launch date. Working planning milestones are November 2026 and May 2027, rather than assuming all obligations already apply. [Official G.S.R. 843(E)](https://www.meity.gov.in/static/uploads/2025/11/c56ceae6c383460ca69577428d36828b.pdf).

The final Rules similarly phase commencement. Rules 3, 5–16, 22 and 23 begin eighteen months after publication. They address understandable notices, security safeguards, guardian verification, rights and breach reporting. Rule 10 requires due diligence about the parent's adulthood and identity. Rule 7 anticipates notification to affected people and the Board without delay, with detailed Board information within 72 hours, subject to permitted extension. Rule 12/Fourth Schedule contains purpose- and entity-limited child-processing exemptions; educational institutions are not a blanket exemption for all processing by a school, NGO or platform vendor. Legal review must decide whether this reading service and each entity qualify. [Final Rules, G.S.R. 846(E)](https://www.meity.gov.in/static/uploads/2025/11/53450e6e5dc0bfa85ebd78686cadad39.pdf). Apply the official [December 2025 corrigendum](https://www.meity.gov.in/static/uploads/2025/12/3c7ebbae0e5456f493f486e6845df86b.pdf) when reviewing the text.

MeitY's parliamentary response specifically discusses students, guardian consent, educational exemptions and the transition period. It states that the IT Act/SPDI framework continues to govern data protection during that transition. Review the applicability of that framework, contractual duties, sector/state rules and the nature of the data, rather than postponing privacy work until DPDP commencement. [Official parliamentary response, 12 December 2025](https://www.pib.gov.in/PressReleasePage.aspx?PRID=2202901&lang=2&reg=48).

CERT-In's separate directions include reporting specified cyber incidents within six hours and maintaining prescribed ICT logs for 180 days within India. Assess the entity, incident category and current directions/FAQs; do not substitute the future DPDP timetable for current incident obligations. [CERT-In directions](https://cert-in.org.in/PDF/CERT-In_Directions_70B_28.04.2022.pdf), [official FAQs](https://www.cert-in.org.in/PDF/FAQs_on_CyberSecurityDirections_May2022.pdf).

## Implemented controls and their limits

The application records a versioned purpose/notice, guardian relationship, reviewer identity, timestamp and explicit verification declarations before permitting student assessment. The simplified New student form records `staff_declaration` without collecting a verification-method selection or evidence reference. Legacy documentary verification requests retain their method and required evidence reference. No Aadhaar field exists. These records support an operator's verification process; there is no automated identity, DigiLocker or guardian-account verification integration. When a legacy documentary verification method is used, the school maintains the referenced evidence. The `verified_guardian_account` method is a declaration of verification already performed, not an integration supplied by this code. No exemption bypass is enabled.

Organization administrators can withdraw authorization, export structured records, record grievances and choose retention within company-configured limits. Assessors cannot change retention or export through the privacy API. Founding administrators and company identities have distinct authority. All organization review actions record actor, timestamp and reason; consent/privacy decisions are audited in the dedicated tenant table.

Default notices are product placeholders describing reading assessment, access administration and security. Before launch, replace them with reviewed, accessible notices in the languages used by each school, identify actual contact details and rights channels, and record the approved notice version. The application terms likewise need an actual approved contract; checking the current terms box does not supply one.

## Responsibility and provider register

Proposed contractual allocation, subject to counsel: each organization decides which students participate and the educational purpose; the company operates the service and handles its own onboarding/security purposes; transcription vendors process only the contracted audio/transcript task. Determine actual Data Fiduciary/processor roles from decisions and contracts, not these labels. Contracts must assign guardian verification, notices, rights response, subprocessors, breach cooperation, deletion, audit and international-transfer obligations.

The production Go worker reads the pinned recording version from the tenant S3 bucket and uploads its bytes to Sarvam's batch storage URL. Sarvam returns transcript JSON; scoring runs in the Go worker and results are stored in the tenant DynamoDB table. The batch protocol can use Azure or Google storage URLs. This is an external data transfer even when AWS runs in `ap-south-1`; actual Sarvam processing, storage, support and subprocessor locations must be contractually confirmed before student data is enabled. The adapter accepts only HTTPS provider storage hosts, and never forwards the Sarvam API credential to signed storage URLs.

Browser requests pass through the application and directly to tenant S3 using short-lived URLs. Cognito handles identity data; SMTP handles invitation addresses and links. Supporting documents remain in the separate restricted bucket. Local Moto/DynamoDB/Mailpit use synthetic data on the development host. Record AWS, Cognito, SMTP and Sarvam locations, subprocessors, transfer terms and support access in the vendor register.

No application feature authorizes provider reuse, training or student profiling. Obtain and retain contractual no-training/no-reuse restrictions and deletion commitments from Sarvam before enabling real audio. The repository cannot prove a provider's internal deletion or reuse behavior. Separately authorized purposes require separate reviewed notices and decisions.

## Retention and deletion design

| Data | Current mechanism | Required operational follow-through |
|---|---|---|
| Students, recording metadata, transcripts, scores | Per-student expiry; hourly worker sweep and explicit erasure jobs; conditional deletion | Monitor failed jobs and maximum erasure latency; no DynamoDB TTL reliance |
| Audio objects (transcripts are in DynamoDB) | Enumerate and delete all versions and delete markers under exact student prefixes | Validate AWS version-list pagination and inventories in staging |
| Transcription provider jobs | Sarvam job identifiers retained in restricted `PROVIDER_ERASURE` records for manual confirmation; no provider deletion API is claimed | Confirm provider-side service copies and contractual residual retention |
| Guardian/consent and privacy audit | Consent decision audit rows are removed with student live-data erasure; minimal privacy status remains | Approve evidence retention and purge policy; audit records can contain personal data |
| Verification documents | Separate versioned restricted bucket | Document lifecycle/erasure automation is not yet implemented; operate a reviewed manual schedule |
| Backups/PITR/restored copies | Production DynamoDB PITR; request status distinguishes live erasure from backup expiry | Apply backup expiry, erasure ledger and restore replay before reopening restored data |
| Email/invitations | Expiring hash-based acceptance; sent email job payload cleared | Purge expired pending invitations, mail-provider copies and failed email payloads on a schedule |
| Logs | Structured technical error codes, no audio/transcript logging | Redact infrastructure/access logs, configure required residency/retention and incident holds |

Live deletion is intentionally not labelled final deletion from every copy. S3 versions, third-party copies, backup recovery points and audit evidence require separate verification. Backup lifecycle, verification-document retention and mailbox purging are documented operational dependencies, not completed background features.

## Incident procedure

Designate an incident commander, technical lead, organization contact and legal/privacy owner before launch. On detection, preserve minimal evidence with access restrictions, record awareness time in UTC, stop affected processing, revoke memberships or suspend affected organizations, rotate compromised credentials and assess scope. Avoid placing transcripts or student names in chat/ticket logs.

Legal/security staff determine notification triggers immediately using the official sources above. Prepare a preliminary report without waiting for full forensic certainty; document affected organizations, categories, timing, impact, containment and contacts. Maintain parallel CERT-In, contractual, organizational and applicable DPDP notification tracks. Notify guardians through the organization's verified channels where required, keep a decision log, issue updates, verify recovery, and run a post-incident review. This repository provides controls and a procedure, not automated regulator notification or an on-call service.

Open legal questions: publication-day calculation; transition-period obligations; organization-specific educational exemptions; acceptable guardian evidence; adult-participant workflow; minimum evidence retention versus erasure; provider training restrictions and locations; grievance deadlines and contacts; state/board-specific rules; Significant Data Fiduciary designation; international transfer restrictions; notice translations and accessibility.
