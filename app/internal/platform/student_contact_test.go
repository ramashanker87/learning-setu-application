package platform

import (
	"context"
	"learning-setu/app/internal/identity"
	"testing"
)

func TestOptionalStudentContact(t *testing.T) {
	a, _, _ := fixture(t)
	ctx := context.Background()
	user := identity.Accounts[4]
	tenant := teacherTenant(t, a, active(t, a, identity.Accounts[2]))
	for _, withContact := range []bool{false, true} {
		input := Doc{"name": "Reader", "age": 8, "className": "3", "schoolId": "test-school"}
		if withContact {
			input["email"] = " reader@example.test "
			input["mobile"] = " +91 98765 43210 "
		}
		student, err := a.CreateStudent(ctx, user, tenant, Doc{"student": input, "consent": consent()})
		must(t, err)
		saved, err := a.Student(ctx, tenant, s(student, "sk"))
		must(t, err)
		expected := ""
		if withContact {
			expected = "reader@example.test"
		}
		if s(saved, "email") != expected {
			t.Fatal("contact did not persist")
		}
		correction := Doc{"name": "Reader updated", "age": 8, "className": "3"}
		_, err = a.Privacy(ctx, user, tenant, Doc{"kind": "correct", "studentId": student["sk"], "correction": correction})
		must(t, err)
		saved, err = a.Student(ctx, tenant, s(student, "sk"))
		must(t, err)
		if s(saved, "email") != expected {
			t.Fatal("older edit removed contact")
		}
		if withContact && s(saved, "mobile") != "+91 98765 43210" {
			t.Fatal("mobile did not persist")
		}
		correction["email"] = ""
		correction["mobile"] = ""
		_, err = a.Privacy(ctx, user, tenant, Doc{"kind": "correct", "studentId": student["sk"], "correction": correction})
		must(t, err)
		saved, err = a.Student(ctx, tenant, s(student, "sk"))
		must(t, err)
		if s(saved, "email") != "" || s(saved, "mobile") != "" {
			t.Fatal("contact was not cleared")
		}
	}
	for _, contact := range []Doc{{"email": "bad-email"}, {"mobile": "call me"}, {"mobile": "123"}, {"email": 123}} {
		contact["name"] = "Reader"
		contact["age"] = 8
		contact["className"] = "3"
		contact["schoolId"] = "test-school"
		_, err := a.CreateStudent(ctx, user, tenant, Doc{"student": contact, "consent": consent()})
		check(t, err, 422)
	}
}
