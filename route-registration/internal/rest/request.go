package rest

import (
	"github.com/vlla-test-organization/qubership-core-lib-go-rest-utils/v2/route-registration/internal/utils"
)

type RequestFactory interface {
	NewRequests(routesByGatewayMap utils.RoutesByGateway) []RegistrationRequest
}

type RegistrationRequest interface {
	ApiVersion() ControlPlaneApiVersion
	Payload() interface{}
}

type ControlPlaneApiVersion string

const (
	V3 ControlPlaneApiVersion = "V3"
)
