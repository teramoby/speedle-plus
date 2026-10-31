// Licensed under the Universal Permissive License (UPL) Version 1.0.
package flags

import (
	"testing"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestStoreFailureGracePeriodPrecedence(t *testing.T) {
	for _, source := range []string{"default", "file", "environment", "flag"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("SPDL_STORE_FAILURE_GRACE_PERIOD", "")
			args := []string{"--insecure=true"}
			want := "0s"
			if source != "default" {
				path := writeFile(t, "health.json", `{"storeFailureGracePeriod":"2s"}`)
				args = append(args, "--config-file", path)
				want = "2s"
			}
			if source == "environment" || source == "flag" {
				t.Setenv("SPDL_STORE_FAILURE_GRACE_PERIOD", "3s")
				want = "3s"
			}
			if source == "flag" {
				args = append(args, "--store-failure-grace-period=4s")
				want = "4s"
			}
			parameters, code := parse(t, args, nil)
			if code != -1 {
				t.Fatalf("exit %d", code)
			}
			configuration, err := parameters.Param2Config(nil)
			if err != nil || configuration.StoreFailureGracePeriod != want {
				t.Fatalf("%s: %+v %v", source, configuration, err)
			}
		})
	}
}

func TestInvalidStoreFailureGracePeriod(t *testing.T) {
	for _, value := range []string{"-1s", "forever", "invalid", "999999999999999999999h"} {
		t.Run(value, func(t *testing.T) {
			parameters, code := parse(t, []string{"--insecure=true", "--store-failure-grace-period=" + value}, nil)
			if code != -1 {
				t.Fatalf("exit %d", code)
			}
			if configuration, err := parameters.Param2Config(nil); configuration != nil || errors.Code(err) != errors.ConfigError {
				t.Fatalf("%q: %v %v", value, configuration, err)
			}
		})
	}
}
