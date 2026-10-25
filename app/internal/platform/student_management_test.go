package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"testing"
)

func TestTeacherStudentCorrectionAndDeletion(t *testing.T) {
	a, repo, _ := fixture(t)
	ctx := context.Background()
	adminUser, teacherUser := identity.Accounts[2], identity.Accounts[4]
	tenant := active(t, a, adminUser)
	teacher := teacherTenant(t, a, tenant)
	studentInput := Doc{"student": Doc{"name": "Original", "age": 8, "className": "3", "schoolId": "test-school"}, "consent": consent()}
	student, err := a.CreateStudent(ctx, identity.Accounts[4], teacher, studentInput)
	must(t, err)
	school, err := a.CreateSchool(ctx, adminUser, tenant, Doc{"name": "Updated School", "block": "B", "district": "D", "state": "S"})
	must(t, err)
	correction := Doc{"name": "Updated", "age": 9, "className": "4", "schoolId": school["sk"], "schoolName": "Forged"}
	request := Doc{"kind": "correct", "studentId": student["sk"], "correction": correction}
	for _, kind := range []string{"export", "withdraw", "grievance"} {
		_, err = a.Privacy(ctx, teacherUser, teacher, Doc{"kind": kind, "studentId": student["sk"]})
		check(t, err, 403)
	}
	viewer := teacher
	viewer.Member = store.Clone(teacher.Member)
	viewer.Member["role"] = "VIEWER"
	for _, kind := range []string{"correct", "delete"} {
		_, err = a.Privacy(ctx, teacherUser, viewer, Doc{"kind": kind, "studentId": student["sk"], "correction": correction})
		check(t, err, 403)
	}
	foreign := active(t, a, identity.Accounts[3])
	foreignSchool, err := a.CreateSchool(ctx, identity.Accounts[3], foreign, Doc{"name": "Foreign", "block": "B", "district": "D", "state": "S"})
	must(t, err)
	correction["schoolId"] = foreignSchool["sk"]
	_, err = a.Privacy(ctx, teacherUser, teacher, request)
	check(t, err, 422)
	correction["schoolId"] = school["sk"]
	correction["age"] = 99
	_, err = a.Privacy(ctx, teacherUser, teacher, request)
	check(t, err, 422)
	correction["age"] = 9
	result, err := a.Privacy(ctx, teacherUser, teacher, request)
	must(t, err)
	if s(result, "actor") != teacherUser.Sub || s(result, "state") != "COMPLETED" {
		t.Fatal("correction not attributed/completed")
	}
	saved, err := a.Student(ctx, tenant, s(student, "sk"))
	must(t, err)
	if s(saved, "name") != "Updated" || n(saved, "age") != 9 || s(saved, "className") != "4" || s(saved, "schoolName") != "Updated School" {
		t.Fatal("correction not applied authoritatively")
	}
	if s(m(saved, "consent"), "recordedBy") != teacherUser.Sub || s(m(saved, "consent"), "status") != "GRANTED" {
		t.Fatal("correction altered consent")
	}
	other, err := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, foreign), studentInput)
	must(t, err)
	for _, kind := range []string{"correct", "delete"} {
		_, err = a.Privacy(ctx, teacherUser, teacher, Doc{"kind": kind, "studentId": other["sk"], "correction": correction})
		check(t, err, 404)
	}
	result, err = a.Privacy(ctx, teacherUser, teacher, Doc{"kind": "delete", "studentId": student["sk"]})
	must(t, err)
	if s(result, "state") != "QUEUED" || s(result, "actor") != teacherUser.Sub {
		t.Fatal("deletion not queued/attributed")
	}
	_, err = a.Student(ctx, teacher, s(student, "sk"))
	check(t, err, 404)
	jobs, err := repo.Query(ctx, a.Config.ControlTable, "JOB", false)
	must(t, err)
	count := 0
	for _, j := range jobs {
		if s(j, "kind") == "DELETE_STUDENT" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("missing deletion job", count)
	}
	_, err = a.Privacy(ctx, teacherUser, teacher, Doc{"kind": "delete", "studentId": student["sk"]})
	check(t, err, 404)
	audits, err := repo.Query(ctx, tenant.Table, "AUDIT#"+s(student, "sk"), false)
	must(t, err)
	if len(audits) < 3 {
		t.Fatal("missing consent/correction/deletion audit records")
	}
}

func TestRevokedTeacherCannotModifyStudentWithStaleMembership(t *testing.T) {
	a, repo, _ := fixture(t)
	ctx := context.Background()
	tenant := active(t, a, identity.Accounts[2])
	teacher := teacherTenant(t, a, tenant)
	student, err := a.CreateStudent(ctx, identity.Accounts[4], teacher, Doc{"student": Doc{"name": "Reader", "age": 8, "className": "3", "schoolId": "test-school"}, "consent": consent()})
	must(t, err)
	member := store.Clone(teacher.Member)
	member["active"] = false
	must(t, repo.Commit(ctx, a.put(member)))
	_, err = a.CreateStudent(ctx, identity.Accounts[4], teacher, Doc{"student": Doc{"name": "Denied reader", "age": 8, "className": "3", "schoolId": "test-school"}, "consent": consent()})
	if err != store.ErrConflict {
		t.Fatalf("revoked teacher created student: %v", err)
	}
	for _, kind := range []string{"correct", "delete"} {
		_, err = a.Privacy(ctx, identity.Accounts[4], teacher, Doc{"kind": kind, "studentId": student["sk"], "correction": Doc{"name": "Changed", "age": 9, "className": "4"}})
		if err != store.ErrConflict {
			t.Fatalf("stale teacher write: %v", err)
		}
	}
	saved, err := a.Student(ctx, tenant, s(student, "sk"))
	must(t, err)
	if s(saved, "name") != "Reader" {
		t.Fatal("stale correction persisted")
	}
}
