// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func writeCredentialFile(t *testing.T, directory, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(directory, "downstream-credentials")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	return path
}

func TestLoadCredentialFile(t *testing.T) {
	directory := t.TempDir()
	token1 := testToken(testDownstreamID, 1)
	token2 := testToken(testDownstreamB, 2)
	contents := "# downstream credentials\n\n" +
		"edge-1:" + hashToken(token1) + ":" + testServiceID1 + "\n" +
		"edge-2:" + hashToken(token2) + ":*\n"
	path := writeCredentialFile(t, directory, contents, 0o600)

	set, err := LoadCredentialFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if set.Len() != 2 {
		t.Errorf("len = %d", set.Len())
	}
	credential, ok := set.Authenticate(token1)
	if !ok || credential.DownstreamID != "edge-1" {
		t.Fatalf("authenticate token1: %v %v", credential, ok)
	}
	if !credential.Allows(testServiceID1) {
		t.Error("edge-1 cannot read its own service")
	}
	if credential.Allows(testServiceID2) {
		t.Error("edge-1 can read a service outside its subset")
	}
	if got := credential.ServicesList(); len(got) != 1 || got[0] != testServiceID1 {
		t.Errorf("services = %v", got)
	}
	credential, ok = set.Authenticate(token2)
	if !ok || !credential.Allows(testUnknownID) {
		t.Errorf("edge-2 is not unrestricted")
	}
	if _, ok := set.Authenticate(testToken(testDownstreamID, 9)); ok {
		t.Error("unknown token authenticated")
	}
}

func TestCredentialFileValidation(t *testing.T) {
	directory := t.TempDir()
	validHash := hashToken("secret")
	cases := map[string]string{
		"two fields":           "edge-1:" + validHash + "\n",
		"bad hash":             "edge-1:nothex:" + testServiceID1 + "\n",
		"short hash":           "edge-1:abcd:" + testServiceID1 + "\n",
		"bad service":          "edge-1:" + validHash + ":not-a-service\n",
		"empty subset":         "edge-1:" + validHash + ":\n",
		"bad downstream id":    ":" + validHash + ":*\n",
		"duplicate id":         "edge-1:" + validHash + ":*\nedge-1:" + hashToken("other") + ":*\n",
		"duplicate token":      "edge-1:" + validHash + ":*\nedge-2:" + validHash + ":*\n",
		"duplicate service":    "edge-1:" + validHash + ":" + testServiceID1 + "," + testServiceID1 + "\n",
		"empty file":           "",
		"only comments":        "# nothing\n",
		"trailing colon field": "edge-1:" + validHash + ":" + testServiceID1 + ":extra\n",
	}
	for name, contents := range cases {
		path := writeCredentialFile(t, t.TempDir(), contents, 0o600)
		if _, err := LoadCredentialFile(path); err == nil {
			t.Errorf("%s: credential file was accepted", name)
		}
	}
	if _, err := LoadCredentialFile(filepath.Join(directory, "missing")); err == nil {
		t.Error("missing file was accepted")
	}
}

func TestCredentialFilePermissions(t *testing.T) {
	path := writeCredentialFile(t, t.TempDir(), "edge-1:"+hashToken("secret")+":*\n", 0o644)
	if _, err := LoadCredentialFile(path); err == nil {
		t.Error("group-readable credential file was accepted")
	}
}

func TestNewCredentialSet(t *testing.T) {
	set, err := NewCredentialSet([]Credential{
		{DownstreamID: "edge-1", Token: "one", Services: []string{testServiceID1}},
		{DownstreamID: "edge-2", Token: "two"},
	})
	if err != nil {
		t.Fatalf("new set: %v", err)
	}
	credential, ok := set.Authenticate("one")
	if !ok || !credential.Allows(testServiceID1) || credential.Allows(testServiceID2) {
		t.Errorf("edge-1 = %+v", credential)
	}
	credential, ok = set.Authenticate("two")
	if !ok || !credential.Allows(testServiceID2) {
		t.Errorf("edge-2 = %+v", credential)
	}
	if _, err := NewCredentialSet([]Credential{
		{DownstreamID: "edge-1", Token: "same"},
		{DownstreamID: "edge-2", Token: "same"},
	}); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("duplicate token: %v", err)
	}
	if _, err := NewCredentialSet([]Credential{{DownstreamID: "edge-1", Token: "one", Services: []string{"*", testServiceID1}}}); err == nil {
		t.Error("* combined with an explicit service was accepted")
	}
}

func TestServerReloadCredentials(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	token1 := testToken(testDownstreamID, 1)
	token2 := testToken(testDownstreamB, 2)
	server := newTestServer(t, mirror, nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token1,
	})
	servicesPath := adsync.APIPrefix + "/services"

	response, _ := server.get(servicesPath, token1, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token1 before reload: status %d", response.StatusCode)
	}

	server.writeCredentialFile(testDownstreamB + ":" + hashToken(token2) + ":*")
	if err := server.server.ReloadCredentials(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	response, _ = server.get(servicesPath, token1, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("retired token: status %d", response.StatusCode)
	}
	response, _ = server.get(servicesPath, token2, nil)
	if response.StatusCode != http.StatusOK {
		t.Errorf("new token: status %d", response.StatusCode)
	}

	// A broken file keeps the previous set in force.
	server.writeCredentialFile("broken")
	if err := server.server.ReloadCredentials(); err == nil {
		t.Fatal("broken file reload succeeded")
	}
	response, _ = server.get(servicesPath, token2, nil)
	if response.StatusCode != http.StatusOK {
		t.Errorf("token after failed reload: status %d", response.StatusCode)
	}
}

func TestServerWatchReload(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	token1 := testToken(testDownstreamID, 1)
	token2 := testToken(testDownstreamB, 2)
	server := newTestServer(t, mirror, nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token1,
	})
	signals := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.server.WatchReload(ctx, signals)
	}()
	defer func() {
		cancel()
		<-done
	}()

	server.writeCredentialFile(testDownstreamB + ":" + hashToken(token2) + ":*")
	signals <- os.Interrupt
	waitFor(t, 5*time.Second, "the credential reload", func() bool {
		response, _ := server.get(adsync.APIPrefix+"/services", token2, nil)
		return response.StatusCode == http.StatusOK
	})
	response, _ := server.get(adsync.APIPrefix+"/services", token1, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("retired token after SIGHUP: status %d", response.StatusCode)
	}
}
