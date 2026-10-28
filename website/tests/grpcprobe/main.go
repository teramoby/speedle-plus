// Binary-level transport/namespace probe used by the documentation runner check.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	ads "github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	pms "github.com/teramoby/speedle-plus/pkg/svcs/pmsgrpc/pb"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"os"
	"time"
)

func main() {
	endpoint := flag.String("endpoint", "", "gRPC address")
	kind := flag.String("service", "pms", "pms, ads or etcd")
	keyName := flag.String("key-name", "", "Owned etcd probe key")
	timeout := flag.Duration("timeout", 2*time.Second, "RPC deadline")
	ca := flag.String("cacert", "", "CA file")
	cert := flag.String("cert", "", "Client certificate")
	key := flag.String("key", "", "Client key")
	plain := flag.Bool("insecure", false, "Use plaintext")
	allowed := flag.Bool("expect-allowed", false, "Require bookstore fixture Allow")
	flag.Parse()
	creds := insecure.NewCredentials()
	if !*plain {
		config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "localhost"}
		if *ca != "" {
			data, err := os.ReadFile(*ca)
			check(err)
			config.RootCAs = x509.NewCertPool()
			if !config.RootCAs.AppendCertsFromPEM(data) {
				check(fmt.Errorf("invalid CA"))
			}
		}
		if *cert != "" {
			pair, err := tls.LoadX509KeyPair(*cert, *key)
			check(err)
			config.Certificates = []tls.Certificate{pair}
		}
		creds = credentials.NewTLS(config)
	}
	conn, err := grpc.NewClient(*endpoint, grpc.WithTransportCredentials(creds))
	check(err)
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if *kind == "etcd" {
		if *keyName == "" {
			check(fmt.Errorf("etcd probe requires an owned key-name"))
		}
		client := etcdserverpb.NewKVClient(conn)
		_, err = client.Put(ctx, &etcdserverpb.PutRequest{Key: []byte(*keyName), Value: []byte("owned probe")})
		check(err)
		value, err := client.Range(ctx, &etcdserverpb.RangeRequest{Key: []byte(*keyName)})
		check(err)
		if len(value.Kvs) != 1 || string(value.Kvs[0].Value) != "owned probe" {
			check(fmt.Errorf("etcd probe value mismatch"))
		}
		_, err = client.DeleteRange(ctx, &etcdserverpb.DeleteRangeRequest{Key: []byte(*keyName)})
		check(err)
		fmt.Println("etcd authenticated write/read/delete succeeded")
		return
	}
	if *kind == "pms" {
		result, err := pms.NewPolicyManagerClient(conn).QueryServices(ctx, &pms.ServiceQueryRequest{})
		check(err)
		check(json.NewEncoder(os.Stdout).Encode(result))
		return
	}
	result, err := ads.NewEvaluatorClient(conn).IsAllowed(ctx, &ads.ContextRequest{Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "Alan"}}}, ServiceName: "onlineBookStore", Resource: "/books/HarryPotter", Action: "download"})
	check(err)
	if *allowed && !result.Allowed {
		check(fmt.Errorf("expected fixture Allow: %v", result))
	}
	check(json.NewEncoder(os.Stdout).Encode(result))
}
func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
