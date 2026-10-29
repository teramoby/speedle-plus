package storetest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/suid"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

// pkg/store/etcd's own tests import this package, so its names are repeated
// here; callers register the etcd store by importing pkg/store/etcd.
const (
	etcdStoreType    = "etcd"
	etcdEndpointKey  = "EtcdEndpoint"
	etcdKeyPrefixKey = "EtcdKeyPrefix"
)

// UseEtcd reports whether STORE_TYPE selects etcd fixtures.
func UseEtcd() bool {
	return os.Getenv("STORE_TYPE") == "etcd"
}

// One embedded etcd on random loopback ports serves every open fixture in
// the test process; it never attaches to an installed etcd.
var shared struct {
	sync.Mutex
	server *embed.Etcd
	dir    string
	users  int
}

func acquireEtcd() (string, error) {
	shared.Lock()
	defer shared.Unlock()
	if shared.server == nil {
		dir, err := os.MkdirTemp("", "speedle-etcd-fixture-")
		if err != nil {
			return "", err
		}
		configuration := embed.NewConfig()
		configuration.Dir = dir
		configuration.LogLevel = "error"
		loopback, _ := url.Parse("http://127.0.0.1:0")
		configuration.ListenClientUrls = []url.URL{*loopback}
		configuration.AdvertiseClientUrls = []url.URL{*loopback}
		configuration.ListenPeerUrls = []url.URL{*loopback}
		configuration.AdvertisePeerUrls = []url.URL{*loopback}
		configuration.InitialCluster = configuration.InitialClusterFromName(configuration.Name)
		server, err := embed.StartEtcd(configuration)
		if err != nil {
			os.RemoveAll(dir)
			return "", err
		}
		select {
		case <-server.Server.ReadyNotify():
		case <-time.After(10 * time.Second):
			server.Close()
			os.RemoveAll(dir)
			return "", fmt.Errorf("fixture etcd did not become ready")
		}
		shared.server, shared.dir = server, dir
	}
	shared.users++
	return shared.server.Clients[0].Addr().String(), nil
}

func releaseEtcd() {
	shared.Lock()
	defer shared.Unlock()
	if shared.users--; shared.users == 0 {
		shared.server.Close()
		os.RemoveAll(shared.dir)
		shared.server = nil
	}
}

// EtcdFixture owns a key prefix on the fixture etcd. Separate fixtures cannot
// overwrite each other's policies. Discover requests use one global etcd
// prefix, which no fixture clears: fixtures open at the same time share those
// requests, and the etcd starts empty and is removed with its last fixture.
type EtcdFixture struct {
	Store  pms.PolicyStoreManager
	Config *cfg.StoreConfig
	client *clientv3.Client
	prefix string
}

func NewEtcdFixture(initial *pms.PolicyStore) (*EtcdFixture, error) {
	endpoint, err := acquireEtcd()
	if err != nil {
		return nil, err
	}
	client, err := clientv3.New(clientv3.Config{Endpoints: []string{endpoint}, DialTimeout: 10 * time.Second})
	if err != nil {
		releaseEtcd()
		return nil, err
	}
	f := &EtcdFixture{client: client, prefix: "/speedle-test/" + suid.New().String() + "/"}
	f.Config = &cfg.StoreConfig{StoreType: etcdStoreType, StoreProps: map[string]interface{}{
		etcdEndpointKey: endpoint, etcdKeyPrefixKey: f.prefix,
	}}
	f.Store, err = store.NewStore(etcdStoreType, f.Config.StoreProps)
	if err != nil {
		f.Close()
		return nil, err
	}
	if initial != nil {
		if err := f.Store.WritePolicyStore(initial); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func (f *EtcdFixture) Close() {
	if f.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = f.client.Delete(ctx, f.prefix, clientv3.WithPrefix())
		cancel()
		_ = f.client.Close()
		f.client = nil
		releaseEtcd()
	}
}
