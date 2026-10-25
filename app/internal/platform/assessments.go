package platform

import (
	"context"
	"github.com/google/uuid"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"learning-setu/app/internal/transcription"
	"strings"
)

func writer(t Tenant) error {
	if !contains(s(t.Member, "role"), "ORG_ADMIN", "ASSESSOR") {
		return fail(403, "Assessor access required")
	}
	return nil
}
func assessor(t Tenant) error {
	if s(t.Member, "role") != "ASSESSOR" {
		return fail(403, "Teacher access required")
	}
	return nil
}
func admin(t Tenant) error {
	if s(t.Member, "role") != "ORG_ADMIN" {
		return fail(403, "Administrator access required")
	}
	return nil
}
func (a *Service) Student(ctx context.Context, t Tenant, id string) (Doc, error) {
	d, e := a.Repo.Get(ctx, t.Table, "STUDENT", id)
	if e != nil {
		return nil, e
	}
	if d == nil || s(d, "orgId") != t.ID() || b(d, "deleting") || n(d, "expiresAt") <= millis() {
		return nil, fail(404, "Student not found")
	}
	return d, nil
}
func (a *Service) CreateStudent(ctx context.Context, u identity.User, t Tenant, input Doc) (Doc, error) {
	if e := assessor(t); e != nil {
		return nil, e
	}
	d := m(input, "student")
	school, err := a.school(ctx, t, s(d, "schoolId"))
	if err != nil {
		return nil, err
	}
	d = store.Clone(d)
	d["schoolName"] = school["name"]
	c := m(input, "consent")
	if e := validateStudent(d); e != nil {
		return nil, e
	}
	if e := validateConsent(c); e != nil {
		return nil, e
	}
	c = store.Clone(c)
	if s(c, "verificationMethod") == "" {
		c["verificationMethod"] = "staff_declaration"
	}
	c["status"] = "GRANTED"
	c["recordedBy"] = u.Sub
	c["recordedAt"] = now()
	student := Doc{"pk": "STUDENT", "sk": uuid.NewString(), "name": d["name"], "age": d["age"], "email": strings.TrimSpace(s(d, "email")), "mobile": strings.TrimSpace(s(d, "mobile")), "className": d["className"], "schoolName": school["name"], "schoolId": school["sk"], "orgId": t.ID(), "createdAt": now(), "consent": c, "expiresAt": millis() + n(t.Org, "retentionDays")*86400000}
	event := audit(s(student, "sk"), u.Sub, "CONSENT_RECORDED", "")
	event["decision"] = c
	writes := append(a.guards(t), store.Put(t.Table, student), store.Put(t.Table, event))
	return student, a.Repo.Commit(ctx, writes...)
}
func (a *Service) Consent(ctx context.Context, u identity.User, t Tenant, id string, c Doc) (Doc, error) {
	if e := assessor(t); e != nil {
		return nil, e
	}
	if e := validateConsent(c); e != nil {
		return nil, e
	}
	d, e := a.Student(ctx, t, id)
	if e != nil {
		return nil, e
	}
	c = store.Clone(c)
	if s(c, "verificationMethod") == "" {
		c["verificationMethod"] = "staff_declaration"
	}
	c["status"] = "GRANTED"
	c["recordedBy"] = u.Sub
	c["recordedAt"] = now()
	d["consent"] = c
	event := audit(id, u.Sub, "CONSENT_RECORDED", "")
	event["decision"] = c
	return d, a.Repo.Commit(ctx, append(a.guards(t), store.Put(t.Table, d), store.Put(t.Table, event))...)
}
func (a *Service) Assessment(ctx context.Context, t Tenant, id string) (Doc, error) {
	d, e := a.assessmentRecord(ctx, t, id)
	if e != nil {
		return nil, e
	}
	return a.hydrateResult(ctx, t, d)
}
func (a *Service) assessmentRecord(ctx context.Context, t Tenant, id string) (Doc, error) {
	d, e := a.Repo.Get(ctx, t.Table, "ASSESSMENT", id)
	if e != nil {
		return nil, e
	}
	if d == nil || s(d, "orgId") != t.ID() {
		return nil, fail(404, "Assessment not found")
	}
	_, e = a.Student(ctx, t, s(d, "studentId"))
	return d, e
}
func authorized(d Doc) error {
	if s(m(d, "consent"), "status") != "GRANTED" || b(d, "deleting") || n(d, "expiresAt") <= millis() {
		return fail(409, "Current authorization is required")
	}
	return nil
}
func (a *Service) InitAssessment(ctx context.Context, u identity.User, t Tenant, input Doc) (Doc, error) {
	if e := a.requireAssessmentProcessing(ctx, t.Org); e != nil {
		return nil, e
	}
	if e := assessor(t); e != nil {
		return nil, e
	}
	student, e := a.Student(ctx, t, s(input, "studentId"))
	if e != nil {
		return nil, e
	}
	if e = authorized(student); e != nil {
		return nil, e
	}
	var passage Doc
	for _, p := range Passages {
		if s(p, "id") == s(input, "passageId") {
			passage = p
		}
	}
	if passage == nil {
		return nil, fail(422, "Choose a passage")
	}
	bytes, duration := n(input, "bytes"), n(input, "duration")
	if !transcription.ValidRecordingDuration(duration) {
		return nil, fail(422, transcription.ErrRecordingDuration.Error())
	}
	if !contains(s(input, "type"), "audio/webm", "audio/wav", "audio/ogg", "audio/mp4") || bytes <= 0 || bytes > float64(a.Config.MaxAudioBytes) || bytes != float64(int64(bytes)) || duration <= 0 || duration > float64(a.Config.MaxAudioSeconds) {
		return nil, fail(422, "Invalid audio format, size or duration")
	}
	id, recording := uuid.NewString(), uuid.NewString()
	d := Doc{"pk": "ASSESSMENT", "sk": id, "orgId": t.ID(), "studentId": student["sk"], "recordingId": recording, "key": "students/" + s(student, "sk") + "/assessments/" + id + "/" + recording, "type": input["type"], "bytes": bytes, "duration": duration, "state": "AWAITING_UPLOAD", "passage": passage, "scoringVersion": "poc-analysis-v1", "provider": a.Config.Provider, "providerVersion": a.Config.SarvamModel, "createdAt": now(), "gpk": "STUDENT#" + s(student, "sk"), "gsk": id}
	if a.Config.Provider == "mock" {
		d["providerVersion"] = "deterministic-v1"
	}
	e = a.Repo.Commit(ctx, append(a.guards(t), store.Write{Table: t.Table, Item: student, Check: true}, store.Put(t.Table, d))...)
	if e != nil {
		return nil, e
	}
	upload, e := a.Objects.Upload(ctx, t.Bucket, s(d, "key"), s(d, "type"), int64(bytes))
	d["upload"] = upload
	return d, e
}
func (a *Service) QueueAssessment(ctx context.Context, u identity.User, t Tenant, id string, retry bool) (Doc, error) {
	if e := a.requireAssessmentProcessing(ctx, t.Org); e != nil {
		return nil, e
	}
	if e := assessor(t); e != nil {
		return nil, e
	}
	d, e := a.assessmentRecord(ctx, t, id)
	if e != nil {
		return nil, e
	}
	student, e := a.Student(ctx, t, s(d, "studentId"))
	if e != nil {
		return nil, e
	}
	if e = authorized(student); e != nil {
		return nil, e
	}
	if retry {
		if s(d, "state") != "FAILED" {
			return nil, fail(409, "Only failed assessments can be retried")
		}
		if s(d, "providerStep") == "CREATING" {
			return nil, fail(409, "Provider job creation is ambiguous; administrator reconciliation required")
		}
	} else {
		if s(d, "state") != "AWAITING_UPLOAD" {
			return d, nil
		}
		obj, e := a.Objects.Inspect(ctx, t.Bucket, s(d, "key"), s(d, "type"), int64(n(d, "bytes")))
		if e != nil {
			return nil, fail(422, "Uploaded recording does not match")
		}
		d["version"] = obj.Version
		d["etag"] = obj.ETag
	}
	d["state"] = "QUEUED"
	delete(d, "error")
	delete(d, "errorCode")
	return d, a.Repo.Commit(ctx, append(a.guards(t), store.Write{Table: t.Table, Item: student, Check: true}, store.Put(t.Table, d), a.put(job("ASSESS", t.ID(), Doc{"assessmentId": id, "recordingId": d["recordingId"]})))...)
}
func (a *Service) Playback(ctx context.Context, t Tenant, id string) (Doc, error) {
	d, e := a.assessmentRecord(ctx, t, id)
	if e != nil {
		return nil, e
	}
	student, e := a.Student(ctx, t, s(d, "studentId"))
	if e != nil {
		return nil, e
	}
	if e = authorized(student); e != nil {
		return nil, e
	}
	if s(d, "version") == "" {
		return nil, fail(409, "Recording unavailable")
	}
	url, e := a.Objects.Playback(ctx, t.Bucket, s(d, "key"), s(d, "version"))
	return Doc{"url": url, "expiresInSeconds": 60}, e
}
func (a *Service) Privacy(ctx context.Context, u identity.User, t Tenant, input Doc) (Doc, error) {
	kind := s(input, "kind")
	if contains(kind, "correct", "delete") {
		if e := assessor(t); e != nil {
			return nil, e
		}
	} else if e := admin(t); e != nil {
		return nil, e
	}
	if !contains(kind, "withdraw", "delete", "export", "correct", "grievance") {
		return nil, fail(422, "Unknown request")
	}
	student, e := a.Student(ctx, t, s(input, "studentId"))
	if e != nil {
		return nil, e
	}
	r := Doc{"pk": "PRIVACY", "sk": uuid.NewString(), "orgId": t.ID(), "studentId": student["sk"], "kind": kind, "state": "OPEN", "createdAt": now(), "actor": u.Sub}
	writes := a.guards(t)
	switch kind {
	case "withdraw":
		c := m(student, "consent")
		c["status"] = "WITHDRAWN"
		c["withdrawnAt"] = now()
		student["consent"] = c
		writes = append(writes, store.Put(t.Table, student))
		r["state"] = "COMPLETED"
	case "delete":
		student["deleting"] = true
		r["state"] = "QUEUED"
		writes = append(writes, store.Put(t.Table, student), a.put(job("DELETE_STUDENT", t.ID(), Doc{"studentId": student["sk"], "requestId": r["sk"]})))
	case "correct":
		correction := m(input, "correction")
		correction = store.Clone(correction)
		if _, changing := correction["schoolId"]; changing || s(student, "schoolId") != "" {
			id := s(student, "schoolId")
			if changing {
				id = s(correction, "schoolId")
			}
			school, err := a.school(ctx, t, id)
			if err != nil {
				return nil, err
			}
			correction["schoolName"] = school["name"]
			student["schoolId"] = school["sk"]
		}
		if e = validateStudent(correction); e != nil {
			return nil, e
		}
		for _, key := range []string{"name", "className", "age"} {
			student[key] = correction[key]
		}
		for _, key := range []string{"email", "mobile"} {
			if _, present := correction[key]; present {
				student[key] = strings.TrimSpace(s(correction, key))
			}
		}
		if _, present := correction["schoolName"]; present {
			student["schoolName"] = strings.TrimSpace(s(correction, "schoolName"))
		}
		writes = append(writes, store.Put(t.Table, student))
		r["state"] = "COMPLETED"
	case "export":
		r["state"] = "COMPLETED"
	}
	writes = append(writes, store.Put(t.Table, r), store.Put(t.Table, audit(s(student, "sk"), u.Sub, "PRIVACY_"+strings.ToUpper(kind), "")))
	if e = a.Repo.Commit(ctx, writes...); e != nil {
		return nil, e
	}
	if kind == "export" {
		items, e := a.AssessmentHistory(ctx, t, s(student, "sk"))
		return Doc{"request": r, "student": student, "assessments": items}, e
	}
	return r, nil
}
func (a *Service) Retention(ctx context.Context, u identity.User, t Tenant, days float64) (Doc, error) {
	if e := admin(t); e != nil {
		return nil, e
	}
	if days < 1 || days > float64(a.Config.RetentionMaxDays) || days != float64(int(days)) {
		return nil, fail(422, "Retention outside company-approved limits")
	}
	t.Org["retentionDays"] = days
	return Doc{"retentionDays": days}, a.Repo.Commit(ctx, a.put(t.Org), store.Write{Table: a.Config.ControlTable, Item: t.Member, Check: true}, a.put(job("RETENTION", t.ID(), Doc{})), a.put(audit(t.ID(), u.Sub, "RETENTION_UPDATED", "")))
}
