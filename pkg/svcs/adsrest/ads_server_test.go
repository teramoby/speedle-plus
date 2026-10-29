// Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
// Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.
package adsrest

import (
	"net/http/httptest"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/etcd"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

var storeLocation = "./fakestore.json"

func NewTestServerWithConfig(t *testing.T, conf *cfg.Config) (*httptest.Server, error) {
	e, err := newEvaluator(t, conf)
	if err != nil {
		return nil, err
	}
	routers, err := NewRouter(e)
	if err != nil {
		return nil, err
	}
	server := httptest.NewUnstartedServer(routers)
	server.Start()

	return server, nil
}

func NewTestServer(t *testing.T) (*httptest.Server, error) {
	conf := GenerateServerConfig()
	evaluator, err := newEvaluator(t, conf)
	if err != nil {
		return nil, err
	}
	routers, err := NewRouter(evaluator)
	if err != nil {
		return nil, err
	}
	server := httptest.NewUnstartedServer(routers)
	server.Start()

	return server, nil
}

func GenerateServerConfig() *cfg.Config {
	var conf cfg.Config
	var storeConf cfg.StoreConfig
	storeConf.StoreType = cfg.StorageTypeFile
	storeConf.StoreProps = make(map[string]interface{})
	storeConf.StoreProps["FileLocation"] = storeLocation
	conf.StoreConfig = &storeConf
	conf.EnableWatch = false
	return &conf
}

func newEvaluator(t *testing.T, conf *cfg.Config) (eval.InternalEvaluator, error) {
	manager := storetest.PolicyStore(t, conf.StoreConfig.StoreProps["FileLocation"].(string))
	evaluator, err := eval.NewWithStore(conf, manager)
	if err != nil {
		return nil, err
	}

	t.Cleanup(func() { _ = evaluator.(*eval.PolicyEvalImpl).Close() })
	as, errLoadAsserter := assertion.NewAsserter(conf.AsserterWebhookConfig, nil)
	if errLoadAsserter != nil {
		return nil, errLoadAsserter
	} else {
		f := func(ctx *adsapi.RequestContext) error {
			if ctx.Subject != nil &&
				len(ctx.Subject.TokenType) != 0 &&
				len(ctx.Subject.Token) != 0 {
				tokenType := ctx.Subject.TokenType
				token := ctx.Subject.Token
				s, err := as.AssertToken(token, tokenType, "", nil)
				if err == nil {
					ctx.Subject.Principals = append(ctx.Subject.Principals, s.Principals...)
				} else {
					return err
				}
			}
			return nil
		}
		evaluator.SetAsserterFunc(f)
	}

	return evaluator, nil
}
