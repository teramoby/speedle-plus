//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/ext"
)

var (
	funcServerCert = `-----BEGIN CERTIFICATE-----\nMIID9zCCAt+gAwIBAgIUMGS5anMcvwluVPkfhVv9KzK3W80wDQYJKoZIhvcNAQEL\nBQAwgYUxCzAJBgNVBAYTAmNuMRAwDgYDVQQIDAdCZWlqaW5nMRAwDgYDVQQHDAdC\nZWlqaW5nMRwwGgYDVQQKDBNEZWZhdWx0IENvbXBhbnkgTHRkMRIwEAYDVQQDDAls\nb2NhbGhvc3QxIDAeBgkqhkiG9w0BCQEWEWJpbGw4MjhAZ21haWwuY29tMB4XDTI2\nMDExNjA3NTUyM1oXDTM2MDExNDA3NTUyM1owgYUxCzAJBgNVBAYTAmNuMRAwDgYD\nVQQIDAdCZWlqaW5nMRAwDgYDVQQHDAdCZWlqaW5nMRwwGgYDVQQKDBNEZWZhdWx0\nIENvbXBhbnkgTHRkMRIwEAYDVQQDDAlsb2NhbGhvc3QxIDAeBgkqhkiG9w0BCQEW\nEWJpbGw4MjhAZ21haWwuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC\nAQEAulDL4yVW7bN5ybYPBaYGZ8mG30GQm1aa9ZzZCV9KmQ3K1urMC4xEvn3S+mni\naTuMAFS2pHF5dBM/AJor6OcuMG7lYtqkB0cuEiSdOXAIHJTHZfy/2vaeXhGsi/Ip\ns/6isIAUOuvEhP84rlpHjrIKmtzwHZm8W4k4qeovF0L4u0wZ4K6KpgyCN/FNd2Pa\nrioVWx55tV4ZpiKObElP85ue4y1kJ2erDPRHQVYQmT8+9/I1k09HIpOs26apaPs6\nJJknHUf0ChDF+x4VJQDPC5yP/m3jJpW/E1HvjU9y6GuSTmIGe/H+nSn1osA9JuzP\nxfoLFSO0jNRlVlt/f5ffS5guvwIDAQABo10wWzAsBgNVHREEJTAjgglsb2NhbGhv\nc3SHBH8AAAGHEAAAAAAAAAAAAAAAAAAAAAEwDAYDVR0TBAUwAwEB/zAdBgNVHQ4E\nFgQUHLEDjZxyoxeX4ka3bTfxWJS+J68wDQYJKoZIhvcNAQELBQADggEBABKat3uS\nGIbi9Rgc7rNRSzYSyEKD1NWiqkHpZSFNHtNfQNMzLof/VlOvkTx1oJfn5oAPnPEN\n+f9TburQNHtNtqogJpfLAycRcwTgdlyxVRMF4UVQA+1ke9TPjdx0VzjwawhdWhm5\nwdLj8iGLjUWyhwYQXGOkMM4rAfzrVds+sfAk24GiH0pM20C/HK4Vb3YjkFXyxkQs\nF88GjqN2HX0Gj5AoADCjpwXESw4Ld9loUT8+TxXcN4VUu0DoJtuqIg/FxyvyDjCS\nelsnfYW9rDnrp0bq1YSdwdvUsAa/08DbMvV511vVYFUzpdjrrGaU+07KriiLIYsL\nyjWGHTEM+dohN5o=\n-----END CERTIFICATE-----\n`
	funcServerKey  = `-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC6UMvjJVbts3nJ\ntg8FpgZnyYbfQZCbVpr1nNkJX0qZDcrW6swLjES+fdL6aeJpO4wAVLakcXl0Ez8A\nmivo5y4wbuVi2qQHRy4SJJ05cAgclMdl/L/a9p5eEayL8imz/qKwgBQ668SE/ziu\nWkeOsgqa3PAdmbxbiTip6i8XQvi7TBngroqmDII38U13Y9quKhVbHnm1XhmmIo5s\nSU/zm57jLWQnZ6sM9EdBVhCZPz738jWTT0cik6zbpqlo+zokmScdR/QKEMX7HhUl\nAM8LnI/+beMmlb8TUe+NT3Loa5JOYgZ78f6dKfWiwD0m7M/F+gsVI7SM1GVWW39/\nl99LmC6/AgMBAAECggEAWEm0K/mENVRweDi46pzPeTwCmOW5UBrciFngcPQCZf+m\nqGwP78Lyym/WW4p0Wxh4EaoS+F67RlleZ/smppFyDkpmkY743mgI+Vj3VgH0HhMU\nYAxCn9BwoDPw10oUH/KghlHTBE63y6wjYF9wfDp7EwJyGBzDKH8gZkgOM5AtTJcF\nHbYW/nbTTrSjDSwyVKJjpR8IQhODsBXukIfGeCGr8UaN8AEJiFLa7rZZmcs/2SV9\nHmI+3eC99He85ZBu2PBXAEPTuY9Flxs3bNj9OjNswD5P+tP8vtGEq7BUU/jlal6k\nmJ1RL6fITKJ71Hwu/zveBJGjDiaB1QNsQTh2ZmQexQKBgQD4AzEHK9ZtnFe2BhJe\nOgCLC52KTqYjMZkM+1d9+BKmB3w4bn33+dWHwf3uHZbuKuuJ9htLjx5YcMWlI2oU\nFLA/s4CEg6VhndcWkd2O1Y8YzCixC87WBi9eN/DSIosFTleNzzgVVbqTDPw1HtKh\nOcZ1ixDX7jL7kTAXK5/tBTtdrQKBgQDAUO2HufMyOcspG4lSeNSNuSPhsAc4TUs8\nBT9EH9ALl23lOZ61uNDMFqoRA6gjITQ0RtsE1hrEG8MixP1Cp7Vu6WQWX+kyLyp1\nwHflh3Ic1xDaXrO9bNqoRdwfvO1cn0OV3cMiDhc+cWPLkTeaMpsq3gwF4AH2iW6t\nz0Um9pEzmwKBgEq9e3rzxQ0HPo+GSObIh/1fJLzXcs3MVplI7Vby+Xu7ab3/3kpq\nqeTdm0608BUaLh1HY3ZjzPtOEOHxSDiA+5RW3fYRTjeav4T3tFMlHJiWffTM4Coz\ndvbn2NUav9Z7g3si5X3Ydf92vFKt1T/tD1fA7vSDvi191YZGCU3+c6OJAoGBAKWM\nRJyEjnva0i7lvFUZHGePSvr5C44Ew1G8dpSPCgkgZoJfEmb93AcDL4yL6E2tRIIH\nyHumTs4n09d3WUfqlD0QfY7hKx1/Cn7omo0kBjAbVi+UPAdA0AzwbieH+4+yrXwx\ntMr49DtVYoGW1RVQoM/K6vCXvzjZX0QRW0bKE34nAoGBAK/ZaxZQF1RSelwCruRP\nPr/qWraMSM4vgrA2GbUvNzaZ4lRl3CdTR+wAF+VvW0IcuepOjHaj+4AYpbVeII8Z\n2zCEfTv68sGmtSzdNgXfMWoHb7bJNGlAQ89zK9o72M3VZrl9119R+yZwc0wiPJME\npK7Mhd4ms/h2aKrILlHgQFXK\n-----END PRIVATE KEY-----`
)

func startFunctionService() {
	http.HandleFunc("/funcs/testsum", CustomFunctionTestSum)

	go http.ListenAndServe("0.0.0.0:12345", nil)

	//We have an assumption that on speedle/sphinx side, certificate is issued by well known CA.
	/*caCert, err := ioutil.ReadFile("client.crt")
	if err != nil {
		log.Fatal(err)
	}*/
	caCertPool := x509.NewCertPool()
	//caCertPool.AppendCertsFromPEM(caCert)

	// Setup HTTPS client
	tlsConfig := &tls.Config{
		ClientCAs: caCertPool,
		//ClientAuth: tls.RequireAndVerifyClientCert,
	}

	server := &http.Server{
		Addr:      "0.0.0.0:23456",
		TLSConfig: tlsConfig,
	}
	server.ListenAndServeTLS("./funcServer.crt", "./funcServer.key")

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
	go startFunctionService()

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
