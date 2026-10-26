# Learning Setu onboarding and operations guide

Use this guide to take an organization from registration to its first completed reading assessment. It covers the AWS service operator, company administrator, organization administrator, teacher and viewer. Follow the stages in order; organization approval, account access and assessment processing are separate requirements.

Updated 23 October 2026. Application: [Learning Setu staging](https://d2n7v84ca43uhr.cloudfront.net). The AWS deployment is `learning-setu-staging` in Mumbai (`ap-south-1`), accessed by operators through the `jso` AWS profile. Use the correct application URL and stack if working in another environment.

## Process and responsibilities

| Stage | Responsible person | Action | Completion check |
|---|---|---|---|
| 1 | AWS service operator | Prepare sign-in, company administrator access, transcription and email services | Company sign-in works; processing prerequisites are configured |
| 2 | Applicant | Register the organization and upload its registration document | Application is `SUBMITTED` |
| 3 | Company administrator | Review, approve and monitor resource setup | Organization is `ACTIVE` |
| 4 | Organization administrator | Activate access and sign in | Organization workspace opens |
| 5 | Company administrator | Set IntegrationsEnabled and assessment permission for the organization | Organization enabled and worker configured |
| 6 | Organization administrator | Add schools, create staff accounts and review retention | Schools appear in student forms; teachers can sign in |
| 7 | Teacher | Add students after verifying guardian authorization | Students appear in the directory |
| 8 | Teacher | Record, review and submit a reading assessment | Assessment is completed and results are visible |
| 9 | Administrators and teachers | Maintain access, student records and privacy requests | Records and access remain current |

Registration and school/student setup can proceed while platform processing is disabled. Submitting audio requires enabled organization integrations, an enabled assessment setting and a configured assessment worker.

## Login routes and roles

Open the application home page and choose the sign-in route for your account.

| Account | Login route | Main responsibility |
|---|---|---|
| Applicant | **Apply to register your organization**; no account needed to apply | Submit organization details and documents |
| Company administrator | **Company administrator sign-in** | Approve organizations, control organization processing, suspend/reactivate and offboard |
| Organization administrator (`ORG_ADMIN`) | **Organization sign-in** | Manage schools, team access and privacy settings; review student results |
| Teacher (`ASSESSOR`) | **Organization sign-in** | Add, edit and delete students; record readings and submit assessments |
| Viewer (`VIEWER`) | **Organization sign-in** | Read student profiles, results and authorized recordings |
| AWS service operator | AWS Console or CLI using the authorized AWS identity | Configure deployment, secrets, service availability and initial company access |

Students and guardians do not have login accounts in the current application. A company account does not automatically receive access to an organization's student records. AWS access is separate from company login.

### Permission matrix

Permissions below describe the application roles. Organization actions require active membership and an active organization.

| Action | Company administrator | Organization administrator | Teacher | Viewer |
|---|---|---|---|---|
| Review and approve organizations | Yes | No | No | No |
| Enable or disable organization assessment processing | Yes | No | No | No |
| Configure AWS processing services | Requires separate AWS access | No | No | No |
| Add schools | No tenant access by default | Yes | No | No |
| Create staff accounts and manage eligible roles | No tenant access by default | Yes | No | No |
| View student records, results and authorized audio | No tenant access by default | Yes | Yes | Yes |
| Add, edit or delete students | No | No | Yes | No |
| Record, submit or retry assessments | No | No | Yes | No |
| Change retention, withdraw consent, export student data and handle grievances | No tenant access by default | Yes | No | No |
| Suspend, reactivate or offboard organizations | Yes | No | No | No |

A teacher's access is scoped to the organization, not to an individually assigned school. Selecting a school is a way to organize student records; it does not create a separate school access boundary.

## Stage 1 Prepare the platform

**Owner: AWS service operator.** Complete this once per deployment, then maintain credentials and service availability as needed.

### Prepare company administrator access

1. Confirm the application URL opens and `/api/health` returns `{"status":"ok"}`.
2. In CloudFormation, open the deployment's **Outputs** to find `CompanyPoolId` and `ControlTable`.
3. Create the intended company administrator in that Cognito user pool through the invitation process. The administrator completes email verification, the first password change and required MFA enrollment.
4. An authorized operator must grant the identity company access in the control table: `pk = COMPANY`, `sk = the Cognito user's immutable sub`, `active = true` as a Boolean, and `rev = 1` as a Number for a new entry. Do not replace an existing entry. This is an operator bootstrap step, not an organization invitation.
5. Verify **Company administrator sign-in** opens **Company review**.

See the [AWS runbook](aws-runbook.md#4-bootstrap-the-company-administrator-once) for deployment and bootstrap details.

### Configure and enable platform processing

The company administrator controls `IntegrationsEnabled` per organization. AWS `DisableIntegrations` is now the default for organizations without an explicit setting, not a master switch. Explicit organization settings take precedence. Credentials are still configured once in AWS.

1. Obtain a working Sarvam API key with access to `saaras:v3` and sufficient provider credit.
2. In **Secrets Manager**, region **Mumbai (`ap-south-1`)**, edit the `SARVAM_API_KEY` field of the managed JSON secret `learning-setu/sarvam`. This deployment uses that secret when `SarvamSecretArn` is empty. Alternatively, set `SarvamSecretArn` to a separate **raw plaintext** key secret; do not use a JSON secret for that override.
3. If SMTP email invitations are needed, configure `SmtpHost`, `SmtpSecretArn` and `MailFrom` in the stack. The SMTP secret must contain JSON `username` and `password`; the deployment uses TLS on port **465**. SMTP credentials are loaded whenever its secret is configured, independent of the default integration setting.
4. Keep `DisableIntegrations=true` to leave organizations disabled until company administrators enable them individually. Setting it to `false` enables integrations by default only for organizations without explicit settings. The current template requires SMTP settings and a raw Sarvam secret ARN for that default-on deployment path.
5. Deploy parameter changes through **CloudFormation → learning-setu-staging → Update stack**, preserving the existing image and unrelated settings. Record the nonsecret configuration in the ignored `scripts/aws/parameters.local.json` as well.
6. After changing an existing secret value, restart the application worker so it loads the new credentials. Follow the [secret refresh procedure](aws-runbook.md#reloading-a-changed-secret).
7. In Company review, check the assessment/email configuration status under **Organization integrations**. The worker publishes only availability booleans, never credentials. Status expires after five minutes without a worker update. “Configured” indicates credential presence, not verified provider credit or successful delivery.
8. Enable only the intended organization and complete an authorized test assessment. This contacts the provider and can consume credits; enabling also resumes eligible queued work.

Cognito sign-in invitations remain independently controlled by `EnableActivationEmails`. Disabling organization integrations does not prevent new-account Cognito invitations or direct staff account creation. General SMTP notices and team invitations follow the organization setting. Assessment processing does not require SMTP to be configured when using explicit organization controls.

## Stage 2 Register an organization

**Owner: applicant or future organization administrator.** Prepare the representative's details, the organization registration number and a registration PDF.

1. Open the application and choose **Apply to register your organization**.
2. Choose **Start application**.
3. Enter **Contact Person Name**, **Registered address**, **Email**, **Phone** and **OrgNo**. OrgNo is the organization's registration number. Use the email of the intended organization administrator.
4. Enter **GSTIN** only if applicable. It is optional and must contain 15 alphanumeric characters when supplied.
5. Read the privacy notice and select **I accept the service terms** and **I am authorized to represent this organization**.
6. Choose **Save application**. Save the application reference and private access key confidentially. Keep the tab/session available so you can return to the application. The contact email cannot be changed after creating the public draft.
7. Choose **Continue application & upload documents**. Under **Supporting documents**, select the document type and a registration PDF of up to **5 MB**, then choose **Upload document**. Do not upload Aadhaar.
8. Save any edited details before choosing **Submit for review**.
9. Confirm the status is `SUBMITTED`. Use **Refresh application status** to follow progress.

Saving the form alone does not submit it for review. Public applications are retained in the current browser session; there is no manual reference/access-key recovery form in the current UI. If session access is lost, contact the service operator with your reference rather than creating duplicate applications.

If the company requests corrections, read its reason, reopen the application, update the details/documents, save and resubmit. Rejected applications cannot be edited through this flow.

## Stage 3 Review and approve the organization

**Owner: company administrator.** Company credentials are different from school staff credentials.

1. Use **Company administrator sign-in** and complete MFA when prompted.
2. Open **Company review**. Search by organization name, email, OrgNo or ID, or filter by status.
3. **Expand** the submitted organization. Check its representative, contact details, registration number and identifiers.
4. Choose **review** to move `SUBMITTED` to `UNDER_REVIEW`.
5. Choose **Review documents**, then open and review the uploaded registration document.
6. Choose the appropriate action:

| Action | When to use it | Result |
|---|---|---|
| **approve** | Details and documents are accepted | Queues organization resource setup |
| **changes** | Corrections or more information are needed | Requires a review reason; applicant can amend and resubmit |
| **reject** | Application is not accepted | Requires a review reason; ends this application's editable flow |

7. Choose **Refresh status** until the organization becomes `ACTIVE`. Approval is not the same as completed setup.
8. If the state becomes `PROVISIONING_FAILED`, ask the AWS operator to check the failure, enter a review reason and choose **retry**.
9. Inspect the login invitation status. If needed, use **Send/resend login invitation** or **Retry login email**. New-user invitations contain a temporary password valid for **7 days**. Existing passwords are preserved.
10. Use **Audit history** to review recorded company actions.

The normal sequence is `DRAFT → SUBMITTED → UNDER_REVIEW → APPROVED → PROVISIONING → ACTIVE`.

## Stage 4 Activate organization administrator access

**Owner: organization administrator.** Use the same email as the approved application.

1. Wait for the organization to become `ACTIVE`.
2. For a new account, check Inbox and Spam for the login invitation. Open the application, choose **Organization sign-in**, enter the invited email and temporary password, then set a new password.
3. For an existing account, use its current password. An approval or resend action does not reset that password.
4. If managed access has not yet been granted, return to the saved application and use **Sign in to activate access**, followed by **Activate administrator access** if shown. This fallback requires the matching verified identity and saved private application key.
5. Open the organization workspace. Confirm you have the administrator navigation, including **Schools** and **Team**. Use the organization selector if one is shown for your memberships.
6. Use **Refresh access** after access changes. If the organization is active but your workspace is unavailable, ask the company administrator to check activation status and the email used.

## Stage 5 Enable organization integrations and assessments

**Owner: company administrator.** Organization administrators and teachers cannot change these controls.

1. Open **Company review** and **Expand** the organization.
2. Under **Organization integrations**, read `IntegrationsEnabled: true/false` and the separate assessment/email service configuration status. An inherited value comes from the deployment default.
3. Enter **Reason for integration change** and select **Enable organization integrations** or **Disable organization integrations**. This saves a Boolean for this organization and overrides the deployment default, including when `DisableIntegrations=true` in AWS.
4. Under **Assessment processing**, keep the assessment-only setting enabled unless this organization should use email without submitting assessments. Enter **Reason for processing change** to change that setting separately.
5. Confirm the saved state. Both kinds of change record the actor, reason and previous value in audit history.

| Situation | What it means | Next action |
|---|---|---|
| Integrations enabled, assessment setting enabled, assessment worker configured | Teacher submissions are allowed subject to access and authorization | Proceed with assessments |
| Integrations disabled | New assessment submissions and general email invitations are blocked; queued work pauses | Company administrator enables organization integrations |
| Integrations enabled but assessment setting disabled | Email may operate but assessment submissions remain blocked | Enable Assessment processing if intended |
| Service not configured or worker unavailable | Credentials are missing/placeholder or worker status has expired | AWS operator checks credentials and worker health |
| Organization suspended | Tenant access is blocked regardless of integration settings | Company administrator resolves suspension separately |

Enabling resumes queued work when the relevant service is configured. Already-running requests may finish after disabling. Existing results and local playback are preserved. Organizations without an explicit integration setting inherit the AWS default; existing assessment-only preferences are also preserved. No other organization is changed when a company administrator saves a setting.

## Stage 6 Add schools and staff

**Owner: organization administrator.** Add at least one school before teachers add students.

### Add a school

1. Sign in through **Organization sign-in** and open **Schools**.
2. Enter **School name**, **Block**, **District** and **State**. All four fields are required.
3. Choose **Add school**.
4. Confirm the school appears in the table. It is now available in the **School** dropdown for students in this organization.
5. Repeat for each school. Check existing entries first to avoid duplicate school records. The current screen provides school creation and listing, not an edit/delete workflow.

### Create teacher and viewer accounts directly

This route works in the deployed Cognito application without SMTP email invitations.

1. Open **Team → Create team member**.
2. Enter **Username (email address)**.
3. Set a **Temporary password** of 14–128 characters containing uppercase and lowercase letters, a number and a symbol, with no spaces.
4. Choose **Assessor / teacher** for staff who add students and conduct readings, or **Read-only viewer** for staff who only review results.
5. Choose **Create member** and confirm the success message.
6. Share the application link, username and temporary password directly through your approved confidential channel. No email is sent by this action.
7. The staff member chooses **Organization sign-in**, signs in and changes the temporary password within **7 days**.

Directly created accounts belong to one organization and can only be teachers or viewers. Existing accounts cannot be overwritten through this form. If creation was interrupted, retry with the same username, role and original temporary password.

### Invite an additional administrator or an existing identity

When SMTP invitations are enabled, **Team** also provides **Email**, **Role** and **Send invitation**. Choose the appropriate role, including **Organization administrator** when needed. The recipient accepts the invitation while signed in with the matching verified email. The invitation is single-use and expires after **48 hours**.

The **Create team member** form cannot create an organization administrator. If SMTP invitations are disabled, ask the service operator to enable that route; do not assign a teacher account expecting administrator permissions.

### Maintain team access and retention

Use **Role → Save role** to update an eligible member, or **Revoke access** to remove access. You cannot change your own membership or the founding administrator through this screen. Directly created staff accounts cannot be promoted to organization administrator. The current screen does not provide an access-restoration button for revoked members; contact the service operator if restoration is needed.

Open **Privacy settings** and review **Retention days**. New organizations start with **90 days**. Enter a value within the displayed limit and choose **Update retention**. Shortening retention also affects existing student records; extending it does not restore expired records.

## Stage 7 Add and maintain students

**Owner: teacher using the `ASSESSOR` role.** An organization administrator can review students but cannot add them or record assessments.

1. Choose **Organization sign-in** with your teacher account. Open **Students** if the directory does not open automatically.
2. Locate **New student**. If it is missing, check your role. If no schools are available, ask the organization administrator to add one.
3. Enter **Student name**, select **Age** from **4–15**, and **Class** from **2nd–10th**. **Email address** and **Mobile number** are optional; leave them blank if unavailable. You can add, change or clear them later using **Edit student**.
4. Choose the correct **School**. The dropdown includes its block, district and state.
5. Explain the stated reading-assessment purpose and verify guardian authorization through your organization's process.
6. Enter **Guardian name** and **Relationship**. Confirm **I verified that the guardian is an adult** and **I verified guardian authority and obtained consent to the stated purpose** only after performing those checks.
7. Choose **Add student with authorization**. Confirm the student appears in the directory.
8. Use **Find a student** and **Filter by school** to find existing records before adding another student.

To correct a record, choose **Edit student**, update the details and choose **Save changes**. Existing assessments and guardian authorization are preserved. To delete a student, choose **Delete student**, read the consequences, type the student's full name exactly, and choose **Delete student permanently**. Access is removed immediately and deletion of the record, assessments and recordings is queued; this cannot be undone.

## Stage 8 Record and review an assessment

**Owner: teacher.** The organization must be active, guardian authorization current, and organization integrations and assessment processing enabled, with a configured assessment worker.

1. Find the student in **Students** and choose **Start reading assessment**.
2. Check the student's name, class and school. Select **Reading language** and **Reading passage**.
3. Find a quiet location, choose **Start recording**, and allow microphone access when the browser asks. Use the HTTPS application URL.
4. Ask the student to read at a natural pace. Choose **Stop recording** when finished. Record for at least 20 seconds and no more than 60 seconds. If recording exceeds 60 seconds, it stops and discards the audio. Choose **Record again**; **Submit assessment** stays disabled until a valid new recording is made. Recordings outside this range are rejected before transcription; record again.
5. Play the recording using **Review your recording**. Choose **Record again** if necessary.
6. Choose **Submit assessment** and wait for upload and queue confirmation. Do not leave while an unsaved recording or upload is in progress.
7. After the assessment is queued, you may return to Students while processing continues. Use **View progress and results**, **View results and history**, or the student's history to check it.
8. Review the transcript, reading accuracy, ASR-derived pronunciation indicator, words per minute and authorized audio playback. These indicators support teacher review; the pronunciation indicator is derived from speech recognition.
9. If processing fails, open the history and use **Retry processing** when offered. Persistent failures require the service operator to check provider availability or the failed job.

Assessment history cards show reading time, speed in words per minute, accuracy, a recognition-based pronunciation indicator, pause availability, and skipped/substituted/added word counts. Expand **Word review** to compare passage words with recognized words, or **Original reading passage** to read the reference. Use **Play recording** to listen alongside the transcript. Missing metrics display **Unavailable**.

If submission is disabled, you can still record and listen locally. The unsaved recording is discarded on navigation or refresh. If a submission fails, keep the page open: the recorder may retain the audio so you can retry. Once a submission is confirmed as queued, its uploaded recording is stored for processing.

## Ongoing company and privacy operations

| Task | Owner | Steps |
|---|---|---|
| Pause assessments only | Company administrator | Organization details → Assessment processing → enter reason → Disable |
| Block all organization access temporarily | Company administrator | Organization details → Review reason → **suspend** |
| Restore a suspended organization | Company administrator | Resolve the cause → Review reason → **reactivate**; check processing preference separately |
| Review student results | Organization administrator, teacher or viewer | Students → locate student → open history and authorized playback |
| Withdraw guardian consent | Organization administrator | Privacy settings → select Student → **Withdraw consent** → **Submit privacy request** |
| Export one student's data | Organization administrator | Privacy settings → select Student → **Export student data** → submit → **Download export**; this is structured JSON, not a school-wide export or audio archive |
| Handle a grievance | Organization administrator | Privacy settings → select Student → **Raise a grievance** → submit; enter Resolution and choose **Resolve grievance** when addressed |
| Permanently remove an organization | Company administrator | Organization details → Review reason → type organization ID under **Permanently offboard organization** → confirm |

Offboarding blocks access immediately and queues irreversible cleanup after an **11-minute** drain period. It deletes live organization records and dedicated storage, including students, assessments, audio, registration documents and memberships. Use **Refresh status** or **Retry offboarding** after an operator has resolved a failure. Backups, downloaded exports and provider copies require separate follow-up; shared sign-in accounts are preserved. Use suspension for a temporary interruption, not permanent offboarding.

## Troubleshooting and who to contact

| Problem | Check or next step | Owner |
|---|---|---|
| No Company review after login | Use Company administrator sign-in; verify company pool and allowlist access | AWS operator |
| Application still in draft | Upload the required PDF, save details and choose Submit for review | Applicant |
| Approved but workspace unavailable | Wait for ACTIVE; check provisioning or activation status | Company administrator |
| Login invitation missing | Check Inbox/Spam and invitation status; resend if appropriate | Company administrator |
| No Add student or Start reading assessment | These are teacher functions, not administrator/viewer functions | Organization administrator checks staff role |
| School dropdown is empty | Add schools under Schools in the same organization | Organization administrator |
| Teacher cannot sign in after account creation | Use Organization sign-in, check username and temporary-password expiry | Organization administrator; operator for account recovery |
| Organization processing is disabled | Enter a reason and enable it in Company review | Company administrator |
| Service is not configured or worker unavailable | Configure the service credentials, restart/check the application worker and refresh Company review | AWS operator |
| Microphone is blocked | Open the HTTPS site and allow microphone permission in the browser | Teacher |
| Assessment remains queued | Check organization integrations, assessment preference, worker health and provider service | Company administrator and AWS operator |
| Student has no current authorization | Do not record until authorization is resolved; contact the organization administrator/operator | Teacher |
| Role or access change is not reflected | Choose Refresh access or sign out and back in | Account holder |

## Checklist before the first school session

- [ ] Organization is ACTIVE and its administrator can sign in.
- [ ] At least one school exists in the organization.
- [ ] Teacher accounts are active and temporary passwords have been changed.
- [ ] Staff understand teacher, viewer and administrator permissions.
- [ ] Retention has been reviewed and guardian authorization procedures are ready.
- [ ] Organization integrations and assessment processing are enabled, and the assessment worker is configured.
- [ ] One authorized test student can be added and found by school.
- [ ] A 20–60 second recording uploads, processes and appears in assessment history.
- [ ] Teachers know who to contact for access, consent and processing issues.

For technical detail, see the [AWS runbook](aws-runbook.md), [operations guide](operations.md), [architecture and roles](architecture.md) and [API reference](api.md).

### Organization administrator password recovery

On the login page, choose **Forgot password? Organization administrators only**, enter the registered administrator email, then enter the emailed verification code and a new password. Passwords must contain at least 14 characters, uppercase and lowercase letters, a number and a symbol. This resets an existing account; it does not register a new user.

Recovery codes are restricted by the tenant Cognito pool to verified accounts with an active `ORG_ADMIN` membership in an active organization. Teacher, viewer and managed team accounts must contact their organization administrator. Company sign-in retains its separate recovery configuration. The form gives the same code-request message for ineligible and unrecognized emails.
