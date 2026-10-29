package storetest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
	mysqlstore "github.com/teramoby/speedle-plus/pkg/store/mysql"
	"github.com/teramoby/speedle-plus/pkg/suid"
)

// MySQLFixture owns a disposable database, never an installed policy store.
// Separate fixtures cannot overwrite each other's policies or discovery data.
type MySQLFixture struct {
	Store    *mysqlstore.Store
	Config   *cfg.StoreConfig
	admin    *sql.DB
	database string
}

func NewMySQLFixture(initial *pms.PolicyStore) (*MySQLFixture, error) {
	dsn := os.Getenv("SPEEDLE_MYSQL_TEST_DSN")
	configuration, err := driver.ParseDSN(dsn)
	if err != nil || dsn == "" || !strings.HasPrefix(configuration.DBName, "speedle_test") {
		return nil, fmt.Errorf("MySQL fixture requires SPEEDLE_MYSQL_TEST_DSN naming a disposable speedle_test database")
	}
	configuration.DBName = ""
	admin, err := sql.Open("mysql", configuration.FormatDSN())
	if err != nil {
		return nil, err
	}
	name := "speedle_test_" + suid.New().String()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin"); err != nil {
		admin.Close()
		return nil, err
	}
	configuration.DBName = name
	conf := &cfg.StoreConfig{StoreType: "mysql", StoreProps: map[string]interface{}{
		mysqlstore.DSNKey: configuration.FormatDSN(), mysqlstore.PollIntervalKey: "20ms", mysqlstore.QueryTimeoutKey: "30s",
	}}
	f := &MySQLFixture{Config: conf, admin: admin, database: name}
	manager, err := store.NewStore("mysql", conf.StoreProps)
	if err != nil {
		f.Close()
		return nil, err
	}
	f.Store = manager.(*mysqlstore.Store)
	if initial != nil {
		if err := f.Store.WritePolicyStore(initial); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func (f *MySQLFixture) Close() {
	if f.Store != nil {
		_ = f.Store.Close()
	}
	if f.admin != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = f.admin.ExecContext(ctx, "DROP DATABASE `"+f.database+"`")
		_ = f.admin.Close()
		f.admin = nil
	}
}

// PolicyStore ports a backend-agnostic test fixture without changing its data
// or assertions. Adapter-specific file/etcd tests keep their own factories;
// STORE_TYPE selects MySQL or the owned external etcd, otherwise the file store.
func PolicyStore(t testing.TB, location string) pms.PolicyStoreManager {
	t.Helper()
	if os.Getenv("STORE_TYPE") != "mysql" && !UseEtcd() {
		s, err := store.NewStore("file", map[string]interface{}{"FileLocation": location})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	raw, err := os.ReadFile(location)
	if err != nil {
		t.Fatal(err)
	}
	var initial pms.PolicyStore
	if err := json.Unmarshal(raw, &initial); err != nil {
		t.Fatal(err)
	}
	if UseEtcd() {
		f, err := NewEtcdFixture(&initial)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.Close)
		return f.Store
	}
	f, err := NewMySQLFixture(&initial)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f.Store
}
