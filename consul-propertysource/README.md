# Consul-propertysource

The package provides Consul property source which is intended for [Configloader](../configloader/README.md).
This property source allows downloading properties from a Consul service.

- [How to get](#how-to-get)
- [Usage](#usage)
- [Plain Consul Client](#plain-consul-client)
  
## How to get
To get `consul-propertysource` use
```go
 go get github.com/netcracker/qubership-core-lib-go-rest-utils/@<latest released version>
```

List of all released versions may be found [here](https://github.com/netcracker/qubership-core-lib-go-rest-utils/-/tags)

## Usage

Create consul property source and add it to config loader([see for additional info](https://github.com/netcracker/qubership-core-lib-go/blob/main/configloader/README.md#Usage)) 
  ```go
  consulPS := consul.NewPropertySource(
    consul.ProviderConfig{
        Address:          "<consul-url>",
        Namespace:        "<namespace>",
        MicroserviceName: "<microservice-name>",
        Paths:            "<consul-kv-paths>",
        Ctx:              "<context>",
        Failsafe:         <failsafe>,
        Token:            "<token>",
      }
    )
  ```

*  **consul-url** - Consul URL (default: value from **consul.url**)
*  **namespace** - microservice namespace (default: value from **microservice.namespace**)
*  **microservice-name** - microservice name (default: value from **microservice.name**)
*  **consul-kv-paths** - a list of path roots for config properties (default: **"config/\<namespace\>/application", "config/\<namespace\>/\<microservice-name\>"**)
*  **context** - custom context (default: **context.Background()**)
*  **failsafe** - if true, then all problems with connection to Consul will be ignored and will not fail the application (default: false)
*  **token** - setting a value in the Token field disables the mechanism for obtaining a Consul token via anonymous request and instead uses the specified token (nil by default is switches property source to use anonymous request authentication mechanism)

All properties are optional.

Also you can watch for config changes
```go
consul.WatchForProperties(consulPS, func(event interface{}, err error) {
    // your code here if any action on event is required
})
```
WatchForProperties automatically refreshes the properties in application by property sources.

### Consul for logging levels update

If you want to use Consul for automatic logging level updates, then you can create special Consul PropertySource and init your configloader with it:
```go
consulPS := consul.NewLoggingPropertySource()
configloader.InitWithSourcesArray(append(configloader.BasePropertySources(), consulPS))
consul.StartWatchingForPropertiesWithRetry(context.Background(), consulPS, func(event interface{}, err error) {
    // your code here if any action on event is required
})
```

Then logging configuration will be automatically gathered from 'logging/<namespace>/<microserviceName>' root.

## Plain Consul Client

To access to Consul KV storage you can use plain KV client

```go
c := consul.NewClient(consul.ClientConfig{
    Address:   "<consul-url>",
    Namespace: "<namespace>",
    Ctx:       "<context>",
})
c.Login()
l, _, _ := c.KV().List("/", &api.QueryOptions{
    Token: c.SecretId(),
})
```

You **must** call c.Login() before first usage.

* **consul-url** - Consul URL
* **namespace** - microservice namespace
* **context** - custom context