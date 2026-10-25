package platform

import (
	"context"
	"errors"
	"strings"
	"testing"

	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
)

type resultObjects struct {
	*objects
	data                         map[string][]byte
	putError, readError, corrupt bool
	reads                        int
}

func (o *resultObjects) Put(_ context.Context, bucket, key, media string, data []byte) (storage.Object, error) {
	if media != "application/json" {
		return storage.Object{}, errors.New("wrong content type")
	}
	o.data[bucket+"/"+key] = append([]byte{}, data...)
	if o.putError {
		return storage.Object{}, errors.New("lost upload response")
	}
	return storage.Object{Version: "pinned-version"}, nil
}
func (o *resultObjects) Read(_ context.Context, bucket, key, version string) ([]byte, error) {
	o.reads++
	if o.readError || version != "pinned-version" {
		return nil, errors.New("read unavailable")
	}
	if o.corrupt {
		return []byte(`{}`), nil
	}
	data, ok := o.data[bucket+"/"+key]
	if !ok {
		return nil, errors.New("object missing")
	}
	return append([]byte{}, data...), nil
}
func (o *resultObjects) DeletePrefix(ctx context.Context, bucket, prefix string) error {
	for key := range o.data {
		if strings.HasPrefix(key, bucket+"/"+prefix) {
			delete(o.data, key)
		}
	}
	return o.objects.DeletePrefix(ctx, bucket, prefix)
}

func resultFixture(t *testing.T) (*Service, *memory, *resultObjects, Tenant, Doc) {
	t.Helper()
	a, r, base := fixture(t)
	tenant := active(t, a, identity.Accounts[2])
	teacher := teacherTenant(t, a, tenant)
	student, err := a.CreateStudent(context.Background(), identity.Accounts[4], teacher, Doc{"student": Doc{"name": "Synthetic", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	must(t, err)
	o := &resultObjects{objects: base, data: map[string][]byte{}}
	a.Objects = o
	d := Doc{"pk": "ASSESSMENT", "sk": "test-assessment", "orgId": tenant.ID(), "studentId": student["sk"], "state": "COMPLETED", "gpk": "STUDENT#" + s(student, "sk"), "gsk": "test-assessment", "passage": Doc{"id": "en-1", "title": "Test", "language": "english", "text": "We read together"}, "result": Doc{"transcript": "We read", "duration": 20, "wordsPerMinute": 6, "score": Doc{"transcript": "we read", "accuracy_score": 66.7, "pronunciation_score": 66.7, "errors": []any{Doc{"error_type": "skipped", "expected_word": "together", "position": 2}}}}}
	must(t, r.Commit(context.Background(), store.Put(tenant.Table, d)))
	d, err = r.Get(context.Background(), tenant.Table, "ASSESSMENT", s(d, "sk"))
	must(t, err)
	return a, r, o, tenant, d
}

func TestResultArchiveRoundTripHistoryExportAndDeletion(t *testing.T) {
	a, r, o, tenant, original := resultFixture(t)
	ctx := context.Background()
	before := len(r.data)
	count, err := a.archiveAssessments(ctx, tenant.ID(), false)
	must(t, err)
	if count != 1 || len(o.data) != 0 || len(r.data) != before {
		t.Fatal("dry run changed storage")
	}
	inline, err := a.Assessment(ctx, tenant, s(original, "sk"))
	must(t, err)
	if digest(inline) != digest(original) || o.reads != 0 {
		t.Fatal("inline compatibility failed")
	}
	count, err = a.archiveAssessments(ctx, tenant.ID(), true)
	must(t, err)
	if count != 1 || len(o.data) != 1 {
		t.Fatal("result not archived")
	}
	saved, err := r.Get(ctx, tenant.Table, "ASSESSMENT", s(original, "sk"))
	must(t, err)
	if s(m(saved, "result"), "transcript") != "" || s(m(saved, "passage"), "text") != "" || m(m(saved, "result"), "score")["errors"] != nil || n(m(m(saved, "result"), "score"), "accuracy_score") != 66.7 {
		t.Fatal("summary contains details or lost scores")
	}
	full, err := a.Assessment(ctx, tenant, s(original, "sk"))
	must(t, err)
	if digest(m(full, "result")) != digest(m(original, "result")) || digest(m(full, "passage")) != digest(m(original, "passage")) {
		t.Fatal("detail round trip changed data")
	}
	history, err := a.AssessmentHistory(ctx, tenant, s(original, "studentId"))
	must(t, err)
	if len(history) != 1 || digest(m(history[0], "result")) != digest(m(original, "result")) {
		t.Fatal("history missing details")
	}
	exported, err := a.Privacy(ctx, identity.Accounts[2], tenant, Doc{"kind": "export", "studentId": original["studentId"]})
	must(t, err)
	if digest(m(exported["assessments"].([]Doc)[0], "result")) != digest(m(original, "result")) {
		t.Fatal("export missing details")
	}
	count, err = a.archiveAssessments(ctx, tenant.ID(), true)
	must(t, err)
	if count != 0 || len(o.data) != 1 {
		t.Fatal("migration is not resumable")
	}
	jobs, err := r.Query(ctx, a.Config.ControlTable, "JOB", false)
	must(t, err)
	for _, j := range jobs {
		if s(j, "kind") == "RESULT_CLEANUP" {
			must(t, NewWorker(a).cleanupResult(ctx, j))
		}
	}
	if len(o.data) != 1 {
		t.Fatal("cleanup deleted a committed result")
	}
	teacher := teacherTenant(t, a, tenant)
	_, err = a.Privacy(ctx, identity.Accounts[4], teacher, Doc{"kind": "delete", "studentId": original["studentId"]})
	must(t, err)
	reads := o.reads
	_, err = a.Assessment(ctx, tenant, s(original, "sk"))
	check(t, err, 404)
	if o.reads != reads {
		t.Fatal("deleted student details were read")
	}
	must(t, NewWorker(a).Tick(ctx))
	if len(o.data) != 0 {
		t.Fatal("student deletion left result objects")
	}
}

type rejectArchiveCommit struct{ store.Repository }

func (r rejectArchiveCommit) Commit(ctx context.Context, writes ...store.Write) error {
	for _, w := range writes {
		if !w.Check && s(w.Item, "pk") == "ASSESSMENT" && len(m(w.Item, "resultStorage")) != 0 {
			return store.ErrConflict
		}
	}
	return r.Repository.Commit(ctx, writes...)
}

func TestResultArchiveFailuresPreserveInlineDataAndCleanOrphans(t *testing.T) {
	for _, scenario := range []string{"upload-timeout", "read-failure", "corruption", "commit-conflict"} {
		t.Run(scenario, func(t *testing.T) {
			a, r, o, tenant, original := resultFixture(t)
			o.putError = scenario == "upload-timeout"
			o.readError = scenario == "read-failure"
			o.corrupt = scenario == "corruption"
			if scenario == "commit-conflict" {
				a.Repo = rejectArchiveCommit{r}
			}
			_, err := a.archiveAssessments(context.Background(), tenant.ID(), true)
			if err == nil {
				t.Fatal("expected failure")
			}
			saved, err := r.Get(context.Background(), tenant.Table, "ASSESSMENT", s(original, "sk"))
			must(t, err)
			if digest(saved) != digest(original) {
				t.Fatal("failed archive mutated the source")
			}
			jobs, err := r.Query(context.Background(), a.Config.ControlTable, "JOB", false)
			must(t, err)
			found := false
			for _, j := range jobs {
				if s(j, "kind") == "RESULT_CLEANUP" {
					found = true
					if n(j, "availableAt") <= millis() {
						t.Fatal("cleanup must be delayed")
					}
					must(t, NewWorker(a).cleanupResult(context.Background(), j))
				}
			}
			if !found || len(o.data) != 0 {
				t.Fatal("orphan cleanup failed")
			}
		})
	}
}

func TestResultReadsRejectForeignReferencesMissingAndCorruptObjects(t *testing.T) {
	a, r, o, tenant, original := resultFixture(t)
	ctx := context.Background()
	_, err := a.archiveAssessments(ctx, tenant.ID(), true)
	must(t, err)
	saved, err := r.Get(ctx, tenant.Table, "ASSESSMENT", s(original, "sk"))
	must(t, err)
	foreign := tenant
	foreign.Org = Doc{"id": "another-org"}
	reads := o.reads
	_, err = a.Assessment(ctx, foreign, s(original, "sk"))
	check(t, err, 404)
	if o.reads != reads {
		t.Fatal("foreign tenant accessed storage")
	}
	bad := clone(saved)
	m(bad, "resultStorage")["key"] = "students/another-student/assessments/test-assessment/results/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/result.json"
	_, err = a.hydrateResult(ctx, tenant, bad)
	check(t, err, 503)
	if o.reads != reads {
		t.Fatal("unsafe key accessed storage")
	}
	o.corrupt = true
	_, err = a.Assessment(ctx, tenant, s(saved, "sk"))
	check(t, err, 503)
	o.corrupt = false
	o.data = map[string][]byte{}
	_, err = a.AssessmentHistory(ctx, tenant, s(saved, "studentId"))
	check(t, err, 503)
}

func TestWorkerStoresCompletedResultInS3(t *testing.T) {
	a, r, o, tenant, d := resultFixture(t)
	ctx := context.Background()
	d["state"] = "QUEUED"
	delete(d, "result")
	d["recordingId"] = "recording"
	d["version"] = "audio-version"
	d["key"] = "students/" + s(d, "studentId") + "/assessments/" + s(d, "sk") + "/recording"
	d["verifiedDuration"] = 20
	j := job("ASSESS", tenant.ID(), Doc{"assessmentId": d["sk"], "recordingId": "recording"})
	must(t, r.Commit(ctx, store.Put(tenant.Table, d), a.put(j)))
	must(t, NewWorker(a).Tick(ctx))
	saved, err := r.Get(ctx, tenant.Table, "ASSESSMENT", s(d, "sk"))
	must(t, err)
	if s(saved, "state") != "COMPLETED" || len(m(saved, "resultStorage")) == 0 || len(o.data) != 1 {
		t.Fatal("worker did not archive result")
	}
	finished, err := r.Get(ctx, a.Config.ControlTable, "JOB", s(j, "sk"))
	must(t, err)
	if s(finished, "state") != "COMPLETED" {
		t.Fatal("job not completed atomically")
	}
	full, err := a.Assessment(ctx, tenant, s(d, "sk"))
	must(t, err)
	if s(m(full, "result"), "transcript") != "We read together" {
		t.Fatal("worker result missing")
	}
}

func TestResultCleanupFencesLateCommit(t *testing.T) {
	a, r, o, tenant, original := resultFixture(t)
	ctx := context.Background()
	archived, err := a.archiveResult(ctx, tenant, original)
	must(t, err)
	j, err := r.Get(ctx, a.Config.ControlTable, "JOB", s(m(archived, "resultStorage"), "cleanupJobId"))
	must(t, err)
	j["availableAt"] = 0
	must(t, r.Commit(ctx, a.put(j)))
	must(t, NewWorker(a).Tick(ctx))
	if len(o.data) != 0 {
		t.Fatal("orphan remains")
	}
	err = r.Commit(ctx, a.resultCleanupWrite(archived), store.Put(tenant.Table, archived))
	if !errors.Is(err, store.ErrConflict) {
		t.Fatal("late writer published deleted object", err)
	}
}
