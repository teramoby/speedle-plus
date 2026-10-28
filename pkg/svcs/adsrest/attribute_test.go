// Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.
package adsrest

import "testing"

func TestConvValueAcceptsMatchingTypes(t *testing.T) {
	cases := []struct {
		dataType string
		value    interface{}
	}{
		{"string", "finance"},
		{"bool", true},
		{"numeric", float64(42)},
		{"datetime", "2026-10-28T15:04:05Z"},
		{"string", []interface{}{"a", "b"}},
		{"bool", []interface{}{true, false}},
	}
	for _, c := range cases {
		if _, err := ConvValue(c.dataType, c.value); err != nil {
			t.Errorf("ConvValue(%q, %#v) returned error: %v", c.dataType, c.value, err)
		}
	}
}

func TestConvValueRejectsMismatchedTypes(t *testing.T) {
	cases := []struct {
		dataType string
		value    interface{}
	}{
		{"string", float64(1)},
		{"string", true},
		{"bool", "true"},
		{"bool", float64(0)},
		{"numeric", "42"},
		{"datetime", float64(1)},
		{"string", []interface{}{"a", float64(1)}},
	}
	for _, c := range cases {
		if _, err := ConvValue(c.dataType, c.value); err == nil {
			t.Errorf("ConvValue(%q, %#v) accepted a mismatched value", c.dataType, c.value)
		}
	}
}

func TestDumpRequestAttributesKeepsStringAndBoolValues(t *testing.T) {
	attrs, err := DumpRequestAttributes([]*JsonAttribute{
		{Name: "department", Type: "string", Value: "finance"},
		{Name: "mfa", Type: "bool", Value: true},
	})
	if err != nil {
		t.Fatalf("DumpRequestAttributes returned error: %v", err)
	}
	if attrs["department"] != "finance" || attrs["mfa"] != true {
		t.Fatalf("unexpected attributes: %#v", attrs)
	}
}
