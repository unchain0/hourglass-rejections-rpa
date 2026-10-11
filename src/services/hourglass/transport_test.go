package hourglass

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
)

// TestClient_SetTransport verifies SetTransport replaces the underlying transport.
func TestClient_SetTransport(t *testing.T) {
	t.Helper()

	client := NewClient()
	original := client.GetTransport()

	// Replace with a custom transport
	custom := &roundTripperStub{}
	client.SetTransport(custom)

	assert.Same(t, custom, client.GetTransport())
	assert.NotSame(t, original, client.GetTransport())
}

// TestClient_GetTransport verifies GetTransport returns the current transport.
func TestClient_GetTransport(t *testing.T) {
	t.Helper()

	client := NewClient()
	transport := client.GetTransport()
	assert.NotNil(t, transport)

	// After SetTransport, GetTransport should return the new one
	custom := &roundTripperStub{}
	client.SetTransport(custom)
	assert.Same(t, custom, client.GetTransport())
}

// TestClient_EnsureValidTokens verifies EnsureValidTokens disabled and enabled paths.
func TestClient_EnsureValidTokens(t *testing.T) {
	t.Helper()

	client := NewClient()
	tokens, err := client.EnsureValidTokens()
	assert.Error(t, err)
	assert.Nil(t, tokens)
	assert.Equal(t, "webauthn authentication not enabled", err.Error())

	client.useWebAuthn = true
	client.tokenManager = nil
	tokens, err = client.EnsureValidTokens()
	assert.Error(t, err)
	assert.Nil(t, tokens)

	want := &webauthn.AuthTokens{HGLogin: "hg", XSRFToken: "xsrf"}
	client.tokenManager = &stubTokenManager{current: want}
	tokens, err = client.EnsureValidTokens()
	assert.NoError(t, err)
	assert.Equal(t, want, tokens)
}

// roundTripperStub is a simple RoundTripper for testing transport replacement.
type roundTripperStub struct {
	roundTripCalled bool
}

func (r *roundTripperStub) RoundTrip(req *http.Request) (*http.Response, error) {
	r.roundTripCalled = true
	return nil, nil
}
