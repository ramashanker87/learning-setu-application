package identity

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"testing"
)

type invitationAPI struct {
	existing *cognitoidentityprovider.AdminGetUserOutput
	input    *cognitoidentityprovider.AdminCreateUserInput
	err      error
}

func (f *invitationAPI) AdminGetUser(context.Context, *cognitoidentityprovider.AdminGetUserInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminGetUserOutput, error) {
	if f.existing == nil {
		return nil, &types.UserNotFoundException{}
	}
	return f.existing, nil
}
func (f *invitationAPI) AdminCreateUser(_ context.Context, in *cognitoidentityprovider.AdminCreateUserInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminCreateUserOutput, error) {
	f.input = in
	if f.err != nil {
		return nil, f.err
	}
	return &cognitoidentityprovider.AdminCreateUserOutput{User: &types.UserType{Attributes: []types.AttributeType{{Name: aws.String("sub"), Value: aws.String("subject")}}}}, nil
}
func testAccount(status types.UserStatusType, verified string) *cognitoidentityprovider.AdminGetUserOutput {
	return &cognitoidentityprovider.AdminGetUserOutput{Enabled: true, UserStatus: status, UserAttributes: []types.AttributeType{{Name: aws.String("sub"), Value: aws.String("subject")}, {Name: aws.String("email"), Value: aws.String("user@example.test")}, {Name: aws.String("email_verified"), Value: aws.String(verified)}}}
}
func TestManagedInvitationNeverHandlesPlaintextPassword(t *testing.T) {
	fake := &invitationAPI{}
	c := &CognitoInviter{client: fake, pool: "tenant-pool"}
	result, err := c.Invite(context.Background(), "user@example.test")
	if err != nil || !result.InvitationRequested || result.Sub != "subject" {
		t.Fatalf("%+v %v", result, err)
	}
	if fake.input.TemporaryPassword != nil || fake.input.MessageAction != "" || len(fake.input.DesiredDeliveryMediums) != 1 || fake.input.DesiredDeliveryMediums[0] != types.DeliveryMediumTypeEmail {
		t.Fatal("expected provider-generated password delivered by email")
	}
	if aws.ToString(fake.input.UserPoolId) != "tenant-pool" || accountAttribute(fake.input.UserAttributes, "email") != "user@example.test" {
		t.Fatal("incorrect pool/recipient")
	}
}
func TestExistingAccountPasswordIsNeverReset(t *testing.T) {
	fake := &invitationAPI{existing: testAccount(types.UserStatusTypeConfirmed, "true")}
	c := &CognitoInviter{client: fake, pool: "tenant-pool"}
	result, err := c.Invite(context.Background(), "user@example.test")
	if err != nil || result.InvitationRequested || result.Sub != "subject" || fake.input != nil {
		t.Fatalf("existing account changed: %+v %v", result, err)
	}
}
func TestTemporaryAccountCanBeResentButUnverifiedAccountsCannot(t *testing.T) {
	for _, status := range []types.UserStatusType{types.UserStatusTypeForceChangePassword, types.UserStatusTypeUnconfirmed, types.UserStatusTypeResetRequired, types.UserStatusTypeConfirmed} {
		for _, verified := range []string{"true", "false"} {
			fake := &invitationAPI{existing: testAccount(status, verified)}
			c := &CognitoInviter{client: fake, pool: "tenant-pool"}
			_, err := c.Invite(context.Background(), "user@example.test")
			allowed := verified == "true" && (status == types.UserStatusTypeForceChangePassword || status == types.UserStatusTypeConfirmed)
			if (err == nil) != allowed {
				t.Fatalf("status=%s verified=%s err=%v", status, verified, err)
			}
			if status == types.UserStatusTypeForceChangePassword && allowed {
				if fake.input == nil || fake.input.MessageAction != types.MessageActionTypeResend || fake.input.TemporaryPassword != nil || len(fake.input.UserAttributes) != 0 {
					t.Fatal("unsafe resend")
				}
			} else if fake.input != nil {
				t.Fatal("unexpected credential mutation")
			}
		}
	}
}
