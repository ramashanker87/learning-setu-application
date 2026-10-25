package platform

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"learning-setu/app/internal/config"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
)

func TestOffboardingAgainstLocalEmulators(t *testing.T) {
	if os.Getenv("SETU_INTEGRATION") != "1" {
		t.Skip("local emulators required")
	}
	ctx := context.Background()
	c, e := config.From(func(k string) string {
		return map[string]string{"APP_ENV": "test", "AUTH_MODE": "local", "LOCAL_AUTH_SECRET": "synthetic-local-only-secret-never-use-in-production", "TRANSCRIPTION_PROVIDER": "mock", "DYNAMODB_ENDPOINT": "http://localhost:8100", "S3_ENDPOINT": "http://localhost:9000", "S3_PUBLIC_ENDPOINT": "http://localhost:9000", "WORKER_MODE": "all"}[k]
	})
	must(t, e)
	run := uuid.NewString()
	c.Prefix = "setu-offtest"
	c.ControlTable = c.Prefix + "-control-" + run
	c.DocumentBucket = c.Prefix + "-docs-" + run
	r, e := store.New(ctx, c)
	must(t, e)
	objects, e := storage.New(ctx, c)
	must(t, e)
	a := &Service{Config: c, Repo: r, Objects: objects}
	must(t, r.EnsureTable(ctx, c.ControlTable, run))
	must(t, objects.EnsureBucket(ctx, c.DocumentBucket, run))
	t.Cleanup(func() {
		// Only this test's uniquely named local resources; never real AWS resources.
		_ = objects.DeletePrefix(ctx, c.DocumentBucket, "")
		_, _ = objects.Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(c.DocumentBucket)})
		_, _ = r.Client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(c.ControlTable)})
	})
	must(t, r.Commit(ctx, a.put(Doc{"pk": "COMPANY", "sk": "company", "active": true})))
	target := active(t, a, identity.Accounts[2])
	other := active(t, a, identity.Accounts[3])
	t.Cleanup(func() {
		for _, tenant := range []Tenant{target, other} {
			_ = objects.DeleteTenantBucket(ctx, tenant.Bucket, tenant.ID())
			_ = r.DeleteTenantTable(ctx, tenant.Table, tenant.ID())
		}
	})
	must(t, r.Commit(ctx, store.Put(target.Table, Doc{"pk": "STUDENT", "sk": "student", "name": "Synthetic"}), store.Put(other.Table, Doc{"pk": "STUDENT", "sk": "keep", "name": "Synthetic other"})))
	for i := 0; i < 2; i++ {
		_, e := objects.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(target.Bucket), Key: aws.String("students/student/audio"), Body: bytes.NewReader([]byte("synthetic audio"))})
		must(t, e)
	}
	_, e = objects.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(target.Bucket), Key: aws.String("students/student/audio")})
	must(t, e)
	_, e = objects.Client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(target.Bucket), Key: aws.String("unfinished")})
	must(t, e)
	targetDoc := "organizations/" + target.ID() + "/doc"
	otherDoc := "organizations/" + other.ID() + "/doc"
	for _, key := range []string{targetDoc, targetDoc, otherDoc} {
		_, e = objects.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(c.DocumentBucket), Key: aws.String(key), Body: bytes.NewReader([]byte("%PDF-synthetic"))})
		must(t, e)
	}
	// A tag mismatch must refuse destructive cleanup even when the resource name matches.
	if e = objects.DeleteTenantBucket(ctx, other.Bucket, target.ID()); e == nil {
		t.Fatal("accepted another organization's bucket")
	}
	_, e = objects.Client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{Bucket: aws.String(target.Bucket), Tagging: &s3types.Tagging{TagSet: []s3types.Tag{{Key: aws.String("Organization"), Value: aws.String(other.ID())}}}})
	must(t, e)
	if e = objects.DeleteTenantBucket(ctx, target.Bucket, target.ID()); e == nil {
		t.Fatal("accepted incorrect ownership tag")
	}
	_, e = objects.Client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{Bucket: aws.String(target.Bucket), Tagging: &s3types.Tagging{TagSet: []s3types.Tag{{Key: aws.String("Organization"), Value: aws.String(target.ID())}}}})
	must(t, e)
	requestOffboard(t, a, target.ID())
	readyOffboard(t, a, target.ID())
	must(t, NewWorker(a).Tick(ctx))
	o, e := r.Get(ctx, c.ControlTable, "ORG", target.ID())
	must(t, e)
	if s(o, "state") != "OFFBOARDED" {
		t.Fatalf("cleanup incomplete: %v", o)
	}
	exists, e := r.OwnedTableExists(ctx, target.Table, target.ID())
	must(t, e)
	if exists {
		t.Fatal("tenant table remains")
	}
	if _, e = objects.Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(target.Bucket)}); e == nil {
		t.Fatal("audio bucket remains")
	}
	versions, e := objects.Client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(c.DocumentBucket), Prefix: aws.String("organizations/" + target.ID() + "/")})
	must(t, e)
	if len(versions.Versions)+len(versions.DeleteMarkers) != 0 {
		t.Fatal("registration versions remain")
	}
	if _, e = objects.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(c.DocumentBucket), Key: aws.String(otherDoc)}); e != nil {
		t.Fatalf("other organization document deleted: %v", e)
	}
	student, e := r.Get(ctx, other.Table, "STUDENT", "keep")
	must(t, e)
	if student == nil {
		t.Fatal("other organization data deleted")
	}
	// Adapter retries after successful deletion are harmless.
	must(t, objects.DeleteTenantBucket(ctx, target.Bucket, target.ID()))
	must(t, r.DeleteTenantTable(ctx, target.Table, target.ID()))
}
