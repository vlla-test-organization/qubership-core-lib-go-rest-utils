package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	"github.com/netcracker/qubership-core-lib-go/v3/const"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
)

var log logging.Logger

func init() {
	log = logging.GetLogger("routemanagement")
	serviceloader.Register(2, &serviceloader.Token{})
}

type ControlPlaneClient struct {
	controlPlaneAddr string
	retryManager     *RetryManager
}

func NewControlPlaneClient(controlPlaneAddr string, retryManager *RetryManager) *ControlPlaneClient {
	return &ControlPlaneClient{controlPlaneAddr: formatAddr(controlPlaneAddr), retryManager: retryManager}
}

func formatAddr(addr string) string {
	for strings.HasSuffix(addr, "/") {
		addr = addr[0 : len(addr)-1]
	}
	log.Debugf("Control plane addr is %v", addr)
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return constants.SelectUrl("http://"+addr, "https://"+addr)
}

func (client *ControlPlaneClient) getApiUrl(request RegistrationRequest) (string, error) {
	switch request.ApiVersion() {
	case V3:
		return fmt.Sprintf("%s/api/v3/routes", client.controlPlaneAddr), nil
	default:
		errorMsg := fmt.Sprintf("control plane api version is not supported: %v", request.ApiVersion())
		log.Error(errorMsg)
		return "", errors.New(errorMsg)
	}
}

func (client *ControlPlaneClient) SendRequest(request RegistrationRequest) {
	url, err := client.getApiUrl(request)
	if err != nil {
		log.Panic("Failed to resolve api version: " + err.Error())
	}
	payload, err := json.Marshal(request.Payload())
	if err != nil {
		log.Panic("Failed to marshall route registration request to JSON: " + err.Error())
	}

	client.sendRequestWithRetry(url, payload)
}

func (client *ControlPlaneClient) sendRequestWithRetry(url string, payload []byte) {
	client.retryManager.DoWithRetry(func() error {
	    tokenProvider := serviceloader.MustLoad[serviceloader.TokenProvider]()
		token, err := tokenProvider.GetToken(context.Background())
		if err != nil {
			log.Errorf("Go error %+v during receiving m2m token", err.Error())
			return err
		}
		req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
		if err != nil {
			log.Errorf("Can not create request: %+v", err)
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" { req.Header.Set("Authorization", "Bearer " + token) }
		client := utils.GetClient()
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return errors.New(fmt.Sprintf("got error status code in route registration response: %d", resp.StatusCode))
		}
		return nil
	})
}

type RetryManager struct {
	progressiveTimeout *ProgressiveTimeout
}

func NewRetryManager(progressiveTimeout *ProgressiveTimeout) *RetryManager {
	return &RetryManager{progressiveTimeout: progressiveTimeout}
}

func (rm *RetryManager) DoWithRetry(action func() error) {
	defer func() {
		if r := recover(); r != nil {
			log.Debug("Can't connect to control plane, retry")
			time.Sleep(rm.progressiveTimeout.NextTimeoutValue())
			rm.DoWithRetry(action)
		}
	}()
	if err := action(); err != nil {
		log.Panic("Action failed with error: " + err.Error())
	}
	rm.progressiveTimeout.Reset()
}

type ProgressiveTimeout struct {
	baseTimeout            time.Duration
	startMultiplier        int
	endMultiplier          int
	multiplierStep         int
	currentMultiplierValue int
	maxTimeoutValue        time.Duration

	mutex *sync.Mutex
}

func NewProgressiveTimeout(baseTimeout time.Duration, startMultiplier int, endMultiplier int, multiplierStep int) *ProgressiveTimeout {
	if endMultiplier <= startMultiplier {
		log.Panic("EndMultiplier must be greater than startMultiplier in ProgressiveTimeout")
	}
	return &ProgressiveTimeout{
		baseTimeout:            baseTimeout,
		startMultiplier:        startMultiplier,
		endMultiplier:          endMultiplier,
		multiplierStep:         multiplierStep,
		currentMultiplierValue: startMultiplier,
		maxTimeoutValue:        time.Duration(int64(endMultiplier) * baseTimeout.Nanoseconds()),
		mutex:                  &sync.Mutex{}}
}

func (pt *ProgressiveTimeout) NextTimeoutValue() time.Duration {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	if pt.currentMultiplierValue >= pt.endMultiplier {
		return pt.maxTimeoutValue
	}

	result := time.Duration(int64(pt.currentMultiplierValue) * pt.baseTimeout.Nanoseconds())
	pt.currentMultiplierValue += pt.multiplierStep
	return result
}

func (pt *ProgressiveTimeout) Reset() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.currentMultiplierValue = pt.startMultiplier
}
