// Licensed under the Universal Permissive License (UPL) Version 1.0.
package storetest

import (
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/teramoby/speedle-plus/pkg/cfg"
)

// OutageConfiguration routes only the ADS fixture through an owned TCP proxy.
// PMS can still revoke permissions while ADS is disconnected. File fixtures
// move their own policy file aside. No shared database or runner is stopped.
func OutageConfiguration(t testing.TB, original *cfg.StoreConfig) (*cfg.StoreConfig, func(bool)) {
	t.Helper()
	props := map[string]interface{}{}
	for key, value := range original.StoreProps {
		props[key] = value
	}
	configuration := &cfg.StoreConfig{StoreType: original.StoreType, StoreProps: props}
	if original.StoreType == "file" {
		path := props["FileLocation"].(string)
		offline := path + ".offline"
		online := true
		set := func(value bool) {
			if value == online {
				return
			}
			from, to := path, offline
			if value {
				from, to = to, from
			}
			if err := os.Rename(from, to); err != nil {
				t.Fatal(err)
			}
			online = value
		}
		t.Cleanup(func() {
			if !online {
				set(true)
			}
		})
		return configuration, set
	}
	var address string
	var rewrite func(string)
	switch original.StoreType {
	case "mysql":
		dsn, err := driver.ParseDSN(props["MySQLDSN"].(string))
		if err != nil {
			t.Fatal("invalid fixture DSN")
		}
		if dsn.Net != "tcp" {
			t.Fatal("outage fixture requires TCP MySQL")
		}
		address = dsn.Addr
		rewrite = func(proxy string) {
			dsn.Addr = proxy
			props["MySQLDSN"] = dsn.FormatDSN()
			props["MySQLQueryTimeout"] = "500ms"
			props["MySQLPollInterval"] = "50ms"
			props["MySQLInitSchema"] = false
		}
	case "etcd":
		endpoint := props["EtcdEndpoint"].(string)
		if !strings.Contains(endpoint, "://") {
			endpoint = "http://" + endpoint
		}
		u, err := url.Parse(endpoint)
		if err != nil {
			t.Fatal("invalid fixture endpoint")
		}
		address = u.Host
		rewrite = func(proxy string) {
			u.Host = proxy
			props["EtcdEndpoint"] = u.String()
			props["IsEmbeddedEtcd"] = false
		}
	case "mongodb":
		u, err := url.Parse(props["MongoURI"].(string))
		if err != nil || u.Scheme != "mongodb" {
			t.Fatal("outage fixture requires a direct MongoDB URI")
		}
		address = u.Host
		rewrite = func(proxy string) {
			u.Host = proxy
			q := u.Query()
			q.Set("directConnection", "true")
			q.Set("serverSelectionTimeoutMS", "500")
			q.Set("connectTimeoutMS", "500")
			q.Set("socketTimeoutMS", "1000")
			u.RawQuery = q.Encode()
			props["MongoURI"] = u.String()
		}
	default:
		t.Fatal("unsupported outage fixture backend")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rewrite(listener.Addr().String())
	var mu sync.Mutex
	online := true
	connections := map[net.Conn]bool{}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			incoming, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			enabled := online
			mu.Unlock()
			if !enabled {
				incoming.Close()
				continue
			}
			outgoing, err := net.DialTimeout("tcp", address, time.Second)
			if err != nil {
				incoming.Close()
				continue
			}
			mu.Lock()
			if !online {
				mu.Unlock()
				incoming.Close()
				outgoing.Close()
				continue
			}
			connections[incoming], connections[outgoing] = true, true
			mu.Unlock()
			for _, pair := range [][2]net.Conn{{incoming, outgoing}, {outgoing, incoming}} {
				workers.Add(1)
				go func(pair [2]net.Conn) {
					defer workers.Done()
					_, _ = io.Copy(pair[0], pair[1])
					incoming.Close()
					outgoing.Close()
					mu.Lock()
					delete(connections, incoming)
					delete(connections, outgoing)
					mu.Unlock()
				}(pair)
			}
		}
	}()
	set := func(value bool) {
		mu.Lock()
		defer mu.Unlock()
		online = value
		if !value {
			for connection := range connections {
				connection.Close()
			}
		}
	}
	t.Cleanup(func() { listener.Close(); set(false); workers.Wait() })
	return configuration, set
}
