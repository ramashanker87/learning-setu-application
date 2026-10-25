package identity

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"testing"
)

func TestTeamAccountUsesSuppressedTemporaryPasswordAndImmutableScope(t *testing.T) {
	f := &invitationAPI{}
	c := &CognitoInviter{client: f, pool: "tenant"}
	result, err := c.CreateTeam(context.Background(), "user@example.test", "Temporary12345!", "org", "request")
	if err != nil || !result.Created || result.Sub != "subject" {
		t.Fatal(result, err)
	}
	if f.input.MessageAction != types.MessageActionTypeSuppress || aws.ToString(f.input.TemporaryPassword) != "Temporary12345!" {
		t.Fatal("incorrect password delivery")
	}
	if accountAttribute(f.input.UserAttributes, "email_verified") != "false" || accountAttribute(f.input.UserAttributes, "custom:team_org") != "org" || accountAttribute(f.input.UserAttributes, "custom:team_request") != "request" {
		t.Fatal("missing isolation attributes")
	}
}
func TestTeamCreationCannotTakeOverExistingAccount(t *testing.T) {
	for _, scope := range []string{"", "another-org", "org"} {
		f := &invitationAPI{existing: testAccount(types.UserStatusTypeConfirmed, "true")}
		f.existing.UserAttributes = append(f.existing.UserAttributes, types.AttributeType{Name: aws.String("custom:team_org"), Value: aws.String(scope)}, types.AttributeType{Name: aws.String("custom:team_request"), Value: aws.String("original-request")})
		c := &CognitoInviter{client: f, pool: "tenant"}
		if _, err := c.CreateTeam(context.Background(), "user@example.test", "Temporary12345!", "org", "different-request"); err != ErrAccountUnavailable || f.input != nil {
			t.Fatal("account takeover allowed")
		}
	}
}
func TestTeamPendingRetryDoesNotResetPassword(t *testing.T) {
	f := &invitationAPI{existing: testAccount(types.UserStatusTypeForceChangePassword, "false")}
	f.existing.UserAttributes = append(f.existing.UserAttributes, types.AttributeType{Name: aws.String("custom:team_org"), Value: aws.String("org")}, types.AttributeType{Name: aws.String("custom:team_request"), Value: aws.String("request")})
	c := &CognitoInviter{client: f, pool: "tenant"}
	result, err := c.CreateTeam(context.Background(), "user@example.test", "IgnoredRetry123!", "org", "request")
	if err != nil || result.Created || result.Sub != "subject" || f.input != nil {
		t.Fatal("pending account reset", err)
	}
	if _, err = c.Invite(context.Background(), "user@example.test"); err == nil {
		t.Fatal("team identity became founder")
	}
}
