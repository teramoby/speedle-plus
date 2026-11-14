package mongodb

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func legacySnapshot(t *testing.T) *pms.PolicyStore {
	t.Helper()
	raw, err := os.ReadFile("testdata/legacy-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot pms.PolicyStore
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	return &snapshot
}

func legacyBSON(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("testdata/legacy-bson.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]string
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

func TestLegacyBSONFormat(t *testing.T) {
	snapshot := legacySnapshot(t)
	golden := legacyBSON(t)
	for name, value := range map[string]any{"service": snapshot.Services[0], "function": snapshot.Functions[0]} {
		t.Run(name, func(t *testing.T) {
			data, err := bson.Marshal(value)
			if err != nil || hex.EncodeToString(data) != golden[name] {
				t.Fatalf("stored BSON changed: %x, err=%v", data, err)
			}
			decoded := reflect.New(reflect.TypeOf(value).Elem()).Interface()
			if err := bson.Unmarshal(data, decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(value, decoded) {
				t.Fatalf("BSON round trip changed fields: %+v", decoded)
			}
		})
	}
}

func TestLegacyDocumentsAndSnapshotRoundTrip(t *testing.T) {
	s := isolatedStore(t)
	expected := legacySnapshot(t)
	// Insert the exact v1 BSON bytes as existing stored documents, then read with v2.
	for name, encoded := range legacyBSON(t) {
		raw, err := hex.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		collection := map[string]string{"service": "services", "function": "functions"}[name]
		if _, err := s.client.Database(s.Database).Collection(collection).InsertOne(context.Background(), bson.Raw(raw)); err != nil {
			t.Fatal(err)
		}
	}
	check := func() {
		t.Helper()
		actual, err := s.ReadPolicyStore()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(expected, actual) {
			t.Fatalf("snapshot changed: got %+v, want %+v", actual, expected)
		}
	}
	check()
	if !supportsTransactions(t, s) {
		t.Skip("snapshot replacement requires a replica set")
	}
	if err := s.WritePolicyStore(expected); err != nil {
		t.Fatal(err)
	}
	check()
	// Stable policy/role IDs, conditions, nested principals, metadata and int64 TTL survive.
	service, err := s.GetService("books")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(service, expected.Services[0]) {
		t.Fatalf("service fields changed: %+v", service)
	}
	function, err := s.GetFunction("allowed")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(function, expected.Functions[0]) {
		t.Fatalf("function fields changed: %+v", function)
	}
}
