package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"testing"
)

func TestAssessmentDurationGate(t *testing.T) {
	a, _, _ := fixture(t)
	ctx := context.Background()
	tenant := teacherTenant(t, a, active(t, a, identity.Accounts[2]))
	student, err := a.CreateStudent(ctx, identity.Accounts[4], tenant, Doc{"student": Doc{"name": "Test", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	must(t, err)
	for _, duration := range []float64{0, 19.9, 20, 60, 60.1, 300} {
		_, err := a.InitAssessment(ctx, identity.Accounts[4], tenant, Doc{"studentId": student["sk"], "passageId": "en-1", "type": "audio/wav", "bytes": 32044, "duration": duration})
		if duration < 20 || duration > 60 {
			check(t, err, 422)
		} else {
			must(t, err)
		}
	}
}
