//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/etcd"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"

	"github.com/teramoby/speedle-plus/api/pms"
	log "github.com/sirupsen/logrus"
)

func WriteToTempFile(content []byte) (string, error) {
	tmpfile, err := ioutil.TempFile("", "authz-evaluator-")
	defer tmpfile.Close()
	if err != nil {
		return "", err
	}

	if _, err := tmpfile.Write(content); err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

var (
	testStoreOnce sync.Once
	testPS        pms.PolicyStoreManager
)

// testStore creates the shared test store on first use. With the etcd store
// that starts an embedded etcd, which must not happen before m.Run:
// testing.M.Run writes os.Stderr, which etcd's serving goroutines read, and
// the race detector reports that.
func testStore() pms.PolicyStoreManager {
	testStoreOnce.Do(func() {
		var err error
		testPS, err = store.NewStore(conf.StoreConfig.StoreType, conf.StoreConfig.StoreProps)
		if err != nil {
			log.Fatal("Fail to NewStore: ", err)
		}
	})
	return testPS
}
var configFile string
var conf *cfg.Config

func testMain(m *testing.M) int {
	var mConfig *cfg.Config
	var configFile string

	log.Infof("STORE_TYPE = %s\n", os.Getenv("STORE_TYPE"))
	if os.Getenv("STORE_TYPE") == "etcd" {
		log.Info("Start etcd!")
		configFile = "../cfg/config_etcd.json"
		defer os.RemoveAll("./speedle.etcd")
	} else {
		log.Info("Start file!")
		configFile = "../cfg/config_file.json"
		defer os.Remove("./ps.json")
		defer os.Remove("./speedle_discover_requests.json")
	}

	var err error
	mConfig, err = cfg.ReadConfig(configFile)
	if err != nil {
		log.Fatal("Fail to read store config")
		os.Exit(1)
	}

	//Since we don't need watch function in the unit tests of evaluator, so disable it.
	mConfig.EnableWatch = false

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
	err = testStore().WritePolicyStore(&ps)
	if err != nil {
		t.Fatal("Fail to prepare data:", err)
		return err
	}
	return nil
}

//This method can help to print service, policy or rolepolicy instance. When debugger cannot work, you can use it to
//print data to help investigation.
func printObject(p interface{}) {
	b, e := json.MarshalIndent(p, "", "	")
	if e != nil {
		log.Warning("Failed to marshal policy!")
	}
	log.Info(string(b))
}
func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}
