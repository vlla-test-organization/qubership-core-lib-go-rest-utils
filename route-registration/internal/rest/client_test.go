package rest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/stretchr/testify/assert"
)

func init () {
	serviceloader.Register(2, &security.DummyToken{})
}

func TestNewProgressiveTimeout(t *testing.T) {
	timeout := NewProgressiveTimeout(time.Second, 2, 8, 1)
	assert.NotNil(t, timeout)
	assert.Equal(t, 2, timeout.startMultiplier)
	assert.Equal(t, 8, timeout.endMultiplier)
}

func TestNewProgressiveTimeout_EndLessThanStart(t *testing.T) {
	assert.Panics(t, func() {
		NewProgressiveTimeout(time.Second, 8, 2, 1)
	})
}

func TestProgressiveTimeout_NextTimeoutValue(t *testing.T) {
	timeout := NewProgressiveTimeout(time.Second, 2, 3, 1)
	firstVal := timeout.NextTimeoutValue()
	assert.Equal(t, 2*time.Second, firstVal)
	secondVal := timeout.NextTimeoutValue()
	assert.Equal(t, 3*time.Second, secondVal)
	thirdVal := timeout.NextTimeoutValue()
	assert.Equal(t, 3*time.Second, thirdVal)
}

func TestProgressiveTimeout_Reset(t *testing.T) {
	timeout := NewProgressiveTimeout(time.Second, 2, 3, 1)
	firstVal := timeout.NextTimeoutValue()
	assert.Equal(t, 2*time.Second, firstVal)
	secondVal := timeout.NextTimeoutValue()
	assert.Equal(t, 3*time.Second, secondVal)
	timeout.Reset()
	thirdVal := timeout.NextTimeoutValue()
	assert.Equal(t, 2*time.Second, thirdVal)
}

func TestRetryManager_DoWithRetryWithoutPanic(t *testing.T) {
	timeout := NewProgressiveTimeout(time.Second, 2, 3, 1)
	valBeforeCall := timeout.currentMultiplierValue
	retryManager := NewRetryManager(timeout)
	retryManager.DoWithRetry(withoutPanic)
	assert.Equal(t, valBeforeCall, timeout.currentMultiplierValue)
}

type mockHandler struct {
	handle func(w http.ResponseWriter, r *http.Request)
}

func TestControlPlaneClient_SendRequest(t *testing.T) {
	var mockHandlers []mockHandler

	mockHandlers = append(mockHandlers, mockHandler{
		handle: successfulResponse,
	})
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	mockHandlers[0].handle(w, r)

	}))
	defer func() {
		testServer.Close()
		os.Clearenv()
	}()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	timeout := NewProgressiveTimeout(time.Second, 2, 3, 1)
	retryManager := NewRetryManager(timeout)
	cpClient := NewControlPlaneClient(testServer.URL, retryManager)
	requestToCP := testRegistrationRequest{reqBody: "test-request"}
	assert.NotPanics(t, func() {
		cpClient.SendRequest(&requestToCP)
	})
}

func withoutPanic() error {
	return nil
}

func successfulResponse(writer http.ResponseWriter, request *http.Request) {
	writer.WriteHeader(http.StatusOK)
}

/*
 Test implementation of RegistrationRequest for control plane tests
*/

type testRegistrationRequest struct {
	reqBody string
}

func (req *testRegistrationRequest) ApiVersion() ControlPlaneApiVersion {
	return V3
}

func (req *testRegistrationRequest) Payload() interface{} {
	return req.reqBody
}
