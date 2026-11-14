// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval_test

import (
	"io"
	"net"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/store/etcd"
)

// A successful nonblocking etcd client constructor must not turn an
// unavailable policy store into a usable, empty authorization evaluator.
func TestEtcdUnavailableEndpointRejectsEvaluatorStartup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			// Own the endpoint but never serve an etcd handshake or snapshot.
			_ = connection.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	manager, err := (etcd.Etcd3StoreBuilder{}).NewStore(map[string]interface{}{
		etcd.EtcdEndpointKey:  "http://" + listener.Addr().String(),
		etcd.EtcdKeyPrefixKey: "/startup-test/",
	})
	if err != nil {
		t.Fatalf("client construction failed before the initial snapshot read: %v", err)
	}
	t.Cleanup(func() { _ = manager.(io.Closer).Close() })
	engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, manager)
	if engine != nil {
		if closer, ok := engine.(io.Closer); ok {
			_ = closer.Close()
		}
		t.Fatal("unavailable etcd produced an authorization evaluator")
	}
	if err == nil {
		t.Fatal("unavailable etcd did not fail evaluator startup")
	}
}
