// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

// TestAckFrameFitsTheLargestServiceSet proves one channel ack can carry every
// service the mirror admits. The ack lists the whole service set, so its frame
// bound must cover the tombstone bound rather than an arbitrary smaller value:
// a relay advertising more services than one frame can encode could not use
// the WebSocket transport at all.
func TestAckFrameFitsTheLargestServiceSet(t *testing.T) {
	mirror := newTestMirror(t, nil)
	epoch := fmt.Sprintf("01HF7YATE%017d", 0)
	for index := 0; index < maxTombstones; index++ {
		// Every field is at its protocol maximum: a 26-character entity id,
		// a 64-character name, a 16-digit revision, and a 26-character epoch.
		serviceID := fmt.Sprintf("01HF7YAT%018d", index)
		snapshot := adsync.Snapshot{
			Name:         fmt.Sprintf("s%063d", index),
			Policies:     []adsync.Policy{},
			RolePolicies: []adsync.Policy{},
		}
		stateHash := adsync.StateHash(snapshot)
		if err := mirror.ApplySnapshot(&adsync.SnapshotResponse{
			ServiceID:  serviceID,
			SourceID:   serviceID,
			Epoch:      epoch,
			Revision:   9007199254740991,
			StateHash:  stateHash,
			CommitHash: stateHash,
			Snapshot:   snapshot,
		}); err != nil {
			t.Fatalf("apply snapshot %d: %v", index, err)
		}
	}
	if got := len(mirror.Services()); got != maxTombstones {
		t.Fatalf("advertised services = %d, want %d", got, maxTombstones)
	}

	server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()
	encoded, err := json.Marshal(server.ack())
	if err != nil {
		t.Fatalf("encode ack: %v", err)
	}
	if len(encoded) > (1 << 20) {
		t.Fatalf("ack of %d bytes exceeds the %d-byte channel frame bound", len(encoded), (1 << 20))
	}
	if len(encoded) <= 64<<10 {
		t.Fatalf("ack of %d bytes does not exercise the previous 64 KiB bound", len(encoded))
	}
}
