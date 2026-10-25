package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"testing"
)

func TestWorkerCountsFailuresRatherThanSuccessfulStageVisits(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	tenant := active(t, a, identity.Accounts[2])
	j := job("ASSESS", tenant.ID(), Doc{"assessmentId": "missing"})
	j["attempts"] = 99 // Earlier successful stage visits/polls must not exhaust error retries.
	must(t, r.Commit(ctx, a.put(j)))
	w := NewWorker(a)
	for count := 1; count <= 5; count++ {
		current, err := r.Get(ctx, "control", "JOB", s(j, "sk"))
		must(t, err)
		current["availableAt"] = 0
		must(t, r.Commit(ctx, a.put(current)))
		must(t, w.Tick(ctx))
		current, err = r.Get(ctx, "control", "JOB", s(j, "sk"))
		must(t, err)
		if n(current, "failureCount") != float64(count) {
			t.Fatal("failure count incorrect")
		}
		expected := "PENDING"
		if count == 5 {
			expected = "FAILED"
		}
		if s(current, "state") != expected {
			t.Fatalf("failure %d: expected %s, got %s", count, expected, s(current, "state"))
		}
	}
}
