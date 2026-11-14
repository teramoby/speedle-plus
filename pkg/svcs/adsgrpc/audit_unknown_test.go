package adsgrpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"google.golang.org/protobuf/proto"
)

// The exact pre-migration generated structs had no XXX_unrecognized field.
// This fixture was measured by unmarshalling field 1000 with the original
// f2591e6b ContextRequest and github.com/golang/protobuf v1.5.4: it was dropped
// from both the old round trip and old text, unlike modern generated messages.
func TestLegacyAuditUnknownFieldBoundary(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy-unknown-audit.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Input      string `json:"input"`
		OldWire    string `json:"old_wire"`
		OldUnknown string `json:"old_unknown"`
		OldText    string `json:"old_text"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	wire, err := hex.DecodeString(fixture.Input)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.OldUnknown != "" || fixture.OldWire == fixture.Input {
		t.Fatal("fixture must retain the measured legacy drop behavior")
	}
	var request pb.ContextRequest
	if err := proto.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	unknown := append([]byte(nil), request.ProtoReflect().GetUnknown()...)
	if len(unknown) == 0 {
		t.Fatal("modern wire decoding must retain unknown fields")
	}
	if got := (protoAuditValue{&request}).String(); got != fixture.OldText {
		t.Fatalf("audit view=%q, want legacy %q", got, fixture.OldText)
	}
	if !bytes.Equal(request.ProtoReflect().GetUnknown(), unknown) {
		t.Fatal("audit view changed unknown wire data")
	}
	roundTrip, err := (proto.MarshalOptions{Deterministic: true}).Marshal(&request)
	if err != nil || !bytes.Equal(roundTrip, wire) {
		t.Fatalf("modern wire round trip=%x, want %x, err=%v", roundTrip, wire, err)
	}
}
