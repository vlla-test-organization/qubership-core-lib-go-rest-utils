# Configserver-propertysource

The package provides configserver property source which is intended for [Configloader](../configloader/README.md).
 This property source allows downloading properties from a config-server service.

- [How to get](#how-to-get)
- [Usage](#usage)
   * [Property source configuration](#property-source-configuration)

## How to get

To get `configserver-propertysource` use
```go
 go get go github.com/netcracker/qubership-core-lib-go-rest-utils@<latest released version>
```

List of all released versions may be found [here](https://github.com/netcracker/qubership-core-lib-go-rest-utils/-/tags)

## Usage

There are two ways to add config-server as a property source. 

**Note!** If you want to use config-server as a property source don't forget to set up a `microservice.name` property (or you'll get a panic with warning). 
You may also set url for config-server with property `config-server.url`

1. Func `configserver.AddConfigServerPropertySource(sources []configloader.PropertySource)`. This allows getting configserver as PropertySource
   to any array of PropertySources. Configserver will have the highest priority, because it will be the last property source in list.

    We recommend to use `configserver.AddConfigServerPropertySource` in such way:
    ```go
        configserver.AddConfigServerPropertySource(configloader.BasePropertySource())
    ```
    In this example three PropertySources will be registered with priority: configserver > environment > yaml.

2. Func `configserver.GetPropertySource(params configserver.ConfigServerPropertySourceConfiguration)`. This allows getting only configserver propertySource.

    You have to pass struct `configserver.ConfigServerPropertySourceConfiguration` as a parameter. This struct has fields:
   
    Example:
    ```go
        params := configserver.ConfigServerPropertySourceConfiguration{
            MicroserviceName: "test",
            ConfigServerUrl: "http://localhost:8888",
        }
        configloader.Init(configserver.PropertySource(params))
    ```

### Property source configuration

Config-server property source may be configured through `configserver.PropertySourceConfiguration` object.
PropertySourceConfiguration has such fields as:

|Property Name | Description | Default |
|--------------|-------------|---------|
|MicroserviceName | This is microservice name |none `mandatory`|
|ConfigServerUrl| Allows to configure config-server url. |http://config-server:8080|


