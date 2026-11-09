// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	speedleerrors "github.com/teramoby/speedle-plus/pkg/errors"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

func TestCheckHealthFollowsLiveness(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	clock := newTestClock()
	store := fixture.open(func(options *Options) {
		options.Clock = clock
		options.HeartbeatInterval = 15 * time.Second
		options.StaleAfter = 20 * time.Second
		options.StopAfter = 60 * time.Second
	})
	ctx := context.Background()
	if err := store.CheckHealth(ctx); err != nil {
		t.Fatalf("a just synchronized store is not healthy: %v", err)
	}

	clock.Advance(25 * time.Second)
	status, known := store.Status("books")
	if !known || status.Instance.Liveness != adsync.LivenessStale {
		t.Fatalf("liveness after 25s: %+v, known %v", status.Instance, known)
	}
	if err := store.CheckHealth(ctx); err != nil {
		t.Fatalf("a stale instance must not fail the store health check: %v", err)
	}

	clock.Advance(36 * time.Second) // 61s since the last confirmed ack
	if status, _ := store.Status("books"); status.Instance.Liveness != adsync.LivenessStopped {
		t.Fatalf("liveness after 61s: %+v", status.Instance)
	}
	err := store.CheckHealth(ctx)
	if speedleerrors.Code(err) != speedleerrors.StoreError || !strings.Contains(err.Error(), "heartbeat_lost") {
		t.Fatalf("stopped store health: %v", err)
	}

	// A sync confirms a fresh ack and re-verifies the local copy, so the
	// instance recovers.
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckHealth(ctx); err != nil {
		t.Fatalf("the instance did not recover: %v", err)
	}
}

func TestCheckHealthHonoursCancellation(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.CheckHealth(ctx); err != context.Canceled {
		t.Fatalf("CheckHealth = %v, want context.Canceled", err)
	}
}

func TestUnreachableSourceStartsEmptyAndFailsClosed(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	clock := newTestClock()
	fixture.source.mutate(func(source *testSource) { source.failAll = true })
	store := fixture.open(func(options *Options) {
		options.Clock = clock
	})

	if policyStore := mustRead(t, store); len(policyStore.Services) != 0 {
		t.Fatalf("an unreachable source still produced services: %+v", policyStore.Services)
	}
	if _, known := store.Status("books"); known {
		t.Fatal("an unreachable source still produced a tracked service")
	}
	ctx := context.Background()
	if err := store.CheckHealth(ctx); err != nil {
		t.Fatalf("the startup grace window must stay healthy: %v", err)
	}

	clock.Advance(2*time.Minute + time.Second)
	if err := store.CheckHealth(ctx); speedleerrors.Code(err) != speedleerrors.StoreError {
		t.Fatalf("an instance without a confirmed ack must fail closed: %v", err)
	}

	fixture.source.mutate(func(source *testSource) { source.failAll = false })
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	if policyStore := mustRead(t, store); len(policyStore.Services) != 1 {
		t.Fatalf("services after recovery: %+v", policyStore.Services)
	}
	if err := store.CheckHealth(ctx); err != nil {
		t.Fatalf("the instance did not recover: %v", err)
	}
}

func TestRevokedCredentialStopsTheInstance(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)

	fixture.source.mutate(func(source *testSource) { source.credential = "rotated" })
	if err := store.Sync(nil); err == nil {
		t.Fatal("a rejected credential was accepted")
	}
	err := store.CheckHealth(context.Background())
	if speedleerrors.Code(err) != speedleerrors.StoreError || !strings.Contains(err.Error(), string(adsync.ReasonRevoked)) {
		t.Fatalf("health after revocation: %v", err)
	}

	// Revocation is terminal: restoring the credential in the source does not
	// revive a running process.
	fixture.source.mutate(func(source *testSource) { source.credential = testCredential })
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckHealth(context.Background()); err == nil {
		t.Fatal("a terminal stop was cleared by a later successful sync")
	}
}
