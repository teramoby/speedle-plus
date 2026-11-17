// Licensed under the Universal Permissive License (UPL) Version 1.0.
package pms

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestIsFunctionType(t *testing.T) {
	valid := []string{"string", "numeric", "bool", "datetime",
		"string[]", "numeric[]", "bool[]", "datetime[]"}
	for _, name := range valid {
		if !IsFunctionType(name) {
			t.Errorf("%q is not accepted", name)
		}
	}
	invalid := []string{"", "String", "integer", "number", "object", "string[][]",
		"[]", "string[", "string ]", " string"}
	for _, name := range invalid {
		if IsFunctionType(name) {
			t.Errorf("%q is accepted", name)
		}
	}
}

func TestValidateTypeDeclarations(t *testing.T) {
	cases := []struct {
		function *Function
		valid    bool
	}{
		{&Function{}, true},
		{&Function{Params: []string{}, Result: ""}, true},
		{&Function{Params: []string{"numeric", "string[]"}, Result: "bool"}, true},
		{&Function{Params: []string{"numeric", "integer"}}, false},
		{&Function{Result: "object"}, false},
		{&Function{Result: "string[][]"}, false},
	}
	for _, c := range cases {
		err := c.function.ValidateTypeDeclarations()
		if (err == nil) != c.valid {
			t.Errorf("%+v: error %v, want valid=%v", c.function, err, c.valid)
		}
	}
}

// TestFunctionTypeDeclarationsJSON pins the management API field names and
// keeps the declarations out of a definition that does not use them.
func TestFunctionTypeDeclarationsJSON(t *testing.T) {
	plain, err := json.Marshal(&Function{Name: "f", FuncURL: "https://f"})
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != `{"name":"f","funcURL":"https://f"}` {
		t.Errorf("undeclared function JSON = %s", plain)
	}

	data, err := json.Marshal(&Function{Name: "f", FuncURL: "https://f", Params: []string{"numeric", "string[]"}, Result: "bool"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded Function
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Params, []string{"numeric", "string[]"}) || decoded.Result != "bool" {
		t.Errorf("round trip = %+v (%s)", decoded, data)
	}
}
