//go:build js && wasm

// Licensed under the Universal Permissive License (UPL) Version 1.0.
package wasmtest

import (
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"strings"
	"testing"
)

func TestWasmCustomFunctionsUnsupported(t *testing.T) {
	cf := &pms.Function{Name: "remote", FuncURL: "https://example.invalid"}
	request := &ext.CustomerFunctionRequest{}
	for _, delegated := range []bool{false, true} {
		var result interface{}
		var err error
		if delegated {
			result, err = eval.CallCustomerFunctionViaDelegator("https://example.invalid", cf, request)
		} else {
			result, err = eval.CallCustomerFunction(cf, request)
		}
		if result != nil || err == nil || !strings.Contains(err.Error(), "custom_function_unsupported") {
			t.Fatalf("result=%v err=%v", result, err)
		}
	}
}
