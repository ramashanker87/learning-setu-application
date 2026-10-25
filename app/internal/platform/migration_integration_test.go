package platform

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dt "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"learning-setu/app/internal/config"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrationAgainstLocalEmulators(t *testing.T) {
	if os.Getenv("SETU_INTEGRATION") != "1" {
		t.Skip("set SETU_INTEGRATION=1 with local Compose running")
	}
	ctx := context.Background()
	c, e := config.From(func(k string) string {
		return map[string]string{"APP_ENV": "test", "AUTH_MODE": "local", "LOCAL_AUTH_SECRET": "synthetic-local-only-secret-never-use-in-production", "TRANSCRIPTION_PROVIDER": "mock", "DYNAMODB_ENDPOINT": "http://localhost:8100", "S3_ENDPOINT": "http://localhost:9000", "S3_PUBLIC_ENDPOINT": "http://localhost:9000"}[k]
	})
	must(t, e)
	r, e := store.New(ctx, c)
	must(t, e)
	objects, e := storage.New(ctx, c)
	must(t, e)
	a := &Service{Config: c, Repo: r, Objects: objects}
	orgs, e := r.Query(ctx, c.ControlTable, "OWNER#beta-admin", true)
	must(t, e)
	if len(orgs) != 1 {
		t.Fatal("seed Beta organization required")
	}
	org := orgs[0]
	suffix := uuid.NewString()
	sourceTable := "migration-source-" + suffix
	sourceBucket := "migration-source-" + suffix
	_, e = r.Client.CreateTable(ctx, &dynamodb.CreateTableInput{TableName: &sourceTable, BillingMode: dt.BillingModePayPerRequest, AttributeDefinitions: []dt.AttributeDefinition{{AttributeName: aws.String("studentId"), AttributeType: dt.ScalarAttributeTypeS}}, KeySchema: []dt.KeySchemaElement{{AttributeName: aws.String("studentId"), KeyType: dt.KeyTypeHash}}})
	must(t, e)
	must(t, dynamodb.NewTableExistsWaiter(r.Client).Wait(ctx, &dynamodb.DescribeTableInput{TableName: &sourceTable}, time.Minute))
	must(t, objects.EnsureBucket(ctx, sourceBucket, "synthetic-migration"))
	audio := []byte("synthetic legacy recording; migration does not rescore")
	_, e = objects.Put(ctx, sourceBucket, "opaque-recording", "audio/wav", audio)
	must(t, e)
	studentID, assessmentID := uuid.NewString(), uuid.NewString()
	row := Doc{"studentId": studentID, "schoolId": "explicit-school", "name": "Synthetic migration learner", "age": 9, "classId": "4", "assessments": []any{Doc{"assessmentId": assessmentID, "audioObjectKey": "opaque-recording", "audioContentType": "audio/wav", "score": 88}}}
	raw, e := attributevalue.MarshalMap(row)
	must(t, e)
	_, e = r.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: &sourceTable, Item: raw})
	must(t, e)
	dir := t.TempDir()
	mappingPath, manifestPath := filepath.Join(dir, "mapping.json"), filepath.Join(dir, "manifest.json")
	mapping := Doc{"organizationId": org["id"], "sourceTable": sourceTable, "sourceBucket": sourceBucket, "sourceSchoolId": "wrong-school", "studentIds": []string{studentID}}
	save := func() { data, _ := json.Marshal(mapping); must(t, os.WriteFile(mappingPath, data, 0600)) }
	save()
	args := []string{"--mapping", mappingPath, "--manifest", manifestPath}
	if Migrate(ctx, a, args) == nil {
		t.Fatal("accepted wrong source school")
	}
	mapping["sourceSchoolId"] = "explicit-school"
	save()
	must(t, Migrate(ctx, a, args))
	targetTable := s(m(org, "resources"), "table")
	target, e := r.Get(ctx, targetTable, "STUDENT", studentID)
	must(t, e)
	if target != nil {
		t.Fatal("dry run wrote data")
	}
	must(t, Migrate(ctx, a, append(args, "--execute")))
	must(t, Migrate(ctx, a, append(args, "--execute")))
	target, e = r.Get(ctx, targetTable, "STUDENT", studentID)
	must(t, e)
	if s(m(target, "migration"), "state") != "VALIDATED" || s(m(target, "consent"), "status") != "PENDING_VERIFICATION" {
		t.Fatal("migration state invalid")
	}
	assessment, e := r.Get(ctx, targetTable, "ASSESSMENT", assessmentID)
	must(t, e)
	copied, e := objects.Read(ctx, s(m(org, "resources"), "bucket"), s(assessment, "key"), s(assessment, "version"))
	must(t, e)
	if string(copied) != string(audio) {
		t.Fatal("audio copy changed")
	}
	original, e := r.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &sourceTable, Key: map[string]dt.AttributeValue{"studentId": &dt.AttributeValueMemberS{Value: studentID}}})
	must(t, e)
	if len(original.Item) == 0 {
		t.Fatal("source was changed")
	}
}
