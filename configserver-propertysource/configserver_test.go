package configserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/test"
	"github.com/stretchr/testify/assert"
)

const (
	configserverKey   = "key"
	configserverValue = "value"
)

func TestConfigServerPropertySource_WithoutMandatoryProperties(t *testing.T) {
	assert.Panics(t, func() {
		configloader.InitWithSourcesArray([]*configloader.PropertySource{GetPropertySource(PropertySourceConfiguration{
			MicroserviceName: "",
			ConfigServerUrl:  "",
		})})
	},
		"There are no microservice.name defined")
}

func TestPanicIfNoMicroserviceName(t *testing.T) {
	assert.Panics(t, func() {
		getMicroserviceNameAndURL(&PropertySourceConfiguration{})
	},
		"There are no microservice.name defined")
}

func TestConfigServerPropertySource(t *testing.T) {
	test.StartMockServer()

	ts := createTestHttpServer()
	defer func() {
		os.Unsetenv("config-server.url")
		os.Unsetenv("microservice.name")
		test.StopMockServer()
		ts.Close()
	}()
	os.Setenv("config-server.url", ts.URL)
	params := PropertySourceConfiguration{
		MicroserviceName: "test",
		ConfigServerUrl:  ts.URL,
	}
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource(), GetPropertySource(params)})
	assert.Equal(t, configserverValue, configloader.GetOrDefaultString(configserverKey, ""))
}

func TestGetPropertySource_WithoutParams(t *testing.T) {
	assert.Panics(t, func() {
		configloader.InitWithSourcesArray([]*configloader.PropertySource{GetPropertySource()})
	},
		"There are no microservice.name defined")
}

func TestConfigServerLoader_ReadBytes(t *testing.T) {
	loader := newConfigServerLoader(&PropertySourceConfiguration{
		MicroserviceName: "name",
		ConfigServerUrl:  "url",
	})
	if _, err := loader.ReadBytes(nil); assert.Error(t, err) {
		assert.Contains(t, err.Error(), "configserver provider does not support this method")
	}
}

func TestConfigServerLoader_Read(t *testing.T) {
	ts := createTestHttpServer()
	defer ts.Close()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	loader := newConfigServerLoader(&PropertySourceConfiguration{
		MicroserviceName: "name",
		ConfigServerUrl:  ts.URL,
	})
	testResp, _ := loader.Read(nil)
	assert.Equal(t, configserverValue, testResp[configserverKey])
	assert.Equal(t, "map-value", testResp["composite-key.map-key"])
}

func TestAddConfigServerPropertySource(t *testing.T) {
	ts := createTestHttpServer()
	os.Setenv("config-server.url", ts.URL)
	prepareEnvironment()
	defer func() {
		ts.Close()
		os.Unsetenv("config-server.url")
		os.Unsetenv("microservice.name")
	}()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	assert.Empty(t, configloader.GetOrDefaultString(configserverKey, ""))

	configloader.InitWithSourcesArray(AddConfigServerPropertySource([]*configloader.PropertySource{configloader.EnvPropertySource()}))
	assert.Equal(t, configserverValue, configloader.GetOrDefaultString(configserverKey, ""))
}

func TestConfigServerReturnFlattenMap(t *testing.T) {
	ts := createTestHttpServer()
	os.Setenv("config-server.url", ts.URL)
	os.Setenv("microservice.name", "test")
	configloader.InitWithSourcesArray(AddConfigServerPropertySource([]*configloader.PropertySource{configloader.EnvPropertySource()}))
	defer func() {
		ts.Close()
		os.Unsetenv("config-server.url")
		os.Unsetenv("microservice.name")
	}()

	testVal := configloader.GetOrDefaultString("composite-key.extra-key.inner-key", "empty")
	assert.Equal(t, "inner-val", testVal)
	testEmpty := configloader.GetOrDefaultString("composite-key.extra-key", "empty")
	assert.Equal(t, "empty", testEmpty)
}

func prepareEnvironment() {
	os.Setenv("microservice.name", "test")
}

func createTestHttpServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		jsonString := createJsonResponse()
		io.WriteString(w, jsonString)
	}))
}

func createJsonResponse() string {
	embeddedMap := map[string]interface{}{
		"map-key": "map-value",
		"extra-key": map[string]interface{}{
			"inner-key": "inner-val",
		},
	}
	propertiesMap := map[string]interface{}{
		"key":           "value",
		"composite-key": embeddedMap,
	}

	entity := configserverPropertySourceEntity{
		Name:   "test",
		Source: propertiesMap,
	}
	configserverEnv := configserverEnv{
		Name:            "test",
		Profiles:        []string{"test", "dev"},
		PropertySources: []configserverPropertySourceEntity{entity},
	}
	jsonResp, _ := json.Marshal(configserverEnv)
	return string(jsonResp)
}

func TestParseBody(t *testing.T) {
	body := bytes.NewBufferString("{\"name\":\"site-management\",\"profiles\":[\"default\"],\"propertySources\":[]}")
	res, err := parseBody(body)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(res))

	body = bytes.NewBufferString("{\"name\":\"site-management\",\"profiles\":[\"default\"],\"propertySources\":[{\"name\":\"consul\",\"source\":{}}]}")
	res, err = parseBody(body)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(res))

	body = bytes.NewBufferString("{\"name\":\"site-management\",\"profiles\":[\"default\"],\"propertySources\":[{\"name\":\"consul\",\"source\":{\"tenant.default.id\": \"61659bdb-4709-4279-a95d-759080d1ad30\"}}]}")
	res, err = parseBody(body)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(res))
}
