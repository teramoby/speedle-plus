//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsproxy"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/eval"
)

// downstreamStore stands in for a policy-source store that serves children.
type downstreamStore struct {
	handler http.Handler
}

func (s downstreamStore) DownstreamHandler() http.Handler { return s.handler }

// downstreamEvaluator overrides the distribution store of a real evaluator.
type downstreamEvaluator struct {
	eval.InternalEvaluator
	store any
}

func (e downstreamEvaluator) DistributionStore() any { return e.store }

func TestNewDownstreamListenerWithoutARelay(t *testing.T) {
	evaluator, err := newEvaluator(fixtureConfig(t))
	if err != nil {
		t.Fatalf("evaluator: %v", err)
	}
	defer closeEvaluator(evaluator)
	listener, err := newDownstreamListener(&flags.Parameters{}, evaluator)
	if err != nil || listener != nil {
		t.Fatalf("a store that does not serve downstream: %v %v", listener, err)
	}

	// A store whose handler is nil does not start a listener either.
	wrapped := downstreamEvaluator{InternalEvaluator: evaluator, store: downstreamStore{}}
	listener, err = newDownstreamListener(&flags.Parameters{}, wrapped)
	if err != nil || listener != nil {
		t.Fatalf("a nil downstream handler: %v %v", listener, err)
	}
}

func TestNewDownstreamListenerRequiresCompleteConfiguration(t *testing.T) {
	evaluator, err := newEvaluator(fixtureConfig(t))
	if err != nil {
		t.Fatalf("evaluator: %v", err)
	}
	defer closeEvaluator(evaluator)
	mirror, err := adsproxy.NewMirror(adsproxy.MirrorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server, err := adsproxy.NewServer(adsproxy.Config{
		InstanceID: "0193b124-9552-7a33-8a1a-000000000001", Mirror: mirror,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	wrapped := downstreamEvaluator{InternalEvaluator: evaluator, store: downstreamStore{
		handler: server,
	}}

	if _, err := newDownstreamListener(&flags.Parameters{}, wrapped); !errors.Is(err, adsproxy.ErrConfig) {
		t.Fatalf("empty listener configuration: %v", err)
	}

	params := &flags.Parameters{}
	params.DownstreamEndpoint.Value = "127.0.0.1:0"
	params.DownstreamCert.Value = filepath.Join(t.TempDir(), "missing.pem")
	params.DownstreamKey.Value = filepath.Join(t.TempDir(), "missing-key.pem")
	params.DownstreamClientCA.Value = filepath.Join(t.TempDir(), "missing-ca.pem")
	params.DownstreamCRLFiles.Value = filepath.Join(t.TempDir(), "missing.crl")
	if _, err := newDownstreamListener(params, wrapped); !errors.Is(err, adsproxy.ErrTLSMaterial) {
		t.Fatalf("unusable listener material: %v", err)
	}

	params.DownstreamCRLFiles.Value = "a.crl,,b.crl"
	if _, err := newDownstreamListener(params, wrapped); err == nil {
		t.Fatal("an empty CRL source entry was accepted")
	}
}

func closeEvaluator(evaluator eval.InternalEvaluator) {
	if closer, ok := evaluator.(io.Closer); ok {
		_ = closer.Close()
	}
}
