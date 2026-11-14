// Licensed under the Universal Permissive License (UPL) Version 1.0.
// Package protocompat verifies the binary contract captured before regenerating bindings.
package protocompat

import (
	"encoding/hex"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"os"
	"reflect"
	"strings"
	"testing"
)

// CheckLegacy checks every generated message in both directions, including unknown fields.
func CheckLegacy(t *testing.T, fixture string, messages []proto.Message) {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]string
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden) != len(messages) {
		t.Fatal("legacy fixture message count changed")
	}
	for _, m := range messages {
		t.Run(reflect.TypeOf(m).Elem().Name(), func(t *testing.T) {
			populate(reflect.ValueOf(m).Elem())
			expected, ok := golden[reflect.TypeOf(m).Elem().Name()]
			if !ok {
				t.Fatal("missing legacy message")
			}
			data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(m)
			if err != nil || hex.EncodeToString(data) != expected {
				t.Fatalf("wire changed: %x, want %s, err=%v", data, expected, err)
			}
			// Field 1000 is unknown to every current message. Forwarders must retain it.
			data = append(data, 0xc0, 0x3e, 0x01)
			decoded := m.ProtoReflect().Type().New().Interface()
			if err := proto.Unmarshal(data, decoded); err != nil {
				t.Fatal(err)
			}
			again, err := (proto.MarshalOptions{Deterministic: true}).Marshal(decoded)
			if err != nil || !reflect.DeepEqual(data, again) {
				t.Fatalf("legacy decode/unknown field round trip: %x, err=%v", again, err)
			}
		})
	}
}
func populate(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		populate(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() && !strings.HasPrefix(v.Type().Field(i).Name, "XXX_") {
				populate(v.Field(i))
			}
		}
	case reflect.String:
		v.SetString("example")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 2, 2))
		for i := 0; i < 2; i++ {
			populate(v.Index(i))
		}
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		k := reflect.New(v.Type().Key()).Elem()
		val := reflect.New(v.Type().Elem()).Elem()
		populate(k)
		populate(val)
		v.SetMapIndex(k, val)
	}
}
