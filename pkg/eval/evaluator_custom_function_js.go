//go:build js && wasm

// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// CallCustomerFunction returns custom_function_unsupported on js/wasm.
// This does not make authorization fail closed: a condition that calls a
// custom function evaluates as not matching, so its GRANT or DENY is skipped.
func CallCustomerFunction(_ *pms.Function, _ *ext.CustomerFunctionRequest) (interface{}, error) {
	return nil, errors.New(errors.EvalEngineError, "custom_function_unsupported: custom functions are unavailable in js/wasm")
}

// CallCustomerFunctionViaDelegator also returns custom_function_unsupported.
func CallCustomerFunctionViaDelegator(_ string, cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
	return CallCustomerFunction(cf, request)
}

// generateCustomerExpressionFunction rejects every call before copying or
// marshalling arguments and before consulting the cache.
func (frc *FuncResultCache) generateCustomerExpressionFunction(_ *string, _ *pms.Function) (govaluate.ExpressionFunction, error) {
	return func(_ ...interface{}) (interface{}, error) {
		return CallCustomerFunction(nil, nil)
	}, nil
}

// No periodic cache cleanup runs on js/wasm. AddToCache remains available to
// embedders; expired entries are reclaimed when ReadFromCache reads the same
// key or when CleanExpiredResult is called explicitly. Entries without a TTL
// remain until they are deleted explicitly or the cache is replaced.
func (p *PolicyEvalImpl) cleanExpiredFunctionResultPeriodically() {}
