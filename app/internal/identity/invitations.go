package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"learning-setu/app/internal/config"
)

type InvitedAccount struct {
	Sub                 string
	InvitationRequested bool
}
type AccountInviter interface {
	Invite(context.Context, string) (InvitedAccount, error)
}
type cognitoAPI interface {
	AdminGetUser(context.Context, *cognitoidentityprovider.AdminGetUserInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminGetUserOutput, error)
	AdminCreateUser(context.Context, *cognitoidentityprovider.AdminCreateUserInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminCreateUserOutput, error)
}
type CognitoInviter struct {
	client cognitoAPI
	pool   string
}

func NewInviter(ctx context.Context, c config.Config) (*CognitoInviter, error) {
	if c.Local() || c.AuthMode != "cognito" {
		return nil, fmt.Errorf("managed invitations require production Cognito")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(c.Region))
	if err != nil {
		return nil, err
	}
	pool := c.Issuer[strings.LastIndex(c.Issuer, "/")+1:]
	if pool == "" || !strings.HasPrefix(pool, c.Region+"_") {
		return nil, fmt.Errorf("invalid tenant pool")
	}
	return &CognitoInviter{cognitoidentityprovider.NewFromConfig(cfg), pool}, nil
}
func accountAttribute(attrs []types.AttributeType, key string) string {
	for _, a := range attrs {
		if aws.ToString(a.Name) == key {
			return aws.ToString(a.Value)
		}
	}
	return ""
}
func (c *CognitoInviter) Invite(ctx context.Context, email string) (InvitedAccount, error) {
	var result InvitedAccount
	existing, err := c.client.AdminGetUser(ctx, &cognitoidentityprovider.AdminGetUserInput{UserPoolId: aws.String(c.pool), Username: aws.String(email)})
	var missing *types.UserNotFoundException
	create := errors.As(err, &missing)
	if err != nil && !create {
		return result, err
	}
	if !create {
		if accountAttribute(existing.UserAttributes, "custom:team_org") != "" {
			return result, fmt.Errorf("organization-scoped team account cannot become a founding administrator")
		}
		if !existing.Enabled || !strings.EqualFold(accountAttribute(existing.UserAttributes, "email"), email) {
			return result, fmt.Errorf("account unavailable or email mismatch")
		}
		// Never reset an established password, confirm an unverified account, or
		// move an email alias to another account to satisfy an application.
		if existing.UserStatus == types.UserStatusTypeConfirmed {
			if accountAttribute(existing.UserAttributes, "email_verified") != "true" {
				return result, fmt.Errorf("account email verification required")
			}
			result.Sub = accountAttribute(existing.UserAttributes, "sub")
			if result.Sub == "" {
				return result, fmt.Errorf("account subject missing")
			}
			return result, nil
		}
		if existing.UserStatus != types.UserStatusTypeForceChangePassword || accountAttribute(existing.UserAttributes, "email_verified") != "true" {
			return result, fmt.Errorf("account confirmation or recovery required")
		}
	}
	input := &cognitoidentityprovider.AdminCreateUserInput{UserPoolId: aws.String(c.pool), Username: aws.String(email), DesiredDeliveryMediums: []types.DeliveryMediumType{types.DeliveryMediumTypeEmail}}
	if create {
		input.UserAttributes = []types.AttributeType{{Name: aws.String("email"), Value: aws.String(email)}, {Name: aws.String("email_verified"), Value: aws.String("true")}}
	} else {
		input.MessageAction = types.MessageActionTypeResend
	}
	// Cognito generates and emails the password. It never enters our database,
	// queue, logs, API response, or process memory as a plaintext credential.
	created, err := c.client.AdminCreateUser(ctx, input)
	if err != nil {
		return result, err
	}
	if created.User == nil {
		return result, fmt.Errorf("missing invited account")
	}
	result.Sub = accountAttribute(created.User.Attributes, "sub")
	if result.Sub == "" {
		return result, fmt.Errorf("invited account subject missing")
	}
	result.InvitationRequested = true
	return result, nil
}
