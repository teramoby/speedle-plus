//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package etcd

import (
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/etcd/server/v3/embed"
	"github.com/teramoby/speedle-plus/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var (
	embededStarted int32 // 0 = false, 1 = true; atomic access
	embedMu        sync.Mutex
)

const embeddedEtcdPort = 2379

//StartEmbeddedEtcd start a embed etcd which use a clean tmp directory to store data
func StartEmbeddedEtcd(dataDir string) (etcd *embed.Etcd, etcdDir string, err error) {
	if atomic.LoadInt32(&embededStarted) == 1 {
		// Already started
		return nil, "", nil
	}
	embedMu.Lock()
	defer embedMu.Unlock()
	if atomic.LoadInt32(&embededStarted) == 1 {
		return nil, "", nil
	}
	if isEtcdPortOccupied() {
		//we assume the embeded etcd is already started by other process, and we use that etcd directly.
		//This is to support starting both mgmt server and atz server, and use the same embeded etcd in dev or test env.
		return nil, "", nil
	}
	etcdDir = dataDir
	if etcdDir == "" {
		etcdDir, err = os.MkdirTemp(os.TempDir(), "etcd.tmp")
		if err != nil {
			log.Error(err)
			return etcd, etcdDir, errors.Wrapf(err, errors.StoreError, "failed to create etcd dir")
		}
		log.Infof("The embedded etcd store dir is %q", etcdDir)
	}

	cfg := embed.NewConfig()
	cfg.Dir = etcdDir
	etcd, err = embed.StartEtcd(cfg)
	if err != nil {
		log.Error(err)
		return etcd, etcdDir, errors.Wrapf(err, errors.StoreError, "failed to start embedded etcd server")
	}

	atomic.StoreInt32(&embededStarted, 1)
	select {
	case <-etcd.Server.ReadyNotify():
		log.Info("Etcd Server is ready!")
	case <-time.After(60 * time.Second):
		etcd.Server.Stop() // trigger a shutdown
		err = errors.New(errors.StoreError, "etcd Server took too long to start")
	}
	return etcd, etcdDir, err
}

//CleanEmbedEtcd free the resource of embed etcd, and remove the tmp directory which is used to store data
func CleanEmbeddedEtcd(etcd *embed.Etcd, etcdDir string) {
	if atomic.LoadInt32(&embededStarted) == 1 {
		etcd.Close()
		os.RemoveAll(etcdDir)
		atomic.StoreInt32(&embededStarted, 0)
	}
}

func isEtcdPortOccupied() bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(embeddedEtcdPort))

	if err != nil {
		return true
	}
	ln.Close()
	return false
}
