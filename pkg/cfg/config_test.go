//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package cfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

func WriteConfig(mConfig Config, fileName string) error {
	jsonFile, err := os.Create(fileName)
	if err != nil {
		return err
	}
	psB, err := json.MarshalIndent(mConfig, "", "    ")
	if err != nil {
		return err
	}
	_, err = jsonFile.Write(psB)
	return err
}

func TestReadConfig(t *testing.T) {
	fileConfig, err := ReadConfig("./config_file.json")
	if err != nil {
		t.Error("Fail to read file store config")
	}
	if fileConfig.StoreConfig.StoreType != StorageTypeFile {
		t.Error("Read config error")
	}

	etcdConfig, err := ReadConfig("./config_etcd.json")
	if err != nil {
		t.Error("Fail to read etcd store config")
	}
	if etcdConfig.StoreConfig.StoreType != "etcd" {
		t.Error("Read config error")
	}
}

func TestWriteConfig(t *testing.T) {
	storeConfig := StoreConfig{
		StoreType: StorageTypeFile,
		StoreProps: map[string]interface{}{
			"FileLocation": "./ps.json",
		},
	}
	mConfig := Config{
		StoreConfig: &storeConfig,
	}
	location := filepath.Join(t.TempDir(), "myconfig.json")
	if err := WriteConfig(mConfig, location); err != nil {
		t.Fatal(err)
	}
	read, err := ReadConfig(location)
	if err != nil || read.StoreConfig.StoreProps["FileLocation"] != "./ps.json" {
		t.Fatalf("ReadConfig = %+v, %v", read, err)
	}
}

func TestReadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{"), 0600)
	for _, location := range []string{bad, filepath.Join(dir, "missing.json")} {
		if _, err := ReadConfig(location); errors.Code(err) != errors.ConfigError {
			t.Errorf("ReadConfig(%s): %v", location, err)
		}
		if _, err := ReadStoreConfig(location); errors.Code(err) != errors.ConfigError {
			t.Errorf("ReadStoreConfig(%s): %v", location, err)
		}
	}
}

func TestReadStoreConfig(t *testing.T) {
	location := filepath.Join(t.TempDir(), "store.json")
	os.WriteFile(location, []byte(`{"storeType":"etcd","storeProps":{"etcdEndpoint":"e:1"}}`), 0600)
	conf, err := ReadStoreConfig(location)
	if err != nil || conf.StoreType != "etcd" || conf.StoreProps["etcdEndpoint"] != "e:1" {
		t.Fatalf("ReadStoreConfig = %+v, %v", conf, err)
	}
}

func TestPolicySourceAPIKeyRedaction(t *testing.T) {
	const secret = "private-config-key-371"
	for _, storeType := range []string{"policy-source", ""} {
		for _, properties := range []string{`{"APIKey":"` + secret + `","UpstreamURL":"https://source.example"}`, `{"APIKey":["` + secret + `"]}`} {
			var configuration Config
			if err := json.Unmarshal([]byte(`{"storeConfig":{"storeType":"`+storeType+`","storeProps":`+properties+`}}`), &configuration); err != nil {
				t.Fatal(err)
			}
			for _, format := range []string{"%v", "%+v", "%#v"} {
				if strings.Contains(fmt.Sprintf(format, configuration), secret) || strings.Contains(fmt.Sprintf(format, configuration.StoreConfig), secret) {
					t.Fatal("configuration formatting leaked APIKey")
				}
			}
			data, err := json.Marshal(configuration)
			if err != nil || strings.Contains(string(data), secret) {
				t.Fatalf("unsafe config dump: %s, %v", data, err)
			}
		}
	}
	var store StoreConfig
	if err := json.Unmarshal([]byte(`{"storeType":"file","storeProps":{"APIKey":"`+secret+`"}}`), &store); err != nil {
		t.Fatal(err)
	}
	if store.StoreProps["APIKey"] != secret {
		t.Fatal("unrelated store changed")
	}
}
