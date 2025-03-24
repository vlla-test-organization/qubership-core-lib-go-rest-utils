package consul

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/knadh/koanf/v2"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_kvToMap(t *testing.T) {
	type args struct {
		key   string
		value string
		res   map[string]interface{}
	}
	tests := []struct {
		name string
		args args
		want map[string]interface{}
	}{
		{
			name: "empty",
			args: args{"", "v", make(map[string]interface{})},
			want: map[string]interface{}{},
		},
		{
			name: "flat",
			args: args{"k", "v", make(map[string]interface{})},
			want: map[string]interface{}{"k": "v"},
		},
		{
			name: "hierarchical",
			args: args{"p/s/k", "v", make(map[string]interface{})},
			want: map[string]interface{}{"p": map[string]interface{}{"s": map[string]interface{}{"k": "v"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kvToMap(tt.args.key, tt.args.value, tt.args.res); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("kvToMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cutPrefix(t *testing.T) {
	type args struct {
		pairs  api.KVPairs
		prefix string
	}
	tests := []struct {
		name string
		args args
		want api.KVPairs
	}{
		{
			name: "empty",
			args: args{
				pairs:  []*api.KVPair{{Key: "", Value: []byte("")}},
				prefix: "",
			},
			want: []*api.KVPair{{
				Key:   "",
				Value: []byte(""),
			}},
		},
		{
			name: "single pair",
			args: args{
				pairs:  []*api.KVPair{{Key: "prefix/key", Value: []byte("val")}},
				prefix: "prefix",
			},
			want: []*api.KVPair{{
				Key:   "key",
				Value: []byte("val"),
			}},
		},
		{
			name: "multi pair",
			args: args{
				pairs:  []*api.KVPair{{Key: "prefix/key1", Value: []byte("val1")}, {Key: "prefix/key2", Value: []byte("val2")}},
				prefix: "prefix",
			},
			want: []*api.KVPair{{Key: "key1", Value: []byte("val1")}, {Key: "key2", Value: []byte("val2")}},
		},
		{
			name: "prefix with slash",
			args: args{
				pairs:  []*api.KVPair{{Key: "prefix/key", Value: []byte("val")}},
				prefix: "/prefix",
			},
			want: []*api.KVPair{{Key: "key", Value: []byte("val")}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cutPrefix(tt.args.pairs, tt.args.prefix); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cutPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_kvPairsAsMap(t *testing.T) {
	type args struct {
		pairs api.KVPairs
	}
	tests := []struct {
		name string
		args args
		want map[string]interface{}
	}{
		{
			name: "empty",
			args: args{
				pairs: []*api.KVPair{{Key: "", Value: []byte("")}},
			},
			want: map[string]interface{}{},
		},
		{
			name: "single pair",
			args: args{
				pairs: []*api.KVPair{{Key: "key", Value: []byte("val")}},
			},
			want: map[string]interface{}{"key": "val"},
		},
		{
			name: "multi pair",
			args: args{
				pairs: []*api.KVPair{{Key: "logging/level/org/qubership/crm/submission", Value: []byte("val1")},
					{Key: "logging/level/org/qubership/crm/submission/handler", Value: []byte("val2")},
					{Key: "logging/level/org/qubership/crm", Value: []byte("val3")}},
			},
			want: map[string]interface{}{
				"logging": map[string]interface{}{"level": map[string]interface{}{"org": map[string]interface{}{"qubership": map[string]interface{}{"crm": map[string]interface{}{"submission": map[string]interface{}{"handler": "val2"}}}}}},
				"logging.level.org.qubership.crm.submission": "val1",
				"logging.level.org.qubership.crm":            "val3",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configMap := make(map[string]interface{})
			if kvPairsAsMap(tt.args.pairs, configMap); !reflect.DeepEqual(configMap, tt.want) {
				t.Errorf("kvPairsAsMap() = %v, want %v", configMap, tt.want)
			}
		})
	}
}

func TestAddConsulPropertySource(t *testing.T) {
	ts := createTestHttpServer()
	testKey := "test-key"
	testValue := "test-value"

	os.Setenv("consul.url", ts.URL)
	defer func() {
		ts.Close()
		os.Clearenv()
	}()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	assert.Empty(t, configloader.GetOrDefaultString(testKey, ""))
	propertySource := AddConsulPropertySource(([]*configloader.PropertySource{configloader.EnvPropertySource()}), ProviderConfig{})
	configloader.InitWithSourcesArray(propertySource)
	assert.Equal(t, testValue, configloader.GetOrDefaultString(testKey, ""))
}

func TestConsulReturnsFlattenMap(t *testing.T) {
	ts := createTestHttpServer()
	os.Setenv("consul.url", ts.URL)
	os.Setenv("key", "env-value")
	defer func() {
		ts.Close()
		os.Unsetenv("consul.url")
		os.Unsetenv("key")
	}()
	propertySource := AddConsulPropertySource(([]*configloader.PropertySource{configloader.EnvPropertySource()}), ProviderConfig{})
	configloader.InitWithSourcesArray(propertySource)
	assert.Equal(t, "env-value", configloader.GetOrDefault("key", ""))
	assert.Equal(t, "test-value-one", configloader.GetOrDefaultString("key.one", ""))
	assert.Equal(t, "test-value-two", configloader.GetOrDefaultString("key.two", ""))
}

func TestConsulPropertySource_InitializeClientSingleTime(t *testing.T) {
	ts := createTestHttpServer()
	os.Setenv("consul.url", ts.URL)
	defer os.Unsetenv("consul.url")
	defer ts.Close()
	sources := AddConsulPropertySource([]*configloader.PropertySource{configloader.EnvPropertySource()}, ProviderConfig{
		Address: ts.URL,
	})
	consulPS := sources[1].Provider.(*ProviderImpl)
	configloader.InitWithSourcesArray(sources)
	assert.NotNil(t, consulPS.client)
	clientAfterInit := consulPS.client
	assert.Nil(t, configloader.Refresh())
	clientAfterRefresh := consulPS.client
	assert.NotNil(t, clientAfterRefresh)
	assert.Same(t, clientAfterInit, clientAfterRefresh)
}

func TestConsulPropertySource_FailsafeDuringLoad(t *testing.T) {
	p := make([]*api.KVPair, 1)
	p[0] = &api.KVPair{
		Key:         "log.level",
		CreateIndex: 0,
		Value:       []byte("DEBUG"),
	}
	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { counter++ }()
		switch counter {
		case 0, 3:
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "")
			break
		case 1:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "{\"SecretID\": \"test-secretId\", \"ExpirationTime\": \""+time.Now().Add(5*time.Minute).Format(time.RFC3339)+"\"}")
			break
		case 2:
			w.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(p)
			io.WriteString(w, string(resp))
		}
	}))

	os.Setenv("consul.url", ts.URL)
	defer os.Unsetenv("consul.url")
	defer ts.Close()

	sources := AddConsulPropertySource([]*configloader.PropertySource{configloader.EnvPropertySource()}, ProviderConfig{
		Address:          ts.URL,
		MicroserviceName: "test",
		Paths:            []string{"log"},
		Failsafe:         true,
	})
	consulPS := sources[1].Provider.(*ProviderImpl)
	configloader.InitWithSourcesArray(sources) // Fail, but suppressed
	require.NotNil(t, consulPS.client)

	loggingLevel := configloader.GetOrDefaultString("log.level", "default")
	require.Equal(t, "default", loggingLevel)

	assert.Nil(t, configloader.Refresh()) // Success
	loggingLevel = configloader.GetOrDefaultString("log.level", "")
	require.Equal(t, "DEBUG", loggingLevel)

	assert.Nil(t, configloader.Refresh()) // Fail, but suppressed
	loggingLevel = configloader.GetOrDefaultString("log.level", "default")
	require.Equal(t, "default", loggingLevel)
}

func TestConsulPropertySource_WatchAndUpdateLogLevels(t *testing.T) {
	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { counter++ }()
		switch counter {
		case 0:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "{\"SecretID\": \"test-secretId\", \"ExpirationTime\": \""+time.Now().Add(5*time.Minute).Format(time.RFC3339)+"\"}")
		case 1:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, createJsonLogResponse(logging.LvlWarn, logging.LvlError))
		case 2, 3:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, createJsonLogResponse(logging.LvlError, logging.LvlWarn))
		case 4, 5:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, createJsonLogResponse(logging.LvlError, -1))
		case 6, 7:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, createJsonLogResponse(-1, -1))
		}
	}))

	os.Setenv("CONSUL_URL", ts.URL)
	os.Setenv("MICROSERVICE_NAMESPACE", "<namespace>")
	os.Setenv("MICROSERVICE_NAME", "<microservice-name>")
	defer func() {
		ts.Close()
		os.Clearenv()
	}()

	var loggerName = "log_package"
	_ = logging.GetLogger(loggerName)

	consulPS := NewLoggingPropertySource(ProviderConfig{
		forLogging: true,
		Failsafe:   true,
	})
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource(), consulPS})

	levels := logging.GetLogLevels()
	logLevel := levels[loggerName]
	require.Equal(t, strings.ToUpper(logging.LvlInfo.String()), logLevel) // Default level

	var wg sync.WaitGroup
	wg.Add(1)
	updateCounter := 0
	StartWatchingForPropertiesWithRetry(context.Background(), consulPS, func(event interface{}, err error) {
		<-time.After(1 * time.Second)
		levels := logging.GetLogLevels()
		require.Nil(t, err)
		logLevel := levels[loggerName]
		if updateCounter == 0 {
			require.Equal(t, strings.ToUpper(logging.LvlWarn.String()), logLevel) // Package changed via Consul
		} else if updateCounter == 1 {
			require.Equal(t, strings.ToUpper(logging.LvlError.String()), logLevel) // Fallback to root level
		} else if updateCounter == 2 {
			require.Equal(t, strings.ToUpper(logging.LvlInfo.String()), logLevel) // Fallback to initial level
			wg.Done()
		}
		updateCounter++
	})
	wg.Wait()
}

func createJsonLogResponse(root logging.Lvl, pgk logging.Lvl) string {
	p := make([]*api.KVPair, 0)
	if root != -1 {
		p = append(p, &api.KVPair{
			Key:         "log.level",
			CreateIndex: 0,
			Value:       []byte(root.String()),
		})
	}
	if pgk != -1 {
		p = append(p, &api.KVPair{
			Key:         "log.level.package.log_package",
			CreateIndex: 0,
			Value:       []byte(pgk.String()),
		})
	}
	resp, _ := json.Marshal(p)
	return string(resp)
}

func createTestHttpServer() *httptest.Server {
	counter := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { counter++ }()
		if counter == 0 {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "{\"SecretID\": \"test-secretId\", \"ExpirationTime\": \""+time.Now().Add(5*time.Minute).Format(time.RFC3339)+"\"}")
			return
		}

		w.WriteHeader(http.StatusOK)
		jsonString := createJsonResponse(counter)
		io.WriteString(w, jsonString)
	}))
}

func createJsonResponse(counter int) string {
	p := make([]*api.KVPair, 4)
	p[0] = &api.KVPair{
		Key:         "test-key",
		CreateIndex: 0,
		Value:       []byte("test-value"),
	}
	p[1] = &api.KVPair{
		Key:         "key/one",
		CreateIndex: 0,
		Value:       []byte("test-value-one"),
	}
	p[2] = &api.KVPair{
		Key:         "key/two",
		CreateIndex: 0,
		Value:       []byte("test-value-two"),
	}
	p[3] = &api.KVPair{
		Key:         "test-key-counter",
		CreateIndex: 0,
		Value:       []byte("value-" + strconv.Itoa(counter)),
	}
	resp, _ := json.Marshal(p)
	return string(resp)
}

func TestConsulCallsWatchMethod(t *testing.T) {
	ts := createTestHttpServer()
	os.Setenv("consul.url", ts.URL)
	defer os.Unsetenv("consul.url")
	defer ts.Close()
	propertySources := []*configloader.PropertySource{configloader.EnvPropertySource()}
	consulPropertySource := NewPropertySource(
		ProviderConfig{
			Address: ts.URL,
		},
	)
	propertySources = append(propertySources, consulPropertySource)
	configloader.Init(propertySources...)

	counterValue := configloader.GetOrDefaultString("test-key-counter", "")
	assert.Equal(t, "value-2", counterValue)

	consulPropertySource = &configloader.PropertySource{Provider: newMockProvider()} // override
	i := 0
	projectFunc := func(event interface{}, err error) {
		i++
	}
	err := WatchForProperties(consulPropertySource, projectFunc)

	assert.Equal(t, 1, i)
	assert.Equal(t, err, nil)

	counterValue = configloader.GetOrDefaultString("test-key-counter", "")
	assert.Equal(t, "value-4", counterValue)
}

func TestConsulCallsWatchMethod2(t *testing.T) {
	ts := createTestHttpServer()
	os.Setenv("consul.url", ts.URL)
	defer os.Unsetenv("consul.url")
	defer ts.Close()
	propertySources := []*configloader.PropertySource{configloader.EnvPropertySource()}
	consulPropertySource := NewPropertySource(
		ProviderConfig{
			Address: ts.URL,
		},
	)
	propertySources = append(propertySources, consulPropertySource)
	configloader.Init(propertySources...)

	counterValue := configloader.GetOrDefaultString("test-key-counter", "")
	assert.Equal(t, "value-2", counterValue)

	consulPropertySource = &configloader.PropertySource{Provider: newMockProvider()} // override
	i := 0
	projectFunc := func(event interface{}, err error) {
		i++
	}
	StartWatchingForPropertiesWithRetry(context.Background(), consulPropertySource, projectFunc)
	<-time.After(10 * time.Second)

	assert.Equal(t, 1, i)

	counterValue = configloader.GetOrDefaultString("test-key-counter", "")
	assert.Equal(t, "value-4", counterValue)
}

type MockProvider struct {
}

func (m MockProvider) ReadBytes(k *koanf.Koanf) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockProvider) Read(k *koanf.Koanf) (map[string]interface{}, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockProvider) Watch(cb func(event interface{}, err error)) error {
	cb(nil, nil)
	return nil
}

func newMockProvider() Provider {
	return &MockProvider{}
}
