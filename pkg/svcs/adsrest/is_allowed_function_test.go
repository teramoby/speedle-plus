//go:build runtime_test

package adsrest

import (
	"fmt"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/testutil"
	"testing"
)

// Exercise the actual SPDL-created function fixtures through the native ADS.
func TestMats_IsAllowed_BuiltinFunctions(t *testing.T) {
	cases := []struct {
		user string
		x, y interface{}
		want bool
	}{
		{"user_cond_sqrt2", 6.0, nil, true}, {"user_cond_sqrt2", 9.0, nil, false},
		{"user_cond_sum1", 1.0, 4.0, true}, {"user_cond_sum1", 6.0, 9.0, false},
		{"user_cond_avg1", 1.0, 4.0, true}, {"user_cond_avg1", 6.0, 9.0, false},
		{"user_cond_min1", 1.0, 4.0, true}, {"user_cond_min1", 6.0, 9.0, false},
		{"user_cond_max2", 4.0, 5.0, true}, {"user_cond_max2", 4.0, 3.0, false},
		{"user_subset_str", []string{"a"}, []string{"a", "b"}, true}, {"user_subset_str", []string{"z"}, []string{"a", "b"}, false},
	}
	data := []testutil.TestCase{}
	for _, c := range cases {
		attrs := []*JsonAttribute{{Name: "s1", Value: c.x}, {Name: "s2", Value: c.y}}
		if c.y == nil {
			attrs = []*JsonAttribute{{Name: "x", Value: c.x}}
		}
		reason := int32(ads.NO_APPLICABLE_POLICIES)
		if c.want {
			reason = int32(ads.GRANT_POLICY_FOUND)
		}
		data = append(data, testutil.TestCase{Name: c.user + "-" + fmt.Sprint(c.want), Executer: testutil.NewRestTestExecuter, Method: testutil.METHOD_IS_ALLOWED, Data: &testutil.RestTestData{URI: URI_IS_ALLOWD, InputBody: &JsonContext{Subject: &JsonSubject{Principals: []*JsonPrincipal{{Type: ads.PRINCIPAL_TYPE_USER, Name: c.user}}}, ServiceName: SERVICE_COND_FUNC, Resource: "res1", Action: "get", Attributes: attrs}, ExpectedStatus: 200, OutputBody: &IsAllowedResponse{}, ExpectedBody: &IsAllowedResponse{Allowed: c.want, Reason: reason}}})
	}
	testutil.RunTestCases(t, &data, nil)
}
