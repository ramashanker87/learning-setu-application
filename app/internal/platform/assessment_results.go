package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"learning-setu/app/internal/store"
)

var resultID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

func resultPrefix(d Doc) (string, error) {
	if !resultID.MatchString(s(d, "studentId")) || !resultID.MatchString(s(d, "sk")) || s(d, "orgId") == "" {
		return "", fmt.Errorf("invalid assessment result ownership")
	}
	return "students/" + s(d, "studentId") + "/assessments/" + s(d, "sk") + "/results/", nil
}

func validResultKey(d Doc, key string) bool {
	prefix, err := resultPrefix(d)
	if err != nil || !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, "/result.json") {
		return false
	}
	_, err = uuid.Parse(strings.TrimSuffix(strings.TrimPrefix(key, prefix), "/result.json"))
	return err == nil
}

// archiveResult writes and verifies an immutable object before returning a smaller
// record for the caller's conditional transaction. A durable, delayed cleanup job
// covers upload timeouts and failed final commits without deleting committed data.
func (a *Service) archiveResult(ctx context.Context, t Tenant, d Doc, guards ...store.Write) (Doc, error) {
	if s(d, "orgId") != t.ID() || s(d, "state") != "COMPLETED" || len(m(d, "result")) == 0 || len(m(d, "resultStorage")) != 0 || t.Bucket == "" {
		return nil, fmt.Errorf("assessment is not eligible for result storage")
	}
	prefix, err := resultPrefix(d)
	if err != nil {
		return nil, err
	}
	payload := Doc{"schemaVersion": 1, "orgId": t.ID(), "studentId": d["studentId"], "assessmentId": d["sk"], "passage": d["passage"], "result": d["result"]}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(data) > 25*1024*1024 {
		return nil, fmt.Errorf("assessment result exceeds object limit")
	}
	key := prefix + uuid.NewString() + "/result.json"
	cleanup := job("RESULT_CLEANUP", t.ID(), Doc{"assessmentId": d["sk"], "studentId": d["studentId"], "key": key})
	// Worker operations are bounded to eight minutes; backfill operations to two.
	cleanup["availableAt"] = millis() + float64((15 * time.Minute).Milliseconds())
	writes := append([]store.Write{}, guards...)
	writes = append(writes, store.Write{Table: t.Table, Item: d, Check: true}, a.put(cleanup))
	if err = a.Repo.Commit(ctx, writes...); err != nil {
		return nil, err
	}
	object, err := a.Objects.Put(ctx, t.Bucket, key, "application/json", data)
	if err != nil {
		return nil, err
	}
	if object.Version == "" {
		return nil, fmt.Errorf("assessment result requires versioned storage")
	}
	stored, err := a.Objects.Read(ctx, t.Bucket, key, object.Version)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(data, stored) {
		return nil, fmt.Errorf("assessment result verification failed")
	}
	out := store.Clone(d)
	out["resultStorage"] = Doc{"schemaVersion": 1, "key": key, "version": object.Version, "sha256": hash(string(data)), "cleanupJobId": cleanup["sk"]}
	result := m(out, "result")
	summary := Doc{}
	for _, key := range []string{"duration", "wordsPerMinute", "provider", "providerVersion", "scoringVersion"} {
		if value, exists := result[key]; exists {
			summary[key] = value
		}
	}
	score := Doc{}
	for _, key := range []string{"accuracy_score", "pronunciation_score", "pause_count"} {
		if value, exists := m(result, "score")[key]; exists {
			score[key] = value
		}
	}
	summary["score"] = score
	out["result"] = summary
	delete(m(out, "passage"), "text")
	return out, nil
}

// Once cleanup claims this job, a late writer must not publish its object.
// The final transaction cancels cleanup by deleting revision 1. A cleanup
// claim advances that revision, fencing any late result publication.
func (a *Service) resultCleanupWrite(d Doc) store.Write {
	return store.Write{Table: a.Config.ControlTable, Item: Doc{"pk": "JOB", "sk": m(d, "resultStorage")["cleanupJobId"], "rev": 1}, Delete: true}
}

func (a *Service) hydrateResult(ctx context.Context, t Tenant, d Doc) (Doc, error) {
	if s(d, "orgId") != t.ID() {
		return nil, fail(404, "Assessment not found")
	}
	ref := m(d, "resultStorage")
	if len(ref) == 0 {
		return d, nil // Existing inline results remain readable during migration.
	}
	if n(ref, "schemaVersion") != 1 || !validResultKey(d, s(ref, "key")) || s(ref, "version") == "" || len(s(ref, "sha256")) != 64 {
		return nil, fail(503, "Assessment details are unavailable")
	}
	data, err := a.Objects.Read(ctx, t.Bucket, s(ref, "key"), s(ref, "version"))
	if err != nil || hash(string(data)) != s(ref, "sha256") {
		return nil, fail(503, "Assessment details are unavailable; try again later")
	}
	var payload Doc
	if json.Unmarshal(data, &payload) != nil || n(payload, "schemaVersion") != 1 || s(payload, "orgId") != t.ID() || s(payload, "studentId") != s(d, "studentId") || s(payload, "assessmentId") != s(d, "sk") || len(m(payload, "result")) == 0 {
		return nil, fail(503, "Assessment details are unavailable")
	}
	out := store.Clone(d)
	out["result"], out["passage"] = payload["result"], payload["passage"]
	return out, nil
}

func (a *Service) AssessmentHistory(ctx context.Context, t Tenant, studentID string) ([]Doc, error) {
	if _, err := a.Student(ctx, t, studentID); err != nil {
		return nil, err
	}
	items, err := a.Repo.Query(ctx, t.Table, "STUDENT#"+studentID, true)
	if err != nil {
		return nil, err
	}
	for i, d := range items {
		if s(d, "studentId") != studentID {
			return nil, fail(404, "Assessment not found")
		}
		items[i], err = a.hydrateResult(ctx, t, d)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (w *Worker) cleanupResult(ctx context.Context, j Doc) error {
	a := w.Service
	o, err := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(j, "orgId"))
	if err != nil {
		return err
	}
	data := m(j, "data")
	identity := Doc{"orgId": o["id"], "studentId": data["studentId"], "sk": data["assessmentId"]}
	key := s(data, "key")
	if !validResultKey(identity, key) || len(m(o, "resources")) == 0 {
		return fmt.Errorf("invalid result cleanup ownership")
	}
	d, err := a.Repo.Get(ctx, s(m(o, "resources"), "table"), "ASSESSMENT", s(data, "assessmentId"))
	if err != nil {
		return err
	}
	if s(m(d, "resultStorage"), "key") == key {
		return nil
	}
	// Each attempt owns its own UUID directory, including every object version.
	return a.Objects.DeletePrefix(ctx, s(m(o, "resources"), "bucket"), strings.TrimSuffix(key, "result.json"))
}
