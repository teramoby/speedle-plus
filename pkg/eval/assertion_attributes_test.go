// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"fmt"
	"reflect"
	"testing"

	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
)

func TestFeatureFailedAssertionDoesNotInjectAttributes(t *testing.T) {
	for _, failure := range []string{"error", "panic", "no-subject"} {
		t.Run(failure, func(t *testing.T) {
			policy := recoveryGrant("conditional")
			policy.Condition = "isAdmin == true"
			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{policy}}}})
			ctx := featureContext()
			ctx.Subject.TokenType, ctx.Subject.Token = "test", "token"
			attributes := map[string]interface{}{"isAdmin": false, "roles": []string{"reader"}, "claims": map[string]interface{}{"admin": false}}
			ctx.Attributes = attributes
			e.SetAsserterFunc(func(request *ads.RequestContext) error {
				request.Attributes["isAdmin"] = true
				request.Attributes["roles"].([]string)[0] = "admin"
				request.Attributes["claims"].(map[string]interface{})["admin"] = true
				if failure == "panic" {
					panic("assertion failed")
				}
				if failure == "no-subject" {
					request.Subject = nil
					return nil
				}
				return fmt.Errorf("assertion failed")
			})
			if err := e.AssertToken(&ctx); err == nil {
				t.Fatal("failed asserter succeeded")
			}
			want := map[string]interface{}{"isAdmin": false, "roles": []string{"reader"}, "claims": map[string]interface{}{"admin": false}}
			if !reflect.DeepEqual(ctx.Attributes, want) || !reflect.DeepEqual(attributes, want) {
				t.Fatalf("failed assertion leaked attributes: %+v", ctx.Attributes)
			}
			e.SetAsserterFunc(nil)
			if allowed, _, err := e.IsAllowed(ctx); allowed || err != nil {
				t.Fatalf("reused context gained privileges: %v %v", allowed, err)
			}
		})
	}
}

func TestFeatureSuccessfulAssertionCommitsAttributes(t *testing.T) {
	e := featureEvaluator(t, &pms.PolicyStore{})
	for _, replacement := range []bool{false, true} {
		t.Run(fmt.Sprint(replacement), func(t *testing.T) {
			ctx := featureContext()
			ctx.Subject.TokenType, ctx.Subject.Token = "test", "token"
			original := map[string]interface{}{"isAdmin": false, "array": [2]string{"a", "b"}, "nil": nil, "empty": []interface{}{}, "typed-nil": []string(nil)}
			ctx.Attributes = original
			e.SetAsserterFunc(func(request *ads.RequestContext) error {
				request.Attributes["isAdmin"] = true
				if replacement {
					request.Attributes = map[string]interface{}{"replacement": true}
				}
				return nil
			})
			if err := e.AssertToken(&ctx); err != nil {
				t.Fatal(err)
			}
			if original["isAdmin"] != true {
				t.Fatal("successful mutation not committed to original map")
			}
			if replacement {
				if ctx.Attributes["replacement"] != true {
					t.Fatal("successful map replacement lost")
				}
			} else {
				original["after"] = true
				if ctx.Attributes["after"] != true {
					t.Fatal("normal assertion replaced map identity")
				}
			}
		})
	}
}

func TestFeatureAssertionRejectsCyclicAttributes(t *testing.T) {
	e := featureEvaluator(t, &pms.PolicyStore{})
	called := false
	e.SetAsserterFunc(func(*ads.RequestContext) error { called = true; return nil })
	ctx := featureContext()
	ctx.Subject.TokenType, ctx.Subject.Token = "test", "token"
	ctx.Attributes = map[string]interface{}{}
	ctx.Attributes["cycle"] = ctx.Attributes
	if err := e.AssertToken(&ctx); err == nil || called || ctx.Subject.Asserted {
		t.Fatal("cyclic assertion attributes were not rejected before callback")
	}
}
