# Deployment and operations

Start with the [AWS runbook](aws-runbook.md) for an end-to-end setup sequence and [monthly costs](monthly-costs.md) for Sarvam estimates and AWS budgeting inputs.

## Moving existing assessment results to S3

Newly completed assessments automatically store full results and passage snapshots in the existing private tenant bucket. DynamoDB retains processing state, summary metrics, history indexes and the pinned S3 reference. No additional bucket or CloudFormation parameter is required; the existing application/API S3 permissions cover result objects.

For older inline results, run this command in a configured application environment using its normal AWS identity and environment variables:

```sh
setu archive-assessments --org ORGANIZATION_ID
setu archive-assessments --org ORGANIZATION_ID --execute
```

The first command is a read-only dry run reporting the eligible count. The second migrates completed results for that organization only. Each result is uploaded, read back using its exact version, verified byte-for-byte, and then replaced by a small DynamoDB summary in a conditional transaction with an audit event. It never reruns transcription or changes scores. Reruns skip already archived results; deleting/expired students and unfinished assessments are skipped. Concurrent changes abort that record without discarding the inline source. Each record has a two-minute deadline. Run organizations individually and keep the worker running for delayed cleanup of abandoned uploads.

Deploy the compatible API and workers before backfilling. Once results are archived, rollback must use a release that understands `resultStorage`; older releases would show incomplete results. Restore both DynamoDB references and their pinned S3 versions together during disaster recovery. Do not add a lifecycle policy that deletes result objects while their assessment records are retained. Student deletion/retention and organization offboarding already remove result objects and all their versions through the existing prefixes/bucket cleanup.

Result uploads register `RESULT_CLEANUP` jobs due after 15 minutes. Successful result commits cancel those jobs atomically; failed or uncertain commits leave them available to the application worker. Inspect FAILED cleanup jobs and restore storage permissions before requeuing them. A cleanup claim fences late writes so an expired writer cannot publish a deleted object. The existing five-attempt worker failure limit applies. History/export load full details from S3, so account for these GET requests when estimating savings.

## Infrastructure

`infrastructure/template.yml` is CloudFormation, validated with cfn-lint. It creates a retained, encrypted control table with PITR and deletion protection, retained private versioned document storage with TLS-only policy, separate tenant/company Cognito pools, an ALB, ECS API/application-worker/provisioning-worker services, log storage and separate task roles. Custom-domain mode uses a public HTTPS ALB; temporary CloudFront mode uses a private ALB and an AWS-provided HTTPS address. The API image also serves `frontend/` for production. The runtime image contains the Go executable, frontend files and ffprobe; it needs neither Node nor Python.

Deploy explicitly to `ap-south-1` unless a reviewed environment configuration selects another region. Supply separate staging/production AWS accounts or strictly separated prefixes, VPC/subnet sets, certificates, Cognito domains, tables, buckets, secrets and DNS. Private ECS subnets need NAT or suitable service endpoints and outbound access to required identity/email APIs and Sarvam/provider storage. The template accepts existing networking and certificate resources; it does not create your DNS zone or certificate.

```sh
cp scripts/aws/parameters.example.json scripts/aws/parameters.local.json
# Fill the reviewed nonsecret parameter values, then explicitly deploy:
AWS_PROFILE=jso AWS_REGION=ap-south-1 ./scripts/aws/deploy.sh staging scripts/aws/parameters.local.json
```

This command is a deployment example, not a deployment performed during implementation. Configure DNS to the emitted ALB name. Provision and verify the initial company identity using Cognito's account-activation process, enroll MFA, and write a controlled `COMPANY/<subject>` active record. Do not expose a public company-role-grant endpoint. Record the bootstrap approver and audit event through the operator change process. Cognito supports tenant MFA; validate enrollment/recovery UX with the selected managed-login configuration. The application never handles plaintext invitation passwords; Cognito can send them through the managed activation flow described below.

## Temporary HTTPS access without a domain

Set `AccessMode=cloudfront` to deploy an AWS-provided `https://….cloudfront.net` address. No custom domain or ACM certificate is required. CloudFormation uses this address for all three services' `APP_ORIGIN`, both Cognito clients' callback/logout URLs, and document-bucket CORS. Newly provisioned tenant buckets also use it. CloudFront forwards all viewer headers (including `Authorization` and `X-Organization-Id`), cookies and query strings, accepts every application HTTP method, and disables caching, including error caching.

This mode creates an internal ALB in `PrivateSubnets` and connects CloudFront through a VPC origin. The ALB's HTTP listener permits only the regional CloudFront origin-facing managed prefix list; there is no public HTTP application endpoint. The application continues to enforce an HTTPS origin and real Cognito authentication. [Cognito requires HTTPS callbacks](https://docs.aws.amazon.com/cognito/latest/developerguide/authorization-endpoint.html), and [browser microphone access requires a secure context](https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getUserMedia).

The VPC must have an internet gateway, available private subnet IPs, and outbound connectivity for ECS image pulls and external services. The deployer needs permission to create CloudFront distributions, VPC origins, and their service-linked role, in addition to the existing stack permissions. See [AWS VPC origin prerequisites](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-vpc-origins.html).

If you have no suitable network, the optional staging template creates an isolated VPC, two public and two private subnets, and one NAT gateway:

```sh
aws --profile jso --region ap-south-1 cloudformation deploy \
  --stack-name learning-setu-staging-network \
  --template-file infrastructure/network.yml \
  --parameter-overrides Prefix=setu-staging
aws --profile jso --region ap-south-1 cloudformation describe-stacks \
  --stack-name learning-setu-staging-network --query 'Stacks[0].Outputs'
```

Use its `VpcId`, `PublicSubnets`, and `PrivateSubnets` outputs in your application parameters. The single NAT gateway has ongoing charges and is a staging availability tradeoff; production networking should provide the required zone redundancy.

```sh
cp scripts/aws/parameters.cloudfront.example.json scripts/aws/parameters.local.json
aws --profile jso --region ap-south-1 ec2 describe-managed-prefix-lists \
  --filters Name=prefix-list-name,Values=com.amazonaws.global.cloudfront.origin-facing \
  --query 'PrefixLists[0].PrefixListId' --output text
# Fill CloudFrontPrefixListId, subnet/VPC IDs, SMTP settings and secret ARNs.
# Leave AppOrigin and CertificateArn omitted.
./scripts/aws/deploy.sh staging scripts/aws/parameters.local.json
```

Open the application's `ApplicationUrl` stack output after deployment finishes. Verify `/api/health`, both sign-in flows, organization registration/document upload, and microphone recording. Initial company-admin bootstrap still requires the controlled process described above.

To launch before SMTP and Sarvam credentials are available, set `"DisableIntegrations": "true"` in the parameter file and omit or leave empty `SmtpHost`, `MailFrom`, `SmtpSecretArn`, and `SarvamSecretArn`. The application worker receives the Sarvam key, but SMTP secrets are not injected or granted in this mode. Organization onboarding/provisioning, student management and privacy jobs remain available; invitation creation and assessment uploads/retries return an unavailable response. Teachers can record and preview audio locally, but submission is disabled; invitation creation and processing-retry controls remain hidden. Existing email/transcription jobs are left queued without calling providers. Cognito's own account verification and recovery email is separate from application invitation email.

By default the template creates the retained Secrets Manager secret `learning-setu/sarvam` with `{ "SARVAM_API_KEY": "dummy" }`. In the AWS console, replace only the `SARVAM_API_KEY` value with your actual key. The application worker extracts that JSON field into its environment; the provider client uses the `api-subscription-key` header and rejects the placeholder locally. The key is not sent to the browser or to provider storage upload/download URLs. `SarvamSecretArn` may optionally select an existing raw-key secret instead of creating the managed secret. The fixed managed name supports one owning stack per account/region; other stacks must use an external secret.

After editing the key, force a new deployment of the ECS application worker so new tasks load the current secret. Updating Secrets Manager alone does not refresh running task environments. The retained secret is not deleted with the stack; an unchanged `SecretString` template property does not reset a console-updated value on ordinary stack updates.

To enable the integrations later, update the Sarvam key, supply `SmtpHost`, `MailFrom`, and `SmtpSecretArn`, and set `"DisableIntegrations": "false"`, then redeploy. The default is `false`, preserving existing deployments' behavior. Old queued jobs resume under their existing retry/deadline rules.

### Switching to your domain later

Set `AccessMode=custom-domain`, provide `AppOrigin=https://your-domain` and a matching regional ACM `CertificateArn`, and redeploy the same application stack. Changing from an internal to an internet-facing ALB replaces the ALB and removes the temporary distribution; plan a maintenance window. Point DNS to the new `LoadBalancerDns` output. Organization records, tenant buckets/tables, and identity pools remain in place.

The stack updates document CORS and login callbacks automatically. **Existing dynamically provisioned tenant buckets need their CORS AllowedOrigins updated to the new AppOrigin separately before recording/upload testing**; they are not managed by CloudFormation. Preserve the existing CORS methods and headers. Already-sent invitations contain the old host: reissue pending invitations after the switch. Browser sessions use a different origin, so users must sign in again.

## Permissions and configuration

API tasks have data access but no bucket/table creation/deletion permissions. Application workers have tenant data/erasure/transcription permissions but no infrastructure creation/deletion permissions. Provisioning workers handle `PROVISION` and `OFFBOARD` jobs: they have scoped resource creation/deletion permissions, tenant-table queries for provider-erasure references, audio-version/multipart deletion, and deletion restricted to organization document prefixes. They cannot read audio/document contents. Control-table deletion is explicitly denied. A production worker must declare `WORKER_MODE=application` or `provisioning`; the `all` mode is local-only. The shared control table necessarily contains security-sensitive membership and job records: restrict operator/backup access and monitor changes independently.

Dynamic tenant resources are scoped to the environment prefix by IAM and verified against live registry/membership in code. This is application-enforced tenant isolation; it does not create a per-tenant runtime IAM role. Consider per-tenant assumed roles/session policies if your threat model requires protection against a compromised application process.

Secrets come from task roles/Secrets Manager, never source files. The SMTP secret is JSON with `username` and `password`, delivered only to the application worker. The managed Sarvam secret contains JSON with `SARVAM_API_KEY`; ECS extracts that field only for the application worker. An optional external `SarvamSecretArn` instead expects a raw-key secret. Production SMTP uses TLS on port 465. Set company/tenant Cognito issuers, clients and domains from stack outputs/resources. Configure `APP_ORIGIN` exactly; CORS and hosted-login callbacks must match it. Production refuses emulator endpoints and local/mock identity/provider modes.

Tenant buckets receive versioning, AES256 server-side encryption, public-access blocking, exact-origin CORS and a deny-insecure-transport policy. Tenant DynamoDB tables receive consistent pk/sk/index/schema and PITR. Resource names are persisted before external side effects. Never rename buckets/tables on organization display-name changes.

## Provisioning and quotas

Before creation, the provisioner counts existing account buckets/tables and compares them with configured `BUCKET_QUOTA` and `TABLE_QUOTA`, reserving five slots. Defaults are operational budgets, not an authoritative account quota lookup. Read current Service Quotas in the target account/region and set these values accordingly. Concurrent creations can still consume headroom; handle AWS quota errors through retries and operator review.

Dedicated resources impose cost and operational growth per organization: IAM scope, bucket/table quotas, backups, logs, inventories, resource-policy updates and schema migration all grow with tenant count. Track approved/unprovisioned organizations, provisioning latency, failed jobs and resource inventories. Do not assume one stack can grow without quota increases. Reconcile control-plane resource mappings against AWS tags periodically.

Retry provisioning from Company review after fixing quota, permission, naming or service errors. The same physical names are reused and policy steps reapplied. No rollback deletes data. A bucket that exists without its expected ownership tag is refused: inspect CloudTrail and creation identity before manually repairing the tag. A conflicting ownership tag or schema must never be overwritten to make the retry pass.

Central definitions are `internal/store.Dynamo.EnsureTable`, `internal/storage.S3.EnsureBucket`, `onboarding-policies.json` and persisted `schemaVersion`. Future resource upgrades need a registry-driven migration job with version preconditions and checkpoints. The repository does not yet implement a fleet-wide schema/policy migration orchestrator.

## Monitoring, backups and restoration

Application logs emit job IDs, types and generic error categories; do not enable SDK request/body logging. Add CloudWatch alarms for API errors, ALB health, failed jobs, expired leases, oldest READY job, DynamoDB throttling, backup health and erasure latency. The template creates log storage but not a complete alert/on-call integration. Failed email/deletion jobs currently require operator investigation and controlled requeue; assessment and provisioning retries have user-facing APIs.

PITR is enabled in AWS, not emulated locally. Adopt a separate S3 backup/recovery plan for recordings/documents and test it; versioning alone is not an independent backup. Define RPO/RTO, backup-region residency, retention and incident/legal holds. Do not promise timely complete erasure until backup and provider-copy procedures are approved.

Restore into **new** resource names. Validate table keys/indexes, ownership tags, bucket policies and counts; replay the erasure ledger before access is enabled. Update the resource registry under revision control during a maintenance window and retain the old mapping for rollback. Never silently recreate a production resource with an old name. Validate recovery with representative synthetic records before a real incident.

## Offboarding

Company administrators can use **Company review → Permanently offboard organization**. Enter a reason, type the exact organization UUID, and confirm the irreversible action. Use this only when deletion is intended; ordinary suspension remains reversible.

Access is blocked immediately. An 11-minute drain period lets issued upload/playback URLs and bounded worker calls expire. New job claims check the organization revision, and ordinary jobs stop being claimed for offboarding organizations. The provisioning worker then checks persisted resource names and ownership tags, captures provider-erasure references, removes all audio versions/delete markers and incomplete uploads, deletes the tenant bucket and organization document versions, disables tenant PITR, deletes the tenant table, and erases memberships, invitations, application documents, previous jobs and audits. Other organizations and shared Cognito accounts remain intact.

**Refresh status** shows the cleanup phase. Transient failures retry automatically; after five failed attempts the organization becomes `OFFBOARDING_FAILED`. Investigate IAM permissions, ownership tags, deletion protection or object locks, then use **Retry offboarding** with the same UUID confirmation. Do not bypass ownership checks or retention locks to force completion. Checkpoints and missing-resource handling let retries resume after partial deletion. Offboarded organizations cannot be reactivated.

Completion retains only an organization UUID/status/timestamp receipt, the completed cleanup job, and a minimal completion audit identifying the requesting company administrator. The receipt deliberately says `LIVE_DATA_DELETED`, not universal erasure. Provider IDs or ambiguous provider-operation references are retained in `PROVIDER_ERASURE` for separate confirmation. Already sent email, exports/downloads, separate AWS Backup/on-demand backups and shared-control-table PITR cannot be removed by this tenant cleanup. Apply their retention/erasure processes separately. Disabling tenant PITR before table deletion avoids creating a new deletion-time system backup; existing separate backups still need review.

## Sarvam recovery

The worker persists `providerStep` and `providerJobId` before progressing from create to upload, start and polling. If a create response is lost, `CREATING` is intentionally not automatically retried: reconcile it with Sarvam support/dashboard, then use a reviewed conditional operator update to attach the existing job ID and `CREATED` step. Only reset to an empty step after confirming no job was created. Do not send audio twice to resolve uncertainty. No administrative reconciliation UI or provider deletion endpoint is supplied; provider erasure requires recorded confirmation. See the [official batch API](https://docs.sarvam.ai/api-reference/speech-to-text/stt/job/initiate) and [result download contract](https://docs.sarvam.ai/api-reference/speech-to-text/stt/job/download).

Batch transcripts do not supply word-level confidence/timing in this adapter. Accuracy uses the original word alignment; pronunciation is an ASR match indicator, not phonetic grading. Pause count is `null` when unavailable. Assessments record passage, provider/model and scoring versions.


### Public organization applications

The home-page application link opens `/#apply` without sign-in. Applicants must save their reference and access key; the key is stored only as a hash in the control row and is not recoverable. Do not ask applicants to send keys through public channels. Public contact emails are unverified during review. Approval provisions storage but does not grant workspace access until the applicant claims the ACTIVE application with both its key and a verified tenant sign-in matching the immutable contact email. Claim consumes the key. Company identities cannot claim applications. Public keys are rejected immediately during offboarding.

A shared `PUBLIC_RATE / APPLICATIONS` counter limits creation to 10 per minute across replicas. Each public application allows eight document initializations, with a revision-checked counter. These baseline limits are not a substitute for edge abuse protection at larger scale. General SMTP notifications are disabled with `DisableIntegrations=true`; Cognito activation invitations have their own `EnableActivationEmails` switch. There is no unauthenticated email lookup/reset. If a key is lost, the operator must establish ownership separately; offboard and reapply if needed rather than bypass verified activation.


### Activation invitations

Enable with CloudFormation `EnableActivationEmails=true` / `ENABLE_ACTIVATION_EMAILS=true`. This is independent of the SMTP/transcription switch. The application worker uses `cognito-idp:AdminGetUser` and `AdminCreateUser`, scoped to the tenant pool, for activation invitations. The API also has these two actions for organization-scoped team account creation. The company pool and existing confirmed passwords are never modified. Cognito's configured email delivery and service quotas apply; current staging uses `COGNITO_DEFAULT`. New users receive an expiring password and must change it before receiving sign-in tokens. New-account email attributes are marked verified during managed invitation; only possession of Cognito's delivered password allows first sign-in. Existing unverified accounts are never marked verified by this workflow.

After ACTIVE, inspect Company review for invitation progress. On FAILED, check Cognito account state, email delivery availability/quotas and task IAM, then use Retry login email. Resending for FORCE_CHANGE_PASSWORD generates a new temporary password and invalidates the older one; confirmed users keep their password. External email delivery is at least once: a failure between Cognito acceptance and the state commit may cause a retry/resend, and the latest temporary password should be used. Ordinary SMTP approval notices for existing accounts require SMTP setup; the UI reports explicitly when those notices are disabled. Offboarding still blocks job execution and drains in-flight calls; it preserves shared Cognito accounts.


### Organization sign-in on shared browsers

Organization sign-in uses Cognito's `/logout` redirect to its `/login` page with a fresh OAuth state and PKCE challenge. This clears the previous hosted session and presents username/email and password for an organization administrator or team member. The tenant pool handles both account types; membership determines their role after sign-in. Company sign-in keeps its separate pool and existing authorization flow. Browser password-manager autofill may still populate fields, but the app does not silently reuse the previous Cognito session.

Run `PLATFORM_URL=https://your-deployed-origin node tests/organization-signin-browser.mjs` for a read-only check of repeated hosted-login navigation, visible username/password fields, and preservation of state and PKCE. Set `TEST_WORKSPACE=1` to test the workspace API module against the deployed provider before deployment. The check never submits credentials.
