//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/ext"
)

var (
	funcServerCert = `-----BEGIN CERTIFICATE-----\nMIID9zCCAt+gAwIBAgIUMGS5anMcvwluVPkfhVv9KzK3W80wDQYJKoZIhvcNAQEL\nBQAwgYUxCzAJBgNVBAYTAmNuMRAwDgYDVQQIDAdCZWlqaW5nMRAwDgYDVQQHDAdC\nZWlqaW5nMRwwGgYDVQQKDBNEZWZhdWx0IENvbXBhbnkgTHRkMRIwEAYDVQQDDAls\nb2NhbGhvc3QxIDAeBgkqhkiG9w0BCQEWEWJpbGw4MjhAZ21haWwuY29tMB4XDTI2\nMDExNjA3NTUyM1oXDTM2MDExNDA3NTUyM1owgYUxCzAJBgNVBAYTAmNuMRAwDgYD\nVQQIDAdCZWlqaW5nMRAwDgYDVQQHDAdCZWlqaW5nMRwwGgYDVQQKDBNEZWZhdWx0\nIENvbXBhbnkgTHRkMRIwEAYDVQQDDAlsb2NhbGhvc3QxIDAeBgkqhkiG9w0BCQEW\nEWJpbGw4MjhAZ21haWwuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC\nAQEAulDL4yVW7bN5ybYPBaYGZ8mG30GQm1aa9ZzZCV9KmQ3K1urMC4xEvn3S+mni\naTuMAFS2pHF5dBM/AJor6OcuMG7lYtqkB0cuEiSdOXAIHJTHZfy/2vaeXhGsi/Ip\ns/6isIAUOuvEhP84rlpHjrIKmtzwHZm8W4k4qeovF0L4u0wZ4K6KpgyCN/FNd2Pa\nrioVWx55tV4ZpiKObElP85ue4y1kJ2erDPRHQVYQmT8+9/I1k09HIpOs26apaPs6\nJJknHUf0ChDF+x4VJQDPC5yP/m3jJpW/E1HvjU9y6GuSTmIGe/H+nSn1osA9JuzP\nxfoLFSO0jNRlVlt/f5ffS5guvwIDAQABo10wWzAsBgNVHREEJTAjgglsb2NhbGhv\nc3SHBH8AAAGHEAAAAAAAAAAAAAAAAAAAAAEwDAYDVR0TBAUwAwEB/zAdBgNVHQ4E\nFgQUHLEDjZxyoxeX4ka3bTfxWJS+J68wDQYJKoZIhvcNAQELBQADggEBABKat3uS\nGIbi9Rgc7rNRSzYSyEKD1NWiqkHpZSFNHtNfQNMzLof/VlOvkTx1oJfn5oAPnPEN\n+f9TburQNHtNtqogJpfLAycRcwTgdlyxVRMF4UVQA+1ke9TPjdx0VzjwawhdWhm5\nwdLj8iGLjUWyhwYQXGOkMM4rAfzrVds+sfAk24GiH0pM20C/HK4Vb3YjkFXyxkQs\nF88GjqN2HX0Gj5AoADCjpwXESw4Ld9loUT8+TxXcN4VUu0DoJtuqIg/FxyvyDjCS\nelsnfYW9rDnrp0bq1YSdwdvUsAa/08DbMvV511vVYFUzpdjrrGaU+07KriiLIYsL\nyjWGHTEM+dohN5o=\n-----END CERTIFICATE-----\n`
)

// startFunctionService serves the test function over HTTP and HTTPS on free
// ports, the HTTPS server with funcServer.crt (valid for 127.0.0.1).
func startFunctionService(t *testing.T) (httpURL, httpsURL string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/funcs/testsum", CustomFunctionTestSum)
	plain := httptest.NewServer(mux)
	t.Cleanup(plain.Close)

	cert, err := tls.LoadX509KeyPair("./funcServer.crt", "./funcServer.key")
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewUnstartedServer(mux)
	secure.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	secure.StartTLS()
	t.Cleanup(secure.Close)
	return plain.URL, secure.URL
}

func CustomFunctionTestSum(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var request ext.CustomerFunctionRequest
	var response ext.CustomerFunctionResponse
	httpSatus := http.StatusOK

	if err := decoder.Decode(&request); err != nil {
		fmt.Println(err)
		response = ext.CustomerFunctionResponse{
			Error: "error decoding request",
		}
		httpSatus = http.StatusBadRequest
	} else {
		fmt.Printf("request = %v\n", request)
		sum := float64(0)
		for index, param := range request.Params {
			fmt.Printf("param %d: value=%v, type=%t\n", index, param, param)
			sum = sum + param.(float64)
		}
		response = ext.CustomerFunctionResponse{
			Result: sum,
		}
	}
	payload, err := json.Marshal(response)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(response)
		fmt.Println("repsonse=", string(payload))
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(httpSatus)
	w.Write(payload)
}

func TestFunctions(t *testing.T) {
	httpURL, httpsURL := startFunctionService(t)

	testCases := []struct {
		condition string
		stream    string
		ctx       adsapi.RequestContext
		want      bool
	}{
		{
			condition: "testsum(1,2) <4",
			stream:    `{"functions":[{"name":"testsum","funcURL":"http://localhost:12345/funcs/testsum"}],"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "testsum(1,2) <4"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 7.99}},
			want:      true,
		},
		{
			condition: "testsum1(1,2) <4",
			stream:    `{"functions":[{"name":"testsum1","funcURL":"https://localhost:23456/funcs/testsum", "CA" : "` + funcServerCert + `"}],"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "testsum1(1,2) <4"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 7.99}},
			want:      true,
		},
		{
			condition: "Sqrt(64) > x",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "Sqrt(64) > x"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 7.99}},
			want:      true,
		},
		{
			condition: "Sqrt(64) > x",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "Sqrt(64) > x"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 8.01}},
			want:      false,
		},
		{
			condition: "Sqrt(x) > 7.99",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "Sqrt(x) > 7.99"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 64}},
			want:      true,
		},
		{
			condition: "Sqrt(x) > 8.01",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "Sqrt(x) > 8.01"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 64}},
			want:      false,
		},
		{
			condition: "Max(-3, x, 5) > y",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "Max(-3, x, 5) > y"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 7, "y": 6}},
			want:      true,
		},
		{
			condition: "Max(-3, x, 5) > y",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "Max(-3, x, 5) > y"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"x": 4, "y": 6}},
			want:      false,
		},

		{
			condition: "IsSubSet(s1,s2)",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "IsSubSet(s1,s2)"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"s1": []int{1, 3}, "s2": []int{1, 2, 3, 4}}},
			want:      true,
		},
		{
			condition: "IsSubSet(s1,s2)",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "IsSubSet(s1,s2)"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"s1": []int{1}, "s2": []int{1, 2, 3, 4}}},
			want:      true,
		},
		{
			condition: "IsSubSet(s1,s2)",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "IsSubSet(s1,s2)"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"s1": []int{1, 5}, "s2": []int{1, 2, 3, 4}}},
			want:      false,
		},
		{
			condition: "IsSubSet(s,('BJ','SH','GZ','SZ'))",
			stream:    `{"services": [{"name": "crm","policies": [{"name": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "IsSubSet(s,('BJ','SH','GZ','SZ'))"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"s": []string{"GZ", "SH"}}},
			want:      true,
		},
		{
			condition: "IsSubSet(s,('BJ','SH','GZ','SZ'))",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "IsSubSet(s,('BJ','SH','GZ','SZ'))"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"s": []string{"GZ", "TJ"}}},
			want:      false,
		},
		{
			condition: "IsSubSet(('BJ', 'SZ'), s)",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "IsSubSet(('BJ', 'SZ'), s)"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"s": []string{"BJ", "GZ", "SH", "SZ"}}},
			want:      true,
		},
		{
			condition: "IsSubSet(('BJ', 'TJ'), s)",
			stream:    `{"services": [{"name": "crm","policies": [{"id": "p1", "effect": "grant", "permissions": [{"resource": "/node1","actions": ["get"]}],"condition": "IsSubSet(('BJ', 'TJ'), s)"}]}]}`,
			ctx:       adsapi.RequestContext{Subject: nil, ServiceName: "crm", Resource: "/node1", Action: "get", Attributes: map[string]interface{}{"s": []string{"BJ", "GZ", "SH", "SZ"}}},
			want:      false,
		},
	}

	for _, tc := range testCases {
		tc.stream = strings.NewReplacer("http://localhost:12345", httpURL, "https://localhost:23456", httpsURL).Replace(tc.stream)
		preparePolicyDataInStore([]byte(tc.stream), t)
		eval, err := NewWithStore(conf, testPS)
		if err != nil {
			t.Errorf("error creating evaluator : %v", err)
			continue
		}
		// Run 3 times
		for i := 0; i < 3; i++ {
			got, _, err := eval.IsAllowed(tc.ctx)
			if err != nil {
				t.Errorf("condition: %s, context: %v, error: %v", tc.condition, tc.ctx.Attributes, err)
			}
			if got != tc.want {
				t.Errorf("condition: %s, context: %v, got %v, want %v", tc.condition, tc.ctx.Attributes, got, tc.want)
			}
		}
	}
}
