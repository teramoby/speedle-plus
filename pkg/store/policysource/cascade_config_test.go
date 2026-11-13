// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func cascadeProperties() map[string]interface{} {
	return map[string]interface{}{
		UpstreamURLKey: "https://relay-a.example:7443", ClientCertFileKey: "client.pem",
		ClientKeyFileKey: "client.key", CAFileKey: "ca.pem", CacheDirKey: "cache",
	}
}

func TestCascadeUpstreamConfiguration(t *testing.T) {
	for _, input := range []interface{}{
		"https://relay-a.example:7443", []string{"https://relay-a.example:7443", "https://relay-b.example"},
		[]interface{}{"https://relay-a.example:7443", "https://relay-b.example"},
	} {
		properties := cascadeProperties()
		properties[UpstreamURLKey] = input
		options, err := optionsFromConfig(properties)
		if err != nil {
			t.Fatal(err)
		}
		if options.UpstreamURL != "https://relay-a.example:7443" || options.APIKey != "" {
			t.Fatalf("options: %+v", options)
		}
		if _, list := input.(string); !list && !reflect.DeepEqual(options.UpstreamURLs, []string{"https://relay-a.example:7443", "https://relay-b.example"}) {
			t.Fatalf("lost priority order: %v", options.UpstreamURLs)
		}
	}
	for _, input := range []interface{}{
		nil, 42, []interface{}{}, []string{}, []interface{}{"https://relay.example", 42},
		[]string{""}, []string{"https://relay.example", "http://other.example"},
		[]string{"https://relay.example", "https://relay.example/"},
		"https://user:secret@relay.example", "https://relay.example?", "https://relay.example#", "https://relay.example?q=x",
	} {
		properties := cascadeProperties()
		properties[UpstreamURLKey] = input
		if _, err := optionsFromConfig(properties); err == nil || !strings.Contains(err.Error(), UpstreamURLKey) {
			t.Fatalf("accepted %v: %v", input, err)
		}
	}
	properties := cascadeProperties()
	properties[APIKeyKey] = testAPIKey
	properties[UpstreamURLKey] = []string{"https://a.example", "https://b.example"}
	if _, err := optionsFromConfig(properties); err == nil {
		t.Fatal("multiple direct upstreams accepted")
	}
	// There is no upstream count ceiling.
	properties = cascadeProperties()
	upstreams := make([]string, 1001)
	for i := range upstreams {
		upstreams[i] = "https://relay.example/" + strings.Repeat("x", i+1)
	}
	properties[UpstreamURLKey] = upstreams
	if options, err := optionsFromConfig(properties); err != nil || len(options.UpstreamURLs) != len(upstreams) {
		t.Fatalf("upstream count limited: %v", err)
	}
}

func TestCascadeRemovedConfiguration(t *testing.T) {
	for _, key := range []string{DownstreamCredentialsKey, ServicesKey} {
		for _, value := range []interface{}{nil, "", "obsolete", []string{"books"}} {
			properties := cascadeProperties()
			properties[key] = value
			_, err := optionsFromConfig(properties)
			if err == nil {
				t.Fatalf("accepted removed %s", key)
			}
			for _, setting := range []string{key, ClientCertFileKey, ClientKeyFileKey, CAFileKey, "downstreamClientCAPath"} {
				if !strings.Contains(err.Error(), setting) {
					t.Fatalf("migration error lacks %s: %v", setting, err)
				}
			}
		}
	}
	for _, key := range []string{MaxDownstreamKey, MaxChainDepthKey, MaxConnectionsKey, "InstanceID"} {
		properties := cascadeProperties()
		properties[key] = 0
		if _, err := optionsFromConfig(properties); err == nil {
			t.Fatalf("accepted removed %s", key)
		}
	}
}

func TestCascadeTLSProperties(t *testing.T) {
	properties := cascadeProperties()
	properties[TLSReloadIntervalKey] = "45s"
	properties[CRLFilesKey] = []interface{}{"relay.crl"}
	properties[CRLURLsKey] = []string{"https://pki.example/relay.crl"}
	properties[CRLRefreshIntervalKey] = "1h"
	properties[CRLMaxAgeKey] = "24h"
	options, err := optionsFromConfig(properties)
	if err != nil || options.TLSReloadInterval != 45*time.Second || options.CRLRefreshInterval != time.Hour || options.CRLMaxAge != 24*time.Hour || len(options.CRLFiles) != 1 || len(options.CRLURLs) != 1 {
		t.Fatalf("TLS properties: %+v, %v", options, err)
	}
	for _, value := range []interface{}{42, []interface{}{false}, []string{""}, "relay.crl"} {
		properties[CRLFilesKey] = value
		if _, err := optionsFromConfig(properties); err == nil {
			t.Fatalf("accepted CRLFiles %v", value)
		}
	}
}

func TestDirectModeRejectsCascadeTLSPropertyPresence(t *testing.T) {
	for _, key := range []string{TLSReloadIntervalKey, CRLFilesKey, CRLURLsKey, CRLRefreshIntervalKey, CRLMaxAgeKey} {
		for _, value := range []interface{}{nil, "", "0s", []string{}} {
			properties := map[string]interface{}{UpstreamURLKey: "https://source.example", APIKeyKey: testAPIKey, key: value}
			if _, err := optionsFromConfig(properties); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("ignored %s: %v", key, err)
			}
		}
	}
}

func TestCascadeRejectsEmptyConfiguredCRLSources(t *testing.T) {
	for _, key := range []string{CRLFilesKey, CRLURLsKey} {
		properties := cascadeProperties()
		properties[key] = []string{}
		if _, err := optionsFromConfig(properties); err == nil || !strings.Contains(err.Error(), "CRL") {
			t.Fatalf("empty configured sources accepted: %v", err)
		}
	}
}
