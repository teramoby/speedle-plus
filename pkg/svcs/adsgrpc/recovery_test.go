package adsgrpc

import (
	"context"
	"testing"

	ads "github.com/teramoby/speedle-plus/api/ads"
)

func TestAsserterPanicIsContained(t *testing.T) {
	service := newService(t)
	service.evaluator.SetAsserterFunc(func(ctx *ads.RequestContext) error {
		// Even if a broken asserter sets this before panicking, the handler
		// must stop on its error instead of passing a trusted identity onward.
		ctx.Subject.Asserted = true
		panic("assertion implementation failed")
	})
	for name, call := range map[string]func() error{
		"IsAllowed": func() error {
			in := request("alice", "res1", "read", nil)
			in.Subject.TokenType = "test"
			in.Subject.Token = "token"
			out, err := service.IsAllowed(context.Background(), in)
			if out != nil && out.Allowed {
				t.Fatal("panic allowed request")
			}
			return err
		},
		"Roles": func() error {
			in := request("alice", "res1", "read", nil)
			in.Subject.TokenType = "test"
			in.Subject.Token = "token"
			_, err := service.GetAllGrantedRoles(context.Background(), in)
			return err
		},
		"Permissions": func() error {
			in := request("alice", "res1", "read", nil)
			in.Subject.TokenType = "test"
			in.Subject.Token = "token"
			_, err := service.GetAllPermissions(context.Background(), in)
			return err
		},
		"Diagnose": func() error {
			in := request("alice", "res1", "read", nil)
			in.Subject.TokenType = "test"
			in.Subject.Token = "token"
			_, err := service.Diagnose(context.Background(), in)
			return err
		},
		"Discover": func() error {
			in := request("alice", "res1", "read", nil)
			in.Subject.TokenType = "test"
			in.Subject.Token = "token"
			_, err := service.Discover(context.Background(), in)
			return err
		},
	} {
		if err := call(); err == nil {
			t.Fatalf("%s ignored asserter error", name)
		}
	}
	service.evaluator.SetAsserterFunc(nil)
	if out, err := service.IsAllowed(context.Background(), request("alice", "res1", "read", nil)); err != nil || !out.Allowed {
		t.Fatalf("request after panic: %v %v", out, err)
	}
}
