package eval

import (
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/store"
)

func TestDiscoverDoesNotStoreToken(t *testing.T) {
	preparePolicyDataInStore([]byte(`{"services": [{"name": "discover-token"}]}`), t)
	evaluator, err := NewWithStore(conf, testPS)
	if err != nil {
		t.Fatal(err)
	}
	subject := &adsapi.Subject{
		Principals: []*adsapi.Principal{{Type: adsapi.PRINCIPAL_TYPE_USER, Name: "alice"}},
		TokenType:  "bearer",
		Token:      "secret-token",
	}
	ctx := adsapi.RequestContext{Subject: subject, ServiceName: "discover-token", Resource: "/r", Action: "get"}
	if _, _, err := evaluator.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	if subject.Token != "secret-token" {
		t.Errorf("the caller's subject was changed: token %q", subject.Token)
	}
	saved, _, err := testPS.(store.DiscoverRequestManager).GetLastDiscoverRequest("discover-token")
	if err != nil || saved == nil || saved.Subject == nil {
		t.Fatalf("no saved discover request: %v", err)
	}
	if saved.Subject.Token != "" {
		t.Errorf("the token was stored: %q", saved.Subject.Token)
	}
	if saved.Subject.TokenType != "bearer" || len(saved.Subject.Principals) != 1 {
		t.Errorf("the rest of the subject was not kept: %+v", saved.Subject)
	}
}
