# Local development and GoLand

Prerequisites: Docker Engine/Desktop with Compose v2 and working container DNS. `make up` builds the Go image, starts Moto S3, DynamoDB Local, Mailpit, runs the idempotent synthetic seed command, then starts the API, worker and nginx frontend. No paid services or production credentials are used.

| Endpoint | Purpose |
|---|---|
| http://localhost:8080 | Frontend |
| http://localhost:3000/api/health | Direct Go API health |
| http://localhost:8025 | Invitation email inbox |
| http://localhost:9000 | Local S3 API (Moto; not a bucket browser) |
| http://localhost:8100 | DynamoDB Local |
| localhost:1025 | Local SMTP |

Local sign-in provides company, applicant, Alpha/Beta administrator, teacher, viewer and invitation-recipient accounts. Local JWTs expire after one hour. `make seed` reruns synthetic seeding. `make logs` follows application logs. `make down` removes containers; emulator state is ephemeral and restarts with fresh synthetic data. Stop/restart the whole stack together rather than restarting only one emulator.

For the full workflow, sign in as Applicant, register and submit an application with a synthetic PDF, then sign in as Company administrator to review/approve it. After activation, the applicant administrator can add a school and invite `invitee@example.test` as ASSESSOR. Sign in as the invited teacher to add a student with synthetic guardian authorization, record audio and view the deterministic score. Open Mailpit to retrieve the single-use invitation link. A modern browser requires microphone permission and localhost/HTTPS; the UI surfaces denial and upload/processing failures.

## Debug in Docker

```sh
make debug
```

Open `app/` in GoLand with Go 1.26+ configured. Create **Go Remote**, host `localhost`, port `2345`, mapping local `app/` to remote `/workspace/app`. The API runs under Delve with optimizations disabled; the normal worker continues processing jobs. Re-run `make debug` after source changes to rebuild. The debugger port is bound only to loopback. Production images and CloudFormation do not include Delve or its ptrace permissions. To return to normal mode: `docker compose up -d --build api`.

## Run Go directly

Start dependencies: `docker compose up -d dynamodb s3 mailpit`. In `app/`, load `app/.env.example` as environment variables (GoLand supports an environment file, or use `set -a; . ./.env.example; set +a` in Bash). Run `go run ./cmd/setu seed`, then `go run ./cmd/setu api` and `go run ./cmd/setu worker` in separate configured terminals. Open http://localhost:3000 for the frontend served by Go. For browser audio in this mode set `APP_ORIGIN=http://localhost:3000` **before seeding** so bucket CORS matches. Install ffmpeg/ffprobe on the host for seed/worker audio validation. In GoLand a Go Build configuration targeting `./cmd/setu`, argument `api` or `worker`, supports native breakpoints without Delve Docker setup.

## Verification

`make test` runs Go unit/service tests with race detection. `make integration` checks explicit-map migration against synthetic local source data. After `make debug`, `make debug-check` verifies Delve connectivity. `make e2e` runs a Node 22+ HTTP client through the running stack, including real emulator uploads and Mailpit delivery. Neither command calls Sarvam. Sarvam protocol tests use a fake HTTP transport and check credential boundaries and signed storage URL restrictions.

Moto and DynamoDB Local do not prove AWS IAM enforcement, bucket ownership behavior, Cognito MFA, PITR/restore, quotas, regional routing, real SigV4 expiry enforcement, or production email/provider behavior. Run these checks in a separate AWS staging environment before real student data. Docker builds need access to Go modules, Docker Hub and Debian repositories; this workstation's Docker DNS required temporary build-time host mappings during verification. No such IPs are hardcoded in the project.

The school workflow browser check is `node tests/schools-browser.mjs`. It uses Chrome at `/usr/bin/google-chrome` and Playwright from `PLAYWRIGHT_MODULE` (default `/tmp/setu-browser-check/node_modules/playwright-core/index.mjs`). It verifies school creation, student association, role restrictions, empty-list guidance, and mobile layout with synthetic local data. Both this check and `tests/e2e.mjs` accept `PLATFORM_URL`; use `PLATFORM_URL=http://localhost:3000` to test the Go-served frontend directly when port 8080 is occupied.


`PLATFORM_URL=http://localhost:3000 node tests/recording-browser.mjs` checks the teacher workspace using synthetic microphone audio and local mock transcription. It covers role restrictions, existing-student selection, passage languages, recording and review, re-recording, submission retry, actual local media processing, microphone cleanup, mobile controls and automatic stop. Its S3 upload proxy accommodates the local stack's port-8080 CORS when testing the direct port-3000 frontend; production CORS remains separately configured by the deployment. Screenshots are written to `/tmp/setu-reading-desktop.png` and `/tmp/setu-reading-mobile.png`. It uses the same Chrome/Playwright prerequisites as the school browser test.


`PLATFORM_URL=http://localhost:3000 node tests/student-management-browser.mjs` verifies teacher edit/cancel/save, unchanged guardian authorization, deletion confirmation/cancel, immediate access removal, worker cleanup of synthetic student/assessment/audio data, mobile layout and viewer restrictions. It creates and deletes only local synthetic test records.

## Interface language checks

Run `node --test tests/workspace-language.test.mjs` for Hindi copy/template checks. With the static frontend served on port 4173, run `node tests/workspace-language-browser.mjs` and `node tests/reading-language-browser.mjs`. They use synthetic API responses and Chrome's fake microphone; they verify English/Hindi switching, remembered language, unchanged form/filter values, organization and company screens, verbatim English/Hindi transcripts and word differences, and switching during recording without changing the passage. `tests/reading-dashboard-browser.mjs` also verifies translated progress cards and charts. Set `PLAYWRIGHT_MODULE` to the installed Playwright module when its path differs from the test default.

Interface translations live in `frontend/workspace-hindi.js` and the landing-page catalog in `frontend/language.js`. Translation updates text nodes and display attributes without recreating views. Mark API/source content with `dataEl(...)` or `translate="no"`; for source-valued select options, use `[value, label, true]`. Form values, API payloads and result data must never be translated. New dynamic UI strings need anchored catalog templates that preserve interpolated names and source text.
