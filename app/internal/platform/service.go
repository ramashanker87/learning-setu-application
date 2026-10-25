// Package platform contains the application use cases shared by API, worker and seed commands.
package platform

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"learning-setu/app/internal/config"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

type Doc = store.Doc

var s = store.S
var n = store.N
var b = store.B
var m = store.M

const Notice = "2026-10-v1"

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string              { return e.Message }
func fail(status int, message string) error { return &Error{status, message} }
func now() string                           { return time.Now().UTC().Format(time.RFC3339Nano) }
func millis() float64                       { return float64(time.Now().UnixMilli()) }
func hash(text string) string               { x := sha256.Sum256([]byte(text)); return hex.EncodeToString(x[:]) }
func contains(x string, values ...string) bool {
	for _, v := range values {
		if x == v {
			return true
		}
	}
	return false
}
func audit(org, actor, action, reason string) Doc {
	return Doc{"pk": "AUDIT#" + org, "sk": now() + "#" + uuid.NewString(), "actor": actor, "action": action, "reason": reason, "at": now()}
}
func job(kind, org string, data Doc) Doc {
	return Doc{"pk": "JOB", "sk": uuid.NewString(), "gpk": "READY", "gsk": now(), "kind": kind, "orgId": org, "data": data, "attempts": 0, "availableAt": millis(), "state": "PENDING"}
}

type Policy struct {
	Identifiers    []string `json:"identifiers"`
	Required       []string `json:"required"`
	Documents      []string `json:"documents"`
	RequiredFields []string `json:"requiredFields"`
}

//go:embed onboarding-policies.json
var policyJSON []byte
var Policies = func() map[string]Policy {
	x := map[string]Policy{}
	if e := json.Unmarshal(policyJSON, &x); e != nil {
		panic(e)
	}
	return x
}()

//go:embed passages.json
var passageJSON []byte
var Passages = func() []Doc {
	samples := []Doc{{"id": "en-1", "version": 1, "language": "english", "title": "A day at school", "text": "The children walked to school. They read a book and played together."}, {"id": "hi-1", "version": 1, "language": "hindi", "title": "हमारा विद्यालय", "text": "हम हर दिन विद्यालय जाते हैं। हम किताब पढ़ते हैं और साथ खेलते हैं।"}}

	var original []Doc
	if e := json.Unmarshal(passageJSON, &original); e != nil {
		panic(e)
	}
	return append(samples, original...)
}()

type Service struct {
	Config       config.Config
	Repo         store.Repository
	Objects      storage.Objects
	TeamAccounts identity.TeamAccountCreator
}
type Tenant struct {
	Org, Member   Doc
	Table, Bucket string
}

func (t Tenant) ID() string              { return s(t.Org, "id") }
func (a *Service) put(d Doc) store.Write { return store.Put(a.Config.ControlTable, d) }
func (a *Service) guards(t Tenant) []store.Write {
	return []store.Write{{Table: a.Config.ControlTable, Item: t.Org, Check: true}, {Table: a.Config.ControlTable, Item: t.Member, Check: true}}
}
func (a *Service) Company(ctx context.Context, u identity.User) error {
	d, e := a.Repo.Get(ctx, a.Config.ControlTable, "COMPANY", u.Sub)
	if e != nil {
		return e
	}
	if !u.Company || !b(d, "active") {
		return fail(403, "Company administrator access required")
	}
	return nil
}
func (a *Service) Tenant(ctx context.Context, u identity.User, id string, roles ...string) (Tenant, error) {
	t := Tenant{}
	if u.TeamOrg != "" && u.TeamOrg != id {
		return t, fail(403, "This team account belongs to another organization")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`).MatchString(id) {
		return t, fail(403, "Organization membership required")
	}
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if e != nil {
		return t, e
	}
	member, e := a.Repo.Get(ctx, a.Config.ControlTable, "MEMBER#"+id, u.Sub)
	if e != nil {
		return t, e
	}
	if len(roles) == 0 {
		roles = []string{"ORG_ADMIN", "ASSESSOR", "VIEWER"}
	}
	if u.TeamOrg != "" && !contains(s(member, "role"), "ASSESSOR", "VIEWER") {
		return t, fail(403, "Team role must be assessor or viewer")
	}
	if s(o, "state") != "ACTIVE" || !b(member, "active") || !contains(s(member, "role"), roles...) {
		return t, fail(403, "Organization or membership is inactive or unauthorized")
	}
	return Tenant{o, member, s(m(o, "resources"), "table"), s(m(o, "resources"), "bucket")}, nil
}
func (a *Service) owned(ctx context.Context, u identity.User, id string) (Doc, error) {
	o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", id)
	if e != nil {
		return nil, e
	}
	if o == nil || s(o, "owner") != u.Sub {
		return nil, fail(403, "Application access denied")
	}
	return o, nil
}
func required(d Doc, keys ...string) error {
	for _, k := range keys {
		v := strings.TrimSpace(s(d, k))
		if v == "" || len(v) > 1000 {
			return fail(422, "Check field: "+k)
		}
	}
	return nil
}
func validEmail(value string) bool {
	v, e := mail.ParseAddress(value)
	return e == nil && v.Address == value && !strings.ContainsAny(value, "\r\n")
}

// Normalize the simplified form without inventing a legal organization name or type.
// Older application payloads retain their existing policy and validation.
func normalizeApplication(d Doc) Doc {
	if s(d, "formVersion") != "organization-v2" {
		return d
	}
	d = store.Clone(d)
	for _, key := range []string{"firstName", "lastName", "address", "contactEmail", "contactPhone", "orgNo"} {
		d[key] = strings.TrimSpace(s(d, key))
	}
	d["contactEmail"] = strings.ToLower(s(d, "contactEmail"))
	d["type"] = "ORGANIZATION"
	d["displayName"] = "Organization " + s(d, "orgNo")
	d["representative"] = Doc{"name": s(d, "firstName") + " " + s(d, "lastName"), "phone": s(d, "contactPhone")}
	ids := m(d, "identifiers")
	for _, key := range []string{"pan", "gstin"} {
		if value, ok := ids[key].(string); ok {
			ids[key] = strings.ToUpper(strings.TrimSpace(value))
		}
	}
	d["identifiers"] = ids
	return d
}

func validateApplication(d Doc) error {
	if s(d, "formVersion") == "organization-v2" {
		if err := required(d, "firstName", "lastName", "address", "contactPhone", "orgNo"); err != nil {
			return err
		}
		if !validEmail(s(d, "contactEmail")) || !b(d, "termsAccepted") || !b(d, "authorityDeclared") || s(d, "noticeVersion") != Notice {
			return fail(422, "Check email, service terms, authority declaration and privacy notice")
		}
		for key, value := range m(d, "identifiers") {
			size := map[string]int{"pan": 10, "gstin": 15}[key]
			text, ok := value.(string)
			if size == 0 || !ok || (text != "" && (len(text) != size || !regexp.MustCompile(`^[A-Z0-9]+$`).MatchString(text))) {
				return fail(422, "Check PAN (10 characters) or GSTIN (15 characters)")
			}
		}
		return nil
	}
	if s(d, "type") == "ORGANIZATION" {
		return fail(422, "Use the current organization application form")
	}

	p, ok := Policies[s(d, "type")]
	if !ok {
		return fail(422, "Choose an organization type")
	}
	if e := required(d, "legalName", "displayName", "address", "state", "district", "contactPhone"); e != nil {
		return e
	}
	if e := required(d, p.RequiredFields...); e != nil {
		return e
	}
	if e := required(m(d, "representative"), "name", "designation", "phone"); e != nil {
		return e
	}
	if !validEmail(s(d, "contactEmail")) || !regexp.MustCompile(`^\d{6}$`).MatchString(s(d, "pin")) || !b(d, "termsAccepted") || !b(d, "authorityDeclared") || s(d, "noticeVersion") != Notice {
		return fail(422, "Check contact information, PIN, terms and privacy notice")
	}
	for k := range m(d, "identifiers") {
		if !contains(k, p.Identifiers...) {
			return fail(422, "Identifier not applicable to this organization")
		}
	}
	for _, k := range p.Required {
		if s(m(d, "identifiers"), k) == "" {
			return fail(422, "Required identifier missing: "+k)
		}
	}
	return nil
}
func validateConsent(d Doc) error {
	if e := required(d, "guardianName", "relationship"); e != nil {
		return e
	}
	if !b(d, "guardianAdultVerified") || !b(d, "authorityVerified") || s(d, "noticeVersion") != Notice || !contains(s(d, "verificationMethod"), "", "staff_declaration", "in_person_document_review", "verified_guardian_account") {
		return fail(422, "A verified guardian authorization decision is required")
	}
	if contains(s(d, "verificationMethod"), "in_person_document_review", "verified_guardian_account") {
		if err := required(d, "evidenceReference"); err != nil {
			return err
		}
	}
	p, ok := d["purposes"].([]any)
	if !ok || len(p) != 1 || p[0] != "reading_assessment" {
		return fail(422, "Reading assessment authorization required")
	}
	return nil
}
func validateStudent(d Doc) error {
	for _, key := range []string{"email", "mobile"} {
		if value, present := d[key]; present {
			if _, ok := value.(string); !ok {
				return fail(422, "Check field: "+key)
			}
		}
	}
	email := strings.TrimSpace(s(d, "email"))
	if email != "" && (len(email) > 254 || !validEmail(email)) {
		return fail(422, "Enter a valid email address or leave it blank")
	}
	mobile := strings.TrimSpace(s(d, "mobile"))
	if mobile != "" {
		digits := regexp.MustCompile(`[0-9]`).FindAllString(mobile, -1)
		if len(mobile) > 30 || len(digits) < 7 || len(digits) > 15 || !regexp.MustCompile(`^\+?[0-9 ()-]+$`).MatchString(mobile) {
			return fail(422, "Enter a valid mobile number or leave it blank")
		}
	}

	if e := required(d, "name", "className"); e != nil {
		return e
	}
	age := n(d, "age")
	if age < 4 || age > 15 || age != float64(int(age)) {
		return fail(422, "Select a whole-number age from 4 to 15")
	}
	if !contains(s(d, "className"), "2", "3", "4", "5", "6", "7", "8", "9", "10") {
		return fail(422, "Select a class from 2nd to 10th")
	}
	if s(d, "formVersion") == "student-v2" || s(d, "schoolName") != "" {
		if err := required(d, "schoolName"); err != nil {
			return err
		}
	}
	return nil
}
func status(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	if errors.Is(err, store.ErrConflict) {
		return 409
	}
	return 500
}
func safeError(err error) string {
	if status(err) == 500 {
		return "Service unavailable. Please retry."
	}
	return err.Error()
}
func resourceNames(c config.Config, id, display string) Doc {
	slug := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(display), "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 4 {
		slug = slug[:4]
	}
	if slug == "" {
		slug = "org"
	}
	name := fmt.Sprintf("%s-%s-%s-%s", c.Prefix, slug, strings.ReplaceAll(id, "-", ""), hash(c.Account + ":" + c.Region)[:6])
	return Doc{"bucket": name, "table": name, "schemaVersion": 1}
}
