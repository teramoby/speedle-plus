//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package console

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The PMS TLS fixtures double as upstream certificates.
const tlsDir = "../svcs/pmsrest/tls/"

func TestNewUpstreamClient(t *testing.T) {
	client, err := newUpstreamClient(tlsDir+"server-ca.crt", tlsDir+"client.crt", tlsDir+"client.key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if client.Timeout != time.Second {
		t.Errorf("timeout %v", client.Timeout)
	}
	if _, err := newUpstreamClient("", "", "", time.Second); err != nil {
		t.Errorf("no TLS files: %v", err)
	}
	notPEM := filepath.Join(t.TempDir(), "ca.crt")
	os.WriteFile(notPEM, []byte("nope"), 0600)
	for name, files := range map[string][3]string{
		"missing CA":  {"missing.crt", "", ""},
		"CA not PEM":  {notPEM, "", ""},
		"bad keypair": {"", tlsDir + "client.crt", tlsDir + "server.key"},
	} {
		if _, err := newUpstreamClient(files[0], files[1], files[2], time.Second); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
