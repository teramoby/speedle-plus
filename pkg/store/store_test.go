//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package store

import (
	"reflect"
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
}
