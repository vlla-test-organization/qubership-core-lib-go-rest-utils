package consul

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	consulApi "github.com/hashicorp/consul/api"
	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/v2"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
)

const (
	baseConfigPath        = "config"
	baseLoggingPath       = "logging"
	thresholdBeforeUpdate = 2 * time.Minute
)

type ProviderConfig struct {
	// Consul URL (default: value from consul.url)
	Address string
	// Microservice namespace (default: value from microservice.namespace)
	Namespace string
	// Microservice name (default: value from microservice.name)
	MicroserviceName string
	// A list of path roots for config properties (default: "config/<namespace>/application", "config/<namespace>/<microservice-name>")
	Paths []string
	// Custom context (default: context.Background())
	Ctx context.Context
	// If true, then all problems with connection to Consul will be ignored and will not fail the application (default: false)
	Failsafe bool
	// Setting a value in the Token field disables the mechanism for obtaining a Consul token via anonymous request and instead uses the specified token
	// nil by default is switches property source to use anonymous request authentication mechanism
	Token string
	// Allows to override getting m2m token logic (default: nil)
	tokenProvider func() (string, error)
	// If true, then default Paths will be "logging/<namespace>/<microservice-name>" (default: false)
	forLogging bool
}

type ProviderImpl struct {
	cfg         ProviderConfig
	client      *Client
	configIndex map[string]uint64
	watchOnce   sync.Once
	tokenProvider func() (string, error)
}

type Provider interface {
	configloader.PropertyProvider
	Watch(cb func(event interface{}, err error)) error
}

func DefaultPathsFor(microserviceName, namespace string) []string {
	return []string{
		baseConfigPath + "/" + namespace + "/application",
		baseConfigPath + "/" + namespace + "/" + microserviceName,
	}
}

func DefaultLoggingPathsFor(microserviceName, namespace string) []string {
	return []string{
		baseLoggingPath + "/" + namespace + "/" + microserviceName,
	}
}

func NewPropertySource(configs ...ProviderConfig) *configloader.PropertySource {
	var config ProviderConfig
	if len(configs) > 0 {
		config = configs[0]
	} else {
		config = ProviderConfig{}
	}
	return &configloader.PropertySource{Provider: newProvider(config)}
}

func NewLoggingPropertySource(configs ...ProviderConfig) *configloader.PropertySource {
	var config ProviderConfig
	if len(configs) > 0 {
		config = configs[0]
		config.forLogging = true
	} else {
		config = ProviderConfig{
			forLogging: true,
			Failsafe:   true,
		}
	}

	return &configloader.PropertySource{Provider: newProvider(config)}
}

func newProvider(cfg ProviderConfig) *ProviderImpl {
	var token *ClientToken
	if cfg.Token != "" {
		token = &ClientToken{}
		token.val.Store(cfg.Token)
	}

	return &ProviderImpl{
		cfg: cfg,
		client: NewClient(ClientConfig{
			Address:     cfg.Address,
			Namespace:   cfg.Namespace,
			Ctx:         cfg.Ctx,
			Token:       token,
			tokenProvider: cfg.tokenProvider,
		}),
		configIndex: make(map[string]uint64),
		tokenProvider: cfg.tokenProvider,
	}
}

func (r *ProviderImpl) ReadBytes(*koanf.Koanf) ([]byte, error) {
	return nil, errors.New("consul provider does not support this method")
}

func (r *ProviderImpl) Read(*koanf.Koanf) (map[string]interface{}, error) {
	r.fillDefaultsIfNeeded()
	err := r.client.Login()
	if err != nil {
		if r.cfg.Failsafe {
			logger.Errorf("error during login to Consul: %w", err)
			return make(map[string]interface{}), nil
		} else {
			return nil, err
		}
	}
	return r.loadConsulConfigParams()
}

func (r *ProviderImpl) Watch(cb func(event interface{}, err error)) error {
	r.watchOnce.Do(func() {
		for _, p := range r.cfg.Paths {
			r.client.subscribeFor(p, r.configIndex[p], cb)
		}
	})
	return nil
}

func AddConsulPropertySource(sources []*configloader.PropertySource, configs ...ProviderConfig) []*configloader.PropertySource {
	return append(sources, NewPropertySource(configs...))
}

func (r *ProviderImpl) fillDefaultsIfNeeded() {
	if r.cfg.Ctx == nil {
		r.cfg.Ctx = context.Background()
		r.client.cfg.Ctx = r.cfg.Ctx
	}
	if r.cfg.Namespace == "" {
		r.cfg.Namespace = configloader.GetOrDefaultString("microservice.namespace", "")
		r.client.cfg.Namespace = r.cfg.Namespace
	}
	if r.cfg.MicroserviceName == "" {
		r.cfg.MicroserviceName = configloader.GetOrDefaultString("microservice.name", "")
	}
	if r.cfg.Address == "" {
		consulUrl := configloader.GetOrDefaultString("consul.url", "")
		_, err := url.ParseRequestURI(consulUrl)
		if err != nil {
			if r.cfg.Failsafe {
				logger.Errorf("can not parse Consul URL: %s", err.Error())
			} else {
				logger.Panicf("can not parse Consul URL: %s", err.Error())
			}
		}
		r.cfg.Address = consulUrl
		r.client.cfg.Address = r.cfg.Address
	}
	if len(r.cfg.Paths) == 0 {
		if r.cfg.forLogging {
			r.cfg.Paths = DefaultLoggingPathsFor(r.cfg.MicroserviceName, r.cfg.Namespace)
		} else {
			r.cfg.Paths = DefaultPathsFor(r.cfg.MicroserviceName, r.cfg.Namespace)
		}
	}
}

func (r *ProviderImpl) loadConsulConfigParams() (map[string]interface{}, error) {
	configMap := make(map[string]interface{})
	for _, path := range r.cfg.Paths {
		list, meta, err := r.client.KV().List(path, nil)
		if err != nil {
			if r.cfg.Failsafe {
				logger.Errorf("error during loading from Consul: %s", err.Error())
				continue
			} else {
				return nil, err
			}
		}

		kvPairsAsMap(cutPrefix(list, path), configMap)
		r.configIndex[path] = meta.LastIndex
	}

	flattenMap, _ := maps.Flatten(configMap, []string{}, ".")
	return flattenMap, nil
}

func cutPrefix(pairs consulApi.KVPairs, prefix string) consulApi.KVPairs {
	prefix = strings.TrimPrefix(prefix, "/")
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	for i, p := range pairs {
		pairs[i].Key = strings.TrimPrefix(p.Key, prefix)
	}
	return pairs
}

func kvToMap(key, value string, res map[string]interface{}) map[string]interface{} {
	if key == "" {
		return res
	}
	splitKeys := strings.SplitN(key, "/", 2)
	_, ok := res[splitKeys[0]]
	if !ok {
		res[splitKeys[0]] = make(map[string]interface{})
	}

	if len(splitKeys) == 1 {
		res[splitKeys[0]] = value
	} else {
		res[splitKeys[0]] = kvToMap(strings.TrimPrefix(splitKeys[1], "/"), value, res[splitKeys[0]].(map[string]interface{}))
	}
	return res
}

func kvPairsAsMap(kvPairs consulApi.KVPairs, addTo map[string]interface{}) {
	sort.Slice(kvPairs, func(i, j int) bool {
		return len(kvPairs[i].Key) > len(kvPairs[j].Key)
	})
	shiftIndex := -1
	for _, kv := range kvPairs {
		logger.Debugf("Key: %v , Value: %v ", kv.Key, string(kv.Value))
		if shiftIndex >= 0 && strings.Contains(kvPairs[shiftIndex].Key, kv.Key) {
			logger.Debugf("Found nested keys: %v and %v", kvPairs[shiftIndex].Key, kv.Key)
			kv.Key = strings.ReplaceAll(kv.Key, "/", ".")
		} else {
			shiftIndex = shiftIndex + 1
		}
		kvToMap(kv.Key, string(kv.Value), addTo)
	}
}
func WatchForProperties(consulPropertySource *configloader.PropertySource, projectFunc func(event interface{}, err error)) error {
	provider := consulPropertySource.Provider.(Provider)
	coreFunc := func() {
		err := configloader.Refresh()
		if err != nil {
			logger.Errorf("error during refresh the configuration: %v", err)
		}
	}
	cbFunc := func(event interface{}, err error) {
		coreFunc()
		projectFunc(event, err)
	}
	err2 := provider.Watch(cbFunc)
	if err2 != nil {
		logger.Errorf("error on starting watch: %v", err2)
		return err2
	}
	return nil
}

func StartWatchingForPropertiesWithRetry(ctx context.Context, consulPropertySource *configloader.PropertySource, projectFunc func(event interface{}, err error)) {
	go func() {
		err := retry.Do(
			func() error {
				if err := WatchForProperties(consulPropertySource, projectFunc); err != nil {
					logger.ErrorC(ctx, "cannot start watching property: %v", err.Error())
					return fmt.Errorf("cannot start watching property: %w", err)
				}
				return nil
			},
			retry.Context(ctx),
			retry.Delay(5*time.Second), retry.MaxDelay(5*time.Minute),
			retry.DelayType(retry.BackOffDelay),
			retry.UntilSucceeded(),
		)

		if err != nil {
			logger.ErrorC(ctx, "stop retrying: cannot start watching property: %v", err.Error())
		}
	}()
}
