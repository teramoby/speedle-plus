// A deterministic mock for the token-assertion tutorial. Never use for real tokens.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type principal struct {
	Type string `json:"type"`
	Name string `json:"name"`
	IDD  string `json:"idd"`
}
type response struct {
	Principals []principal `json:"principals,omitempty"`
	ErrCode    int         `json:"errCode"`
	ErrMessage string      `json:"errMessage,omitempty"`
}

func assert(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		send(405, response{ErrCode: 405, ErrMessage: "GET required"})
		return
	}
	// Fixed fixtures model successful assertion. This is not signature validation.
	domains := map[string]string{"githubtoken": "github", "googletoken": "google", "gitlabtoken": "gitlab"}
	domain, ok := domains[r.Header.Get("x-token")]
	if !ok || domain != r.Header.Get("x-idp") {
		send(401, response{ErrCode: 401, ErrMessage: "invalid demonstration token or identity provider"})
		return
	}
	if allowed := r.Header.Get("x-allowedIDD"); allowed != "" {
		found := false
		for _, name := range strings.Split(allowed, ",") {
			found = found || strings.TrimSpace(name) == domain
		}
		if !found {
			send(403, response{ErrCode: 403, ErrMessage: "identity domain is not allowed"})
			return
		}
	}
	send(200, response{Principals: []principal{{Type: "user", Name: "user1", IDD: domain}}})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "Local demonstration endpoint")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assert", assert)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
