package platform

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResetCognitoEndpointAllowedByCSP(t *testing.T) {
	a, _, _ := fixture(t)
	a.Config.Issuer = "https://cognito-idp.ap-south-1.amazonaws.com/synthetic"
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/api/config", nil))
	policy := response.Header().Get("Content-Security-Policy")
	connect := strings.Split(strings.Split(policy, "connect-src ")[1], ";")[0]
	if !strings.Contains(connect, "https://cognito-idp.ap-south-1.amazonaws.com") {
		t.Fatal("reset API blocked by browser policy")
	}
	if !strings.Contains(response.Body.String(), `"cognitoIssuer":"https://cognito-idp.ap-south-1.amazonaws.com/synthetic"`) {
		t.Fatal("issuer missing from public configuration")
	}
}
