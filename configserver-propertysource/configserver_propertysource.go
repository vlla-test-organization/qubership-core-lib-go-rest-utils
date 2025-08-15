package configserver

import "github.com/vlla-test-organization/qubership-core-lib-go/v3/configloader"

type PropertySourceConfiguration struct {
	MicroserviceName string // name of microservice
	ConfigServerUrl  string // URL to config-server, example is <http://config-server:8080>
}

func GetPropertySource(params ...PropertySourceConfiguration) *configloader.PropertySource {
	var configuration PropertySourceConfiguration
	if len(params) > 0 {
		configuration = params[0]
	}
	return &configloader.PropertySource{Provider: newConfigServerLoader(&configuration)}
}

func AddConfigServerPropertySource(sources []*configloader.PropertySource, params ...PropertySourceConfiguration) []*configloader.PropertySource {
	return append(sources, GetPropertySource(params...))
}
