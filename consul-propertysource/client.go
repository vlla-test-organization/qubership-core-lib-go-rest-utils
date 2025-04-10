package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	consulApi "github.com/hashicorp/consul/api"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("consul-property-source")
}

type ClientConfig struct {
	Address       string
	Namespace     string
	Ctx           context.Context
	Token         *ClientToken
	tokenProvider func() (string, error)
}

type Client struct {
	consul     *consulApi.Client
	cfg        ClientConfig
	token      *ClientToken
	tokenWatch *sync.Once
	mutex      *sync.Mutex
}

type ClientToken struct {
	val            atomic.Value
	expirationTime time.Time
}

func NewClient(cfg ClientConfig) *Client {
	if cfg.tokenProvider == nil {
		cfg.tokenProvider = func() (string, error) {
			tokenProvider := serviceloader.MustLoad[security.TokenProvider]()
			return tokenProvider.GetToken(cfg.Ctx)
		}
	}
	return &Client{
		consul:     newConsulApiClient(cfg.Address, ""),
		cfg:        cfg,
		token:      cfg.Token,
		tokenWatch: &sync.Once{},
		mutex:      &sync.Mutex{},
	}
}

func (r *Client) KV() *consulApi.KV {
	tokenValue, _ := r.token.val.Load().(string)
	r.consul = newConsulApiClient(r.cfg.Address, tokenValue)
	return r.consul.KV()
}

func (r *Client) Login() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.token != nil {
		return nil
	}
	secretId, expTime, err := r.getSecretId()
	if err != nil {
		return err
	}
	r.token = &ClientToken{}
	r.token.val.Store(secretId)
	r.token.expirationTime = *expTime
	r.consul = newConsulApiClient(r.cfg.Address, secretId)

	r.tokenWatch.Do(func() {
		r.startWatchSecretId(expTime.Sub(time.Now().Add(thresholdBeforeUpdate)))
	})
	return nil
}

func (r *Client) SecretId() string {
	tokenValue, _ := r.token.val.Load().(string)
	return tokenValue
}
func newConsulApiClient(addr, token string) *consulApi.Client {
	consulConfig := consulApi.DefaultConfig()
	consulConfig.Address = strings.TrimSuffix(addr, "/")
	consulConfig.Token = token
	consulConfig.TLSConfig = consulApi.TLSConfig{
		CAFile:   utils.GetCaCertFile(),
		CertFile: utils.GetCertFile(),
		KeyFile:  utils.GetKeyFile(),
	}
	client, err := consulApi.NewClient(consulConfig)
	if err != nil {
		logger.Panicf("can not create Consul client: %s", err.Error())
		return nil
	}
	return client
}

func (r *Client) getSecretIdByToken(token string) (string, *time.Time, error) {
	consulUrl := strings.TrimSuffix(r.cfg.Address, "/")
	payload := map[string]string{
		"Accept": "application/json",
	}
	returnOnError := "anonymous"
	if token != "" {
		returnOnError = ""
		payload = map[string]string{
			"AuthMethod":  r.cfg.Namespace,
			"BearerToken": token,
		}
	}
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		return returnOnError, nil, err
	}
	requestUrl := consulUrl + "/v1/acl/login"
	response, err := http.Post(requestUrl, "application/json", strings.NewReader(string(payloadJson)))
	if err != nil {
		return returnOnError, nil, fmt.Errorf("failed to send request '%s': %w", requestUrl, err)
	}
	defer response.Body.Close()
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return returnOnError, nil, fmt.Errorf("failed to read response content: %w", err)
	}

	if response.StatusCode != 200 {
		return returnOnError, nil, fmt.Errorf("non 200 response from Consul to request '%s': %d - %s",
			requestUrl, response.StatusCode, string(respBody))
	}

	var respBodyMap map[string]interface{}
	err = json.Unmarshal(respBody, &respBodyMap)
	if err != nil {
		return returnOnError, nil, fmt.Errorf("failed to parse response content: %w", err)
	}

	secretId := respBodyMap["SecretID"].(string)
	expirationTimeString := respBodyMap["ExpirationTime"].(string)
	expirationTime, err := time.Parse(time.RFC3339, expirationTimeString)
	if err != nil {
		return returnOnError, nil, fmt.Errorf("failed to parse ExpirationTime '%s': %w", expirationTimeString, err)
	}
	return secretId, &expirationTime, nil
}

func (r *Client) getSecretId() (string, *time.Time, error) {
	token, err := r.cfg.tokenProvider()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get token: %w", err)
	}
	secretId, expTime, err := r.getSecretIdByToken(token)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get consul secret ID by token: %w", err)
	}
	return secretId, expTime, nil
}

func (r *Client) startWatchSecretId(every time.Duration) {
	ticker := time.NewTicker(every)
	go func() {
		for {
			select {
			case <-ticker.C:
				secretId, expTime, err := r.getSecretId()
				if err != nil {
					logger.ErrorC(r.cfg.Ctx, "can not update consul secretId: %s", err.Error())
					continue
				}
				ticker.Reset(expTime.Sub(time.Now().Add(thresholdBeforeUpdate)))
				r.token.val.Store(secretId)
			case <-r.cfg.Ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (r *Client) subscribeFor(path string, keyIndex uint64, cb func(event interface{}, err error)) {
	currentIndex := keyIndex
	go func() {
		for {
			err := retry.Do(
				func() error {
					err := r.Login()
					if err != nil {
						logger.Errorf("Error during login to Consul: %w", err)
						return fmt.Errorf("error during login to Consul: %w", err)
					}

					list, meta, err := r.KV().List(path, (&consulApi.QueryOptions{WaitIndex: currentIndex}).WithContext(r.cfg.Ctx))
					if err != nil {
						logger.ErrorC(r.cfg.Ctx, "Error read from KV: key=%s; err=%s", path, err.Error())
						return fmt.Errorf("error reading from KV: %w", err)
					}
					if list == nil {
						logger.ErrorC(r.cfg.Ctx, "there is no path created for '%s'", path)
						return fmt.Errorf("path for '%s' is not exists", path)
					}

					if list != nil && meta != nil {
						configMap := make(map[string]interface{})
						kvPairsAsMap(cutPrefix(list, path), configMap)
						cb(nil, nil)
						currentIndex = meta.LastIndex
					}
					return nil
				},
				retry.Context(r.cfg.Ctx),
				retry.Delay(5*time.Second),
				retry.MaxDelay(5*time.Minute),
				retry.DelayType(retry.BackOffDelay),
				retry.UntilSucceeded(),
			)
			if err != nil {
				logger.ErrorC(r.cfg.Ctx, "Stopped subscription: %v", err.Error())
				return
			}
		}
	}()
}
