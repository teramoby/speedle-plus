// Licensed under the Universal Permissive License (UPL) Version 1.0.
package store_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/store/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// This fixture captures the real driver's bytes without requiring mongod. It
// implements only handshake, ping and single-batch aggregate replies, following
// https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/.
// Unlike live integration tests this guard always runs in the normal PR gate.
type mongoWireFixture struct {
	listener    net.Listener
	mu          sync.Mutex
	closed      bool
	connections map[net.Conn]bool
	commands    []bson.Raw
	workers     sync.WaitGroup
}

func newMongoWireFixture(t *testing.T) *mongoWireFixture {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &mongoWireFixture{listener: listener, connections: map[net.Conn]bool{}}
	fixture.workers.Add(1)
	go func() {
		defer fixture.workers.Done()
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			fixture.mu.Lock()
			if fixture.closed {
				fixture.mu.Unlock()
				connection.Close()
				return
			}
			fixture.connections[connection] = true
			fixture.workers.Add(1)
			fixture.mu.Unlock()
			go func() {
				defer fixture.workers.Done()
				defer connection.Close()
				defer func() { fixture.mu.Lock(); delete(fixture.connections, connection); fixture.mu.Unlock() }()
				fixture.serve(connection)
			}()
		}
	}()
	t.Cleanup(func() {
		fixture.mu.Lock()
		fixture.closed = true
		fixture.listener.Close()
		for connection := range fixture.connections {
			connection.Close()
		}
		fixture.mu.Unlock()
		fixture.workers.Wait()
	})
	return fixture
}

func (fixture *mongoWireFixture) serve(connection net.Conn) {
	for {
		header := make([]byte, 16)
		if _, err := io.ReadFull(connection, header); err != nil {
			return
		}
		size := int(binary.LittleEndian.Uint32(header[:4]))
		if size < 16 || size > 16<<20 {
			return
		}
		body := make([]byte, size-16)
		if _, err := io.ReadFull(connection, body); err != nil {
			return
		}
		opcode := binary.LittleEndian.Uint32(header[12:16])
		offset := 0
		switch opcode {
		case 2004: // The driver's initial legacy OP_QUERY hello.
			if len(body) < 4 {
				return
			}
			end := bytes.IndexByte(body[4:], 0)
			if end < 0 {
				return
			}
			offset = 4 + end + 1 + 8
		case 2013: // OP_MSG, one kind-0 BSON body section.
			if len(body) < 5 || body[4] != 0 {
				return
			}
			offset = 5
		default:
			return
		}
		if offset+4 > len(body) {
			return
		}
		length := int(binary.LittleEndian.Uint32(body[offset : offset+4]))
		if length < 5 || offset+length > len(body) {
			return
		}
		command := bson.Raw(body[offset : offset+length])
		elements, err := command.Elements()
		if err != nil || len(elements) == 0 {
			return
		}
		name := strings.ToLower(elements[0].Key())
		response := bson.D{{Key: "ok", Value: 1}}
		switch name {
		case "hello", "ismaster":
			response = append(response,
				bson.E{Key: "isWritablePrimary", Value: true}, bson.E{Key: "ismaster", Value: true},
				bson.E{Key: "helloOk", Value: true}, bson.E{Key: "minWireVersion", Value: 0}, bson.E{Key: "maxWireVersion", Value: 25},
				bson.E{Key: "logicalSessionTimeoutMinutes", Value: 30}, bson.E{Key: "maxBsonObjectSize", Value: 16777216},
				bson.E{Key: "maxMessageSizeBytes", Value: 48000000}, bson.E{Key: "maxWriteBatchSize", Value: 100000})
		case "aggregate":
			fixture.mu.Lock()
			fixture.commands = append(fixture.commands, append(bson.Raw(nil), command...))
			fixture.mu.Unlock()
			service := bson.D{{Key: "_id", Value: "app"}, {Key: "policyCount", Value: 1}, {Key: "rolepolicyCount", Value: 1}, {Key: "policycount", Value: 1},
				{Key: "policies", Value: bson.A{bson.D{{Key: "_id", Value: "p"}, {Key: "name", Value: "read"}}}},
				{Key: "rolepolicies", Value: bson.A{bson.D{{Key: "_id", Value: "r"}, {Key: "name", Value: "reader"}}}}}
			response = append(response, bson.E{Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "ns", Value: "speedle_test_wire.services"}, {Key: "firstBatch", Value: bson.A{service}}}})
		}
		// A fire-and-forget OP_MSG does not expect a response.
		if opcode == 2013 && binary.LittleEndian.Uint32(body[:4])&2 != 0 {
			continue
		}
		document, err := bson.Marshal(response)
		if err != nil {
			return
		}
		var reply []byte
		responseOpcode := uint32(2013)
		if opcode == 2004 {
			responseOpcode = 1
			reply = make([]byte, 20)
			binary.LittleEndian.PutUint32(reply[16:20], 1)
		} else {
			reply = make([]byte, 5)
		}
		reply = append(reply, document...)
		frame := make([]byte, 16)
		binary.LittleEndian.PutUint32(frame[:4], uint32(16+len(reply)))
		binary.LittleEndian.PutUint32(frame[4:8], 1)
		binary.LittleEndian.PutUint32(frame[8:12], binary.LittleEndian.Uint32(header[4:8]))
		binary.LittleEndian.PutUint32(frame[12:16], responseOpcode)
		if _, err := io.Copy(connection, bytes.NewReader(append(frame, reply...))); err != nil {
			return
		}
	}
}

func TestMongoAggregationWireBudgetsWithoutMongoDB(t *testing.T) {
	fixture := newMongoWireFixture(t)
	manager, err := (mongodb.MongoStoreBuilder{}).NewStore(map[string]interface{}{
		mongodb.MongoURIKey:          fmt.Sprintf("mongodb://%s/?directConnection=true&serverSelectionTimeoutMS=1000&connectTimeoutMS=1000", fixture.listener.Addr()),
		mongodb.MongoDatabaseNameKey: "speedle_test_wire",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.(*mongodb.Store).Close(); err != nil {
			t.Error(err)
		}
	})
	for _, test := range []struct {
		name  string
		limit int64
		run   func() error
	}{
		{"service names", 2000, func() error { _, err := manager.GetServiceNames(); return err }},
		{"policy list", 2000, func() error { _, err := manager.ListAllPolicies("app", ""); return err }},
		{"role policy list", 2000, func() error { _, err := manager.ListAllRolePolicies("app", ""); return err }},
		{"combined counts", 5000, func() error { _, err := manager.GetPolicyAndRolePolicyCounts(); return err }},
		{"policy count", 5000, func() error { _, err := manager.GetPolicyCount("app"); return err }},
		{"role policy count", 5000, func() error { _, err := manager.GetRolePolicyCount("app"); return err }},
		{"policy", 5000, func() error { _, err := manager.GetPolicy("app", "p"); return err }},
		{"role policy", 5000, func() error { _, err := manager.GetRolePolicy("app", "r"); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture.mu.Lock()
			fixture.commands = nil
			fixture.mu.Unlock()
			if err := test.run(); err != nil {
				t.Fatal(err)
			}
			fixture.mu.Lock()
			commands := append([]bson.Raw(nil), fixture.commands...)
			fixture.mu.Unlock()
			if len(commands) == 0 {
				t.Fatal("no aggregate command captured on the wire")
			}
			for _, command := range commands {
				fields, err := command.Elements()
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, field := range fields {
					if field.Key() == "maxTimeMS" {
						count++
						value, ok := field.Value().AsInt64OK()
						if !ok || value != test.limit {
							t.Fatalf("maxTimeMS=%v, want %d", field.Value(), test.limit)
						}
					}
				}
				if count != 1 {
					t.Fatalf("wire command contains %d maxTimeMS fields, want exactly one: %s", count, command)
				}
			}
		})
	}
}
