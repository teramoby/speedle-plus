//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.
package eval

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	defaultCustomerFunctionCallTimeout = 5 * time.Second
)

var (
	// Shared HTTP client with connection pooling for plain HTTP calls.
	defaultHTTPClient = &http.Client{
		Timeout: defaultCustomerFunctionCallTimeout,
	}

	// Cached HTTPS clients keyed by CA certificate content.
	httpsClients sync.Map
)

// getHTTPSClient returns a cached or newly created HTTPS client for the given CA.
func getHTTPSClient(ca string) *http.Client {
	if client, ok := httpsClients.Load(ca); ok {
		return client.(*http.Client)
	}
	caCertPool := x509.NewCertPool()
	if len(ca) > 0 {
		caCertPool.AppendCertsFromPEM([]byte(ca))
	}
	tlsConfig := &tls.Config{
		RootCAs: caCertPool,
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		Proxy:           http.ProxyFromEnvironment,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   defaultCustomerFunctionCallTimeout,
	}
	actual, _ := httpsClients.LoadOrStore(ca, client)
	return actual.(*http.Client)
}

type Request2Delegator struct {
	Function *pms.Function                `json:"function"`
	Request  *ext.CustomerFunctionRequest `json:"request"`
}

type FuncResult struct {
	Result interface{}
	TTL    int64
}

type FuncResultCache struct {
	sync.RWMutex
	Results map[string]FuncResult
}

func (frc *FuncResultCache) get(key string) (FuncResult, bool) {
	frc.RLock()
	defer frc.RUnlock()
	result, ok := frc.Results[key]
	return result, ok
}
func (frc *FuncResultCache) delete(key string) {
	frc.Lock()
	defer frc.Unlock()
	delete(frc.Results, key)
}
func (frc *FuncResultCache) add(key string, value FuncResult) {
	frc.Lock()
	defer frc.Unlock()
	frc.Results[key] = value
}

func (frc *FuncResultCache) AddToCache(key string, cf *pms.Function, result interface{}) {
	if cf.ResultCachable {
		ttl := int64(0)
		if cf.ResultTTL > 0 {
			ttl = time.Now().Unix() + cf.ResultTTL
		}
		cachedResult := FuncResult{
			Result: result,
			TTL:    ttl,
		}
		frc.add(key, cachedResult)
	}
}

func (frc *FuncResultCache) ReadFromCache(key string, cf *pms.Function) interface{} {
	if cf.ResultCachable {
		ret, ok := frc.get(key)
		if ok {
			if ret.TTL == 0 || time.Now().Unix() <= ret.TTL {
				return ret.Result
			}
			// Entry is expired. Return nil and let the periodic cleaner
			// (CleanExpiredResult) handle deletion to avoid write-lock
			// contention on the read path.
		}
	}
	return nil
}

func (frc *FuncResultCache) DeleteFromCache(funcName string) {
	frc.Lock()
	defer frc.Unlock()
	for key := range frc.Results {
		if isFunc(key, funcName) {
			delete(frc.Results, key)
		}
	}
}

func (frc *FuncResultCache) CleanExpiredResult() {
	frc.Lock()
	defer frc.Unlock()
	for key, value := range frc.Results {
		if value.TTL > 0 && time.Now().Unix() > value.TTL {
			delete(frc.Results, key)
		}
	}
}

func (frc *FuncResultCache) generateCustomerExpressionFunction(cfdUrl *string, cf *pms.Function) (govaluate.ExpressionFunction, error) {
	return func(arguments ...interface{}) (interface{}, error) {
		params := []interface{}{}
		for _, param := range arguments {
			params = append(params, param)
		}
		request := &ext.CustomerFunctionRequest{
			Params: params,
		}
		key := getKey(cf.Name, arguments)
		var result interface{}
		var err error
		if result = frc.ReadFromCache(key, cf); result != nil {
			return result, nil
		}
		if *cfdUrl == "" { //no delegator configured, request goes directly to customer function service
			result, err = CallCustomerFunction(cf, request)
		} else { //delegator configured, send request to delegator over http, and delegator sends request to customer function service over https
			result, err = CallCustomerFunctionViaDelegator(*cfdUrl, cf, request)
		}
		if err == nil {
			frc.AddToCache(key, cf, result)
		}
		return result, err
	}, nil
}

func getKey(funcName string, arguments []interface{}) string {
	return fmt.Sprintf("%s(%v)", funcName, arguments)
}

func isFunc(key, funcName string) bool {
	return strings.HasPrefix(key, funcName+"(")
}

func CallCustomerFunctionViaDelegator(delegatorUrl string, cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
	req2Delegator := Request2Delegator{
		Function: cf,
		Request:  request,
	}
	buf, err := json.Marshal(req2Delegator)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", delegatorUrl, bytes.NewBuffer(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return getFunctionResp(defaultHTTPClient, req, cf)
}

func CallCustomerFunction(cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
	var client *http.Client
	if strings.HasPrefix(strings.ToLower(cf.FuncURL), "https:") {
		client = getHTTPSClient(cf.CA)
	} else if strings.HasPrefix(strings.ToLower(cf.FuncURL), "http:") {
		client = defaultHTTPClient
	} else {
		return nil, errors.Errorf(errors.CustomerFuncError, "URL of customer function %q is not supported", cf.FuncURL)
	}

	buf, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", cf.FuncURL, bytes.NewBuffer(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return getFunctionResp(client, req, cf)

}

func getFunctionResp(client *http.Client, request *http.Request, cf *pms.Function) (interface{}, error) {
	resp, err := client.Do(request)
	if err != nil {
		log.Errorf("error happens when calling customer function %s, err is: %v\n", cf.Name, err)
		return nil, errors.Wrapf(err, errors.CustomerFuncError, "failed to do customer function request for customer function %q", cf.Name)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		//TODO: We might need to limit the larget size we want to receive
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("error reading response from customer function %s, err is: %v\n", cf.Name, err)
			return nil, errors.Wrapf(err, errors.CustomerFuncError, "fail to read response for customer function %q", cf.Name)
		}
		response := ext.CustomerFunctionResponse{}
		if err = json.Unmarshal(body, &response); err != nil {
			log.Errorf("error unmarshaling response from customer function %s, err is: %v\n", cf.Name, err)
			return nil, errors.Wrapf(err, errors.CustomerFuncError, "fail to unmarshal response for customer function %q", cf.Name)
		} else if response.Error != "" {
			log.Errorf("error in response from customer function %s, err is: %v\n", cf.Name, response.Error)
			return nil, errors.Errorf(errors.CustomerFuncError, "customer function %q returns error %q", cf.Name, response.Error)
		} else {
			return response.Result, nil
		}
	default:
		log.Errorf("Invalid status code returns when calling customer function %s, status code is : %v\n", cf.Name, resp.StatusCode)
		return nil, errors.Errorf(errors.CustomerFuncError, "unexpected http status %d returned when calling customer function %s", resp.StatusCode, cf.Name)
	}
}
