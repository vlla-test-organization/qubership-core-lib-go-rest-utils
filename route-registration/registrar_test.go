package routeregistration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vlla-test-organization/qubership-core-lib-go-rest-utils/v2/route-registration/internal/utils"
	"github.com/vlla-test-organization/qubership-core-lib-go/v3/configloader"
)

const (
	meshGateway = "test-gateway"
	pathFromV1  = "/v1"
	pathToV1    = "/v1/test"
)

var params = configloader.YamlPropertySourceParams{ConfigFilePath: "./testdata/application.yaml"}

func TestNewRegistrar(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar()
	assert.NotNil(t, registrar)
}

func TestWithRoutes_PublicRoute(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar().(*registrar)
	testRoute := createTestRoute(Public, "", pathFromV1, pathToV1)
	registrar.WithRoutes(testRoute)
	routesByGateway := registrar.routesByGatewayMap

	assert.True(t, contains(routesByGateway, PublicGatewayService))
	publicGatewayService := routesByGateway[PublicGatewayService]
	assert.True(t, publicGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, PrivateGatewayService))
	privateGatewayService := routesByGateway[PrivateGatewayService]
	assert.True(t, privateGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, InternalGatewayService))
	internalGatewayRoutes := routesByGateway[InternalGatewayService]
	assert.True(t, internalGatewayRoutes[0].Allowed)
}

func TestWithRoutes_PrivateRoute(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar().(*registrar)
	testRoute := createTestRoute(Private, "", pathFromV1, pathToV1)
	registrar.WithRoutes(testRoute)
	routesByGateway := registrar.routesByGatewayMap

	assert.True(t, contains(routesByGateway, PublicGatewayService))
	publicGatewayService := routesByGateway[PublicGatewayService]
	assert.False(t, publicGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, PrivateGatewayService))
	privateGatewayService := routesByGateway[PrivateGatewayService]
	assert.True(t, privateGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, InternalGatewayService))
	internalGatewayRoutes := routesByGateway[InternalGatewayService]
	assert.True(t, internalGatewayRoutes[0].Allowed)
}

func TestWithRoutes_InternalRoute(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar().(*registrar)
	testRoute := createTestRoute(Internal, "", pathFromV1, pathToV1)
	registrar.WithRoutes(testRoute)
	routesByGateway := registrar.routesByGatewayMap

	assert.True(t, contains(routesByGateway, PublicGatewayService))
	publicGatewayService := routesByGateway[PublicGatewayService]
	assert.False(t, publicGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, PrivateGatewayService))
	privateGatewayService := routesByGateway[PrivateGatewayService]
	assert.False(t, privateGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, InternalGatewayService))
	internalGatewayRoutes := routesByGateway[InternalGatewayService]
	assert.True(t, internalGatewayRoutes[0].Allowed)
}

func TestWithRoutes_MeshRoute(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar().(*registrar)
	testRoute := createTestRoute("", meshGateway, pathFromV1, pathToV1)
	registrar.WithRoutes(testRoute)
	routesByGateway := registrar.routesByGatewayMap
	assert.Equal(t, 1, len(routesByGateway))
	meshGatewayRoutes := routesByGateway[meshGateway]
	assert.True(t, meshGatewayRoutes[0].Allowed)
	assert.Equal(t, meshGateway, meshGatewayRoutes[0].Gateway)
}

func TestWithRoutes_NoRouteTypeButGatewayIsPublic(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar().(*registrar)
	testRoute := createTestRoute("", PublicGatewayService, pathFromV1, pathToV1)
	registrar.WithRoutes(testRoute)
	routesByGateway := registrar.routesByGatewayMap

	assert.True(t, contains(routesByGateway, PublicGatewayService))
	publicGatewayService := routesByGateway[PublicGatewayService]
	assert.True(t, publicGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, PrivateGatewayService))
	privateGatewayService := routesByGateway[PrivateGatewayService]
	assert.True(t, privateGatewayService[0].Allowed)

	assert.True(t, contains(routesByGateway, InternalGatewayService))
	internalGatewayRoutes := routesByGateway[InternalGatewayService]
	assert.True(t, internalGatewayRoutes[0].Allowed)
}

func TestWithRoutes_WithoutRouteTypeAndWithoutGatewayName(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar()
	testRoute := createTestRoute("", "", pathFromV1, pathToV1)
	assert.Panics(t, func() { registrar.WithRoutes(testRoute) }, "Expected panic "+
		"because: No type and no target gateway specified for route")
}

func TestAssertPath(t *testing.T) {
	assertPath("/api/v4/tenant-manager/activate/create-os-tenant-alias-routes/rollback/{tenantId}")
	assertPath("/api/*")
	assertPath("/api/v4/tenant-manager/activate/create-os-tenant-alias-routes/{tenantId}/rollback")
	assertPath("/api")
}

func TestAssertPath_Panic(t *testing.T) {
	assert.Panics(t, func() { assertPath("api/") })
	assert.Panics(t, func() { assertPath("/api/:tenantId") })
}

func TestWithRoutes_MeshRouteTypeAndWithoutGatewayName(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar().(*registrar)
	testRoute := createTestRoute(Mesh, "", pathFromV1, pathToV1)
	registrar.WithRoutes(testRoute)
	routesByGateway := registrar.routesByGatewayMap
	microserviceName := configloader.GetOrDefaultString("microservice.name", "")
	meshGatewayRoutes := routesByGateway[microserviceName]
	assert.True(t, meshGatewayRoutes[0].Allowed)
	assert.Equal(t, utils.Mesh, meshGatewayRoutes[0].RouteType)
}

func TestWithRoutes_ConflictRouteTypeWithGateway(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar()
	testRoute := createTestRoute("custom", meshGateway, pathFromV1, pathToV1)
	assert.Panics(t, func() { registrar.WithRoutes(testRoute) }, "Conflicting field "+
		"RouteType and Gateway values in route")
}

func TestWithRoutes_IncorrectPairFromTo(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar()
	testRoute := Route{
		From:           "v1",
		To:             "v1/test",
		Forbidden:      false,
		Timeout:        0,
		RouteType:      Public,
		Gateway:        "",
		VirtualService: "",
		Hosts:          nil,
	}
	assert.Panics(t, func() { registrar.WithRoutes(testRoute) }, "Path doesn't satisfy validation regexp")
}

func TestWithRoutes_BulkRegister(t *testing.T) {
	configloader.InitWithSourcesArray(configloader.BasePropertySources(params))
	registrar := NewRegistrar().(*registrar)
	testRoute := createTestRoute(Public, "", pathFromV1, pathToV1)
	testRoute2 := createTestRoute(Private, "", "/v2", "/v2/test")
	testRoute3 := createTestRoute(Internal, "", "/v3", "/v3/test")
	registrar.WithRoutes(testRoute, testRoute2, testRoute3)
	routesByGateway := registrar.routesByGatewayMap

	assert.True(t, contains(routesByGateway, PublicGatewayService))
	publicGatewayService := routesByGateway[PublicGatewayService]
	assert.Equal(t, 3, len(publicGatewayService))
	assert.True(t, publicGatewayService[0].Allowed)
	assert.False(t, publicGatewayService[1].Allowed)
	assert.False(t, publicGatewayService[2].Allowed)

	assert.True(t, contains(routesByGateway, PrivateGatewayService))
	privateGatewayService := routesByGateway[PrivateGatewayService]
	assert.Equal(t, 3, len(privateGatewayService))
	assert.True(t, privateGatewayService[0].Allowed)
	assert.True(t, privateGatewayService[1].Allowed)
	assert.False(t, privateGatewayService[2].Allowed)

	assert.True(t, contains(routesByGateway, InternalGatewayService))
	internalGatewayRoutes := routesByGateway[InternalGatewayService]
	assert.Equal(t, 3, len(internalGatewayRoutes))
	assert.True(t, internalGatewayRoutes[0].Allowed)
	assert.True(t, internalGatewayRoutes[0].Allowed)
	assert.True(t, internalGatewayRoutes[0].Allowed)
}

func createTestRoute(rType RouteType, gateway string, from string, to string) Route {
	return Route{
		From:           from,
		To:             to,
		Forbidden:      false,
		Timeout:        0,
		RouteType:      rType,
		Gateway:        gateway,
		VirtualService: "",
		Hosts:          nil,
	}
}

func contains(routesMap utils.RoutesByGateway, key string) bool {
	_, isFound := routesMap[key]
	return isFound
}
