# Approximate monthly costs: Sarvam and AWS

Pricing checked **21 October 2026**. These are planning estimates, not measured usage or a quotation. Sarvam and AWS are separate bills. No actual student counts, recording durations, account discounts or monthly AWS invoice were supplied.

## Sarvam rate used

The live [Sarvam API pricing page](https://www.sarvam.ai/api-pricing), under **Speech to text → Batch per minute**, lists **₹0.49 per audio minute** for pay-as-you-go (equivalent to **₹29.40 per audio hour**, roughly ₹30/hour). Its pay-as-you-go plan lists ₹0/month commitment. The same page lists batch with diarization at ₹0.59/minute, which is not used in this estimate.

The implementation currently uses `saaras:v3` and asynchronous batch jobs, without requesting diarization. English uses `language_code: en-IN` with `mode: transcribe`; Hindi uses `language_code: hi-IN` with `mode: verbatim`. English standard transcription avoids the mixed Devanagari/Latin output observed with verbatim mode and may normalize punctuation and numbers. Transcription is the paid provider operation; word alignment and reading-score calculations run in our application worker. There is no additional LLM or text-to-speech call in this workflow. See [the official batch API guide](https://docs.sarvam.ai/api/api-guides-tutorials/speech-to-text/batch-api) and [the adapter](../../app/internal/transcription/sarvam.go).

Search snippets and older Sarvam pages can show different prices. The estimates use the directly opened pricing page above. Confirm the applicable model/account rate in the Sarvam billing dashboard before purchasing credits. Paid-plan fees/benefits, enterprise discounts, taxes, minimum purchase amounts and billing-rounding rules are not assumed here. We do not interpret discounted per-minute prices as the full cost of a paid plan.

## Formula and examples

```text
Monthly audio minutes = students × assessments per student per month × average recording minutes
Estimated Sarvam usage cost (INR) = monthly audio minutes × 0.49
Planning allowance = estimated usage cost × 1.10
```

The 10% allowance is our budgeting assumption for extra recordings/reprocessing, not a Sarvam surcharge. Count all submitted audio duration, including silence; do not use the time spent in the website or the number of teacher accounts as billable audio.

Assume **four assessments per student per month**, averaging **two minutes each**:

| Active students | Assessments/month | Audio minutes/month | Audio hours/month | Sarvam estimate | With 10% allowance |
|---:|---:|---:|---:|---:|---:|
| 100 | 400 | 800 | 13.33 | ₹392 | ₹431.20 |
| 500 | 2,000 | 4,000 | 66.67 | ₹1,960 | ₹2,156 |
| 1,000 | 4,000 | 8,000 | 133.33 | ₹3,920 | ₹4,312 |
| 5,000 | 20,000 | 40,000 | 666.67 | ₹19,600 | ₹21,560 |
| 10,000 | 40,000 | 80,000 | 1,333.33 | ₹39,200 | ₹43,120 |

For **1,000 students and four assessments each**, recording duration makes this difference:

| Average recording | Audio minutes/month | Sarvam estimate |
|---:|---:|---:|
| 1 minute | 4,000 | ₹1,960 |
| 2 minutes | 8,000 | ₹3,920 |
| 3 minutes | 12,000 | ₹5,880 |
| 5 minutes | 20,000 | ₹9,800 |

One two-minute recording costs approximately **₹0.98** at the assumed rate. Ten schools with 100 active students each, four two-minute assessments per student, have the same estimated total as the 1,000-student row: **₹3,920/month**, or **₹4,312 with the allowance**, before taxes and AWS.

These estimates exclude free credits and negotiated discounts. Failed calls, retried processing and minimum billing increments may be treated differently by the provider; reconcile with actual charged minutes rather than assuming every failed request is free. The adapter persists job progress to avoid unnecessary duplicate jobs, but new recordings and intentional reprocessing can increase usage. Budget for credit purchases separately from consumption if a minimum top-up applies.

## AWS running costs are additional

The repository's default configuration keeps three Linux/x86 Fargate tasks running: the API uses **0.25 vCPU and 0.5 GB memory**, and each worker uses **0.5 vCPU and 1 GB memory**. Assuming **730 hours/month** and one task per service:

| Input to AWS Pricing Calculator | Monthly quantity / configuration |
|---|---|
| Fargate CPU | (0.25 + 0.5 + 0.5) × 730 = **912.5 vCPU-hours** |
| Fargate memory | (0.5 + 1 + 1) × 730 = **1,825 GB-hours** |
| Application Load Balancer | One × 730 hours, plus load-balancer capacity units |
| NAT gateway, supplied network | One × 730 hours, plus processed traffic and applicable public IPv4 charges |
| CloudFront | HTTPS requests, traffic and applicable plan/usage charges |
| DynamoDB | Shared control table plus one tenant table per organization; reads/writes, storage and PITR |
| S3 | Documents, recordings, versions, requests and transfer |
| Cognito | Monthly active users and applicable identity feature tier |
| Secrets Manager | Sarvam secret and SMTP secret, plus API requests |
| CloudWatch / ECR | Log ingestion/retention and stored images |
| SMTP provider / backups | Provider-specific charges and additional backup resources |

These quantities are calculated from `infrastructure/template.yml` and `infrastructure/network.yml`, not from a live billing export. Deployment overlap, replacement tasks, extra replicas or longer months increase compute usage. Network/data resources also incur charges while application usage is low.

Use the [AWS Pricing Calculator](https://calculator.aws/) with **Asia Pacific (Mumbai), ap-south-1**, Linux/x86, On-Demand and the quantities above. Use current regional prices from [Fargate pricing](https://aws.amazon.com/fargate/pricing/) and [VPC/NAT pricing](https://aws.amazon.com/vpc/pricing/); do not substitute US-region example rates. AWS Fargate charges depend on configured CPU/memory and running duration; NAT adds time and data-processing charges. No fixed AWS rupee total is stated because traffic, stored audio, region-specific rates, exchange rate, account benefits and taxes must be selected explicitly.

```text
Monthly system budget = AWS estimate + Sarvam audio usage + SMTP/other provider charges + applicable taxes
Example for 1,000 students = AWS estimate + ₹3,920 Sarvam usage + other charges/taxes
With the transcription allowance = AWS estimate + ₹4,312 + other charges/taxes
```

## Refine the estimate after launch

At month end, record active students, assessment count, total submitted audio minutes, repeat recordings, actual Sarvam charges and AWS Cost Explorer totals. Compare actual duration with the two-minute assumption and adjust the next month's forecast. Keep AWS service costs separate so idle infrastructure is visible. Set account spending alerts and monitor Sarvam credit balance before peak assessment periods.

Recording duration is bounded by application configuration (default maximum 300 seconds), but the scenario tables use averages. More schools increase organization storage/administration; transcription cost grows with total audio submitted across all organizations, not school count alone. While integrations remain disabled and no transcription requests are made, this application generates no Sarvam audio usage; AWS resources still run.
