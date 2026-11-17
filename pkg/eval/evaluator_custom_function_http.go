//go:build !js || !wasm

// Licensed under the Universal Permissive License (UPL) Version 1.0.
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
	"time"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/logging"
)

// Native evaluators retain the existing custom-function behavior.
func validatePolicyStoreCustomFunctions(_ *pms.PolicyStore) error { return nil }
func validateServiceCustomFunctions(_ *pms.Service, _ map[string]struct{}) error {
	return nil
}
func validateConditionCustomFunctions(_, _, _, _ string, _ map[string]struct{}) error {
	return nil
}
func validateFunctionAdditionCustomFunctions(_ *RuntimePolicyStore, _ *pms.Function) error {
	return nil
}

func (frc *FuncResultCache) generateCustomerExpressionFunction(cfdUrl *string, cf *pms.Function) (govaluate.ExpressionFunction, error) {
	return func(arguments ...interface{}) (interface{}, error) {
		return frc.callCustomFunction(cf, arguments, func(request *ext.CustomerFunctionRequest) (interface{}, error) {
			if *cfdUrl == "" { //no delegator configured, request goes directly to customer function service
				return CallCustomerFunction(cf, request)
			}
			//delegator configured, send request to delegator over http, and delegator sends request to customer function service over https
			return CallCustomerFunctionViaDelegator(*cfdUrl, cf, request)
		})
	}, nil
}

func CallCustomerFunctionViaDelegator(delegatorUrl string, cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
	req2Delegator := Request2Delegator{
		Function: cf,
		Request:  request,
	}
	//assume that http is used when communicate with delegator.
	var client *http.Client = &http.Client{
		Timeout: defaultCustomerFunctionCallTimeout,
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
	return getFunctionResp(client, req, cf)
}

// systemCertPool is x509.SystemCertPool, replaced in tests.
var systemCertPool = x509.SystemCertPool

func CallCustomerFunction(cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
	var client *http.Client
	if strings.HasPrefix(strings.ToLower(cf.FuncURL), "https:") {
		//TODO: load sphinx cert in case func server verifies client
		/*var cert tls.Certificate
		cert, err := tls.LoadX509KeyPair("./client.crt",	"./client.key")
		if err != nil {
			log.Fatal(err)
		}*/

		// Trust the system roots, plus the function's own CA when it has one
		// (only needed for a certificate signed by an unknown CA). An empty
		// pool would reject every publicly signed function server.
		caCertPool, err := systemCertPool()
		if err != nil {
			caCertPool = x509.NewCertPool()
		}
		if len(cf.CA) > 0 {
			caCertPool.AppendCertsFromPEM([]byte(cf.CA))
		}

		// Setup HTTPS client
		tlsConfig := &tls.Config{
			//Certificates: []tls.Certificate{cert},
			RootCAs: caCertPool,
		}
		transport := &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
		}
		client = &http.Client{
			Transport: transport,
			Timeout:   defaultCustomerFunctionCallTimeout,
		}

	} else if strings.HasPrefix(strings.ToLower(cf.FuncURL), "http:") {
		client = &http.Client{
			Timeout: defaultCustomerFunctionCallTimeout,
		}
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
	// Each call builds its own client; release its connections when done.
	defer client.CloseIdleConnections()
	resp, err := client.Do(request)
	if err != nil {
		logging.Default().Error(fmt.Sprintf("error happens when calling customer function %s, err is: %v\n", cf.Name, err))
		return nil, errors.Wrapf(err, errors.CustomerFuncError, "failed to do customer function request for customer function %q", cf.Name)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxCustomerFunctionResponseBytes+1))
		if err != nil {
			logging.Default().Error(fmt.Sprintf("error reading response from customer function %s, err is: %v\n", cf.Name, err))
			return nil, errors.Wrapf(err, errors.CustomerFuncError, "fail to read response for customer function %q", cf.Name)
		}
		if len(body) > maxCustomerFunctionResponseBytes {
			return nil, errors.Errorf(errors.CustomerFuncError, "response of customer function %q exceeds %d bytes", cf.Name, maxCustomerFunctionResponseBytes)
		}
		response := ext.CustomerFunctionResponse{}
		if err = json.Unmarshal(body, &response); err != nil {
			logging.Default().Error(fmt.Sprintf("error unmarshaling response from customer function %s, err is: %v\n", cf.Name, err))
			return nil, errors.Wrapf(err, errors.CustomerFuncError, "fail to unmarshal response for customer function %q", cf.Name)
		} else if response.Error != "" {
			logging.Default().Error(fmt.Sprintf("error in response from customer function %s, err is: %v\n", cf.Name, response.Error))
			return nil, errors.Errorf(errors.CustomerFuncError, "customer function %q returns error %q", cf.Name, response.Error)
		} else {
			return response.Result, nil
		}
	default:
		logging.Default().Error(fmt.Sprintf("Invalid status code returns when calling customer function %s, status code is : %v\n", cf.Name, resp.StatusCode))
		return nil, errors.Errorf(errors.CustomerFuncError, "unexpected http status %d returned when calling customer function %s", resp.StatusCode, cf.Name)
	}
}

func (p *PolicyEvalImpl) cleanExpiredFunctionResultPeriodically() {
	ticker := time.NewTicker(30 * time.Minute)
	p.cleanupStop = make(chan struct{})
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.CleanExpiredFunctionResult()
			case <-p.cleanupStop:
				return
			}
		}
	}()
}
