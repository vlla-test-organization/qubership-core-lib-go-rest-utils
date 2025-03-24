package utils

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/const"
)

// RouteType stands for the type of route to be registered.
//
// Public type declares that routes should be registered in PUBLIC, PRIVATE and INTERNAL gateways.
//
// Private type declares that routes should be registered in PRIVATE and INTERNAL gateways.
//
// Internal type declares that routes should be registered only in INTERNAL gateway.
//
// Mesh type declares that routes should be registered in MESH gateway with the specified name.
type RouteType string

const (
	Public   RouteType = "public"
	Private  RouteType = "private"
	Internal RouteType = "internal"
	Mesh     RouteType = "mesh"
)

type RoutesByGateway map[string][]Route

type Route struct {
	From           string
	To             string
	Allowed        bool
	Timeout        time.Duration
	RouteType      RouteType
	Gateway        string
	VirtualService string
	Hosts          []string
}

func (r Route) TimeoutAsInt64() *int64 {
	if r.Timeout <= 0 {
		return nil
	}
	result := r.Timeout.Milliseconds()
	return &result
}

func GetMicroserviceName() (string, error) {
	microserviceName := configloader.GetOrDefaultString("microservice.name", "")
	if microserviceName == "" {
		return "", errors.New("microservice.name wasn't set up")
	}
	return microserviceName, nil
}

func GetDeploymentVersion() (string, error) {
	deploymentVersion := configloader.GetOrDefaultString("deployment.version", "")
	if deploymentVersion == "" {
		openshiftServiceName := configloader.GetOrDefaultString("openshift.service.name", "")
		microserviceName, err := GetMicroserviceName()
		if err != nil {
			return "", err
		}
		if openshiftServiceName != "" && microserviceName != "" && microserviceName != openshiftServiceName {
			deploymentVersion = openshiftServiceName[len(microserviceName)+1:]
		}
	}
	return deploymentVersion, nil
}

func FormatMicroserviceInternalURL(microserviceName string) string {
	url := configloader.GetOrDefaultString("microservice.url", "")
	if url == "" {
		postRoutesAppnameDisabled := configloader.GetOrDefaultString("apigateway.routes.registration.appname.disabled", "")
		host := microserviceName
		if postRoutesAppnameDisabled != "" {
			host = ""
		}
		defaultPort := constants.SelectUrl("8080", "8443")
		defaultProtocol := constants.SelectUrl("http", "https")
		serverPort := configloader.GetOrDefaultString("server.port", defaultPort)
		url = fmt.Sprintf("%s://%s:%s", defaultProtocol, host, serverPort)
	}
	return url
}

func FormatCloudNamespace(cloudNamespace string) string {
	resolvedNamespace := cloudNamespace

	localDevNamespace := os.Getenv("LOCALDEV_NAMESPACE")
	if len(localDevNamespace) > 0 {
		resolvedNamespace = localDevNamespace
	}

	return resolvedNamespace
}
