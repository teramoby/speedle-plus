// Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
// Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.
package eval

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

const (
	defaultCustomerFunctionCallTimeout = 5 * time.Second
	// maxCustomerFunctionResponseBytes bounds how much of a customer
	// function's response ADS reads.
	maxCustomerFunctionResponseBytes = 1 << 20
	// maxFuncResultCacheEntries bounds the function result cache. When it is
	// full, new results are returned without being cached.
	maxFuncResultCacheEntries = 10000
)

type Request2Delegator struct {
	Function *pms.Function                `json:"function"`
	Request  *ext.CustomerFunctionRequest `json:"request"`
}

type FuncResult struct {
	Result interface{}
	TTL    int64
}

type FuncResultCache struct {
	sync.RWMutex
	Results map[string]FuncResult
}

func (frc *FuncResultCache) get(key string) (FuncResult, bool) {
	frc.RLock()
	defer frc.RUnlock()
	result, ok := frc.Results[key]
	return result, ok
}
func (frc *FuncResultCache) add(key string, value FuncResult) {
	frc.Lock()
	defer frc.Unlock()
	if _, ok := frc.Results[key]; !ok && len(frc.Results) >= maxFuncResultCacheEntries {
		return
	}
	frc.Results[key] = value
}

func (frc *FuncResultCache) AddToCache(key string, cf *pms.Function, result interface{}) {
	if cf.ResultCachable {
		ttl := int64(0)
		if cf.ResultTTL > 0 {
			ttl = time.Now().Unix() + cf.ResultTTL
		}
		cachedResult := FuncResult{
			Result: result,
			TTL:    ttl,
		}
		frc.add(key, cachedResult)
	}
}

func (frc *FuncResultCache) ReadFromCache(key string, cf *pms.Function) interface{} {
	if cf.ResultCachable {
		ret, ok := frc.get(key)
		if ok {
			if ret.TTL == 0 || time.Now().Unix() <= ret.TTL {
				return ret.Result
			}
			frc.deleteIfExpired(key)
		}
	}
	return nil
}

func (frc *FuncResultCache) DeleteFromCache(funcName string) {
	frc.Lock()
	defer frc.Unlock()
	for key := range frc.Results {
		if isFunc(key, funcName) {
			delete(frc.Results, key)
		}
	}
}

func (frc *FuncResultCache) deleteIfExpired(key string) {
	frc.Lock()
	defer frc.Unlock()
	ret, ok := frc.Results[key]
	if ok {
		if ret.TTL != 0 && time.Now().Unix() > ret.TTL {
			delete(frc.Results, key)
		}
	}
}

func (frc *FuncResultCache) CleanExpiredResult() {
	frc.Lock()
	defer frc.Unlock()
	for key, value := range frc.Results {
		if value.TTL > 0 && time.Now().Unix() > value.TTL {
			delete(frc.Results, key)
		}
	}
}

// callCustomFunction runs one custom function call: it consults the result
// cache, invokes call when the cache has no usable entry, and caches a
// successful result. Every error it returns is a *CustomFunctionError, and a
// failed call is never cached.
func (frc *FuncResultCache) callCustomFunction(cf *pms.Function, arguments []interface{}, call func(*ext.CustomerFunctionRequest) (interface{}, error)) (interface{}, error) {
	key, err := getKey(cf.Name, arguments)
	if err != nil {
		return nil, newCustomFunctionError(cf.Name, errors.Wrapf(err, errors.CustomerFuncError, "failed to encode parameters for customer function %q", cf.Name))
	}
	if result := frc.ReadFromCache(key, cf); result != nil {
		return result, nil
	}
	result, err := call(&ext.CustomerFunctionRequest{Params: arguments})
	if err != nil {
		return nil, newCustomFunctionError(cf.Name, err)
	}
	frc.AddToCache(key, cf, result)
	return result, nil
}

func getKey(funcName string, arguments []interface{}) (string, error) {
	// JSON matches the function wire format and preserves type/argument boundaries.
	data, err := json.Marshal(arguments)
	if err != nil {
		return "", err
	}
	return funcName + "(" + string(data) + ")", nil
}

func isFunc(key, funcName string) bool {
	return strings.HasPrefix(key, funcName+"(")
}
