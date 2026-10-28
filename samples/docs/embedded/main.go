package main

import (
	"flag"
	"fmt"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
)

func main() {
	policies := flag.String("policies", "samples/docs/embedded/policies.json", "Policy file")
	flag.Parse()
	evaluator, err := eval.NewFromConfig(&cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: "file", StoreProps: map[string]interface{}{"FileLocation": *policies}}, EnableWatch: false})
	if err != nil {
		log.Fatal(err)
	}
	defer evaluator.Close()
	for _, user := range []string{"alice", "bob"} {
		allowed, reason, err := evaluator.IsAllowed(ads.RequestContext{Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: user}}}, ServiceName: "library", Resource: "book", Action: "read"})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: allowed=%t reason=%s\n", user, allowed, reason.String())
	}
}
