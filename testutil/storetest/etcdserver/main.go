// Command etcdserver runs a disposable etcd for the etcd full regression:
// random loopback ports (never 2379/2380, which the etcd implementation tests
// use), data in the given directory, and the client endpoint written to a file
// once the server is ready. It stops on SIGINT or SIGTERM.
//
//	etcdserver DATA_DIR ENDPOINT_FILE
package main

import (
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.etcd.io/etcd/server/v3/embed"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: etcdserver DATA_DIR ENDPOINT_FILE")
	}
	configuration := embed.NewConfig()
	configuration.Dir = os.Args[1]
	configuration.LogLevel = "error"
	loopback, _ := url.Parse("http://127.0.0.1:0")
	configuration.ListenClientUrls = []url.URL{*loopback}
	configuration.AdvertiseClientUrls = []url.URL{*loopback}
	configuration.ListenPeerUrls = []url.URL{*loopback}
	configuration.AdvertisePeerUrls = []url.URL{*loopback}
	configuration.InitialCluster = configuration.InitialClusterFromName(configuration.Name)
	server, err := embed.StartEtcd(configuration)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()
	select {
	case <-server.Server.ReadyNotify():
	case <-time.After(30 * time.Second):
		log.Fatal("etcd did not become ready")
	}
	// Write then rename, so a reader never sees a partial endpoint.
	endpoint := os.Args[2]
	if err := os.WriteFile(endpoint+".tmp", []byte(server.Clients[0].Addr().String()), 0600); err != nil {
		log.Fatal(err)
	}
	if err := os.Rename(endpoint+".tmp", endpoint); err != nil {
		log.Fatal(err)
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
	case err := <-server.Err():
		log.Fatal(err)
	}
}
