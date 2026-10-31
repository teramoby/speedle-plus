//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"io"
	"time"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"

	log "github.com/sirupsen/logrus"
)

// New creates a policy evaluator based on the given configuration file
func New(configFile string) (InternalEvaluator, error) {
	conf, err := cfg.ReadConfig(configFile)
	if err != nil {
		return nil, err
	}
	return NewFromConfig(conf)
}

// NewFromFile loads policies from a policy file, and returns an evaluator instance
func NewFromFile(fileLoc string, isWatch bool) (adsapi.PolicyEvaluator, error) {
	storeConfig := cfg.StoreConfig{
		StoreType: "file",
		StoreProps: map[string]interface{}{
			"FileLocation": fileLoc,
		},
	}

	// For file store, watchuing a policy store is disabled
	return NewFromConfig(&cfg.Config{
		StoreConfig: &storeConfig,
		EnableWatch: isWatch,
	})
}

// NewFromConfig creates a policy evaluator based on the given configuration file
func NewFromConfig(conf *cfg.Config) (evaluator InternalEvaluator, err error) {
	defer recoverEvaluation("NewFromConfig", &err)
	s, err := store.NewStore(conf.StoreConfig.StoreType, conf.StoreConfig.StoreProps)
	if err != nil {
		return nil, err
	}

	evaluator, err = NewWithStore(conf, s)
	if err != nil {
		if closer, ok := s.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	return evaluator, err
}

// NewWithStore creates a policy evaluator with policy store
func NewWithStore(conf *cfg.Config, s pms.PolicyStoreManagerADS) (evaluator InternalEvaluator, err error) {
	var watching bool
	defer func() {
		if err != nil && watching {
			stopFailedWatch(s)
		}
	}()
	defer recoverEvaluation("NewWithStore", &err)
	var grace time.Duration
	if conf.StoreFailureGracePeriod != "" {
		grace, err = time.ParseDuration(conf.StoreFailureGracePeriod)
		if err != nil || grace < 0 {
			return nil, errors.New(errors.ConfigError, "storeFailureGracePeriod must be a finite nonnegative duration")
		}
	}
	ps, err := s.ReadPolicyStore()
	if err != nil {
		return nil, err
	}

	var updateChan pms.StorageChangeChannel
	if conf.EnableWatch {
		log.Info("Watch policy store.")
		updateChan, err = s.Watch()
		if err != nil {
			return nil, err
		}
		watching = true
		// File/Mongo watches are installed synchronously; etcd waits for its
		// created acknowledgement. Read again after subscribing to close the
		// initial read/subscribe gap. Queued events invalidate this snapshot.
		ps, err = s.ReadPolicyStore()
		if err != nil {
			return nil, err
		}
	}

	runtimePolicyStore := NewRuntimePolicyStore()
	runtimePolicyStore.init(ps, conf.FuncsvcEndpoint)

	p := &PolicyEvalImpl{
		RuntimePolicyStore:      runtimePolicyStore,
		Store:                   s,
		storeFailureGracePeriod: grace,
		lastHealthCheck:         time.Now(),
	}
	p.healthChecker, _ = s.(pms.StoreHealthChecker)
	p.watchManaged = updateChan != nil

	// start a goroutine watching to the channel for update events and
	// refresh runtime cache accordingly once receiving any events
	if updateChan != nil {
		go p.followStoreChanges(updateChan)
	}

	p.cleanExpiredFunctionResultPeriodically()

	return p, nil
}

func stopFailedWatch(s pms.PolicyStoreManagerADS) {
	var err error
	defer recoverEvaluation("StopWatchAfterFailedInitialization", &err)
	s.StopWatch()
}
