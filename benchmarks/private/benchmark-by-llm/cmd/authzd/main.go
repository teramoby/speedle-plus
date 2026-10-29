// Command authzd serves one embedded engine behind the same thin HTTP API,
// so the three engines can be load-tested with identical server code.
//
//	authzd -engine opa -n 10000 -addr 127.0.0.1:9000
//	POST /check {"user":"user5000-7","action":"read","resource":"/books/book5000"}
//	-> {"allowed":true}
//
// With -gen DIR it only writes the generated policy files (for the official
// OPA and Speedle+ servers) and exits.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	ab "authzbench"
)

func main() {
	engine := flag.String("engine", "opa", "casbin | opa | speedle")
	kind := flag.String("kind", "plain", "plain | manyusers")
	n := flag.Int("n", 10000, "number of roles (10 users each)")
	addr := flag.String("addr", "127.0.0.1:9000", "listen address")
	gen := flag.String("gen", "", "write policy files to this directory and exit")
	flag.Parse()

	dir := *gen
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "authzd-"); err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	g, err := ab.GenPolicies(dir, *kind, *n, 10)
	if err != nil {
		log.Fatal(err)
	}
	if *gen != "" {
		fmt.Println(g.OPARego, g.OPAJSON, g.SpeedleSPDL)
		return
	}

	start := time.Now()
	var e ab.Engine
	switch *engine {
	case "casbin":
		e, err = ab.NewCasbinSynced(g.CasbinConf, g.CasbinCSV, ab.CasbinArgs)
	case "opa":
		e, err = ab.NewOPA(g.OPARego, g.OPAJSON)
	case "speedle":
		e, err = ab.NewSpeedle(g.SpeedleSPDL, false)
	default:
		log.Fatalf("unknown engine %q", *engine)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s loaded %d roles in %v; listening on %s", *engine, *n, time.Since(start).Round(time.Millisecond), *addr)

	http.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ User, Action, Resource string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ok, err := e.Allowed(ab.Req{User: req.User, Action: req.Action, Resource: req.Resource})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"allowed":%v}`, ok)
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	log.Fatal(http.ListenAndServe(*addr, nil))
}
