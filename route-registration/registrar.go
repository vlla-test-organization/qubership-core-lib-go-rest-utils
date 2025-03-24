package routeregistration

import (
	"regexp"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go-rest-utils/v2/route-registration/internal/rest"
	v3 "github.com/netcracker/qubership-core-lib-go-rest-utils/v2/route-registration/internal/rest/v3"
	"github.com/netcracker/qubership-core-lib-go-rest-utils/v2/route-registration/internal/utils"
	"github.com/netcracker/qubership-core-lib-go/v3/const"
)

var log logging.Logger

func init() {
	log = logging.GetLogger("routeregistration")
}

type registrar struct {
	routesByGatewayMap  utils.RoutesByGateway
	transformationRules map[RouteType]transformationRule

	reqFactory rest.RequestFactory
	cpClient   *rest.ControlPlaneClient
}

func newRegistrar() *registrar {
	namespace := configloader.GetOrDefaultString("microservice.namespace", "")
	namespace = utils.FormatCloudNamespace(namespace)
	microserviceName, err := utils.GetMicroserviceName()
	if err != nil {
		log.Panicf("Got error during parsing microservice name : %v", err.Error())
	}
	microserviceUrl := utils.FormatMicroserviceInternalURL(microserviceName)
	if microserviceUrl == "" {
		log.Panicf("Got error during parsing microservice url : microservice url is absent")
	}
	deploymentVersion, err := utils.GetDeploymentVersion()
	if err != nil {
		log.Panicf("Got error during parsing deployment version: %v", err.Error())
	}
	defaultUrl := constants.SelectUrl("http://control-plane:8080", "https://control-plane:8443")
	controlPlaneAddr := configloader.GetOrDefaultString("apigateway.control-plane.url", defaultUrl)
	log.Debugf("Create new Registrar for microservice %v in namespace %v", microserviceName, namespace)

	return &registrar{
		routesByGatewayMap: make(map[string][]utils.Route),
		transformationRules: map[RouteType]transformationRule{
			Public:   transformBorderGatewayRoute,
			Private:  transformBorderGatewayRoute,
			Internal: transformBorderGatewayRoute,
			Mesh:     transformMeshRoute,
		},

		reqFactory: v3.NewRequestFactory(namespace, microserviceUrl, microserviceName, deploymentVersion),
		cpClient: rest.NewControlPlaneClient(controlPlaneAddr,
			rest.NewRetryManager(rest.NewProgressiveTimeout(1*time.Second, 1, 10, 1)))}
}

func (registrar *registrar) WithRoutes(routes ...Route) Registrar {
	for _, r := range routes {
		validate(r)

		log.Debugf("Going to transform provided route %+v", r)
		actualRoutes := registrar.transformRoute(r)
		for gatewayName, gatewayRoutes := range actualRoutes {
			log.Debugf("Transformed routes for gateway %v : %+v", gatewayName, gatewayRoutes)
			if _, found := registrar.routesByGatewayMap[gatewayName]; found {
				registrar.routesByGatewayMap[gatewayName] = append(registrar.routesByGatewayMap[gatewayName], gatewayRoutes...)
			} else {
				registrar.routesByGatewayMap[gatewayName] = gatewayRoutes
			}
		}
	}
	return registrar
}

func (registrar *registrar) Register() {
	requests := registrar.reqFactory.NewRequests(registrar.routesByGatewayMap)

	for _, request := range requests {
		log.Infof("Request %+v is going to be sent to control plane", request.Payload())
		registrar.cpClient.SendRequest(request)
	}
	log.Info("All routes were registered in control plane")
}

func (registrar *registrar) transformRoute(r Route) utils.RoutesByGateway {
	routeType := r.RouteType
	if r.RouteType == "" {
		internalRouteType := routeTypeByGateway(r.Gateway)
		routeType = internalRouteType
		r.RouteType = internalRouteType
		log.Debugf("Route %+v has no type, defined type is %v", r, routeType)
	}
	return registrar.transformationRules[routeType](r)
}

type transformationRule func(route Route) utils.RoutesByGateway

func transformMeshRoute(r Route) utils.RoutesByGateway {
	meshGatewayName := r.Gateway
	if meshGatewayName == "" {
		meshGatewayName, _ = utils.GetMicroserviceName()
		log.Infof("Using microservice name %v as mesh gateway name", meshGatewayName)
	}
	return map[string][]utils.Route{
		meshGatewayName: {{
			From:           r.From,
			To:             r.To,
			Allowed:        !r.Forbidden,
			Timeout:        r.Timeout,
			RouteType:      r.RouteType.toInternalRouteType(),
			Gateway:        r.Gateway, // keep original gateway name for debug purposes
			VirtualService: r.VirtualService,
			Hosts:          r.Hosts,
		}}}
}

func transformBorderGatewayRoute(prototype Route) utils.RoutesByGateway {
	result := make(map[string][]utils.Route, 3)
	result[PublicGatewayService] = []utils.Route{createCommonGatewayRoute(prototype, PublicGatewayService)}
	result[PrivateGatewayService] = []utils.Route{createCommonGatewayRoute(prototype, PrivateGatewayService)}
	result[InternalGatewayService] = []utils.Route{createCommonGatewayRoute(prototype, InternalGatewayService)}
	return result
}

func createCommonGatewayRoute(prototype Route, targetGateway string) utils.Route {
	return utils.Route{
		From:           prototype.From,
		To:             prototype.To,
		Allowed:        isCommonGatewayRouteAllowed(prototype, targetGateway),
		Timeout:        prototype.Timeout,
		RouteType:      prototype.RouteType.toInternalRouteType(), // keep original routeType for debug purposes
		Gateway:        prototype.Gateway,                         // keep original gateway name for debug purposes
		VirtualService: prototype.VirtualService,
		Hosts:          prototype.Hosts,
	}
}

func isCommonGatewayRouteAllowed(route Route, targetGatewayName string) bool {
	if route.Forbidden {
		return false
	}
	switch route.RouteType {
	case Public:
		return targetGatewayName == PublicGatewayService ||
			targetGatewayName == PrivateGatewayService ||
			targetGatewayName == InternalGatewayService
	case Private:
		return targetGatewayName == PrivateGatewayService ||
			targetGatewayName == InternalGatewayService
	case Internal:
		return targetGatewayName == InternalGatewayService
	default:
		return true
	}
}

func validate(route Route) {
	assertPath(route.From)
	assertPath(route.To)
	validateEffectiveRouteType(route)
}

func validateEffectiveRouteType(route Route) {
	if route.RouteType == "" {
		if route.Gateway == "" {
			log.Panicf("No type and no target gateway specified for route %+v", route)
		}
	} else {
		if route.Gateway != "" && route.RouteType != routeTypeByGateway(route.Gateway) {
			log.Panicf("Conflicting field RouteType and Gateway values in route %v", route)
		}
	}
}

func routeTypeByGateway(gateway string) RouteType {
	switch gateway {
	case PublicGatewayService:
		return Public
	case PrivateGatewayService:
		return Private
	case InternalGatewayService:
		return Internal
	default:
		return Mesh
	}
}

func (t RouteType) toInternalRouteType() utils.RouteType {
	switch t {
	case Public:
		return utils.Public
	case Private:
		return utils.Private
	case Internal:
		return utils.Internal
	case Mesh:
		return utils.Mesh
	}
	log.Panicf("Unknown route type: %v", t)
	return ""
}

func assertPath(path string) {
	var validPath = regexp.MustCompile(`^(/[a-zA-Z0-9{}*_-]*)+$`)
	if !validPath.MatchString(path) {
		log.Panicf("Path doesn't satisfy validation regexp: %v", path)
	}
}
