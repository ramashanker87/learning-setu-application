package platform

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"time"
)

// ReadingProgress reads only metric/identity projections. Transcripts, recordings,
// guardian details and S3 result objects are not needed for this dashboard.
func (a *Service) ReadingProgress(ctx context.Context, t Tenant, language, period string) (Doc, error) {
	if language == "" {
		language = "english"
	}
	if !contains(language, "english", "hindi") {
		return nil, fail(422, "Choose English or Hindi")
	}
	if period == "" {
		period = "90"
	}
	if !contains(period, "30", "90", "180", "365") {
		return nil, fail(422, "Choose a supported date range")
	}
	days, _ := strconv.Atoi(period)
	clock := time.Now().UTC()
	end := time.Date(clock.Year(), clock.Month(), clock.Day()+1, 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -days)
	students, err := a.Repo.QueryFields(ctx, t.Table, "STUDENT", false, []string{"sk", "orgId", "name", "className", "schoolId", "schoolName", "deleting", "expiresAt"})
	if err != nil {
		return nil, err
	}
	active := map[string]bool{}
	learners := []Doc{}
	for _, d := range students {
		if s(d, "orgId") != t.ID() || b(d, "deleting") || n(d, "expiresAt") <= millis() {
			continue
		}
		active[s(d, "sk")] = true
		learners = append(learners, Doc{"id": d["sk"], "name": d["name"], "className": d["className"], "schoolId": d["schoolId"], "schoolName": d["schoolName"]})
	}
	schools, err := a.Repo.QueryFields(ctx, t.Table, "SCHOOL", false, []string{"sk", "orgId", "name", "block", "district", "state"})
	if err != nil {
		return nil, err
	}
	schoolRows := []Doc{}
	for _, d := range schools {
		if s(d, "orgId") == t.ID() {
			schoolRows = append(schoolRows, Doc{"id": d["sk"], "name": d["name"], "block": d["block"], "district": d["district"], "state": d["state"]})
		}
	}
	fields := []string{"sk", "orgId", "studentId", "state", "createdAt", "completedAt", "passage.id", "passage.version", "passage.language", "passage.title", "provider", "providerVersion", "scoringVersion", "result.provider", "result.providerVersion", "result.scoringVersion", "result.wordsPerMinute", "result.score.accuracy_score"}
	assessments, err := a.Repo.QueryFields(ctx, t.Table, "ASSESSMENT", false, fields)
	if err != nil {
		return nil, err
	}
	rows := []Doc{}
	for _, d := range assessments {
		if s(d, "orgId") != t.ID() || !active[s(d, "studentId")] || s(d, "state") != "COMPLETED" || s(m(d, "passage"), "language") != language {
			continue
		}
		at := s(d, "completedAt")
		if at == "" {
			at = s(d, "createdAt")
		}
		date, err := time.Parse(time.RFC3339Nano, at)
		if err != nil || date.Before(start) || !date.Before(end) {
			continue
		}
		result, passage := m(d, "result"), m(d, "passage")
		row := Doc{"id": d["sk"], "studentId": d["studentId"], "at": date.UTC().Format(time.RFC3339Nano), "passageId": passage["id"], "passageVersion": passage["version"], "passageTitle": passage["title"], "language": language, "accuracy": progressNumber(m(result, "score")["accuracy_score"], 100), "wpm": progressNumber(result["wordsPerMinute"], math.MaxFloat64)}
		for _, key := range []string{"provider", "providerVersion", "scoringVersion"} {
			value := s(result, key)
			if value == "" {
				value = s(d, key)
			}
			row[key] = value
		}
		rows = append(rows, row)
		if len(rows) > 10000 {
			return nil, fail(422, "Too many assessments for this view. Choose a shorter date range.")
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if s(rows[i], "at") == s(rows[j], "at") {
			return s(rows[i], "id") < s(rows[j], "id")
		}
		return s(rows[i], "at") < s(rows[j], "at")
	})
	sort.Slice(learners, func(i, j int) bool { return s(learners[i], "name") < s(learners[j], "name") })
	sort.Slice(schoolRows, func(i, j int) bool { return s(schoolRows[i], "name") < s(schoolRows[j], "name") })
	return Doc{"students": learners, "schools": schoolRows, "assessments": rows, "language": language, "days": days, "from": start.Format("2006-01-02"), "through": end.AddDate(0, 0, -1).Format("2006-01-02"), "generatedAt": clock.Format(time.RFC3339)}, nil
}

func progressNumber(value any, maximum float64) any {
	var number float64
	switch v := value.(type) {
	case float64:
		number = v
	case int:
		number = float64(v)
	case int64:
		number = float64(v)
	case json.Number:
		var err error
		number, err = v.Float64()
		if err != nil {
			return nil
		}
	default:
		return nil
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > maximum {
		return nil
	}
	return number
}
