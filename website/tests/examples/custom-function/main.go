package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"time"
)

type request struct {
	Params []interface{} `json:"params"`
}
type response struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func isValid(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		send(405, response{Error: "POST required"})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input request
	if err := decoder.Decode(&input); err != nil || len(input.Params) == 0 {
		send(400, response{Error: "params must contain at least one number"})
		return
	}
	valid := true
	for _, param := range input.Params {
		value, ok := param.(float64)
		if !ok {
			send(400, response{Error: "all params must be numbers"})
			return
		}
		if value < 100 || value > 50000 {
			valid = false
		}
	}
	send(200, response{Result: valid})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:23456", "Function listen address")
	cert := flag.String("cert", "", "TLS server certificate")
	key := flag.String("key", "", "TLS server key")
	flag.Parse()
	if (*cert == "") != (*key == "") {
		log.Fatal("provide both --cert and --key")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/func/isValid", isValid)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	if *cert != "" {
		log.Fatal(server.ListenAndServeTLS(*cert, *key))
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("plaintext examples require a loopback IP; provide TLS certificates for other addresses")
	}
	log.Fatal(server.ListenAndServe())
}
