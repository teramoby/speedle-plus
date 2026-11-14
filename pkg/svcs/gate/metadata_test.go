// Licensed under the Universal Permissive License (UPL) Version 1.0.
package gate_test

import (
	"reflect"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/svcs/gate"
)

func TestFilterMetadata(t *testing.T) {
	input := map[string][]string{
		"X-Policy-Revision": {"42", "43"}, "X-Quota": {"", "7"},
		"Connection": {" X-Hop, x-other ", "X-Last"},
		"x-hop":      {"secret"}, "X-Other": {"secret"}, "X-Last": {"secret"},
		"": {"empty key"}, "bad key": {"invalid"}, ":status": {"200"},
		"X-Invalid": {"bad\r\nInjected: true", "bad\tvalue", "非ASCII", "safe"},
		"X-Empty":   {}, "grpc-status": {"0"}, "example-bin": {"binary"},
	}
	for _, key := range []string{
		"Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"TE", "Trailer", "Transfer-Encoding", "Upgrade", "Set-Cookie",
		"Content-Length", "Content-Type", "Content-Encoding", "Content-Range",
		"Content-MD5", "Digest", "Content-Digest", "Repr-Digest", "ETag", "Last-Modified",
		"Accept-Ranges", "Content-Disposition", "Content-Location", "Content-Language",
		"Cache-Control", "Retry-After",
		"X-Speedle-Decision-Source", "Speedle-Principals", "WWW-Authenticate", "Date", "Server",
	} {
		input[key] = []string{"secret"}
	}
	want := map[string][]string{"x-policy-revision": {"42", "43"}, "x-quota": {"", "7"}, "x-invalid": {"safe"}}
	got := gate.FilterMetadata(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
	got["x-policy-revision"][0] = "changed"
	if input["X-Policy-Revision"][0] != "42" || len(input["Connection"]) != 2 {
		t.Fatal("filter modified the gate's metadata")
	}
	if got := gate.FilterMetadata(nil); got != nil {
		t.Fatalf("nil metadata = %#v", got)
	}
}
