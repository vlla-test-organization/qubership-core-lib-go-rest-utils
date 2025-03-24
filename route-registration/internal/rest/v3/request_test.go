package v3

import (
	"testing"
	"time"

	"github.com/netcracker/qubership-core-lib-go-rest-utils/v2/route-registration/internal/rest"
	"github.com/netcracker/qubership-core-lib-go-rest-utils/v2/route-registration/internal/utils"
	"github.com/stretchr/testify/assert"
)

const (
	PublicGatewayService   = "public-gateway-service"
	PrivateGatewayService  = "private-gateway-service"
	InternalGatewayService = "internal-gateway-service"
	namespace              = "namespace"
	microserviceUrl        = "http://service:8080"
	microserviceName       = "test-service"
	deploymentVersion      = "v2"
)

func TestNewRequestFactory(t *testing.T) {
	reqFactory := NewRequestFactory(namespace, microserviceUrl, microserviceName, deploymentVersion)
	assert.NotNil(t, reqFactory)
	assert.Equal(t, namespace, reqFactory.namespace)
	assert.Equal(t, microserviceUrl, reqFactory.microserviceUrl)
	assert.Equal(t, microserviceName, reqFactory.microserviceName)
}

func TestRequestFactory_NewRequests(t *testing.T) {
	var rbg utils.RoutesByGateway
	rbg = make(map[string][]utils.Route)
	rbg[PublicGatewayService] = createRoutes("v1", utils.Public)
	rbg[PrivateGatewayService] = createRoutes("v2", utils.Private)
	reqFactory := NewRequestFactory(namespace, microserviceUrl, microserviceName, deploymentVersion)
	ListOfRequests := reqFactory.NewRequests(rbg)
	assert.Equal(t, 2, len(ListOfRequests))
}

func TestRegistrationRequest_ApiVersion(t *testing.T) {
	var routesByGatewayMap utils.RoutesByGateway
	routesByGatewayMap = make(map[string][]utils.Route)
	routesByGatewayMap[PublicGatewayService] = createRoutes("v1", utils.Public)
	reqFactory := NewRequestFactory(namespace, microserviceUrl, microserviceName, deploymentVersion)
	listOfRequests := reqFactory.NewRequests(routesByGatewayMap)
	assert.Equal(t, rest.V3, listOfRequests[0].ApiVersion())
}

func TestRequestFactory_NewRequests_AllowedRoutes(t *testing.T) {
	reqFactory := NewRequestFactory(namespace, microserviceUrl, microserviceName, deploymentVersion)
	virtualServicesMap := reqFactory.groupByVirtualServices(PublicGatewayService, createRoutes("v1", utils.Public))
	listOfRequests := reqFactory.newRequestsFromVirtualServices(PublicGatewayService, virtualServicesMap)
	config := listOfRequests[0].Payload().(*routingConfigRequest)
	service := config.VirtualServices[0]
	routes := service.RouteConfiguration.Routes[0]
	assert.True(t, *routes.Rules[0].Allowed)
	assert.False(t, *routes.Rules[1].Allowed)
}

func createRoutes(version string, rType utils.RouteType) []utils.Route {
	return []utils.Route{
		{
			"/" + version + "/",
			"/" + version + "/test",
			true,
			5 * time.Second,
			rType,
			"",
			"",
			[]string{""},
		},
		{
			"/" + version + "/",
			"/" + version + "/test",
			false,
			5 * time.Second,
			rType,
			"",
			"",
			[]string{""},
		},
	}

}
