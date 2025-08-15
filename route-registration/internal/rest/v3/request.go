package v3

import (
	"github.com/vlla-test-organization/qubership-core-lib-go-rest-utils/v2/route-registration/internal/rest"
	"github.com/vlla-test-organization/qubership-core-lib-go-rest-utils/v2/route-registration/internal/utils"
	"github.com/vlla-test-organization/qubership-core-lib-go/v3/logging"
)

var log logging.Logger

func init() {
	log = logging.GetLogger("routemanagement")
}

type requestFactory struct {
	namespace         string
	microserviceUrl   string
	microserviceName  string
	deploymentVersion string
}

func NewRequestFactory(namespace string, microserviceUrl string, microserviceName string, deploymentVersion string) *requestFactory {
	return &requestFactory{namespace: namespace, microserviceUrl: microserviceUrl, microserviceName: microserviceName, deploymentVersion: deploymentVersion}
}

type virtualServiceData struct {
	name     string
	hostsSet map[string]bool
	routes   []utils.Route
}

func (srv *virtualServiceData) addHosts(hosts ...string) {
	for _, host := range hosts {
		srv.hostsSet[host] = true
	}
}

func (srv *virtualServiceData) getHosts() []string {
	result := make([]string, 0, len(srv.hostsSet))
	for host, _ := range srv.hostsSet {
		result = append(result, host)
	}
	return result
}

func (f *requestFactory) NewRequests(routesByGatewayMap utils.RoutesByGateway) []rest.RegistrationRequest {
	result := make([]rest.RegistrationRequest, 0)
	for gatewayName, gatewayRoutes := range routesByGatewayMap {
		virtualServicesMap := f.groupByVirtualServices(gatewayName, gatewayRoutes)
		result = append(result, f.newRequestsFromVirtualServices(gatewayName, virtualServicesMap)...)
	}
	return result
}

func (f *requestFactory) groupByVirtualServices(gatewayName string, gatewayRoutes []utils.Route) map[string]virtualServiceData {
	virtualServicesMap := make(map[string]virtualServiceData)
	for _, r := range gatewayRoutes {
		virtualServiceName := r.VirtualService
		if virtualServiceName == "" {
			virtualServiceName = gatewayName
		}
		actualHosts := r.Hosts
		if len(actualHosts) == 0 {
			log.Debug("There are no actual hosts, use default hosts")
			actualHosts = []string{"*"} // default hosts
		}
		srv, ok := virtualServicesMap[virtualServiceName]
		if ok {
			srv.addHosts(actualHosts...)
			srv.routes = append(srv.routes, r)
		} else {
			srv = virtualServiceData{
				name:     virtualServiceName,
				hostsSet: make(map[string]bool, 5),
				routes:   []utils.Route{r},
			}
			srv.addHosts(actualHosts...)
		}
		virtualServicesMap[virtualServiceName] = srv
	}
	return virtualServicesMap
}

func (f *requestFactory) newRequestsFromVirtualServices(gatewayName string, virtualServices map[string]virtualServiceData) []rest.RegistrationRequest {
	result := make([]rest.RegistrationRequest, 0)
	for virtualServiceName, virtualSrv := range virtualServices {
		request := &registrationRequest{
			&routingConfigRequest{
				Namespace: f.namespace,
				Gateways:  []string{gatewayName},
				VirtualServices: []virtualService{{
					Name:  virtualServiceName,
					Hosts: virtualSrv.getHosts(),
					RouteConfiguration: routeConfig{
						Version: f.deploymentVersion,
						Routes: []routeV3{{
							Destination: routeDestination{
								Cluster:  f.microserviceName,
								Endpoint: f.microserviceUrl,
								Tls:      tls{}},
							Rules: f.buildRules(virtualSrv.routes)},
						},
					},
				}},
			},
		}
		result = append(result, request)
	}
	return result
}

func (f *requestFactory) buildRules(routes []utils.Route) []rule {
	result := make([]rule, 0, len(routes))
	for _, r := range routes {
		allowed := r.Allowed
		result = append(result, rule{
			Match:         routeMatch{Prefix: r.From},
			PrefixRewrite: r.To,
			Allowed:       &allowed,
			Timeout:       r.TimeoutAsInt64(),
		})
	}
	return result
}

type registrationRequest struct {
	reqBody *routingConfigRequest
}

func (req *registrationRequest) ApiVersion() rest.ControlPlaneApiVersion {
	return rest.V3
}

func (req *registrationRequest) Payload() interface{} {
	return req.reqBody
}

type routingConfigRequest struct {
	Namespace       string           `json:"namespace"`
	Gateways        []string         `json:"gateways"`
	VirtualServices []virtualService `json:"virtualServices"`
}

type virtualService struct {
	Name               string             `json:"name"`
	Hosts              []string           `json:"hosts"`
	AddHeaders         []headerDefinition `json:"addHeaders"`
	RemoveHeaders      []string           `json:"removeHeaders"`
	RouteConfiguration routeConfig        `json:"routeConfiguration"`
}

type headerDefinition struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type routeConfig struct {
	Version string    `json:"version"`
	Routes  []routeV3 `json:"routes"`
}

type routeV3 struct {
	Destination routeDestination `json:"destination"`
	Rules       []rule           `json:"rules"`
}

type routeDestination struct {
	Cluster  string `json:"cluster"`
	Endpoint string `json:"endpoint"`
	Tls      tls    `json:"tls" `
}

type rule struct {
	Match         routeMatch         `json:"match"`
	PrefixRewrite string             `json:"prefixRewrite"`
	AddHeaders    []headerDefinition `json:"addHeaders"`
	RemoveHeaders []string           `json:"removeHeaders"`
	Allowed       *bool              `json:"allowed"`
	Timeout       *int64             `json:"timeout"`
}

type routeMatch struct {
	Prefix         string          `json:"prefix"`
	Regexp         string          `json:"regExp"`
	Path           string          `json:"path"`
	HeaderMatchers []headerMatcher `json:"headers"`
}

// TODO recheck after service mesh discussion
type tls struct {
	Enabled   bool   `json:"enabled"`
	Insecure  bool   `json:"insecure"`
	TrustedCA string `json:"trustedCA"`
	SNI       string `json:"sni"`
}

type headerMatcher struct {
	Name           string     `json:"name"`
	ExactMatch     string     `json:"exactMatch"`
	SafeRegexMatch string     `json:"safeRegexMatch"`
	RangeMatch     rangeMatch `json:"rangeMatch"`
	PresentMatch   *bool      `json:"presentMatch"`
	PrefixMatch    string     `json:"prefixMatch"`
	SuffixMatch    string     `json:"suffixMatch"`
	InvertMatch    bool       `json:"invertMatch"`
}

type rangeMatch struct {
	Start *int `json:"start"`
	End   *int `json:"end"`
}
