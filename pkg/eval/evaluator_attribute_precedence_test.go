// Licensed under the Universal Permissive License (UPL) Version 1.0.

package eval

import (
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
)

// These tests pin the two attribute rules stated in the SPDL reference
// (docs/hugo/content/docs/spdl.md): a request attribute with the same name as
// a built-in takes precedence over it, and a condition on an attribute that is
// missing from the request does not match, so a deny with such a condition
// does not apply either.

func TestRequestAttributesOverrideBuiltIns(t *testing.T) {
	policies := []*pms.Policy{
		// True only when the PEP-supplied value replaces the built-in "alice".
		{ID: "request-user", Effect: pms.Grant, Condition: "request_user == 'mallory'",
			Permissions: []*pms.Permission{{Resource: "user-profile", Actions: []string{"read"}}}},
		// True only when the PEP-supplied groups replace the built-in ["staff"].
		{ID: "request-groups", Effect: pms.Grant, Condition: "'staff' in request_groups",
			Permissions: []*pms.Permission{{Resource: "team-files", Actions: []string{"read"}}}},
		// True only when the PEP-supplied request_time replaces "now".
		{ID: "request-time", Effect: pms.Grant, Condition: "request_time < '2000-01-01 00:00:00'",
			Permissions: []*pms.Permission{{Resource: "archive", Actions: []string{"read"}}}},
	}
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: policies}}})

	allowed := func(resource string, attributes map[string]interface{}) bool {
		t.Helper()
		ctx := featureContext()
		ctx.Subject.Principals = append(ctx.Subject.Principals,
			&adsapi.Principal{Type: adsapi.PRINCIPAL_TYPE_GROUP, Name: "staff"})
		ctx.Resource = resource
		ctx.Attributes = attributes
		ok, _, err := e.IsAllowed(ctx)
		if err != nil {
			t.Fatalf("%s: %v", resource, err)
		}
		return ok
	}

	// Without overrides the built-ins decide: alice is not mallory.
	if allowed("user-profile", nil) {
		t.Fatal("built-in request_user must not match 'mallory'")
	}
	if !allowed("user-profile", map[string]interface{}{"request_user": "mallory"}) {
		t.Fatal("PEP-supplied request_user must override the built-in")
	}

	// The built-in group list would match; the supplied one must win.
	if !allowed("team-files", nil) {
		t.Fatal("built-in request_groups must contain staff")
	}
	if allowed("team-files", map[string]interface{}{"request_groups": []interface{}{"visitor"}}) {
		t.Fatal("PEP-supplied request_groups must override the built-in")
	}

	// The built-in request_time is now, which is after 2000; the supplied
	// epoch timestamp is before it.
	if allowed("archive", nil) {
		t.Fatal("built-in request_time must not predate 2000")
	}
	if !allowed("archive", map[string]interface{}{"request_time": float64(0)}) {
		t.Fatal("PEP-supplied request_time must override the built-in")
	}
}

func TestConditionOnMissingAttributeDoesNotMatch(t *testing.T) {
	policies := []*pms.Policy{
		{ID: "conditional-grant", Effect: pms.Grant, Condition: "department == 'finance'",
			Permissions: []*pms.Permission{{Resource: "report", Actions: []string{"read"}}}},
		{ID: "unconditional-grant", Effect: pms.Grant,
			Permissions: []*pms.Permission{{Resource: "memo", Actions: []string{"read"}}}},
		{ID: "conditional-deny", Effect: pms.Deny, Condition: "department == 'finance'",
			Permissions: []*pms.Permission{{Resource: "memo", Actions: []string{"read"}}}},
	}
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: policies}}})

	decide := func(resource string, attributes map[string]interface{}) (bool, adsapi.Reason) {
		t.Helper()
		ctx := featureContext()
		ctx.Resource = resource
		ctx.Attributes = attributes
		ok, reason, err := e.IsAllowed(ctx)
		if err != nil {
			t.Fatalf("%s: %v", resource, err)
		}
		return ok, reason
	}

	// A grant whose condition references a missing attribute does not apply.
	ok, reason := decide("report", nil)
	if ok || reason != adsapi.NO_APPLICABLE_POLICIES {
		t.Fatalf("missing department: allowed=%v reason=%v, want false %v", ok, reason, adsapi.NO_APPLICABLE_POLICIES)
	}
	if ok, _ := decide("report", map[string]interface{}{"department": "finance"}); !ok {
		t.Fatal("department=finance must allow the conditional grant")
	}
	if ok, _ := decide("report", map[string]interface{}{"department": "sales"}); ok {
		t.Fatal("department=sales must not allow the conditional grant")
	}

	// A deny whose condition references a missing attribute does not apply,
	// so the unconditional grant stands.
	if ok, _ := decide("memo", nil); !ok {
		t.Fatal("a deny with a condition on a missing attribute must not apply")
	}
	if ok, _ := decide("memo", map[string]interface{}{"department": "finance"}); ok {
		t.Fatal("the conditional deny must apply when department=finance")
	}
	if ok, _ := decide("memo", map[string]interface{}{"department": "sales"}); !ok {
		t.Fatal("the conditional deny must not apply when department=sales")
	}
}
