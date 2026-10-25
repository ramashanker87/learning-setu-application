package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"learning-setu/app/internal/config"
	"strings"
	"time"
)

type Dynamo struct {
	Client *dynamodb.Client
	Config config.Config
}

func New(ctx context.Context, c config.Config) (*Dynamo, error) {
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(c.Region)}
	if c.DynamoEndpoint != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("local", "local", "")))
	}
	ac, e := awsconfig.LoadDefaultConfig(ctx, opts...)
	if e != nil {
		return nil, e
	}
	return &Dynamo{Client: dynamodb.NewFromConfig(ac, func(o *dynamodb.Options) {
		if c.DynamoEndpoint != "" {
			o.BaseEndpoint = aws.String(c.DynamoEndpoint)
		}
	}), Config: c}, nil
}
func key(pk, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: pk}, "sk": &types.AttributeValueMemberS{Value: sk}}
}
func (d *Dynamo) Get(ctx context.Context, t, pk, sk string) (Doc, error) {
	r, e := d.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(t), Key: key(pk, sk), ConsistentRead: aws.Bool(true)})
	if e != nil {
		return nil, e
	}
	if len(r.Item) == 0 {
		return nil, nil
	}
	var item Doc
	e = attributevalue.UnmarshalMap(r.Item, &item)
	return item, e
}
func (d *Dynamo) Query(ctx context.Context, t, pk string, index bool) ([]Doc, error) {
	return d.QueryFields(ctx, t, pk, index, nil)
}
func (d *Dynamo) QueryFields(ctx context.Context, t, pk string, index bool, fields []string) ([]Doc, error) {
	out := []Doc{}
	k := "pk"
	input := &dynamodb.QueryInput{TableName: aws.String(t), ConsistentRead: aws.Bool(!index), KeyConditionExpression: aws.String("#k = :k"), ExpressionAttributeValues: map[string]types.AttributeValue{":k": &types.AttributeValueMemberS{Value: pk}}}
	if index {
		k = "gpk"
		input.IndexName = aws.String("lookup")
	}
	input.ExpressionAttributeNames = map[string]string{"#k": k}
	if len(fields) > 0 {
		paths := []string{}
		for i, field := range fields {
			parts := strings.Split(field, ".")
			for j, part := range parts {
				name := fmt.Sprintf("#p%d_%d", i, j)
				input.ExpressionAttributeNames[name] = part
				parts[j] = name
			}
			paths = append(paths, strings.Join(parts, "."))
		}
		input.ProjectionExpression = aws.String(strings.Join(paths, ", "))
	}
	for {
		r, e := d.Client.Query(ctx, input)
		if e != nil {
			return nil, e
		}
		for _, raw := range r.Items {
			var item Doc
			if e = attributevalue.UnmarshalMap(raw, &item); e != nil {
				return nil, e
			}
			out = append(out, item)
		}
		if len(r.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = r.LastEvaluatedKey
	}
	return out, nil
}
func (d *Dynamo) Commit(ctx context.Context, writes ...Write) error {
	if len(writes) == 0 {
		return nil
	}
	items := make([]types.TransactWriteItem, 0, len(writes))
	for _, w := range writes {
		condition := "attribute_not_exists(pk)"
		var values map[string]types.AttributeValue
		if rev := N(w.Item, "rev"); rev > 0 {
			condition = "rev = :rev"
			values = map[string]types.AttributeValue{":rev": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.0f", rev)}}
		}
		t := aws.String(w.Table)
		k := key(S(w.Item, "pk"), S(w.Item, "sk"))
		x := types.TransactWriteItem{}
		switch {
		case w.Check:
			x.ConditionCheck = &types.ConditionCheck{TableName: t, Key: k, ConditionExpression: &condition, ExpressionAttributeValues: values}
		case w.Delete:
			x.Delete = &types.Delete{TableName: t, Key: k, ConditionExpression: &condition, ExpressionAttributeValues: values}
		default:
			copy := Clone(w.Item)
			copy["rev"] = N(w.Item, "rev") + 1
			raw, e := attributevalue.MarshalMap(copy)
			if e != nil {
				return e
			}
			x.Put = &types.Put{TableName: t, Item: raw, ConditionExpression: &condition, ExpressionAttributeValues: values}
		}
		items = append(items, x)
	}
	_, e := d.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	var conflict *types.TransactionCanceledException
	if errors.As(e, &conflict) {
		return ErrConflict
	}
	return e
}
func (d *Dynamo) EnsureTable(ctx context.Context, name, org string) error {
	existing, e := d.Client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: &name})
	var missing *types.ResourceNotFoundException
	if errors.As(e, &missing) {
		if d.Config.DynamoEndpoint == "" {
			n := 0
			p := dynamodb.NewListTablesPaginator(d.Client, &dynamodb.ListTablesInput{})
			for p.HasMorePages() {
				r, e := p.NextPage(ctx)
				if e != nil {
					return e
				}
				n += len(r.TableNames)
			}
			if n >= d.Config.TableQuota-5 {
				return fmt.Errorf("table quota headroom exhausted")
			}
		}
		defs := []types.AttributeDefinition{}
		for _, k := range []string{"pk", "sk", "gpk", "gsk"} {
			defs = append(defs, types.AttributeDefinition{AttributeName: aws.String(k), AttributeType: types.ScalarAttributeTypeS})
		}
		schema := func(a, b string) []types.KeySchemaElement {
			return []types.KeySchemaElement{{AttributeName: aws.String(a), KeyType: types.KeyTypeHash}, {AttributeName: aws.String(b), KeyType: types.KeyTypeRange}}
		}
		_, e = d.Client.CreateTable(ctx, &dynamodb.CreateTableInput{TableName: &name, BillingMode: types.BillingModePayPerRequest, AttributeDefinitions: defs, KeySchema: schema("pk", "sk"), GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{IndexName: aws.String("lookup"), KeySchema: schema("gpk", "gsk"), Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}}}, SSESpecification: &types.SSESpecification{Enabled: aws.Bool(true)}, Tags: []types.Tag{{Key: aws.String("Organization"), Value: &org}}})
		var busy *types.ResourceInUseException
		if e != nil && !errors.As(e, &busy) {
			return e
		}
	} else if e != nil {
		return e
	}
	if e = dynamodb.NewTableExistsWaiter(d.Client).Wait(ctx, &dynamodb.DescribeTableInput{TableName: &name}, 2*time.Minute); e != nil {
		return e
	}
	existing, e = d.Client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: &name})
	if e != nil {
		return e
	}
	valid := len(existing.Table.KeySchema) == 2 && len(existing.Table.GlobalSecondaryIndexes) == 1 && aws.ToString(existing.Table.GlobalSecondaryIndexes[0].IndexName) == "lookup"
	for _, k := range existing.Table.KeySchema {
		if k.KeyType == types.KeyTypeHash && aws.ToString(k.AttributeName) != "pk" {
			valid = false
		}
		if k.KeyType == types.KeyTypeRange && aws.ToString(k.AttributeName) != "sk" {
			valid = false
		}
	}
	if valid {
		index := existing.Table.GlobalSecondaryIndexes[0]
		valid = len(index.KeySchema) == 2 && index.Projection != nil && index.Projection.ProjectionType == types.ProjectionTypeAll
		for _, k := range index.KeySchema {
			if k.KeyType == types.KeyTypeHash && aws.ToString(k.AttributeName) != "gpk" {
				valid = false
			}
			if k.KeyType == types.KeyTypeRange && aws.ToString(k.AttributeName) != "gsk" {
				valid = false
			}
		}
	}
	if !valid {
		return fmt.Errorf("table schema collision")
	}
	if d.Config.DynamoEndpoint == "" {
		tags, e := d.Client.ListTagsOfResource(ctx, &dynamodb.ListTagsOfResourceInput{ResourceArn: existing.Table.TableArn})
		if e != nil {
			return e
		}
		owned := false
		for _, t := range tags.Tags {
			if aws.ToString(t.Key) == "Organization" && aws.ToString(t.Value) == org {
				owned = true
			}
		}
		if !owned {
			return fmt.Errorf("table ownership collision")
		}
		_, e = d.Client.UpdateContinuousBackups(ctx, &dynamodb.UpdateContinuousBackupsInput{TableName: &name, PointInTimeRecoverySpecification: &types.PointInTimeRecoverySpecification{PointInTimeRecoveryEnabled: aws.Bool(true)}})
		if e != nil {
			return e
		}
		// Do not activate the organization while production recovery protection is pending.
		ready := false
		for attempt := 0; attempt < 30; attempt++ {
			backup, err := d.Client.DescribeContinuousBackups(ctx, &dynamodb.DescribeContinuousBackupsInput{TableName: &name})
			if err != nil {
				return err
			}
			if backup.ContinuousBackupsDescription != nil && backup.ContinuousBackupsDescription.PointInTimeRecoveryDescription != nil && backup.ContinuousBackupsDescription.PointInTimeRecoveryDescription.PointInTimeRecoveryStatus == types.PointInTimeRecoveryStatusEnabled {
				ready = true
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		if !ready {
			return fmt.Errorf("point-in-time recovery not ready")
		}
	}
	return nil
}
