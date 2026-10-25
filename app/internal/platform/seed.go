package platform

import (
	"context"
	"encoding/binary"
	"fmt"
	"learning-setu/app/internal/identity"
)

func (a *Service) Seed(ctx context.Context) error {
	if a.Config.Env != "local" || a.Config.AuthMode != "local" || a.Config.Provider != "mock" {
		return fmt.Errorf("synthetic seeding requires local authentication and mock transcription")
	}
	if e := a.Repo.EnsureTable(ctx, a.Config.ControlTable, "control-plane"); e != nil {
		return e
	}
	if e := a.Objects.EnsureBucket(ctx, a.Config.DocumentBucket, "control-plane"); e != nil {
		return e
	}
	company := identity.Accounts[0]
	old, e := a.Repo.Get(ctx, a.Config.ControlTable, "COMPANY", company.Sub)
	if e != nil {
		return e
	}
	if old == nil {
		if e = a.Repo.Commit(ctx, a.put(Doc{"pk": "COMPANY", "sk": company.Sub, "active": true})); e != nil {
			return e
		}
	}
	w := NewWorker(a)
	for _, user := range identity.Accounts[2:4] {
		name := "Alpha School"
		if user.Sub == "beta-admin" {
			name = "Beta School"
		}
		items, e := a.Repo.Query(ctx, a.Config.ControlTable, "OWNER#"+user.Sub, true)
		if e != nil {
			return e
		}
		var o Doc
		if len(items) > 0 {
			o = items[0]
		} else {
			o, e = a.CreateOrganization(ctx, user, Doc{"legalName": name, "displayName": name, "type": "SCHOOL", "address": "Synthetic address", "state": "Bihar", "district": "Patna", "pin": "800001", "website": "", "contactEmail": user.Email, "contactPhone": "+919999999999", "representative": Doc{"name": "Synthetic representative", "designation": "Principal", "phone": "+919999999999"}, "registrationAuthority": "Synthetic authority", "registrationNumber": "SYNTHETIC", "identifiers": Doc{"recognition": "SYNTHETIC"}, "termsAccepted": true, "authorityDeclared": true, "noticeVersion": Notice})
			if e != nil {
				return e
			}
		}
		id := s(o, "id")
		if s(o, "state") == "DRAFT" {
			pdf := []byte("%PDF-1.4\nSynthetic local document\n%%EOF")
			d, e := a.Document(ctx, user, id, "", Doc{"kind": "recognition", "type": "application/pdf", "bytes": len(pdf)})
			if e != nil {
				return e
			}
			if _, e = a.Objects.Put(ctx, a.Config.DocumentBucket, s(d, "key"), "application/pdf", pdf); e != nil {
				return e
			}
			if _, e = a.Document(ctx, user, id, s(d, "sk"), nil); e != nil {
				return e
			}
			o, e = a.Submit(ctx, user, id)
			if e != nil {
				return e
			}
		}
		if s(o, "state") == "SUBMITTED" {
			o, e = a.Review(ctx, company, id, "review", "")
			if e != nil {
				return e
			}
		}
		if s(o, "state") == "UNDER_REVIEW" {
			o, e = a.Review(ctx, company, id, "approve", "")
			if e != nil {
				return e
			}
		}
		if e = w.Tick(ctx); e != nil {
			return e
		}
		t, e := a.Tenant(ctx, user, id)
		if e != nil {
			return e
		}
		if user.Sub == "alpha-admin" {
			for _, subject := range []string{"teacher", "viewer"} {
				old, e := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+id, subject)
				if e != nil {
					return e
				}
				if old == nil {
					role := "ASSESSOR"
					if subject == "viewer" {
						role = "VIEWER"
					}
					if e = a.Repo.Commit(ctx, a.put(Doc{"pk": "MEMBER#" + id, "sk": subject, "orgId": id, "email": subject + "@example.test", "role": role, "active": true, "gpk": "USER#" + subject, "gsk": id})); e != nil {
						return e
					}
				}
			}
		}
		schoolList, e := a.Schools(ctx, t)
		if e != nil {
			return e
		}
		schools := schoolList["items"].([]Doc)
		if len(schools) == 0 {
			school, e := a.CreateSchool(ctx, user, t, Doc{"name": "Synthetic School", "block": "Demo Block", "district": "Demo District", "state": "Demo State"})
			if e != nil {
				return e
			}
			schools = append(schools, school)
		}
		students, e := a.Repo.Query(ctx, t.Table, "STUDENT", false)
		if e != nil {
			return e
		}
		if len(students) == 0 {
			teacherUser := identity.Accounts[4]
			if user.Sub != "alpha-admin" {
				teacherUser.Sub = "synthetic-beta-teacher"
				teacherUser.Email = "synthetic-beta-teacher@example.test"
			}
			member, err := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+id, teacherUser.Sub)
			if err != nil {
				return err
			}
			if member == nil {
				if err = a.Repo.Commit(ctx, a.put(Doc{"pk": "MEMBER#" + id, "sk": teacherUser.Sub, "orgId": id, "email": teacherUser.Email, "role": "ASSESSOR", "active": true, "gpk": "USER#" + teacherUser.Sub, "gsk": id})); err != nil {
					return err
				}
			}
			teacher, err := a.Tenant(ctx, teacherUser, id)
			if err != nil {
				return err
			}
			student, e := a.CreateStudent(ctx, teacherUser, teacher, Doc{"student": Doc{"name": "Synthetic learner", "age": 9, "className": "4", "schoolId": schools[0]["sk"]}, "consent": Doc{"guardianName": "Synthetic guardian", "relationship": "parent", "verificationMethod": "in_person_document_review", "evidenceReference": "synthetic-seed-only", "noticeVersion": Notice, "purposes": []any{"reading_assessment"}, "guardianAdultVerified": true, "authorityVerified": true}})
			if e != nil {
				return e
			}
			data := make([]byte, 640044)
			copy(data, "RIFF")
			binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
			copy(data[8:], "WAVEfmt ")
			binary.LittleEndian.PutUint32(data[16:], 16)
			binary.LittleEndian.PutUint16(data[20:], 1)
			binary.LittleEndian.PutUint16(data[22:], 1)
			binary.LittleEndian.PutUint32(data[24:], 16000)
			binary.LittleEndian.PutUint32(data[28:], 32000)
			binary.LittleEndian.PutUint16(data[32:], 2)
			binary.LittleEndian.PutUint16(data[34:], 16)
			copy(data[36:], "data")
			binary.LittleEndian.PutUint32(data[40:], 640000)
			assessment, e := a.InitAssessment(ctx, teacherUser, teacher, Doc{"studentId": student["sk"], "passageId": "en-1", "type": "audio/wav", "bytes": len(data), "duration": 20})
			if e != nil {
				return e
			}
			if _, e = a.Objects.Put(ctx, t.Bucket, s(assessment, "key"), "audio/wav", data); e != nil {
				return e
			}
			if _, e = a.QueueAssessment(ctx, teacherUser, teacher, s(assessment, "sk"), false); e != nil {
				return e
			}
			if e = w.Tick(ctx); e != nil {
				return e
			}
		}
	}
	return nil
}
