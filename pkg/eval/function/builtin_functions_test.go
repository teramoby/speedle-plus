package function

import "testing"

func TestNumericBuiltIns(t *testing.T) {
	testCases := []struct {
		name string
		fn   func(...interface{}) (interface{}, error)
		args []interface{}
		want float64
	}{
		{"Max uses every argument", Max, []interface{}{1.0, 2.0, 3.0}, 3},
		{"Max of one", Max, []interface{}{4.0}, 4},
		{"Min uses every argument", Min, []interface{}{3.0, 2.0, 1.0}, 1},
		{"Sum", Sum, []interface{}{1.0, 2.0, 3.0}, 6},
		{"Avg", Avg, []interface{}{1.0, 2.0, 3.0}, 2},
		{"Sqrt", Sqrt, []interface{}{16.0}, 4},
	}
	for _, tc := range testCases {
		got, err := tc.fn(tc.args...)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}

func TestBuiltInsRejectBadArguments(t *testing.T) {
	testCases := []struct {
		name string
		fn   func(...interface{}) (interface{}, error)
		args []interface{}
	}{
		{"Max string first", Max, []interface{}{"x", 1.0}},
		{"Max string last", Max, []interface{}{1.0, "x"}},
		{"Max nil", Max, []interface{}{nil}},
		{"Max no arguments", Max, nil},
		{"Min string first", Min, []interface{}{"x", 1.0}},
		{"Sum nil", Sum, []interface{}{1.0, nil}},
		{"Avg string", Avg, []interface{}{1.0, "x"}},
		{"Sqrt nil", Sqrt, []interface{}{nil}},
		{"Sqrt two arguments", Sqrt, []interface{}{1.0, 2.0}},
		{"IsSubSet nil", IsSubSet, []interface{}{[]interface{}{"a"}, nil}},
	}
	for _, tc := range testCases {
		if got, err := tc.fn(tc.args...); err == nil {
			t.Errorf("%s: got %v, want an error", tc.name, got)
		}
	}
}
