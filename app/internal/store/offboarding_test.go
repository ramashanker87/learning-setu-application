package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"learning-setu/app/internal/config"
)

func TestTenantDeletionChecksOwnershipAndDisablesRecovery(t *testing.T) {
	org := "12345678-1234-1234-1234-123456789abc"
	name := "setu-test-demo-12345678123412341234123456789abc-123abc"
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong-owner", true: "owned"}[owned], func(t *testing.T) {
			deleted := false
			disabled := false
			destructive := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-amz-json-1.0")
				op := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "DynamoDB_20120810.")
				switch op {
				case "DescribeTable":
					if deleted {
						w.WriteHeader(400)
						_, _ = w.Write([]byte(`{"__type":"ResourceNotFoundException","message":"missing"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Table": map[string]any{"TableStatus": "ACTIVE", "TableArn": "arn:aws:dynamodb:ap-south-1:000000000000:table/" + name}})
				case "ListTagsOfResource":
					owner := "another-org"
					if owned {
						owner = org
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Tags": []map[string]string{{"Key": "Organization", "Value": owner}}})
				case "UpdateContinuousBackups":
					destructive = true
					var input map[string]any
					_ = json.NewDecoder(r.Body).Decode(&input)
					disabled = input["PointInTimeRecoverySpecification"].(map[string]any)["PointInTimeRecoveryEnabled"] == false
					_, _ = w.Write([]byte(`{}`))
				case "DescribeContinuousBackups":
					_, _ = w.Write([]byte(`{"ContinuousBackupsDescription":{"PointInTimeRecoveryDescription":{"PointInTimeRecoveryStatus":"DISABLED"}}}`))
				case "DeleteTable":
					destructive = true
					if !disabled {
						t.Error("deleted table before disabling PITR")
					}
					deleted = true
					_, _ = w.Write([]byte(`{}`))
				default:
					t.Errorf("unexpected operation %s", op)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			client := dynamodb.New(dynamodb.Options{Region: "ap-south-1", BaseEndpoint: aws.String(server.URL), Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")})
			d := &Dynamo{Client: client, Config: config.Config{Prefix: "setu-test", ControlTable: "control"}}
			err := d.DeleteTenantTable(context.Background(), name, org)
			if owned {
				if err != nil || !deleted {
					t.Fatalf("deletion failed: %v", err)
				}
			} else if err == nil || destructive {
				t.Fatalf("ownership protection failed: %v", err)
			}
		})
	}
}
