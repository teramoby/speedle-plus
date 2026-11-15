//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package assertion

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion/config"
	"github.com/teramoby/speedle-plus/pkg/logging"
)

const (
	// TokenKey http header key name for asserting token
	TokenKey = "x-token"
	// IdpTypeKey http header key name for idp
	IdpTypeKey = "x-idp"
	// AllowedIDDKey http header key name for allowed IDD
	AllowedIDDKey = "x-allowedIDD"
	// RequestHeaderKey http header key name for extra headers which will be passed to IDP
	RequestHeaderKey = "x-ecid"
)

// AssertResponse assertion response
type AssertResponse struct {
	Principals []*adsapi.Principal    `json:"principals,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	ErrCode    int                    `json:"errCode"`
	ErrMessage string                 `json:"errMessage,omitempty"`
}

// TokenAsserter asserter interface
type TokenAsserter interface {
	// AssertToken assert token and generate subject to represent the identity
	AssertToken(token string, idpType string, allowedIDD string, requestHeaders map[string]string) (*AssertResponse, error)
}

// AsserterConfig preserves the original source name for the transport-free
// config.AsserterConfig type. Reflection and gob type names use the config
// package path; see docs/WASM.md for compatibility details.
type AsserterConfig = config.AsserterConfig

// WebHookAsserter implements asserter client interface
type WebHookAsserter struct {
	ServerEndpoint string
	caCert         string
	clientCert     string
	clientKey      string
	arstenant      *string
	httpClient     *http.Client
}

// NewAsserter create asserter webhook client
func NewAsserter(conf *AsserterConfig, tenant *string) (TokenAsserter, error) {
	logging.Default().Debug(fmt.Sprintf("conf: %v, tenant: %v", conf, tenant))
	if conf == nil || len(conf.Endpoint) == 0 {
		return nil, fmt.Errorf("asserter configuration is nil or endpoint is empty")
	}
	if conf.HTTPTimeout <= 0 {
		conf.HTTPTimeout = 10
	}
	a := WebHookAsserter{
		ServerEndpoint: strings.ToLower(conf.Endpoint),
		caCert:         conf.CACert,
		clientCert:     conf.ClientCert,
		clientKey:      conf.ClientKey,
		arstenant:      tenant,
	}

	tr := http.Transport{
		MaxIdleConns:    1000,
		IdleConnTimeout: 60 * time.Second,
		Proxy:           http.ProxyFromEnvironment,
	}

	if strings.HasPrefix(a.ServerEndpoint, "https") {
		tlsConf := &tls.Config{}
		if len(a.caCert) > 0 {
			caCert, err := os.ReadFile(a.caCert)
			if err != nil {
				return nil, err
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConf.RootCAs = caCertPool
		}
		if len(a.clientCert) > 0 && len(a.clientKey) > 0 {
			cert, err := tls.LoadX509KeyPair(a.clientCert, a.clientKey)
			if err != nil {
				return nil, err
			}
			tlsConf.Certificates = []tls.Certificate{cert}
		}
		tr.TLSClientConfig = tlsConf
	}

	a.httpClient = &http.Client{
		Transport: &tr,
		Timeout:   time.Duration(conf.HTTPTimeout) * time.Second,
	}
	logging.Default().Debug(fmt.Sprintf("asserter created: %v", a))

	return &a, nil
}

// AssertToken assert token via webhook
func (a *WebHookAsserter) AssertToken(token string, idpType string, allowedIDD string, requestHeaders map[string]string) (*AssertResponse, error) {
	logging.Default().Debug(fmt.Sprintf("idpType: %s, allowedIDD: %s", idpType, allowedIDD))

	if len(token) == 0 {
		logging.Default().Error("token is empty")
		return nil, fmt.Errorf("token is empty")
	}

	requestURL := a.ServerEndpoint
	if a.arstenant != nil {
		requestURL += *a.arstenant
	}

	req, errReq := http.NewRequest(http.MethodGet, requestURL, nil)
	if errReq != nil {
		logging.Default().Error(fmt.Sprintf("NewRequest error: %v", errReq))
		return nil, errReq
	}
	req.Header.Add(TokenKey, token)
	req.Header.Add(IdpTypeKey, idpType)

	if len(allowedIDD) > 0 {
		req.Header.Add(AllowedIDDKey, allowedIDD)
	}
	if requestHeaders != nil {
		keys := ""
		for k, v := range requestHeaders {
			req.Header.Add(k, v)
			keys = keys + k + ","
		}
		if len(keys) > 1 {
			keys = keys[0 : len(keys)-1]
		}
		req.Header.Add(RequestHeaderKey, keys)
	}

	resp, errResp := a.httpClient.Do(req)
	if errResp != nil {
		logging.Default().Error(fmt.Sprintf("Do error: %v", errResp))
		return nil, errResp
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logging.Default().Error(fmt.Sprintf("assertion error, status code: %d", resp.StatusCode))
		return nil, fmt.Errorf("asserter error, status code: %d", resp.StatusCode)
	}

	raw, errRaw := io.ReadAll(resp.Body)
	if errRaw != nil {
		logging.Default().Error(fmt.Sprintf("ReadAll error: %v", errRaw))
		return nil, errRaw
	}

	var ar AssertResponse

	errJSON := json.Unmarshal(raw, &ar)
	if errJSON != nil {
		logging.Default().Error(fmt.Sprintf("Unmarshal error: %v", errJSON))
		return nil, errJSON
	}

	// flag error if asserter indicates failure
	if ar.ErrCode != 0 {
		logging.Default().Error(fmt.Sprintf("assertion failure: %v", ar))
		return nil, fmt.Errorf("ErrCode: %d, ErrMsg: %s", ar.ErrCode, ar.ErrMessage)
	}
	logging.Default().Debug(fmt.Sprintf("asserted: %v", ar))

	return &ar, nil
}
