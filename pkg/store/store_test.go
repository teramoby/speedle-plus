//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package store

import (
	"reflect"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

type fakeBuilder struct {
	params map[string]string
}

func (b fakeBuilder) NewStore(config map[string]interface{}) (pms.PolicyStoreManager, error) {
	return nil, errors.New(errors.StoreError, "fake")
}

func (b fakeBuilder) GetStoreParams() map[string]string { return b.params }

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	f()
}

func TestRegistry(t *testing.T) {
	defer unregisterAllStoreBuilders()
	unregisterAllStoreBuilders()

	Register("b", fakeBuilder{params: map[string]string{"b-flag": "bKey"}})
	Register("a", fakeBuilder{params: map[string]string{"a-flag": "aKey"}})
	mustPanic(t, "registering a type twice", func() { Register("a", fakeBuilder{}) })
	mustPanic(t, "registering nil", func() { Register("c", nil) })

	if got := StoreBuilders(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("StoreBuilders() = %v", got)
	}
	if got := GetAllStoreParams(); !reflect.DeepEqual(got, map[string]string{"a-flag": "aKey", "b-flag": "bKey"}) {
		t.Errorf("GetAllStoreParams() = %v", got)
	}
	if _, err := NewStore("a", nil); errors.Code(err) != errors.StoreError {
		t.Errorf("NewStore(a) = %v", err)
	}
	if _, err := NewStore("missing", nil); errors.Code(err) != errors.ConfigError {
		t.Errorf("NewStore(missing) = %v", err)
	}
	Register("policy-source", fakeBuilder{})
	if _, err := NewStore("policy-source", nil); errors.Code(err) != errors.StoreError || !strings.Contains(err.Error(), "fake") {
		t.Errorf("registered policy-source builder error = %v", err)
	}
}

func TestRemovedPolicySourceStore(t *testing.T) {
	result, err := NewStore("policy-source", nil)
	if result != nil || errors.Code(err) != errors.ConfigError || !strings.Contains(err.Error(), "v1.8.0") || !strings.Contains(err.Error(), "policy-source") {
		t.Fatalf("removed store: %v, %v", result, err)
	}
}
