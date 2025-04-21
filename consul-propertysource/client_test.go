package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/stretchr/testify/assert"
)

func TestConsul_getSecretIdByToken(t *testing.T) {
	timeStr := time.Now().Format(time.RFC3339)
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		reqBody, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		var reqBodyMap map[string]interface{}
		err = json.Unmarshal(reqBody, &reqBodyMap)
		assert.Equal(t, "application/json", reqBodyMap["Accept"])

		res.WriteHeader(http.StatusOK)

		res.Write([]byte("{\"SecretID\": \"anonymous\", \"ExpirationTime\": \"" + timeStr + "\"}"))
	}))
	defer func() { testServer.Close() }()

	provider := newProvider(ProviderConfig{
		Address:          testServer.URL,
		Namespace:        "test-namespace",
		Paths:            DefaultPathsFor("control-plane", "cloudbss-kube-core-demo-2"),
		MicroserviceName: "test-ms",
		Ctx:              context.Background(),
	})

	token, expTime, err := provider.client.getSecretIdByToken("")
	assert.NoError(t, err)
	assert.Equal(t, "anonymous", token)
	assert.Equal(t, timeStr, expTime.Format(time.RFC3339))
}

func TestClient_subscribeFor(t *testing.T) {
	try := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		try++
		if try == 1 {
			res.WriteHeader(http.StatusInternalServerError) // must retry on error
		}
		res.WriteHeader(http.StatusOK)
		p := make([]*api.KVPair, 1)
		p[0] = &api.KVPair{
			Key:         "test-key",
			CreateIndex: 0,
			Value:       []byte("test-value"),
		}
		resp, err := json.Marshal(p)
		assert.NoError(t, err)
		res.Write(resp)
	}))
	defer func() { testServer.Close() }()

	ctx, done := context.WithCancel(context.Background())
	client := NewClient(ClientConfig{
		Address:   testServer.URL,
		Namespace: "test",
		Ctx:       ctx,
	})

	client.token = &ClientToken{}
	client.token.val.Store("test-token")
	var wg sync.WaitGroup
	ch := make(chan map[string]interface{})
	client.subscribeFor("/", 1, func(event interface{}, err error) {
		ch <- map[string]interface{}{"test-key": "test-value"}
	})
	val := <-ch
	assert.Equal(t, "test-value", val["test-key"])
	done()
	assert.Eventuallyf(t, func() bool { wg.Wait(); return true }, 5*time.Second, 100*time.Millisecond, "must stop on done")
}

func TestClient_subscribeFor_no_path(t *testing.T) {
	callCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		callCount++
		res.WriteHeader(http.StatusNotFound)
	}))
	defer func() { testServer.Close() }()

	ctx, _ := context.WithCancel(context.Background())
	client := NewClient(ClientConfig{
		Address:   testServer.URL,
		Namespace: "test",
		Ctx:       ctx,
	})

	client.token = &ClientToken{}
	client.token.val.Store("test-token")
	var wg sync.WaitGroup
	wg.Add(1)
	client.subscribeFor("/", 1, func(event interface{}, err error) {})
	go func() {
		time.Sleep(time.Second)
		wg.Done()
	}()
	wg.Wait()
	assert.NotZero(t, callCount)
	assert.LessOrEqual(t, callCount, 1)
}

func TestClient_Login(t *testing.T) {
	testSecretId := "anonymous"
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	timeStr := time.Now().Add(5 * time.Minute).Format(time.RFC3339)
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
		res.Write([]byte("{\"SecretID\": \"" + testSecretId + "\", \"ExpirationTime\": \"" + timeStr + "\"}"))
	}))
	defer func() { testServer.Close() }()
	client := NewClient(ClientConfig{
		Address:   testServer.URL,
		Namespace: "test",
		Ctx:       context.Background(),
	})
	assert.Nil(t, client.token)
	err := client.Login()
	assert.NoError(t, err)
	assert.Equal(t, timeStr, client.token.expirationTime.Format(time.RFC3339))
	assert.Equal(t, testSecretId, client.token.val.Load())
}

func TestClient_LoginError(t *testing.T) {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusForbidden)
		res.Write([]byte("Permission denied"))
	}))
	defer func() { testServer.Close() }()
	client := NewClient(ClientConfig{
		Address:   testServer.URL,
		Namespace: "test",
		Ctx:       context.Background(),
	})
	assert.Nil(t, client.token)
	err := client.Login()
	assert.NotNil(t, err)
	expectedError := fmt.Sprintf("failed to get consul secret ID by token: non 200 response from Consul to request '%s/v1/acl/login': 403 - Permission denied", testServer.URL)
	assert.Equal(t, expectedError, err.Error())
}

func TestClient_LoginEmptyToken(t *testing.T) {
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	client := NewClient(ClientConfig{
		Address:   "test:8500",
		Namespace: "test",
		Ctx:       context.Background(),
		tokenProvider: func() (string, error) {
			return "", nil
		},
	})
	assert.Nil(t, client.token)
	err := client.Login()
	assert.NoError(t, err)
	assert.Nil(t, client.token.val.Load())
}

func TestClient_startWatchSecretId(t *testing.T) {
	var wg sync.WaitGroup
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	testSecretId := "anonymous"
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
		expTime := time.Now().Add(thresholdBeforeUpdate).Add(10 * time.Second).Format(time.RFC3339)
		res.Write([]byte("{\"SecretID\": \"" + testSecretId + "\", \"ExpirationTime\": \"" + expTime + "\"}"))
	}))
	client := NewClient(ClientConfig{
		Address:   testServer.URL,
		Namespace: "test",
		Ctx:       context.Background(),
	})
	client.token = &ClientToken{}
	assert.Empty(t, client.token.val.Load())
	assert.Empty(t, client.token.expirationTime)
	client.startWatchSecretId(100 * time.Millisecond)
	wg.Add(1)
	go func() {
		assert.Eventuallyf(t, func() bool { return client.token.val.Load() == testSecretId }, 5*time.Second, 50*time.Millisecond, "must set secretId")
		wg.Done()
	}()
	wg.Wait()
}
