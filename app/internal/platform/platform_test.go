package platform

import (
	"context"
	"encoding/json"
	"errors"
	"learning-setu/app/internal/config"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
	"strings"
	"sync"
	"testing"
)

type memory struct {
	mu     sync.Mutex
	data   map[string]Doc
	tables map[string]bool
}

func TestDisabledIntegrationsRejectBeforeCreatingJobsOrUploads(t *testing.T) {
	// No repository/storage/provider: these calls must stop before any external work.
	a := &Service{Config: config.Config{IntegrationsDisabled: true}}
	ctx := context.Background()
	user := identity.User{}
	tenant := Tenant{Member: Doc{"role": "ORG_ADMIN"}}
	_, inviteErr := a.Invite(ctx, user, tenant, Doc{})
	_, initErr := a.InitAssessment(ctx, user, tenant, Doc{})
	_, queueErr := a.QueueAssessment(ctx, user, tenant, "assessment", false)
	_, retryErr := a.QueueAssessment(ctx, user, tenant, "assessment", true)
	for _, err := range []error{inviteErr, initErr, queueErr, retryErr} {
		if status(err) != 503 {
			t.Fatalf("expected unavailable, got %v", err)
		}
	}
}

func clone(d Doc) Doc {
	if d == nil {
		return nil
	}
	raw, _ := json.Marshal(d)
	var out Doc
	_ = json.Unmarshal(raw, &out)
	return out
}
func key(t, p, s string) string { return t + "|" + p + "|" + s }
func (r *memory) Get(_ context.Context, t, p, k string) (Doc, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return clone(r.data[key(t, p, k)]), nil
}
func (r *memory) Query(_ context.Context, t, p string, index bool) ([]Doc, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Doc{}
	field := "pk"
	if index {
		field = "gpk"
	}
	for k, d := range r.data {
		if len(k) > len(t) && k[:len(t)+1] == t+"|" && s(d, field) == p {
			out = append(out, clone(d))
		}
	}
	return out, nil
}
func (r *memory) QueryFields(ctx context.Context, t, p string, index bool, fields []string) ([]Doc, error) {
	return r.Query(ctx, t, p, index)
}
func (r *memory) Commit(_ context.Context, writes ...store.Write) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	for _, w := range writes {
		k := key(w.Table, s(w.Item, "pk"), s(w.Item, "sk"))
		if seen[k] {
			return errors.New("duplicate transaction key")
		}
		seen[k] = true
		if n(r.data[k], "rev") != n(w.Item, "rev") {
			return store.ErrConflict
		}
	}
	for _, w := range writes {
		k := key(w.Table, s(w.Item, "pk"), s(w.Item, "sk"))
		if w.Check {
			continue
		}
		if w.Delete {
			delete(r.data, k)
		} else {
			d := clone(w.Item)
			d["rev"] = n(d, "rev") + 1
			r.data[k] = d
		}
	}
	return nil
}
func (r *memory) EnsureTable(_ context.Context, t, o string) error { r.tables[t] = true; return nil }
func (r *memory) OwnedTableExists(_ context.Context, table, org string) (bool, error) {
	return r.tables[table], nil
}
func (r *memory) DeleteTenantTable(_ context.Context, table, org string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.data {
		if strings.HasPrefix(k, table+"|") {
			delete(r.data, k)
		}
	}
	delete(r.tables, table)
	return nil
}

type objects struct {
	buckets         map[string]bool
	fail            bool
	failDelete      bool
	deletedPrefixes []string
}

func (o *objects) EnsureBucket(_ context.Context, b, id string) error {
	if o.fail {
		return errors.New("quota failure")
	}
	o.buckets[b] = true
	return nil
}
func (o *objects) Upload(context.Context, string, string, string, int64) (storage.Upload, error) {
	return storage.Upload{URL: "https://example.test/upload"}, nil
}
func (o *objects) Inspect(context.Context, string, string, string, int64) (storage.Object, error) {
	return storage.Object{Version: "1", ETag: "etag"}, nil
}
func (o *objects) Read(context.Context, string, string, string) ([]byte, error) {
	return []byte("%PDF-1.4"), nil
}
func (o *objects) Playback(context.Context, string, string, string) (string, error) {
	return "https://example.test/audio", nil
}
func (o *objects) DeletePrefix(_ context.Context, bucket, prefix string) error {
	o.deletedPrefixes = append(o.deletedPrefixes, bucket+"/"+prefix)
	return nil
}
func (o *objects) DeleteTenantBucket(_ context.Context, bucket, org string) error {
	if o.failDelete {
		return errors.New("deletion denied")
	}
	delete(o.buckets, bucket)
	return nil
}
func (o *objects) Put(context.Context, string, string, string, []byte) (storage.Object, error) {
	return storage.Object{Version: "1"}, nil
}
func fixture(t *testing.T) (*Service, *memory, *objects) {
	t.Helper()
	c := config.Config{Env: "test", Mode: "all", Provider: "mock", ControlTable: "control", DocumentBucket: "documents", Prefix: "setu-test", Region: "ap-south-1", Account: "000000000000", MaxAudioBytes: 100000, MaxAudioSeconds: 300, RetentionMaxDays: 365, Origin: "http://localhost:8080"}
	r := &memory{data: map[string]Doc{}, tables: map[string]bool{}}
	o := &objects{buckets: map[string]bool{}}
	a := &Service{Config: c, Repo: r, Objects: o}
	must(t, r.Commit(context.Background(), a.put(Doc{"pk": "COMPANY", "sk": "company", "active": true})))
	return a, r, o
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func check(t *testing.T, e error, want int) {
	t.Helper()
	if e == nil || status(e) != want {
		t.Fatalf("want status %d, got %v", want, e)
	}
}
func reviewed(t *testing.T, a *Service, user identity.User) Doc {
	t.Helper()
	ctx := context.Background()
	o, e := a.CreateOrganization(ctx, user, Doc{"displayName": "Test School"})
	must(t, e)
	o, e = a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(o, "id"))
	must(t, e)
	o["state"] = "UNDER_REVIEW"
	must(t, a.Repo.Commit(ctx, a.put(o)))
	return o
}
func active(t *testing.T, a *Service, user identity.User) Tenant {
	t.Helper()
	ctx := context.Background()
	o := reviewed(t, a, user)
	_, e := a.Review(ctx, identity.Accounts[0], s(o, "id"), "approve", "")
	must(t, e)
	must(t, NewWorker(a).Tick(ctx))
	tenant, e := a.Tenant(ctx, user, s(o, "id"))
	must(t, e)
	must(t, a.Repo.Commit(ctx, store.Put(tenant.Table, Doc{"pk": "SCHOOL", "sk": "test-school", "orgId": tenant.ID(), "name": "Test School", "block": "Block", "district": "District", "state": "State"})))
	return tenant
}
func teacherTenant(t *testing.T, a *Service, tenant Tenant) Tenant {
	t.Helper()
	if existing, err := a.Tenant(context.Background(), identity.Accounts[4], tenant.ID()); err == nil {
		return existing
	}
	member := store.Clone(tenant.Member)
	member["sk"] = identity.Accounts[4].Sub
	member["role"] = "ASSESSOR"
	member["gpk"] = "USER#" + identity.Accounts[4].Sub
	delete(member, "rev")
	must(t, a.Repo.Commit(context.Background(), a.put(member)))
	result, err := a.Tenant(context.Background(), identity.Accounts[4], tenant.ID())
	must(t, err)
	return result
}
func consent() Doc {
	return Doc{"guardianName": "Synthetic parent", "relationship": "parent", "verificationMethod": "in_person_document_review", "evidenceReference": "test", "guardianAdultVerified": true, "authorityVerified": true, "noticeVersion": Notice, "purposes": []any{"reading_assessment"}}
}
func TestIsolationAndLiveRevocation(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	u, v := identity.Accounts[2], identity.Accounts[3]
	ta, tb := active(t, a, u), active(t, a, v)
	student, e := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, ta), Doc{"student": Doc{"name": "Test", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	must(t, e)
	id := s(student, "sk")
	_, e = a.Tenant(ctx, v, ta.ID())
	check(t, e, 403)
	_, e = a.Student(ctx, tb, id)
	check(t, e, 404)
	for _, kind := range []string{"correct", "export", "delete", "withdraw"} {
		if kind == "correct" || kind == "delete" {
			_, e = a.Privacy(ctx, identity.Accounts[4], teacherTenant(t, a, tb), Doc{"kind": kind, "studentId": id})
		} else {
			_, e = a.Privacy(ctx, v, tb, Doc{"kind": kind, "studentId": id})
		}
		check(t, e, 404)
	}
	teacher := teacherTenant(t, a, ta)
	d, e := a.InitAssessment(ctx, identity.Accounts[4], teacher, Doc{"studentId": id, "passageId": "en-1", "type": "audio/wav", "bytes": 32044, "duration": 20})
	must(t, e)
	_, e = a.Playback(ctx, tb, s(d, "sk"))
	check(t, e, 404)
	_, e = a.Assessment(ctx, tb, s(d, "sk"))
	check(t, e, 404)
	_, e = a.Review(ctx, u, ta.ID(), "suspend", "test")
	check(t, e, 403)
	_, e = a.Review(ctx, identity.Accounts[0], ta.ID(), "suspend", "test")
	must(t, e)
	_, e = a.Tenant(ctx, u, ta.ID())
	check(t, e, 403)
	_, e = a.Review(ctx, identity.Accounts[0], ta.ID(), "reactivate", "test")
	must(t, e)
	member, e := r.Get(ctx, "control", "MEMBER#"+ta.ID(), u.Sub)
	must(t, e)
	member["active"] = false
	must(t, r.Commit(ctx, a.put(member)))
	_, e = a.Tenant(ctx, u, ta.ID())
	check(t, e, 403)
}
func TestApprovalIdempotencyAndPartialRecovery(t *testing.T) {
	a, r, o := fixture(t)
	ctx := context.Background()
	org := reviewed(t, a, identity.Accounts[2])
	id := s(org, "id")
	_, e := a.Review(ctx, identity.Accounts[0], id, "approve", "")
	must(t, e)
	_, e = a.Review(ctx, identity.Accounts[0], id, "approve", "")
	must(t, e)
	jobs, e := r.Query(ctx, "control", "JOB", false)
	must(t, e)
	if len(jobs) != 1 {
		t.Fatal("duplicate approval created a job")
	}
	w := NewWorker(a)
	o.fail = true
	for i := 0; i < 5; i++ {
		must(t, w.Tick(ctx))
		j, _ := r.Get(ctx, "control", "JOB", s(jobs[0], "sk"))
		j["availableAt"] = 0
		must(t, r.Commit(ctx, a.put(j)))
	}
	org, _ = r.Get(ctx, "control", "ORG", id)
	if s(org, "state") != "PROVISIONING_FAILED" || len(r.tables) != 1 {
		t.Fatal("partial progress not retained")
	}
	names := digest(m(org, "resources"))
	o.fail = false
	_, e = a.Review(ctx, identity.Accounts[0], id, "retry", "Quota resolved")
	must(t, e)
	must(t, w.Tick(ctx))
	org, _ = r.Get(ctx, "control", "ORG", id)
	if s(org, "state") != "ACTIVE" || digest(m(org, "resources")) != names || len(r.tables) != 1 || len(o.buckets) != 1 {
		t.Fatal("retry changed resources")
	}
}
func TestInvitationBindingExpiryAndSingleUse(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	u := identity.Accounts[2]
	tenant := active(t, a, u)
	_, e := a.Invite(ctx, u, tenant, Doc{"email": "invitee@example.test", "role": "COMPANY_ADMIN"})
	check(t, e, 422)
	for _, test := range []struct {
		token   string
		expires float64
	}{{"valid", millis() + 10000}, {"expired", millis() - 1}} {
		must(t, r.Commit(ctx, a.put(Doc{"pk": "INVITE", "sk": hash(test.token), "orgId": tenant.ID(), "email": "invitee@example.test", "role": "ASSESSOR", "state": "PENDING", "expiresAt": test.expires})))
	}
	_, e = a.Accept(ctx, u, "valid")
	check(t, e, 403)
	_, e = a.Accept(ctx, identity.Accounts[6], "expired")
	check(t, e, 403)
	_, e = a.Accept(ctx, identity.Accounts[6], "valid")
	must(t, e)
	_, e = a.Accept(ctx, identity.Accounts[6], "valid")
	check(t, e, 403)
}
func TestConsentRetentionAndDuplicateQueue(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	u := identity.Accounts[2]
	tenant := active(t, a, u)
	_, e := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, tenant), Doc{"student": Doc{"name": "Test", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": Doc{}})
	check(t, e, 422)
	student, e := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, tenant), Doc{"student": Doc{"name": "Test", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	must(t, e)
	teacher := teacherTenant(t, a, tenant)
	d, e := a.InitAssessment(ctx, identity.Accounts[4], teacher, Doc{"studentId": student["sk"], "passageId": "en-1", "type": "audio/wav", "bytes": 32044, "duration": 20})
	must(t, e)
	_, e = a.QueueAssessment(ctx, identity.Accounts[4], teacher, s(d, "sk"), false)
	must(t, e)
	_, e = a.QueueAssessment(ctx, identity.Accounts[4], teacher, s(d, "sk"), false)
	must(t, e)
	jobs, _ := r.Query(ctx, "control", "JOB", false)
	count := 0
	for _, j := range jobs {
		if s(j, "kind") == "ASSESS" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("duplicate processing job")
	}
	_, e = a.Retention(ctx, u, tenant, 366)
	check(t, e, 422)
	_, e = a.Privacy(ctx, u, tenant, Doc{"kind": "withdraw", "studentId": student["sk"]})
	must(t, e)
	_, e = a.Playback(ctx, tenant, s(d, "sk"))
	check(t, e, 409)
}

func TestConcurrentApprovalCreatesOneJob(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	org := reviewed(t, a, identity.Accounts[2])
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := a.Review(ctx, identity.Accounts[0], s(org, "id"), "approve", "")
			if e != nil && !errors.Is(e, store.ErrConflict) {
				t.Errorf("approval: %v", e)
			}
		}()
	}
	wg.Wait()
	jobs, e := r.Query(ctx, "control", "JOB", false)
	must(t, e)
	if len(jobs) != 1 {
		t.Fatalf("created %d jobs", len(jobs))
	}
}
func TestExpiredStudentCannotBeAssessed(t *testing.T) {
	a, r, _ := fixture(t)
	ctx := context.Background()
	u := identity.Accounts[2]
	tenant := active(t, a, u)
	student, e := a.CreateStudent(ctx, identity.Accounts[4], teacherTenant(t, a, tenant), Doc{"student": Doc{"name": "Test", "age": 9, "className": "4", "schoolId": "test-school"}, "consent": consent()})
	must(t, e)
	student, e = r.Get(ctx, tenant.Table, "STUDENT", s(student, "sk"))
	must(t, e)
	student["expiresAt"] = millis() - 1
	must(t, r.Commit(ctx, store.Put(tenant.Table, student)))
	_, e = a.Student(ctx, tenant, s(student, "sk"))
	check(t, e, 404)
	must(t, NewWorker(a).Sweep(ctx))
	student, e = r.Get(ctx, tenant.Table, "STUDENT", s(student, "sk"))
	must(t, e)
	if !b(student, "deleting") {
		t.Fatal("expired student was not scheduled for deletion")
	}
}
