package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vlla-test-organization/qubership-core-lib-go/v3/configloader"
)

func TestGetDeploymentVersion_WithDeploymentVersion(t *testing.T) {
	os.Setenv("DEPLOYMENT_VERSION", "v2")
	defer os.Clearenv()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	actualVersion := "v2"
	testVersion, _ := GetDeploymentVersion()
	assert.Equal(t, actualVersion, testVersion)
}

func TestGetDeploymentVersion_WithServiceName(t *testing.T) {
	os.Setenv("OPENSHIFT_SERVICE_NAME", "test-servicev11-v3")
	os.Setenv("MICROSERVICE_NAME", "test-servicev11")
	defer os.Clearenv()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	actualVersion := "v3"
	testVersion, _ := GetDeploymentVersion()
	assert.Equal(t, actualVersion, testVersion)
}

func TestGetDeploymentVersion_EmptyDeploymentVersion(t *testing.T) {
	os.Setenv("MICROSERVICE_NAME", "test-servicev11")
	defer os.Clearenv()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	val, err := GetDeploymentVersion()
	assert.Nil(t, err)
	assert.Empty(t, val)
}

func TestGetDeploymentVersion_MicroserviceNameIsEqualToOpenshiftName(t *testing.T) {
	os.Setenv("OPENSHIFT_SERVICE_NAME", "test-servicev11-v3")
	os.Setenv("MICROSERVICE_NAME", "test-servicev11-v3")
	defer os.Clearenv()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	val, err := GetDeploymentVersion()
	assert.Nil(t, err)
	assert.Empty(t, val)
}

func TestFormatMicroserviceInternalURL_WithEnv(t *testing.T) {
	testUrl := "https://test-url:8888"
	os.Setenv("MICROSERVICE_URL", testUrl)
	defer os.Clearenv()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	val := FormatMicroserviceInternalURL("test-service-name")
	assert.Equal(t, testUrl, val)
}

func TestFormatMicroserviceInternalURL_WithMicroserviceName(t *testing.T) {
	testUrl := "http://test-service-name:8888"
	os.Setenv("SERVER_PORT", "8888")
	defer os.Clearenv()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	val := FormatMicroserviceInternalURL("test-service-name")
	assert.Equal(t, testUrl, val)
}

func TestFormatCloudNamespace(t *testing.T) {
	localdevNamespace := "10.236.138.87.nip.io"
	os.Setenv("LOCALDEV_NAMESPACE", localdevNamespace)
	defer os.Clearenv()
	resolvedNamespace := FormatCloudNamespace("test-namespace")
	assert.Equal(t, localdevNamespace, resolvedNamespace)
}
