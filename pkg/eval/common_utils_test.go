//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/etcd"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func WriteToTempFile(content []byte) (string, error) {
	tmpfile, err := os.CreateTemp("", "authz-evaluator-")
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	if _, err := tmpfile.Write(content); err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

var testPS pms.PolicyStoreManager
var conf *cfg.Config

func testMain(m *testing.M) int {
	var mConfig *cfg.Config
	var configFile string
	logging.Default().Info(fmt.Sprintf("STORE_TYPE = %s\n", os.Getenv("STORE_TYPE")))
	if storetest.SelectedBackend() != "file" {
		fixture, err := storetest.NewFixture(&pms.PolicyStore{})
		if err != nil {
			logging.Default().Info(fmt.Sprint(err))
			return 1
		}
		defer fixture.Close()
		testPS = fixture.Store
		conf = &cfg.Config{StoreConfig: fixture.Config}
		return m.Run()
	}
	logging.Default().Info("Start file!")
	configFile = "../cfg/config_file.json"
	defer os.Remove("./ps.json")
	defer os.Remove("./speedle_discover_requests.json")

	var err error
	mConfig, err = cfg.ReadConfig(configFile)
	if err != nil {
		logging.Fatal("Fail to read store config")
		os.Exit(1)
	}

	//Since we don't need watch function in the unit tests of evaluator, so disable it.
	mConfig.EnableWatch = false

	testPS, err = store.NewStore(mConfig.StoreConfig.StoreType, mConfig.StoreConfig.StoreProps)
	if err != nil {
		logging.Fatal("Fail to NewStore")
		os.Exit(1)
	}
	conf = mConfig
	ret := m.Run()
	return ret
}

func preparePolicyDataInStore(data []byte, t *testing.T) error {
	var ps pms.PolicyStore
	err := json.Unmarshal(data, &ps)
	if err != nil {
		t.Fatal("Fail to prepare data", err)
		return err
	}
	err = testPS.WritePolicyStore(&ps)
	if err != nil {
		t.Fatal("Fail to prepare data:", err)
		return err
	}
	return nil
}

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}
