package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"learning-setu/app/internal/identity"
)

func publicDraft() Doc {
	return Doc{"formVersion": "organization-v2", "firstName": "Test", "lastName": "Representative", "address": "Test address", "contactEmail": "alpha@example.test", "contactPhone": "9999999999", "orgNo": "TEST", "identifiers": Doc{}, "termsAccepted": true, "authorityDeclared": true, "noticeVersion": Notice}
}
func TestPublicApplicationLifecycleAndAccessBoundaries(t *testing.T) {
	a, repo, _ := fixture(t)
	ctx := context.Background()
	a.Config.AuthMode = "local"
	a.Config.LocalSecret = strings.Repeat("s", 32)
	h := a.Handler()
	request := func(method, path, key string, body Doc, want int) Doc {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("X-Application-Key", key)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
		var out Doc
		must(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out
	}
	request("POST", "/api/public/applications", "", Doc{}, 422)
	created := request("POST", "/api/public/applications", "", publicDraft(), 201)
	id, key := s(m(created, "application"), "id"), s(created, "accessKey")
	path := "/api/public/applications/" + id
	request("GET", path, "", nil, 403)
	request("GET", path, strings.Repeat("x", 43), nil, 403)
	other := request("POST", "/api/public/applications", "", publicDraft(), 201)
	request("GET", path, s(other, "accessKey"), nil, 403)
	visible := request("GET", path, key, nil, 200)
	if visible["applicationKeyHash"] != nil || visible["owner"] != nil {
		t.Fatal("internal fields disclosed")
	}
	o, _ := repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if s(o, "applicationKeyHash") == key {
		t.Fatal("raw key persisted")
	}
	_, err := a.ClaimApplication(ctx, identity.Accounts[2], id, key)
	check(t, err, 409)
	request("POST", path+"/submit", key, Doc{}, 422)
	doc := request("POST", path+"/documents", key, Doc{"kind": "registration", "type": "application/pdf", "bytes": 8}, 200)
	request("POST", path+"/documents/"+s(doc, "sk")+"/complete", key, Doc{}, 200)
	request("POST", path+"/submit", key, Doc{}, 200)
	request("PUT", path, key, publicDraft(), 409)
	_, err = a.Review(ctx, identity.Accounts[0], id, "review", "")
	must(t, err)
	_, err = a.Review(ctx, identity.Accounts[0], id, "approve", "")
	must(t, err)
	must(t, NewWorker(a).Tick(ctx))
	members, err := repo.Query(ctx, a.Config.ControlTable, "MEMBER#"+id, false)
	must(t, err)
	if len(members) != 0 {
		t.Fatal("public applicant received membership before verified sign-in")
	}
	_, err = a.ClaimApplication(ctx, identity.Accounts[3], id, key)
	check(t, err, 403)
	company := identity.Accounts[0]
	company.Email = "alpha@example.test"
	_, err = a.ClaimApplication(ctx, company, id, key)
	check(t, err, 403)
	_, err = a.ClaimApplication(ctx, identity.Accounts[2], id, key)
	must(t, err)
	_, err = a.Tenant(ctx, identity.Accounts[2], id)
	must(t, err)
	request("GET", path, key, nil, 403)
	request("POST", "/api/applications/"+id+"/claim", "", Doc{"accessKey": key}, 401)
}
func TestPublicApplicationLimitsAndOffboardingRevocation(t *testing.T) {
	a, repo, _ := fixture(t)
	ctx := context.Background()
	created, err := a.CreatePublicApplication(ctx, publicDraft())
	must(t, err)
	id, key := s(m(created, "application"), "id"), s(created, "accessKey")
	u, _, err := a.publicApplicant(ctx, id, key)
	must(t, err)
	for i := 0; i < 8; i++ {
		_, err = a.Document(ctx, u, id, "", Doc{"kind": "registration", "type": "application/pdf", "bytes": 8})
		must(t, err)
	}
	_, err = a.Document(ctx, u, id, "", Doc{"kind": "registration", "type": "application/pdf", "bytes": 8})
	check(t, err, 422)
	for i := 0; i < 9; i++ {
		_, err = a.CreatePublicApplication(ctx, publicDraft())
		must(t, err)
	}
	_, err = a.CreatePublicApplication(ctx, publicDraft())
	check(t, err, 429)
	_, err = a.Offboard(ctx, identity.Accounts[0], id, Doc{"confirmation": id, "reason": "Test cleanup"})
	must(t, err)
	_, _, err = a.publicApplicant(ctx, id, key)
	check(t, err, 403)
	limiter, _ := repo.Get(ctx, a.Config.ControlTable, "PUBLIC_RATE", "APPLICATIONS")
	limiter["until"] = 0
	must(t, repo.Commit(ctx, a.put(limiter)))
	_, err = a.CreatePublicApplication(ctx, publicDraft())
	must(t, err)
}
func TestPublicApplicationRejectsForeignOrigin(t *testing.T) {
	a, _, _ := fixture(t)
	raw, _ := json.Marshal(publicDraft())
	r := httptest.NewRequest(http.MethodPost, "/api/public/applications", bytes.NewReader(raw))
	r.Header.Set("Origin", "https://unrelated.example")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestSimplifiedApplicationValidationAndNormalization(t *testing.T) {
	a, _, _ := fixture(t)
	for _, key := range []string{"firstName", "lastName", "address", "contactPhone", "contactEmail", "orgNo", "termsAccepted", "authorityDeclared", "noticeVersion"} {
		d := publicDraft()
		delete(d, key)
		_, err := a.CreatePublicApplication(context.Background(), d)
		check(t, err, 422)
	}
	for _, ids := range []Doc{{"pan": "short"}, {"gstin": "short"}, {"unknown": "value"}, {"pan": 123}} {
		d := publicDraft()
		d["identifiers"] = ids
		_, err := a.CreatePublicApplication(context.Background(), d)
		check(t, err, 422)
	}
	d := publicDraft()
	d["firstName"] = " Test "
	d["identifiers"] = Doc{"pan": " abcde1234f ", "gstin": ""}
	created, err := a.CreatePublicApplication(context.Background(), d)
	must(t, err)
	saved := m(m(created, "application"), "draft")
	if s(saved, "type") != "ORGANIZATION" || s(saved, "displayName") != "Organization TEST" || s(m(saved, "representative"), "name") != "Test Representative" || s(m(saved, "identifiers"), "pan") != "ABCDE1234F" {
		t.Fatalf("unexpected normalized application: %#v", saved)
	}
}

func TestContactPersonApplication(t *testing.T) {
	for _, name := range []string{"  Asha  ", "  Asha Devi Kumar  ", "", "   "} {
		t.Run(name, func(t *testing.T) {
			a, _, _ := fixture(t)
			d := publicDraft()
			d["contactPersonName"] = name
			delete(d, "firstName")
			delete(d, "lastName")
			created, err := a.CreatePublicApplication(context.Background(), d)
			if name == "" || name == "   " {
				check(t, err, 422)
				return
			}
			must(t, err)
			saved := m(m(created, "application"), "draft")
			want := "Asha"
			if name == "  Asha Devi Kumar  " {
				want = "Asha Devi Kumar"
			}
			if s(saved, "contactPersonName") != want || s(m(saved, "representative"), "name") != want {
				t.Fatalf("contact person name was not preserved: %#v", saved)
			}
		})
	}
}
