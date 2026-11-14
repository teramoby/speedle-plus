// Licensed under the Universal Permissive License (UPL) Version 1.0.
package adsgrpc

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// protoAuditValue preserves the prior compact text representation for ADS audit
// fields. JSON still uses the message's existing struct tags. This formatter is
// only an audit view; RPC serialization continues to use the protobuf runtime.
type protoAuditValue struct{ message proto.Message }

func (value protoAuditValue) MarshalJSON() ([]byte, error) { return json.Marshal(value.message) }

func (value protoAuditValue) String() string {
	if value.message == nil || !value.message.ProtoReflect().IsValid() {
		return "<nil>"
	}
	var out strings.Builder
	writeAuditMessage(&out, value.message.ProtoReflect())
	return out.String()
}

func auditPermissions(permissions []*pb.AllPermissionResponse_Permission) []protoAuditValue {
	if permissions == nil {
		return nil
	}
	values := make([]protoAuditValue, len(permissions))
	for i, permission := range permissions {
		values[i] = protoAuditValue{permission}
	}
	return values
}

func writeAuditMessage(out *strings.Builder, message protoreflect.Message) {
	fields := message.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if !message.Has(field) {
			continue
		}
		value := message.Get(field)
		if field.IsMap() {
			type entry struct {
				key   protoreflect.MapKey
				value protoreflect.Value
			}
			var entries []entry
			value.Map().Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
				entries = append(entries, entry{key, value})
				return true
			})
			sort.Slice(entries, func(i, j int) bool {
				a, b := entries[i].key, entries[j].key
				switch field.MapKey().Kind() {
				case protoreflect.BoolKind:
					return !a.Bool() && b.Bool()
				case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
					return a.Int() < b.Int()
				case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
					return a.Uint() < b.Uint()
				default:
					return a.String() < b.String()
				}
			})
			for _, item := range entries {
				out.WriteString(string(field.Name()) + ":<")
				writeAuditField(out, field.MapKey(), item.key.Value())
				writeAuditField(out, field.MapValue(), item.value)
				out.WriteString("> ")
			}
		} else if field.IsList() {
			list := value.List()
			for j := 0; j < list.Len(); j++ {
				writeAuditField(out, field, list.Get(j))
			}
		} else {
			writeAuditField(out, field, value)
		}
	}
	// ADS' legacy generated structs did not retain unknown fields in their text
	// view. Do not expose runtime state or unknown wire data through audit output.
}

func writeAuditField(out *strings.Builder, field protoreflect.FieldDescriptor, value protoreflect.Value) {
	out.WriteString(string(field.Name()) + ":")
	switch field.Kind() {
	case protoreflect.MessageKind:
		out.WriteByte('<')
		writeAuditMessage(out, value.Message())
		out.WriteByte('>')
	case protoreflect.StringKind:
		writeAuditString(out, value.String())
	case protoreflect.BytesKind:
		writeAuditString(out, string(value.Bytes()))
	case protoreflect.EnumKind:
		if named := field.Enum().Values().ByNumber(value.Enum()); named != nil {
			out.WriteString(string(named.Name()))
		} else {
			fmt.Fprint(out, value.Enum())
		}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		switch number := value.Float(); {
		case math.IsInf(number, 1):
			out.WriteString("inf")
		case math.IsInf(number, -1):
			out.WriteString("-inf")
		case math.IsNaN(number):
			out.WriteString("nan")
		default:
			fmt.Fprint(out, value.Interface())
		}
	default:
		fmt.Fprint(out, value.Interface())
	}
	out.WriteByte(' ')
}

func writeAuditString(out *strings.Builder, value string) {
	out.WriteByte('"')
	for i := 0; i < len(value); i++ {
		character := value[i]
		switch character {
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteByte(character)
		default:
			if character >= 0x20 && character < 0x7f {
				out.WriteByte(character)
			} else {
				out.WriteByte('\\')
				out.WriteByte('0' + character/64)
				out.WriteByte('0' + character/8%8)
				out.WriteByte('0' + character%8)
			}
		}
	}
	out.WriteByte('"')
}
