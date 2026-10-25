package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/scoring"
	"learning-setu/app/internal/store"
	"learning-setu/app/internal/transcription"
	"log"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"time"
)

type Worker struct {
	Service               *Service
	Provider              *transcription.Sarvam
	Accounts              identity.AccountInviter
	lastIntegrationStatus time.Time
}

func NewWorker(a *Service) *Worker {
	return &Worker{Service: a, Provider: transcription.New(a.Config.SarvamURL, a.Config.SarvamKey, a.Config.SarvamModel)}
}
func (w *Worker) Tick(ctx context.Context) error {
	a := w.Service
	if err := w.publishIntegrationStatus(ctx); err != nil {
		return err
	}
	items, e := a.Repo.Query(ctx, a.Config.ControlTable, "READY", true)
	if e != nil {
		return e
	}
	for _, candidate := range items {
		kind := s(candidate, "kind")
		if kind == "ACTIVATION" && !a.Config.ActivationEmailsEnabled {
			continue
		}
		infrastructureJob := contains(kind, "PROVISION", "OFFBOARD")
		if (a.Config.Mode == "provisioning" && !infrastructureJob) || (a.Config.Mode == "application" && infrastructureJob) {
			continue
		}
		j, e := a.Repo.Get(ctx, a.Config.ControlTable, "JOB", s(candidate, "sk"))
		if e != nil {
			return e
		}
		if j == nil || s(j, "gpk") != "READY" || n(j, "availableAt") > millis() || n(j, "leaseUntil") > millis() {
			continue
		}
		org, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
		if e != nil {
			return e
		}
		if org == nil || (kind != "OFFBOARD" && offboardingState(s(org, "state"))) {
			continue
		}
		if contains(kind, "ASSESS", "EMAIL") && !a.integrationsEnabled(org) {
			continue
		}
		ready := a.configuredServices()
		if kind == "EMAIL" && !b(ready, "emailConfigured") {
			continue
		}
		if kind == "ASSESS" && (!assessmentEnabled(org) || !b(ready, "assessmentConfigured")) {
			continue
		}
		lease := uuid.NewString()
		j["lease"] = lease
		j["leaseUntil"] = millis() + 600000
		j["state"] = "RUNNING"
		j["attempts"] = n(j, "attempts") + 1
		if n(j, "startedAt") == 0 {
			j["startedAt"] = millis()
		}
		if e = a.Repo.Commit(ctx, a.put(j), store.Write{Table: a.Config.ControlTable, Item: org, Check: true}); errors.Is(e, store.ErrConflict) {
			continue
		} else if e != nil {
			return e
		}
		j, e = a.Repo.Get(ctx, a.Config.ControlTable, "JOB", s(j, "sk"))
		if e != nil {
			return e
		}
		if s(j, "lease") != lease {
			continue
		}
		jobCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
		writes, pending, err := w.process(jobCtx, j)
		cancel()
		if err == nil {
			j["failureCount"] = 0
			j["leaseUntil"] = 0
			if pending {
				j["state"] = "PENDING"
				delay := 15000.0
				if a.Config.Local() {
					delay = 1500
				}
				j["availableAt"] = millis() + delay
			} else {
				j["state"] = "COMPLETED"
				j["gpk"] = "DONE"
				j["completedAt"] = now()
				if kind == "EMAIL" {
					j["data"] = Doc{}
				}
			}
			err = a.Repo.Commit(ctx, append(writes, a.put(j))...)
		}
		if err != nil {
			current, e := a.Repo.Get(ctx, a.Config.ControlTable, "JOB", s(j, "sk"))
			if e != nil {
				return e
			}
			if n(current, "rev") != n(j, "rev") {
				continue
			}
			failureCode := "PROCESSING_FAILED"
			if errors.Is(err, transcription.ErrTemporaryStorage) {
				failureCode = "TEMP_STORAGE_UNAVAILABLE"
			}
			if errors.Is(err, transcription.ErrStorageURL) {
				failureCode = "PROVIDER_STORAGE_UNSUPPORTED"
			}
			if errors.Is(err, transcription.ErrRecordingDuration) {
				failureCode = "INVALID_RECORDING_DURATION"
			}
			j["errorCode"] = failureCode
			j["failureCount"] = n(j, "failureCount") + 1
			failed := n(j, "attempts") >= 5
			if kind == "ASSESS" {
				failed = n(j, "failureCount") >= 5
			}
			if failureCode == "INVALID_RECORDING_DURATION" {
				failed = true
			}
			j["state"] = "PENDING"
			j["leaseUntil"] = 0
			j["availableAt"] = millis() + 5000
			j["error"] = "Processing failed; review service availability and authorization."
			writes = []store.Write{}
			if failed {
				j["state"] = "FAILED"
				j["gpk"] = "FAILED"
				o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
				if e != nil {
					return e
				}
				if o != nil && kind == "PROVISION" && contains(s(o, "state"), "APPROVED", "PROVISIONING") {
					o["state"] = "PROVISIONING_FAILED"
					o["error"] = "Resource setup failed; administrator retry required."
					writes = append(writes, a.put(o), a.put(audit(s(o, "id"), "worker", "PROVISIONING_FAILED", "")))
				}
				if o != nil && kind == "OFFBOARD" && s(o, "state") == "OFFBOARDING" && s(o, "offboardingJobId") == s(j, "sk") {
					o["state"] = "OFFBOARDING_FAILED"
					o["error"] = "Cleanup incomplete. Check ownership, permissions or retention locks, then retry offboarding."
					writes = append(writes, a.put(o))
				}
				if o != nil && kind == "ACTIVATION" && s(o, "state") == "ACTIVE" && s(m(o, "activationEmail"), "jobId") == s(j, "sk") {
					m(o, "activationEmail")["state"] = "FAILED"
					writes = append(writes, a.put(o))
				}
				if o != nil && kind == "ASSESS" && !offboardingState(s(o, "state")) {
					table := s(m(o, "resources"), "table")
					d, e := a.Repo.Get(ctx, table, "ASSESSMENT", s(m(j, "data"), "assessmentId"))
					if e != nil {
						return e
					}
					if d != nil && s(d, "recordingId") == s(m(j, "data"), "recordingId") && s(d, "state") != "COMPLETED" {
						d["state"] = "FAILED"
						d["error"] = "Recording could not be processed. Check authorization, audio and provider availability."
						d["errorCode"] = failureCode
						if failureCode == "INVALID_RECORDING_DURATION" {
							d["error"] = transcription.ErrRecordingDuration.Error()
						}
						if failureCode == "TEMP_STORAGE_UNAVAILABLE" {
							d["error"] = "The processing server could not prepare temporary audio storage. Your uploaded recording is preserved; contact the service operator, then retry processing."
						}
						writes = append(writes, store.Put(table, d))
					}
				}
			}
			if e = a.Repo.Commit(ctx, append(writes, a.put(j))...); e != nil && !errors.Is(e, store.ErrConflict) {
				return e
			}
			log.Printf("event=job_failed kind=%s job=%s failure_code=%s", kind, s(j, "sk"), failureCode)
		}
	}
	return nil
}
func (w *Worker) process(ctx context.Context, j Doc) ([]store.Write, bool, error) {
	switch s(j, "kind") {
	case "RESULT_CLEANUP":
		return nil, false, w.cleanupResult(ctx, j)
	case "ACTIVATION":
		x, e := w.activateAccount(ctx, j)
		return x, false, e
	case "OFFBOARD":
		return w.offboard(ctx, j)
	case "PROVISION":
		x, e := w.provision(ctx, j)
		return x, false, e
	case "ASSESS":
		return w.assess(ctx, j)
	case "EMAIL":
		a := w.Service
		org, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
		if err != nil {
			return nil, false, err
		}
		if !a.integrationsEnabled(org) || !b(a.configuredServices(), "emailConfigured") {
			return nil, true, nil
		}
		return nil, false, w.email(ctx, j)
	case "DELETE_STUDENT":
		x, e := w.erase(ctx, j)
		return x, false, e
	case "RETENTION":
		return nil, false, w.retention(ctx, s(j, "orgId"))
	}
	return nil, false, fmt.Errorf("unknown job kind")
}
func (w *Worker) provision(ctx context.Context, j Doc) ([]store.Write, error) {
	a := w.Service
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
	if e != nil {
		return nil, e
	}
	if o == nil || !contains(s(o, "state"), "APPROVED", "PROVISIONING", "ACTIVE") {
		return nil, fail(409, "Provisioning state changed")
	}
	if s(o, "state") == "ACTIVE" {
		return nil, nil
	}
	if s(o, "state") == "APPROVED" {
		o["state"] = "PROVISIONING"
		if e = a.Repo.Commit(ctx, a.put(o)); e != nil {
			return nil, e
		}
		o, e = a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(o, "id"))
		if e != nil {
			return nil, e
		}
	}
	res := m(o, "resources")
	if e = a.Repo.EnsureTable(ctx, s(res, "table"), s(o, "id")); e != nil {
		return nil, e
	}
	if !b(m(o, "progress"), "table") {
		o["progress"] = Doc{"table": true}
		if e = a.Repo.Commit(ctx, a.put(o)); e != nil {
			return nil, e
		}
		o, e = a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(o, "id"))
		if e != nil {
			return nil, e
		}
	}
	if e = a.Objects.EnsureBucket(ctx, s(res, "bucket"), s(o, "id")); e != nil {
		return nil, e
	}
	o["progress"] = Doc{"table": true, "bucket": true}
	o["state"] = "ACTIVE"
	o["activatedAt"] = now()
	delete(o, "error")
	member, e := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+s(o, "id"), s(o, "owner"))
	if e != nil {
		return nil, e
	}
	writes := []store.Write{}
	if a.Config.ActivationEmailsEnabled {
		activation := activationJob(o)
		m(o, "activationEmail")["lastRequestedAt"] = millis()
		writes = append(writes, a.put(activation))
	}
	writes = append(writes, a.put(o), a.put(audit(s(o, "id"), "worker", "ACTIVATED", "")))
	if member == nil && !b(o, "publicApplication") {
		writes = append(writes, a.put(Doc{"pk": "MEMBER#" + s(o, "id"), "sk": o["owner"], "orgId": o["id"], "email": o["email"], "role": "ORG_ADMIN", "active": true, "gpk": "USER#" + s(o, "owner"), "gsk": o["id"]}))
	}
	return writes, nil
}
func (w *Worker) assess(ctx context.Context, j Doc) ([]store.Write, bool, error) {
	a := w.Service
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
	if e != nil {
		return nil, false, e
	}
	if s(o, "state") != "ACTIVE" {
		return nil, false, fail(409, "Organization inactive")
	}
	if !a.integrationsEnabled(o) || !assessmentEnabled(o) || !b(a.configuredServices(), "assessmentConfigured") {
		return nil, true, nil
	}
	table, bucket := s(m(o, "resources"), "table"), s(m(o, "resources"), "bucket")
	d, e := a.Repo.Get(ctx, table, "ASSESSMENT", s(m(j, "data"), "assessmentId"))
	if e != nil {
		return nil, false, e
	}
	if d == nil || s(d, "orgId") != s(o, "id") || s(d, "recordingId") != s(m(j, "data"), "recordingId") || s(d, "version") == "" || s(d, "key") != "students/"+s(d, "studentId")+"/assessments/"+s(d, "sk")+"/"+s(d, "recordingId") {
		return nil, false, fail(422, "Job ownership mismatch")
	}
	if s(d, "state") == "COMPLETED" {
		return nil, false, nil
	}
	student, e := a.Repo.Get(ctx, table, "STUDENT", s(d, "studentId"))
	if e != nil {
		return nil, false, e
	}
	if e = authorized(student); e != nil {
		return nil, false, e
	}
	if millis()-n(j, "startedAt") > 30*60000 {
		return nil, false, fail(422, "Processing deadline exceeded")
	}
	save := func() error {
		if e := a.Repo.Commit(ctx, store.Write{Table: a.Config.ControlTable, Item: j, Check: true}, store.Put(table, d)); e != nil {
			return e
		}
		var e error
		d, e = a.Repo.Get(ctx, table, "ASSESSMENT", s(d, "sk"))
		return e
	}
	if s(d, "state") == "QUEUED" {
		d["state"] = "PROCESSING"
		if e = save(); e != nil {
			return nil, false, e
		}
	}
	var bytes []byte
	duration := n(d, "verifiedDuration")
	if duration == 0 || (a.Config.Provider == "sarvam" && s(d, "providerStep") == "CREATED") {
		bytes, e = a.Objects.Read(ctx, bucket, s(d, "key"), s(d, "version"))
		if e != nil {
			return nil, false, e
		}
		if len(bytes) != int(n(d, "bytes")) {
			return nil, false, fail(422, "Recording changed")
		}
		duration, e = transcription.Validate(ctx, bytes, s(d, "type"), a.Config.MaxAudioSeconds)
		if e != nil {
			return nil, false, e
		}
		d["verifiedDuration"] = duration
		if e = save(); e != nil {
			return nil, false, e
		}
	}
	if !transcription.ValidRecordingDuration(duration) {
		return nil, false, transcription.ErrRecordingDuration
	}
	transcript := s(m(d, "passage"), "text")
	if a.Config.Provider == "sarvam" {
		switch s(d, "providerStep") {
		case "":
			d["providerStep"] = "CREATING"
			if e = save(); e != nil {
				return nil, false, e
			}
			language := s(m(d, "passage"), "language")
			id, e := w.Provider.Create(ctx, language)
			if e != nil {
				return nil, false, e
			}
			d["providerJobId"] = id
			d["providerStep"] = "CREATED"
			if e = save(); e != nil {
				return nil, false, e
			}
			return nil, true, nil
		case "CREATING":
			return nil, false, fail(409, "Ambiguous provider creation requires reconciliation")
		case "CREATED":
			if e = w.Provider.Upload(ctx, s(d, "providerJobId"), s(d, "type"), bytes); e != nil {
				return nil, false, e
			}
			d["providerStep"] = "UPLOADED"
			if e = save(); e != nil {
				return nil, false, e
			}
			return nil, true, nil
		case "UPLOADED": // Starting an existing job is safe to retry; polling recovers a lost start response.
			if e = w.Provider.Start(ctx, s(d, "providerJobId")); e != nil {
				state, pollErr := w.Provider.State(ctx, s(d, "providerJobId"))
				if pollErr != nil || !contains(state, "Running", "Completed") {
					return nil, false, e
				}
			}
			d["providerStep"] = "STARTED"
			if e = save(); e != nil {
				return nil, false, e
			}
			return nil, true, nil
		case "STARTED":
			var ready bool
			transcript, ready, e = w.Provider.Poll(ctx, s(d, "providerJobId"))
			if e != nil {
				return nil, false, e
			}
			if !ready {
				return nil, true, nil
			}
		default:
			return nil, false, fmt.Errorf("invalid provider step")
		}
	}
	// Refresh authorization after all external work and fence the final result transaction.
	current, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(o, "id"))
	if e != nil {
		return nil, false, e
	}
	student, e = a.Repo.Get(ctx, table, "STUDENT", s(d, "studentId"))
	if e != nil {
		return nil, false, e
	}
	if s(current, "state") != "ACTIVE" {
		return nil, false, fail(409, "Organization inactive")
	}
	if e = authorized(student); e != nil {
		return nil, false, e
	}
	d["state"] = "COMPLETED"
	d["completedAt"] = now()
	d["result"] = Doc{"transcript": transcript, "score": scoring.Score(s(m(d, "passage"), "text"), transcript, a.Config.Provider == "mock"), "duration": duration, "wordsPerMinute": int(float64(len(scoring.Words(transcript)))*60/duration + .5), "provider": a.Config.Provider, "providerVersion": d["providerVersion"], "scoringVersion": d["scoringVersion"]}
	d, e = a.archiveResult(ctx, Tenant{Org: current, Table: table, Bucket: bucket}, d,
		store.Write{Table: a.Config.ControlTable, Item: j, Check: true},
		store.Write{Table: a.Config.ControlTable, Item: current, Check: true},
		store.Write{Table: table, Item: student, Check: true})
	if e != nil {
		return nil, false, e
	}
	return []store.Write{{Table: a.Config.ControlTable, Item: current, Check: true}, {Table: table, Item: student, Check: true}, a.resultCleanupWrite(d), store.Put(table, d)}, false, nil
}
func (w *Worker) email(ctx context.Context, j Doc) error {
	c := w.Service.Config
	email, link := s(m(j, "data"), "email"), s(m(j, "data"), "link")
	if c.SMTPHost == "" || !validEmail(email) || !validEmail(c.MailFrom) || !strings.HasPrefix(link, c.Origin+"/#invite=") || strings.ContainsAny(link, "\r\n") {
		return fmt.Errorf("invalid email configuration or job")
	}
	return w.sendMail(ctx, email, "Your Learning Setu invitation", "Sign in with this email and accept within 48 hours: "+link+"\r\nThis link can be used once.\r\n")
}
func (w *Worker) sendActivationNotice(ctx context.Context, email string) error {
	return w.sendMail(ctx, email, "Your Learning Setu organization is ready", "Your organization is approved and ACTIVE.\r\nEmail: "+email+"\r\nSign in: "+w.Service.Config.Origin+"/\r\nChoose Organization sign-in and use your existing password. Use Forgot your password? on the sign-in page if you need to reset it.\r\n")
}
func (w *Worker) sendMail(ctx context.Context, email, subject, text string) error {
	c := w.Service.Config
	if c.SMTPHost == "" || !validEmail(email) || !validEmail(c.MailFrom) || strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("invalid mail settings")
	}
	address := net.JoinHostPort(c.SMTPHost, c.SMTPPort)
	dialer := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var e error
	if c.Local() {
		conn, e = dialer.DialContext(ctx, "tcp", address)
	} else {
		conn, e = (&tls.Dialer{NetDialer: &dialer, Config: &tls.Config{ServerName: c.SMTPHost, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", address)
	}
	if e != nil {
		return e
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	client, e := smtp.NewClient(conn, c.SMTPHost)
	if e != nil {
		return e
	}
	defer client.Close()
	if c.SMTPUser != "" {
		if e = client.Auth(smtp.PlainAuth("", c.SMTPUser, c.SMTPPassword, c.SMTPHost)); e != nil {
			return e
		}
	}
	if e = client.Mail(c.MailFrom); e != nil {
		return e
	}
	if e = client.Rcpt(email); e != nil {
		return e
	}
	body, e := client.Data()
	if e != nil {
		return e
	}
	_, e = fmt.Fprintf(body, "From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", c.MailFrom, email, subject, text)
	if e != nil {
		return e
	}
	if e = body.Close(); e != nil {
		return e
	}
	return client.Quit()
}
func (w *Worker) erase(ctx context.Context, j Doc) ([]store.Write, error) {
	a := w.Service
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
	if e != nil {
		return nil, e
	}
	if len(m(o, "resources")) == 0 {
		return nil, fail(404, "Organization resources missing")
	}
	table, bucket := s(m(o, "resources"), "table"), s(m(o, "resources"), "bucket")
	id := s(m(j, "data"), "studentId")
	if !regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`).MatchString(id) {
		return nil, fail(422, "Invalid student job")
	}
	student, e := a.Repo.Get(ctx, table, "STUDENT", id)
	if e != nil {
		return nil, e
	}
	if student != nil && !b(student, "deleting") {
		return nil, fail(409, "Student not marked for deletion")
	}
	items, e := a.Repo.Query(ctx, table, "ASSESSMENT", false)
	if e != nil {
		return nil, e
	}
	providerPending := false
	for _, d := range items {
		if s(d, "studentId") != id {
			continue
		}
		if s(d, "providerJobId") != "" {
			providerPending = true
			copy := Doc{"pk": "PROVIDER_ERASURE", "sk": s(d, "providerJobId"), "orgId": o["id"], "provider": "sarvam", "state": "MANUAL_CONFIRMATION_REQUIRED", "createdAt": now()}
			old, e := a.Repo.Get(ctx, a.Config.ControlTable, "PROVIDER_ERASURE", s(copy, "sk"))
			if e != nil {
				return nil, e
			}
			if old == nil {
				if e = a.Repo.Commit(ctx, a.put(copy)); e != nil {
					return nil, e
				}
			}
		}
		if e = a.Repo.Commit(ctx, store.Write{Table: table, Item: d, Delete: true}); e != nil {
			return nil, e
		}
	}
	if e = a.Objects.DeletePrefix(ctx, bucket, "students/"+id+"/"); e != nil {
		return nil, e
	}
	events, e := a.Repo.Query(ctx, table, "AUDIT#"+id, false)
	if e != nil {
		return nil, e
	}
	for _, event := range events {
		if e = a.Repo.Commit(ctx, store.Write{Table: table, Item: event, Delete: true}); e != nil {
			return nil, e
		}
	}
	writes := []store.Write{}
	if student != nil {
		writes = append(writes, store.Write{Table: table, Item: student, Delete: true})
	}
	if rid := s(m(j, "data"), "requestId"); rid != "" {
		r, e := a.Repo.Get(ctx, table, "PRIVACY", rid)
		if e != nil {
			return nil, e
		}
		if r != nil {
			r["state"] = "LIVE_DATA_DELETED"
			r["completedAt"] = now()
			r["backupStatus"] = "EXPIRES_WITH_BACKUP_RETENTION"
			if providerPending {
				r["providerStatus"] = "MANUAL_CONFIRMATION_REQUIRED"
			}
			writes = append(writes, store.Put(table, r))
		}
	}
	writes = append(writes, a.put(audit(s(o, "id"), "worker", "STUDENT_LIVE_DATA_DELETED", "")))
	return writes, nil
}
func (w *Worker) retention(ctx context.Context, id string) error {
	a := w.Service
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if e != nil {
		return e
	}
	table := s(m(o, "resources"), "table")
	if table == "" || offboardingState(s(o, "state")) {
		return nil
	}
	items, e := a.Repo.Query(ctx, table, "STUDENT", false)
	if e != nil {
		return e
	}
	for _, d := range items {
		created, e := time.Parse(time.RFC3339Nano, s(d, "createdAt"))
		if e != nil {
			return e
		}
		deadline := float64(created.UnixMilli()) + n(o, "retentionDays")*86400000
		if n(d, "expiresAt") < deadline {
			deadline = n(d, "expiresAt")
		}
		writes := []store.Write{}
		if deadline <= millis() && !b(d, "deleting") {
			d["deleting"] = true
			d["expiresAt"] = deadline
			writes = append(writes, store.Put(table, d), a.put(job("DELETE_STUDENT", id, Doc{"studentId": d["sk"]})))
		} else if deadline < n(d, "expiresAt") {
			d["expiresAt"] = deadline
			writes = append(writes, store.Put(table, d))
		}
		if len(writes) > 0 {
			writes = append(writes, store.Write{Table: a.Config.ControlTable, Item: o, Check: true})
		}
		if e = a.Repo.Commit(ctx, writes...); e != nil {
			return e
		}
	}
	return nil
}
func (w *Worker) Sweep(ctx context.Context) error {
	a := w.Service
	items, e := a.Repo.Query(ctx, a.Config.ControlTable, "ORG", false)
	if e != nil {
		return e
	}
	for _, o := range items {
		if contains(s(o, "state"), "ACTIVE", "SUSPENDED") {
			if e = w.retention(ctx, s(o, "id")); e != nil {
				return e
			}
		}
	}
	return nil
}
