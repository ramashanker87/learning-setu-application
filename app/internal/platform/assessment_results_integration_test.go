package platform

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"learning-setu/app/internal/config"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
)

func TestAssessmentResultsAgainstLocalEmulators(t *testing.T) {
	if os.Getenv("SETU_INTEGRATION") != "1" {
		t.Skip("set SETU_INTEGRATION=1 with local DynamoDB and S3 emulators")
	}
	ctx := context.Background()
	id := uuid.NewString()
	c := config.Config{Env: "test", Region: "ap-south-1", ControlTable: "archive-control-" + id, DynamoEndpoint: "http://localhost:8100", S3Endpoint: "http://localhost:9000", Origin: "http://localhost:8080"}
	r, err := store.New(ctx, c)
	must(t, err)
	objects, err := storage.New(ctx, c)
	must(t, err)
	table, bucket := "archive-tenant-"+id, "archive-results-"+id
	for _, name := range []string{c.ControlTable, table} {
		must(t, r.EnsureTable(ctx, name, id))
		t.Cleanup(func() {
			_, err := r.Client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: &name})
			if err != nil {
				t.Error(err)
			}
		})
	}
	must(t, objects.EnsureBucket(ctx, bucket, id))
	t.Cleanup(func() {
		if err := objects.DeletePrefix(ctx, bucket, ""); err != nil {
			t.Error(err)
		}
		if _, err := objects.Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &bucket}); err != nil {
			t.Error(err)
		}
	})
	a := &Service{Config: c, Repo: r, Objects: objects}
	o := Doc{"pk": "ORG", "sk": id, "id": id, "state": "ACTIVE", "resources": Doc{"table": table, "bucket": bucket}}
	student := Doc{"pk": "STUDENT", "sk": "synthetic-reader", "orgId": id, "expiresAt": millis() + 86400000}
	d := Doc{"pk": "ASSESSMENT", "sk": "synthetic-assessment", "orgId": id, "studentId": student["sk"], "gpk": "STUDENT#synthetic-reader", "gsk": "synthetic-assessment", "state": "PROCESSING", "passage": Doc{"text": "We read together", "language": "english"}}
	must(t, r.Commit(ctx, a.put(o), store.Put(table, student), store.Put(table, d)))
	o, err = r.Get(ctx, c.ControlTable, "ORG", id)
	must(t, err)
	student, err = r.Get(ctx, table, "STUDENT", "synthetic-reader")
	must(t, err)
	d, err = r.Get(ctx, table, "ASSESSMENT", "synthetic-assessment")
	must(t, err)
	tenant := Tenant{Org: o, Table: table, Bucket: bucket}
	// The completed payload exceeds DynamoDB's item limit, but never enters it.
	d["state"] = "COMPLETED"
	d["completedAt"] = now()
	d["result"] = Doc{"transcript": strings.Repeat("recognized word ", 40000), "score": Doc{"accuracy_score": 0, "errors": []any{Doc{"error_type": "inserted", "spoken_word": "extra"}}}}
	guards := []store.Write{{Table: c.ControlTable, Item: o, Check: true}, {Table: table, Item: student, Check: true}}
	archived, err := a.archiveResult(ctx, tenant, d, guards...)
	must(t, err)
	must(t, r.Commit(ctx, append(guards, a.resultCleanupWrite(archived), store.Put(table, archived))...))
	full, err := a.Assessment(ctx, tenant, "synthetic-assessment")
	must(t, err)
	if digest(m(full, "result")) != digest(m(d, "result")) {
		t.Fatal("large result changed")
	}
	history, err := a.AssessmentHistory(ctx, tenant, "synthetic-reader")
	must(t, err)
	if len(history) != 1 || len(s(m(history[0], "result"), "transcript")) < 400000 {
		t.Fatal("history missing S3 data")
	}
	// Older inline data is migrated once and remains byte-equivalent to readers.
	legacy := Doc{"pk": "ASSESSMENT", "sk": "legacy-assessment", "orgId": id, "studentId": "synthetic-reader", "state": "COMPLETED", "passage": Doc{"text": "पुराना पाठ"}, "result": Doc{"transcript": "पुराना पाठ", "score": Doc{"accuracy_score": 100}}}
	must(t, r.Commit(ctx, store.Put(table, legacy)))
	count, err := a.archiveAssessments(ctx, id, false)
	must(t, err)
	if count != 1 {
		t.Fatal("dry run count")
	}
	count, err = a.archiveAssessments(ctx, id, true)
	must(t, err)
	if count != 1 {
		t.Fatal("migration count")
	}
	count, err = a.archiveAssessments(ctx, id, true)
	must(t, err)
	if count != 0 {
		t.Fatal("migration did not resume safely")
	}
	full, err = a.Assessment(ctx, tenant, "legacy-assessment")
	must(t, err)
	if digest(m(full, "result")) != digest(m(legacy, "result")) {
		t.Fatal("legacy migration changed result")
	}
	// Student-prefix deletion removes all result object versions.
	must(t, objects.DeletePrefix(ctx, bucket, "students/synthetic-reader/"))
	versions, err := objects.Client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: &bucket})
	must(t, err)
	if len(versions.Versions) != 0 || len(versions.DeleteMarkers) != 0 {
		t.Fatal("result versions remain after deletion")
	}
	progress, err := a.ReadingProgress(ctx, tenant, "english", "90")
	must(t, err)
	if len(progress["assessments"].([]Doc)) != 1 {
		t.Fatal("projected dashboard summaries require no result objects")
	}
}
