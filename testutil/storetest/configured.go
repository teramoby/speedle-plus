package storetest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/mongodb"
	"github.com/teramoby/speedle-plus/pkg/suid"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func SelectedBackend() string {
	if backend := os.Getenv("STORE_TYPE"); backend != "" {
		return backend
	}
	return "file"
}

// Fixture owns one isolated store. Connection properties come from the backend's
// native JSON map in SPEEDLE_TEST_STORE_PROPS; test data and assertions are shared.
type Fixture struct {
	Store   pms.PolicyStoreManager
	Config  *cfg.StoreConfig
	cleanup func()
}

func (f *Fixture) Close() {
	if f.cleanup != nil {
		f.cleanup()
		f.cleanup = nil
	}
}

func FixtureConfig(location string) (*cfg.StoreConfig, error) {
	backend := SelectedBackend()
	properties := map[string]interface{}{}
	if raw := os.Getenv("SPEEDLE_TEST_STORE_PROPS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &properties); err != nil {
			return nil, fmt.Errorf("invalid SPEEDLE_TEST_STORE_PROPS: %w", err)
		}
		if properties == nil {
			return nil, fmt.Errorf("SPEEDLE_TEST_STORE_PROPS must be a JSON object")
		}
	}
	switch backend {
	case "file":
		properties["FileLocation"] = location
	case "etcd":
		if _, ok := properties["EtcdEndpoint"]; !ok {
			endpoint := os.Getenv("SPEEDLE_ETCD_ENDPOINT")
			if endpoint == "" {
				endpoint = "localhost:2379"
			}
			properties["EtcdEndpoint"] = endpoint
		}
		prefix, _ := properties["EtcdKeyPrefix"].(string)
		properties["EtcdKeyPrefix"] = strings.TrimRight(prefix, "/") + "/speedle_test_" + suid.New().String() + "/"
	case "mongodb":
		if _, ok := properties["MongoURI"]; !ok {
			properties["MongoURI"] = os.Getenv("SPEEDLE_MONGO_URI")
		}
		if uri, ok := properties["MongoURI"].(string); !ok || uri == "" {
			return nil, fmt.Errorf("mongodb requires MongoURI in SPEEDLE_TEST_STORE_PROPS or SPEEDLE_MONGO_URI")
		}
		prefix, _ := properties["MongoDatabase"].(string)
		if prefix == "" {
			prefix = "speedle_test"
		}
		if !strings.HasPrefix(prefix, "speedle_test") {
			return nil, fmt.Errorf("MongoDB test database prefix must start with speedle_test")
		}
		properties["MongoDatabase"] = prefix + "_" + suid.New().String()
	case "mysql":
		if _, ok := properties["MySQLDSN"]; !ok {
			properties["MySQLDSN"] = os.Getenv("SPEEDLE_MYSQL_TEST_DSN")
		}
		if dsn, ok := properties["MySQLDSN"].(string); !ok || dsn == "" {
			return nil, fmt.Errorf("mysql requires MySQLDSN in SPEEDLE_TEST_STORE_PROPS or SPEEDLE_MYSQL_TEST_DSN")
		}
	default:
		return nil, fmt.Errorf("unsupported test backend %q", backend)
	}
	return &cfg.StoreConfig{StoreType: backend, StoreProps: properties}, nil
}

func NewFixture(initial *pms.PolicyStore) (*Fixture, error) {
	configuration, err := FixtureConfig("")
	if err != nil {
		return nil, err
	}
	if configuration.StoreType == "mysql" {
		fixture, err := NewMySQLFixture(initial, configuration.StoreProps)
		if err != nil {
			return nil, err
		}
		return &Fixture{Store: fixture.Store, Config: fixture.Config, cleanup: fixture.Close}, nil
	}
	cleanup := func() {}
	if configuration.StoreType == "file" {
		file, err := os.CreateTemp("", "speedle_test_*.json")
		if err != nil {
			return nil, err
		}
		location := file.Name()
		if err := file.Close(); err != nil {
			os.Remove(location)
			return nil, err
		}
		// An empty file is not a valid store snapshot.
		if err := os.WriteFile(location, []byte(`{}`), 0600); err != nil {
			os.Remove(location)
			return nil, err
		}
		configuration.StoreProps["FileLocation"] = location
		cleanup = func() { os.Remove(location) }
	}
	manager, err := store.NewStore(configuration.StoreType, configuration.StoreProps)
	if err != nil {
		cleanup()
		return nil, err
	}
	fixture := &Fixture{Store: manager, Config: configuration}
	// Retain the exact owned namespace even if a caller changes its config map.
	mongoURI, _ := configuration.StoreProps["MongoURI"].(string)
	mongoDatabase, _ := configuration.StoreProps["MongoDatabase"].(string)
	fixture.cleanup = func() {
		if closer, ok := manager.(interface{ Close() error }); ok {
			_ = closer.Close()
		} else {
			manager.StopWatch()
		}
		if configuration.StoreType == "mongodb" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
			if err == nil {
				_ = client.Database(mongoDatabase).Drop(ctx)
				_ = client.Disconnect(ctx)
			}
		}
		cleanup()
	}
	if initial != nil {
		if err := manager.WritePolicyStore(initial); err != nil {
			fixture.Close()
			return nil, err
		}
	}
	return fixture, nil
}

// PolicyStore reuses the MySQL PR's shared fixture entry point for all backends.
// Adapter-specific file/etcd tests keep their own factories.
func PolicyStore(t testing.TB, location string) pms.PolicyStoreManager {
	t.Helper()
	if SelectedBackend() == "file" {
		manager, err := store.NewStore("file", map[string]interface{}{"FileLocation": location})
		if err != nil {
			t.Fatal(err)
		}
		return manager
	}
	raw, err := os.ReadFile(location)
	if err != nil {
		t.Fatal(err)
	}
	var initial pms.PolicyStore
	if err := json.Unmarshal(raw, &initial); err != nil {
		t.Fatal(err)
	}
	fixture, err := NewFixture(&initial)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	t.Logf("integration fixture backend=%s", fixture.Config.StoreType)
	return fixture.Store
}
