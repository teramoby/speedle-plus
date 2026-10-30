package function

import (
	"fmt"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"math"
	"testing"
)

func TestNumericArgumentMatrix(t *testing.T) {
	for name, fn := range map[string]func(...interface{}) (interface{}, error){"Sqrt": Sqrt, "Max": Max, "Min": Min, "Sum": Sum, "Avg": Avg} {
		for _, bad := range []interface{}{nil, "1", true, []float64{1}, math.NaN(), math.Inf(1), math.Inf(-1)} {
			for position := 0; position < 2; position++ {
				t.Run(fmt.Sprintf("%s/%T/%v/%d", name, bad, bad, position), func(t *testing.T) {
					args := []interface{}{1.0, 2.0}
					args[position] = bad
					if name == "Sqrt" {
						args = []interface{}{bad}
					}
					value, err := fn(args...)
					if value != nil || errors.Code(err) != errors.BuiltInFuncError {
						t.Fatalf("result=%v error=%v", value, err)
					}
				})
			}
		}
	}
	for name, fn := range map[string]func(...interface{}) (interface{}, error){"Sqrt": Sqrt, "Max": Max, "Min": Min} {
		if _, err := fn(); errors.Code(err) != errors.BuiltInFuncError {
			t.Errorf("%s empty: %v", name, err)
		}
	}
	for _, fn := range []func(...interface{}) (interface{}, error){Sum, Avg} {
		if value, err := fn(); value != 0.0 || err != nil {
			t.Fatalf("legacy empty result %v %v", value, err)
		}
	}
	if _, err := Sqrt(-1.0); errors.Code(err) != errors.BuiltInFuncError {
		t.Fatal(err)
	}
	if _, err := Sum(math.MaxFloat64, math.MaxFloat64); errors.Code(err) != errors.BuiltInFuncError {
		t.Fatal(err)
	}
	if value, err := Avg(math.MaxFloat64, math.MaxFloat64); value != math.MaxFloat64 || err != nil {
		t.Fatalf("finite mean %v %v", value, err)
	}
}
func TestSubsetContract(t *testing.T) {
	for _, c := range []struct {
		name string
		a, b interface{}
		want bool
		bad  bool
	}{
		{"strings", []string{"b", "a"}, []string{"a", "b", "c"}, true, false},
		{"duplicates", []string{"a", "a", "b"}, []string{"a", "b"}, true, false},
		{"numeric", []int{1, 2}, []float64{2, 1, 3}, true, false},
		{"large-integer-precision", []uint64{math.MaxUint64}, []uint64{math.MaxUint64 - 1}, false, false},
		{"bool", []bool{true}, []bool{false, true}, true, false},
		{"absent", []string{"z"}, []string{"a"}, false, false},
		{"empty-left", []string{}, []string{"a"}, false, false},
		{"empty-right", []string{"a"}, []string{}, false, false},
		{"nested", []interface{}{[]string{"a"}}, []interface{}{[]string{"a"}}, false, true},
		{"map", []interface{}{map[string]int{"a": 1}}, []interface{}{map[string]int{"a": 1}}, false, true},
		{"mixed", []interface{}{"a", true}, []interface{}{"a", true}, false, true},
		{"nil-element", []interface{}{nil}, []string{"a"}, false, true},
		{"nil-right", []string{"a"}, nil, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, e := IsSubSet(c.a, c.b)
			if c.bad {
				if v != nil || errors.Code(e) != errors.BuiltInFuncError {
					t.Fatalf("%v %v", v, e)
				}
			} else if e != nil || v != c.want {
				t.Fatalf("%v %v want %v", v, e, c.want)
			}
		})
	}
	if v, e := IsSubSet("a", "b", []string{"a", "b"}); v != true || e != nil {
		t.Fatalf("flattened %v %v", v, e)
	}
}
