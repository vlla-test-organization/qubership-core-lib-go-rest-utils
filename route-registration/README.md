# Route-registration

This library allows sending requests to control plane for route registration.  The library can register routes in 
`internal`, `private`, `public`, and `facade` gateways. Thanks to this any microservice can make REST call without knowing an url host. 

- [Installation](#installation)
- [Usage](#usage)
  * [Route Structure](#route-structure)
  * [Route types and Gateways](#route-types-and-gateways)
- [Quick example](#quick-example)


## Installation 

To get `route-registration` use
```go
 go get github.com/netcracker/qubership-core-lib-go-rest-utils/@<latest released version>
```

List of all released versions may be found [here](https://github.com/netcracker/qubership-core-lib-go-rest-utils/-/tags)

## Usage

There are several parameters for route-registration process configuration.

Also user may specify such parameters as:

|Field           | Description                                                                                                   | Optional | Default    |
|----------------|---------------------------------------------------------------------------------------------------------------|----------|------------|
| microservice.name | Name of microservice, for example _tenant-manager_ | false | - |
| microservice.url | Url which corresponds to microservice. If absent, url will be constructed out of microservice name  | true | - |
| server.port | If microservice.url is absent, this value will be used if url configuration | true | 8080 |

Other required params will be taken from Openshift. To read more about that params visit 
[Prepare Deployment Configuration](#https://github.com/netcracker/control-plane/blob/main/docs/mesh/bluegreen-migration-guide.md)

**Route Registration Builder.**

Public API is located in `routeregistration` package. There you can find constant values for Route Types and for Getaways.

To register routes use builder.
```go
    routeregistration.NewRegistrar().WithRoutes(routes ...routeregistration.Route).Register()
```

Func `WithRoutes(routes ...routeregistration.Route)` will create request with routes for registration.
Func `Register()` will send this request to control-plane.

### Route structure

Route has next structure, some fields are optional, and some are mandatory.
```go
type Route struct {
	From           string
	To             string
	Forbidden      bool
	Timeout        time.Duration
	RouteType      RouteType
	Gateway        string
	VirtualService string
	Hosts          []string
}
```

|Field           | Description                                                                                                   | Optional | Default    |
|----------------|---------------------------------------------------------------------------------------------------------------|----------|------------|
| From           | 'from' path prefix - prefix to match request path in gateway                                                  | false    |     -      |
| To             | 'to' path prefix - prefix to rewrite matched 'from' path prefix before forwarding request to the microservice | false    |     -      |
| Forbidden      | whether this is allowed or forbidden route (forbidden route will respond 404 on any requests).                | true     | false      |
| Timeout        | route timeout in milliseconds.                                                                                | true     | 0          |
| RouteType      | see [Route types and Gateways](#route-type-and-gateways) section for more info                                | true     | Mesh       |
| Gateway        | see [Route types and Gateways](#route-type-and-gateways) section for more info                                | true     | -          |
| VirtualService | virtual service name                                                                                          | true     | equals to the actual target gateway name |
| Hosts          | virtual service hosts.                                                                                        | true     |  ["*"]     |

Hosts from different routes are merged in single VirtualService, which is unique by pair `<gateway name>` + `<VirtualService name>`. 
So each gateway has its own virtual service and their hosts may differ, even if virtual service names are equal.

### Route types and gateways

RouteType stands for the type of route to be registered.

* Public type declares that routes should be registered in PUBLIC, PRIVATE and INTERNAL gateways.
* Private type declares that routes should be registered in PRIVATE and INTERNAL gateways.
* Internal type declares that routes should be registered only in INTERNAL gateway.
* Mesh type declares that routes should be registered in MESH gateway with the specified name.

Route type parameter can have one of the following values: `routeregistration.Public`, `routeregistration.Private`, `routeregistration.Internal`
and `routeregistration.Mesh`. If you didn't specify RouteType, default value (`mesh`) would be used.

This example will register route in internal, private and public gateways.
```go
  routeregistration.NewRegistrar().WithRoutes(
    routeregistration.Route{ // route with type Public
      From:      "/api/v1/sample-service/public-resource/private-resource",
      To:        "/api/v1/public-resource/private-resource",
      RouteType: routeregistration.Public, 
    },
  ).Register()
```

Gateways may have one of const values, which are `routeregistration.PublicGatewayService`, `routeregistration.PrivateGatewayService` and
`routeregistration.InternalGatewayService` or specify gateway name for mesh routes.

_Please note that_
* when Route#RouteType is empty and Route#Gateway is border gateway, Route#Gateway is threated as RouteType: 3 routes will be registered
* when Route#RouteType and Route#Gateway are not empty and have conflict, library panics with error message.

To find more info about gateways see [Gateways](https://github.com/netcracker/control-plane/blob/main/docs/mesh/development-guide.md#gateway-types)

## Quick example

_Don't forget to init configloader before registering routes._

```go
package main

import (
  "github.com/configloader-base/configloader"
  "github.com/netcracker/go-route-registration-lib/routeregistration"
  "time"
)github.com

func init() {
  configloader.Init(configloader.BasePropertySources())
}

func main() {
  routeregistration.NewRegistrar().WithRoutes(
    routeregistration.Route{ // route with type Public
      From:      "/api/v1/sample-service/public-resource",
      To:        "/api/v1/public-resource",
      Forbidden: false,                    // optional field
      Timeout:   1000 * time.Millisecond,  // optional field
      RouteType: routeregistration.Public, // default Hosts for border gateways == '*'
    },

    routeregistration.Route{ // route with type Private
      From:      "/api/v1/sample-service/public-resource/private-resource",
      To:        "/api/v1/public-resource/private-resource",
      RouteType: routeregistration.Private, // default Hosts for border gateways == '*'
    },

    routeregistration.Route{ // routes with type Internal [route 1]
      From:      "/api/v1/sample-service/internal-resource-1",
      To:        "/api/v1/internal-resource-1",
      RouteType: routeregistration.Internal, // default Hosts for border gateways == '*'
    },
    routeregistration.Route{ // routes with type Internal [route 2]
      From:      "/api/v1/sample-service/internal-resource-2",
      To:        "/api/v1/internal-resource-2",
      RouteType: routeregistration.Internal, // default Hosts for border gateways == '*'
    },
    routeregistration.Route{
      From:           "/api/v1/sample-service/activate/{tenantId}",
      To:             "/api/v1/{tenantId}/activate",
      RouteType: routeregistration.Internal, // default Hosts for border gateways == '*'
    },

    // 2 routes for same gateway but different hosts
    routeregistration.Route{
      From:           "/",
      To:             "/",
      Gateway:        "ingress-service", // default type == Mesh
      Hosts:          []string{"sample-service:8080"},
      VirtualService: "sample-service-ingress",
    },
    routeregistration.Route{
      From:           "/api/v1/another-service/ingress",
      To:             "/api/v1/ingress",
      Gateway:        "ingress-service", // default type == Mesh
      Hosts:          []string{"another-service:8080"},
      VirtualService: "another-service-ingress",
    },

    routeregistration.Route{ // this microservice facade gateway route [option 1]
      From:    "/",
      To:      "/",
      Gateway: "another-service",                   // default type == Mesh
      Hosts:   []string{routeregistration.AnyHost}, // default VirtualService == gateway name ("another-service")
    },
    routeregistration.Route{ // this microservice facade gateway route [option 2]
      From:      "/",
      To:        "/",
      RouteType: routeregistration.Mesh,              // default mesh gateway == microserviceName
      Hosts:     []string{routeregistration.AnyHost}, // default VirtualService == gateway name ("another-service")
    },

    routeregistration.Route{ // another microservice facade gateway route
      From:    "/",
      To:      "/",
      Gateway: "sample-service",                    // default type == Mesh
      Hosts:   []string{routeregistration.AnyHost}, // default VirtualService == gateway name ("sample-service")
    },
  ).Register()
}

```


