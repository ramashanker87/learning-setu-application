package identity

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

type TeamAccount struct {
	Sub     string
	Created bool
}
type TeamAccountCreator interface {
	CreateTeam(context.Context, string, string, string, string) (TeamAccount, error)
}

var ErrAccountUnavailable = errors.New("username unavailable for this team request")

// The immutable scope and request attributes bind a newly-created identity to
// one organization. Email is deliberately not marked verified: an administrator
// choosing a password does not prove that the member controls that mailbox.
func (c *CognitoInviter) CreateTeam(ctx context.Context, email, password, org, request string) (TeamAccount, error) {
	var result TeamAccount
	existing, err := c.client.AdminGetUser(ctx, &cognitoidentityprovider.AdminGetUserInput{UserPoolId: aws.String(c.pool), Username: aws.String(email)})
	var missing *types.UserNotFoundException
	if err == nil {
		if !existing.Enabled || accountAttribute(existing.UserAttributes, "custom:team_org") != org || accountAttribute(existing.UserAttributes, "custom:team_request") != request || accountAttribute(existing.UserAttributes, "email") != email {
			return result, ErrAccountUnavailable
		}
		if existing.UserStatus != types.UserStatusTypeForceChangePassword && existing.UserStatus != types.UserStatusTypeConfirmed {
			return result, ErrAccountUnavailable
		}
		result.Sub = accountAttribute(existing.UserAttributes, "sub")
		if result.Sub == "" {
			return result, fmt.Errorf("team account subject missing")
		}
		return result, nil // Retry a pending membership commit; never reset a password.
	}
	if !errors.As(err, &missing) {
		return result, err
	}
	created, err := c.client.AdminCreateUser(ctx, &cognitoidentityprovider.AdminCreateUserInput{
		UserPoolId: aws.String(c.pool), Username: aws.String(email), TemporaryPassword: aws.String(password), MessageAction: types.MessageActionTypeSuppress,
		UserAttributes: []types.AttributeType{
			{Name: aws.String("email"), Value: aws.String(email)},
			{Name: aws.String("email_verified"), Value: aws.String("false")},
			{Name: aws.String("custom:team_org"), Value: aws.String(org)},
			{Name: aws.String("custom:team_request"), Value: aws.String(request)},
		},
	})
	if err != nil {
		return result, err
	}
	if created.User == nil {
		return result, fmt.Errorf("team account missing")
	}
	result.Sub = accountAttribute(created.User.Attributes, "sub")
	result.Created = true
	if result.Sub == "" {
		return result, fmt.Errorf("team account subject missing")
	}
	return result, nil
}
