package command

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/store"
	storeetcd "github.com/teramoby/speedle-plus/pkg/store/etcd"
	storefile "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsrest"
	"github.com/teramoby/speedle-plus/testutil"
	"go.etcd.io/etcd/server/v3/embed"
)

func TestMain(m *testing.M) {
	os.Exit(runCLITests(m))
}

func runCLITests(m *testing.M) int {
	// Preserve the explicit external-fixture mode used by run_*_test.sh.
	if os.Getenv(testutil.SP_APP_NAME) != "" {
		return m.Run()
	}
	directory, err := os.MkdirTemp("", "speedle-cli-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(directory)
	binary := filepath.Join(directory, "spctl")
	build := exec.Command("go", "build", "-o", binary, "..")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fixture spctl: %v\n%s", err, output)
		return 1
	}

	storeType := testutil.GetOSEnv("STORE_TYPE", "file")
	properties := map[string]interface{}{storefile.FileLocationKey: filepath.Join(directory, "policies.json")}
	if storeType == "etcd" {
		// Own random loopback listeners; never attach to an installed etcd.
		configuration := embed.NewConfig()
		configuration.Dir = filepath.Join(directory, "etcd")
		configuration.LogLevel = "error"
		loopback, _ := url.Parse("http://127.0.0.1:0")
		configuration.ListenClientUrls = []url.URL{*loopback}
		configuration.AdvertiseClientUrls = []url.URL{*loopback}
		configuration.ListenPeerUrls = []url.URL{*loopback}
		configuration.AdvertisePeerUrls = []url.URL{*loopback}
		configuration.InitialCluster = configuration.InitialClusterFromName(configuration.Name)
		server, err := embed.StartEtcd(configuration)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer server.Close()
		select {
		case <-server.Server.ReadyNotify():
		case <-time.After(10 * time.Second):
			fmt.Fprintln(os.Stderr, "fixture etcd did not become ready")
			return 1
		}
		properties = map[string]interface{}{
			storeetcd.EtcdEndpointKey:  server.Clients[0].Addr().String(),
			storeetcd.EtcdKeyPrefixKey: "/speedle-cli-tests/",
		}
	} else if storeType != "file" {
		fmt.Fprintf(os.Stderr, "unsupported CLI fixture store %q\n", storeType)
		return 1
	}
	manager, err := store.NewStore(storeType, properties)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer manager.Close()
	router, err := pmsrest.NewRouter(manager)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for name, value := range map[string]string{
		testutil.SP_APP_NAME: binary, testutil.PMS_ENDPOINT: server.URL,
		testutil.PMS_ADMIN_TOKEN: "", testutil.CA_LOCATION: "",
		testutil.CERT_LOCATION: "", testutil.KEY_LOCATION: "",
	} {
		if err := os.Setenv(name, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}
