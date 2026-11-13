// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSyncAcceptsLargeProtocolSnapshotAndCache(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	spdl := strings.Repeat("<", 128) + strings.Repeat("s", 8192-128)
	policies := make([]Policy, 5000)
	rolePolicies := make([]Policy, 5000)
	for index := 0; index < 5000; index++ {
		policies[index] = Policy{
			ID: fmt.Sprintf("policy-%04d", index), Name: "read-books", SPDL: spdl,
		}
		rolePolicies[index] = Policy{
			ID: fmt.Sprintf("role-%04d", index), Name: "reader", SPDL: spdl,
		}
	}
	large := Snapshot{Name: "books", Policies: policies, RolePolicies: rolePolicies}
	encoded, err := json.Marshal(large)
	if err != nil {
		t.Fatalf("encode large snapshot: %v", err)
	}
	if len(encoded) <= 6<<20 {
		t.Fatalf("large snapshot is only %d bytes; test must exceed the old transport cap", len(encoded))
	}
	fixture.source.configure(func(source *fixtureSource) {
		current := source.services[service.ID]
		current.State = large
		current.StateHash = StateHash(large)
		current.CommitHash = current.StateHash
	})

	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, func(config *Config) {
		config.RequestTimeout = 10 * time.Second
		config.SnapshotTimeout = 10 * time.Second
	})
	state := syncFixtureService(t, syncer, "books")
	if len(state.Snapshot.Policies) != 5000 || len(state.Snapshot.RolePolicies) != 5000 {
		t.Fatalf("large snapshot policy counts = %d/%d", len(state.Snapshot.Policies), len(state.Snapshot.RolePolicies))
	}
	cache, err := NewCache(directory, fixture.keys)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	info, err := os.Stat(cache.path(service.ID))
	if err != nil {
		t.Fatalf("stat large cache: %v", err)
	}
	if info.Size() <= 8<<20 {
		t.Fatalf("large cache is only %d bytes; test must exceed the old cache cap", info.Size())
	}

	restarted := fixture.newSyncer(directory, nil)
	reloaded := syncFixtureService(t, restarted, "books")
	if reloaded.StateHash != state.StateHash {
		t.Error("large cache did not survive restart")
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Errorf("snapshots after large-cache restart = %d, want 1", count)
	}
}

func TestChangesAcceptsLargeProtocolPage(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	client := fixture.newClient(func(config *Config) {
		config.RequestTimeout = 10 * time.Second
	})
	snapshot, _, err := client.Snapshot(context.Background(), service.ID, "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	spdl := strings.Repeat("s", 8192)
	for commitIndex := 0; commitIndex < 2; commitIndex++ {
		changes := make([]Change, 201)
		for index := range changes {
			changes[index] = Change{
				Op:   "upsert",
				Kind: "authz",
				ID:   fmt.Sprintf("policy-%d-%03d", commitIndex, index),
				Name: stringPointer("read-books"),
				SPDL: &spdl,
			}
		}
		service.publish(t, fixture.signer, changes...)
	}

	page, err := client.Changes(context.Background(), service.ID, snapshot.SourceID, snapshot.Epoch,
		snapshot.Revision, snapshot.CommitHash, changePageLimit)
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(page.Commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(page.Commits))
	}
	for _, commit := range page.Commits {
		if len(commit.Changes) != 201 {
			t.Errorf("commit %d changes = %d, want %d", commit.Revision, len(commit.Changes), 201)
		}
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("encode large page: %v", err)
	}
	if len(encoded) <= 1<<20 {
		t.Errorf("large changes page is only %d bytes; test must exceed the old transport cap", len(encoded))
	}
}

func TestDiscoveryAndChannelAccept101Services(t *testing.T) {
	services := make([]*fixtureService, 101)
	for index := range services {
		services[index] = newFixtureService(fmt.Sprintf("%026d", index+1), fmt.Sprintf("service-%d", index))
	}
	fixture := newFixture(t, services...)
	client := fixture.newClient(nil)
	discovered, err := client.Services(context.Background())
	if err != nil || len(discovered.Services) != 101 {
		t.Fatalf("101-service discovery: %v, %v", discovered, err)
	}
	machine, _ := newTestMachine(t)
	mustAck(t, machine, ackWith(discovered.Services...))
	channel := &Channel{machine: machine, fallback: func() FallbackState {
		ids := make([]string, 101)
		for i := range ids {
			ids[i] = services[i].ID
		}
		return FallbackState{Services: ids}
	}}
	if got := len(channel.localServices()); got != 101 {
		t.Fatalf("announced services = %d", got)
	}
	if got := len(channel.heartbeatMessage(TransportLongPoll).Fallback.Services); got != 101 {
		t.Fatalf("fallback services = %d", got)
	}
	ack := validChannelAck()
	ack.Services = make([]channelAckService, len(discovered.Services))
	for i, s := range discovered.Services {
		ack.Services[i] = channelAckService{ID: s.ID, Name: s.Name, Head: s.Revision, OriginHead: s.Revision, Epoch: s.Epoch, HeadStateHash: s.HeadStateHash}
	}
	data, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeServerMessage(data); err != nil {
		t.Fatalf("101-service ack rejected: %v", err)
	}
}

func TestUnboundedPolicyFields(t *testing.T) {
	long := strings.Repeat("n", 129)
	spdl := strings.Repeat("s", 8192)
	if err := validatePolicies("policy", []Policy{{ID: long, Name: long, SPDL: spdl}}); err != nil {
		t.Fatal(err)
	}
	change := Change{Op: "upsert", Kind: "authz", ID: long, Name: &long, SPDL: &spdl}
	if err := change.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFetchesBeyond1000PagesUntilHead(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	fixture.source.configure(func(source *fixtureSource) { source.pageSize = 1 })
	syncer := fixture.newSyncer("", nil)
	syncFixtureService(t, syncer, "books")
	for i := 0; i < 1001; i++ {
		service.publish(t, fixture.signer, Change{Op: "delete", Kind: "authz", ID: "missing"})
	}
	state := syncFixtureService(t, syncer, "books")
	if state.Revision != service.Revision {
		t.Fatalf("revision %d, want head %d", state.Revision, service.Revision)
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Fatalf("pagination fell back to snapshot: %d snapshots", count)
	}
	if count := fixture.requestCount(testServiceID1 + "/changes"); count != 1001 {
		t.Fatalf("changes pages = %d, want 1001", count)
	}
}
