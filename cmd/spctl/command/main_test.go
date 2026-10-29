package command

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/etcd"
	storefile "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsrest"
	"github.com/teramoby/speedle-plus/testutil"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

var cliFixtureSupportsDiscover = true

func TestMain(m *testing.M) {
	os.Exit(runCLITests(m))
}

func runCLITests(m *testing.M) int {
	// Preserve the explicit external-fixture mode used by run_*_test.sh.
	if os.Getenv(testutil.SP_APP_NAME) != "" {
		// The external PMS's store is not reachable from here; only the file
		// and etcd stores implement Discover.
		storeType := testutil.GetOSEnv("STORE_TYPE", "file")
		cliFixtureSupportsDiscover = storeType == "file" || storeType == "etcd"
		return m.Run()
	}
	directory, err := os.MkdirTemp("", "speedle-cli-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(directory)
	binary := filepath.Join(directory, "spctl")
	buildArgs := []string{"build", "-o", binary}
	// The tests run spctl as a subprocess, so its coverage is collected
	// separately: with SPEEDLE_CLI_COVERPROFILE set, spctl is built with
	// coverage and its counters are written there as a text profile.
	coverProfile := os.Getenv("SPEEDLE_CLI_COVERPROFILE")
	coverDir := filepath.Join(directory, "cover")
	if coverProfile != "" {
		if err := os.Mkdir(coverDir, 0700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		os.Setenv("GOCOVERDIR", coverDir)
		buildArgs = append(buildArgs, "-cover", "-coverpkg=github.com/teramoby/speedle-plus/cmd/spctl/...")
	}
	build := exec.Command("go", append(buildArgs, "..")...)
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fixture spctl: %v\n%s", err, output)
		return 1
	}
	if coverProfile != "" {
		defer func() {
			convert := exec.Command("go", "tool", "covdata", "textfmt", "-i", coverDir, "-o", coverProfile)
			if output, err := convert.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "write spctl coverage: %v\n%s", err, output)
			}
		}()
	}

	storeType := testutil.GetOSEnv("STORE_TYPE", "file")
	properties := map[string]interface{}{storefile.FileLocationKey: filepath.Join(directory, "policies.json")}
	if storeType != "file" {
		fixture, err := storetest.NewFixture(nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer fixture.Close()
		properties = fixture.Config.StoreProps
	}
	manager, err := store.NewStore(storeType, properties)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if closer, ok := manager.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	_, cliFixtureSupportsDiscover = manager.(store.DiscoverRequestManager)
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
