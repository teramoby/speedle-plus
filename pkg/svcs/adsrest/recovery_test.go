package adsrest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func TestIsAllowedPanicReturnsFalse(t *testing.T) {
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(handlerPolicies), 0600); err != nil {
		t.Fatal(err)
	}
	manager := storetest.PolicyStore(t, location)
	e, err := eval.NewWithStore(&cfg.Config{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.(*eval.PolicyEvalImpl).Close() })
	e.SetAsserterFunc(func(*ads.RequestContext) error { panic("private assertion failure") })
	router, err := NewRouter(e)
	if err != nil {
		t.Fatal(err)
	}
	rec := post(t, router, "is-allowed", `{"subject":{"principals":[{"type":"user","name":"alice"}],"tokenType":"test","token":"token"},"serviceName":"app","resource":"res1","action":"read"}`, 200)
	var response IsAllowedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Allowed || response.Reason != int32(ads.ERROR_IN_EVALUATION) || response.ErrorMessage == "" {
		t.Fatalf("panic response: %+v", response)
	}
	e.SetAsserterFunc(nil)
	rec = post(t, router, "is-allowed", `{`+alice+`,"resource":"res1","action":"read"}`, 200)
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Allowed {
		t.Fatalf("healthy request after panic: %s", rec.Body.String())
	}
}
