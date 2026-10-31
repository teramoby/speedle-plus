// Licensed under the Universal Permissive License (UPL) Version 1.0.
package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
)

func TestWatchMissingFileDoesNotExitAndRecovers(t *testing.T) {
	path, events := watchTempStore(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.Type != pms.STORE_UNAVAILABLE {
			t.Fatalf("missing file event %v", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("missing file was not reported")
	}
	// Regression for the previous log.Fatalf after a two-second rewatch timeout.
	time.Sleep(2200 * time.Millisecond)
	writeFile(t, path, `{"services":[{"name":"recovered"}]}`)
	expectReload(t, events, "recovery after prolonged loss")
	writeFile(t, path, `{}`)
	expectReload(t, events, "update after recovery")
}

func TestStopWatchUnblocksWithoutConsumer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	writeFile(t, path, `{}`)
	s := &Store{FileLocation: path}
	_, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, `{"services":[]}`)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { s.StopWatch(); s.StopWatch(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop blocked with no consumer")
	}
}
