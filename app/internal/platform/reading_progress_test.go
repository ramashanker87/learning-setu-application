package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"learning-setu/app/internal/store"
)

func TestReadingProgressUsesSummariesAndExcludesIneligibleRecords(t *testing.T) {
	a, r, objects, tenant, d := resultFixture(t)
	ctx := context.Background()
	d["completedAt"] = now()
	d["provider"] = "sarvam"
	d["resultStorage"] = Doc{"key": "deliberately-unavailable-object"}
	m(d, "result")["transcript"] = "PRIVATE_TRANSCRIPT"
	m(m(d, "result"), "score")["accuracy_score"] = 0
	m(d, "result")["wordsPerMinute"] = nil
	must(t, r.Commit(ctx, store.Put(tenant.Table, d)))
	for _, change := range []Doc{{"orgId": "foreign"}, {"studentId": "missing"}, {"state": "FAILED"}, {"completedAt": "invalid"}, {"completedAt": time.Now().AddDate(-2, 0, 0).Format(time.RFC3339)}, {"completedAt": time.Now().AddDate(0, 0, 2).Format(time.RFC3339)}, {"passage": Doc{"language": "hindi"}}} {
		other := store.Clone(d)
		other["sk"] = "excluded-" + hash(digest(change))[:8]
		delete(other, "rev")
		for k, v := range change {
			other[k] = v
		}
		must(t, r.Commit(ctx, store.Put(tenant.Table, other)))
	}
	result, err := a.ReadingProgress(ctx, tenant, "english", "90")
	must(t, err)
	rows := result["assessments"].([]Doc)
	if len(rows) != 1 || n(rows[0], "accuracy") != 0 || rows[0]["wpm"] != nil {
		t.Fatal("incorrect assessment selection", rows)
	}
	if objects.reads != 0 {
		t.Fatal("dashboard fetched S3 details")
	}
	encoded, _ := json.Marshal(result)
	for _, private := range []string{"PRIVATE_TRANSCRIPT", "guardianName", "resultStorage", "expected_word"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private/detail field leaked", private)
		}
	}
	student, err := r.Get(ctx, tenant.Table, "STUDENT", s(d, "studentId"))
	must(t, err)
	student["deleting"] = true
	must(t, r.Commit(ctx, store.Put(tenant.Table, student)))
	result, err = a.ReadingProgress(ctx, tenant, "english", "90")
	must(t, err)
	if len(result["assessments"].([]Doc)) != 0 || len(result["students"].([]Doc)) != 0 {
		t.Fatal("deleted student contributes to dashboard")
	}
}

func TestReadingProgressValidationAndIsolation(t *testing.T) {
	a, _, _, tenant, _ := resultFixture(t)
	ctx := context.Background()
	for _, args := range [][2]string{{"unknown", "90"}, {"english", "0"}, {"english", "9000"}} {
		_, err := a.ReadingProgress(ctx, tenant, args[0], args[1])
		check(t, err, 422)
	}
	foreign := tenant
	foreign.Org = Doc{"id": "foreign"}
	result, err := a.ReadingProgress(ctx, foreign, "english", "90")
	must(t, err)
	if len(result["students"].([]Doc)) != 0 || len(result["schools"].([]Doc)) != 0 || len(result["assessments"].([]Doc)) != 0 {
		t.Fatal("cross-tenant information leak")
	}
	if progressNumber(nil, 100) != nil || progressNumber(-1, 100) != nil || progressNumber(101, 100) != nil || progressNumber("90", 100) != nil {
		t.Fatal("invalid score accepted")
	}
}
