package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dt "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
	"os"
	"regexp"
)

func digest(v any) string { data, _ := json.Marshal(v); return hash(string(data)) }

// Migrate copies only explicitly mapped POC records. No source mutation or automatic cutover.
func Migrate(ctx context.Context, a *Service, args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	mappingPath := flags.String("mapping", "", "explicit mapping JSON")
	manifestPath := flags.String("manifest", "", "reviewable plan JSON")
	execute := flags.Bool("execute", false, "execute previously reviewed plan")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if *mappingPath == "" || *manifestPath == "" {
		return fmt.Errorf("--mapping and --manifest are required")
	}
	raw, e := os.ReadFile(*mappingPath)
	if e != nil {
		return e
	}
	var mapping struct {
		OrganizationID string   `json:"organizationId"`
		SourceTable    string   `json:"sourceTable"`
		SourceBucket   string   `json:"sourceBucket"`
		SourceSchoolID string   `json:"sourceSchoolId"`
		StudentIDs     []string `json:"studentIds"`
	}
	if e = json.Unmarshal(raw, &mapping); e != nil {
		return e
	}
	if mapping.OrganizationID == "" || mapping.SourceTable == "" || mapping.SourceBucket == "" || mapping.SourceSchoolID == "" || len(mapping.StudentIDs) == 0 {
		return fmt.Errorf("explicit organization, source resources, school and student IDs required")
	}
	repo, ok := a.Repo.(*store.Dynamo)
	if !ok {
		return fmt.Errorf("migration requires DynamoDB adapter")
	}
	objects, ok := a.Objects.(*storage.S3)
	if !ok {
		return fmt.Errorf("migration requires S3 adapter")
	}
	org, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", mapping.OrganizationID)
	if e != nil {
		return e
	}
	table, bucket := s(m(org, "resources"), "table"), s(m(org, "resources"), "bucket")
	if s(org, "state") != "ACTIVE" || table == mapping.SourceTable || bucket == mapping.SourceBucket {
		return fmt.Errorf("active destination with different source resources required")
	}
	ids := regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)
	rows := []Doc{}
	planned := []Doc{}
	for _, id := range mapping.StudentIDs {
		if !ids.MatchString(id) {
			return fmt.Errorf("student ID requires remediation")
		}
		r, e := repo.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &mapping.SourceTable, Key: map[string]dt.AttributeValue{"studentId": &dt.AttributeValueMemberS{Value: id}}, ConsistentRead: aws.Bool(true)})
		if e != nil {
			return e
		}
		var row Doc
		if e = attributevalue.UnmarshalMap(r.Item, &row); e != nil {
			return e
		}
		if s(row, "schoolId") != mapping.SourceSchoolID {
			return fmt.Errorf("source school mismatch")
		}
		rows = append(rows, row)
		planned = append(planned, Doc{"studentId": id, "sourceHash": digest(row)})
	}
	plan := Doc{"mappingHash": digest(mapping), "organizationId": mapping.OrganizationID, "targetTable": table, "targetBucket": bucket, "students": planned}
	if !*execute {
		out, _ := json.MarshalIndent(plan, "", "  ")
		if e = os.WriteFile(*manifestPath, out, 0600); e != nil {
			return e
		}
		fmt.Println("Dry run saved. Review the manifest before adding --execute.")
		return nil
	}
	raw, e = os.ReadFile(*manifestPath)
	if e != nil {
		return e
	}
	var approved Doc
	if e = json.Unmarshal(raw, &approved); e != nil {
		return e
	}
	if digest(plan) != digest(approved) {
		return fmt.Errorf("mapping or source changed; review a new dry run")
	}
	for _, row := range rows {
		id := s(row, "studentId")
		sourceHash := digest(row)
		student, e := a.Repo.Get(ctx, table, "STUDENT", id)
		if e != nil {
			return e
		}
		if student != nil && s(m(student, "migration"), "sourceHash") != sourceHash {
			return fmt.Errorf("destination student collision")
		}
		if student == nil {
			class := s(row, "className")
			if class == "" {
				class = s(row, "classId")
			}
			student = Doc{"pk": "STUDENT", "sk": id, "orgId": mapping.OrganizationID, "name": row["name"], "age": row["age"], "className": class, "createdAt": now(), "expiresAt": millis() + n(org, "retentionDays")*86400000, "consent": Doc{"status": "PENDING_VERIFICATION"}, "migration": Doc{"sourceHash": sourceHash, "state": "COPYING"}}
			if e = a.Repo.Commit(ctx, store.Put(table, student)); e != nil {
				return e
			}
		}
		assessments, _ := row["assessments"].([]any)
		for _, value := range assessments {
			legacy, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid legacy assessment")
			}
			aid := s(legacy, "assessmentId")
			if !ids.MatchString(aid) {
				return fmt.Errorf("assessment ID requires remediation")
			}
			target, e := a.Repo.Get(ctx, table, "ASSESSMENT", aid)
			if e != nil {
				return e
			}
			if target != nil {
				if s(m(target, "migration"), "sourceHash") != digest(legacy) {
					return fmt.Errorf("destination assessment collision")
				}
				continue
			}
			recording := digest(Doc{"sourceHash": sourceHash, "id": aid})[:32]
			key := "students/" + id + "/assessments/" + aid + "/" + recording
			version := ""
			if sourceKey := s(legacy, "audioObjectKey"); sourceKey != "" {
				head, e := objects.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &mapping.SourceBucket, Key: &sourceKey})
				if e != nil {
					return e
				}
				source, e := objects.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: &mapping.SourceBucket, Key: &sourceKey, VersionId: head.VersionId, IfMatch: head.ETag})
				if e != nil {
					return e
				}
				data, e := io.ReadAll(io.LimitReader(source.Body, 26*1024*1024))
				source.Body.Close()
				if len(data) > 25*1024*1024 {
					return fmt.Errorf("source audio exceeds migration limit")
				}
				if e != nil {
					return e
				}
				checksum := hash(string(data))
				contentType := s(legacy, "audioContentType")
				if contentType == "" {
					contentType = "audio/wav"
				}
				existing, e := objects.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &bucket, Key: &key})
				if e == nil {
					if existing.Metadata["sha256"] != checksum {
						return fmt.Errorf("destination audio collision")
					}
					version = aws.ToString(existing.VersionId)
				} else {
					r, e := objects.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: &bucket, Key: &key, ContentType: &contentType, Body: bytes.NewReader(data), IfNoneMatch: aws.String("*"), Metadata: map[string]string{"sha256": checksum}})
					if e != nil {
						return e
					}
					version = aws.ToString(r.VersionId)
				}
				copied, e := objects.Read(ctx, bucket, key, version)
				if e != nil {
					return e
				}
				if hash(string(copied)) != checksum {
					return fmt.Errorf("copy validation failed")
				}
			}
			target = Doc{"pk": "ASSESSMENT", "sk": aid, "orgId": mapping.OrganizationID, "studentId": id, "recordingId": recording, "key": key, "version": version, "state": "IMPORTED_REVIEW_REQUIRED", "legacy": legacy, "scoringVersion": "legacy-poc", "provider": "legacy-import", "gpk": "STUDENT#" + id, "gsk": aid, "migration": Doc{"sourceHash": digest(legacy)}}
			if e = a.Repo.Commit(ctx, store.Put(table, target)); e != nil {
				return e
			}
		}
		student, e = a.Repo.Get(ctx, table, "STUDENT", id)
		if e != nil {
			return e
		}
		m(student, "migration")["state"] = "VALIDATED"
		if e = a.Repo.Commit(ctx, store.Put(table, student)); e != nil {
			return e
		}
	}
	fmt.Println("Copy validated; source unchanged and no cutover performed.")
	return nil
}
