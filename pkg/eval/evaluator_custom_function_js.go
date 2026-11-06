//go:build js && wasm

// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// CallCustomerFunction fails closed: the js/wasm engine has no HTTP transport.
func CallCustomerFunction(_ *pms.Function, _ *ext.CustomerFunctionRequest) (interface{}, error) {
	return nil, errors.New(errors.EvalEngineError, "custom_function_unsupported: custom functions are unavailable in js/wasm")
}

// CallCustomerFunctionViaDelegator also rejects delegated network calls.
func CallCustomerFunctionViaDelegator(_ string, cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
	return CallCustomerFunction(cf, request)
}

// No custom results can be cached in js/wasm; do not start a cleanup timer.
func (p *PolicyEvalImpl) cleanExpiredFunctionResultPeriodically() {}
