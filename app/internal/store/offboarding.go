package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func (d *Dynamo) OwnedTableExists(ctx context.Context, name, org string) (bool, error) {
	if org == "" || name == d.Config.ControlTable || !strings.HasPrefix(name, d.Config.Prefix+"-") || !strings.Contains(name, "-"+strings.ReplaceAll(org, "-", "")+"-") {
		return false, fmt.Errorf("unsafe tenant table mapping")
	}
	table, err := d.Client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: &name})
	var missing *types.ResourceNotFoundException
	if errors.As(err, &missing) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// DynamoDB Local does not implement table tags; real AWS always checks them.
	if d.Config.DynamoEndpoint == "" {
		tags, err := d.Client.ListTagsOfResource(ctx, &dynamodb.ListTagsOfResourceInput{ResourceArn: table.Table.TableArn})
		if err != nil {
			return false, err
		}
		owned := false
		for _, tag := range tags.Tags {
			owned = owned || (aws.ToString(tag.Key) == "Organization" && aws.ToString(tag.Value) == org)
		}
		if !owned {
			return false, fmt.Errorf("table ownership collision")
		}
	}
	return true, nil
}

func (d *Dynamo) DeleteTenantTable(ctx context.Context, name, org string) error {
	exists, err := d.OwnedTableExists(ctx, name, org)
	if err != nil || !exists {
		return err
	}
	table, err := d.Client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: &name})
	var missing *types.ResourceNotFoundException
	if errors.As(err, &missing) {
		return nil
	}
	if err != nil {
		return err
	}
	if table.Table.TableStatus != types.TableStatusDeleting {
		// Offboarding intentionally removes recovery points for this tenant.
		// Disable PITR first so DeleteTable does not create a new full-data
		// system backup. Separately managed backups remain operator follow-up.
		if d.Config.DynamoEndpoint == "" {
			_, err = d.Client.UpdateContinuousBackups(ctx, &dynamodb.UpdateContinuousBackupsInput{
				TableName: &name, PointInTimeRecoverySpecification: &types.PointInTimeRecoverySpecification{PointInTimeRecoveryEnabled: aws.Bool(false)},
			})
			if err != nil {
				return err
			}
			disabled := false
			for attempt := 0; attempt < 30; attempt++ {
				result, e := d.Client.DescribeContinuousBackups(ctx, &dynamodb.DescribeContinuousBackupsInput{TableName: &name})
				if e != nil {
					return e
				}
				if result.ContinuousBackupsDescription != nil && result.ContinuousBackupsDescription.PointInTimeRecoveryDescription != nil &&
					result.ContinuousBackupsDescription.PointInTimeRecoveryDescription.PointInTimeRecoveryStatus == types.PointInTimeRecoveryStatusDisabled {
					disabled = true
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Second):
				}
			}
			if !disabled {
				return fmt.Errorf("waiting for recovery protection removal")
			}
		}
		if _, err = d.Client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: &name}); err != nil && !errors.As(err, &missing) {
			return err
		}
	}
	return dynamodb.NewTableNotExistsWaiter(d.Client).Wait(ctx, &dynamodb.DescribeTableInput{TableName: &name}, 3*time.Minute)
}
