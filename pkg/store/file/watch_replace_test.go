//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
)

// watchTempStore watches a policy file of its own and returns its path and the
// change channel.
func watchTempStore(t *testing.T) (string, pms.StorageChangeChannel) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ps.json")
	writeFile(t, path, `{"services":[]}`)
	s := &Store{FileLocation: path}
	ch, err := s.Watch()
	if err != nil {
		t.Fatal("fail to watch:", err)
	}
	t.Cleanup(s.StopWatch)
	return path, ch
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// expectReload waits for a reload event, then drains the events the same
// change may still produce.
func expectReload(t *testing.T, ch pms.StorageChangeChannel, after string) {
	t.Helper()
	for {
		select {
		case e := <-ch:
			if e.Type == pms.STORE_UNAVAILABLE {
				continue
			}
			if e.Type != pms.FULL_RELOAD {
				t.Fatalf("after %s: event type %d, want %d", after, e.Type, pms.FULL_RELOAD)
			}
			goto received
		case <-time.After(5 * time.Second):
			t.Fatalf("after %s: no reload event", after)
		}
	}
received:
	for {
		select {
		case <-ch:
		case <-time.After(200 * time.Millisecond):
			return
		}
	}
}

func TestWatchSurvivesAtomicReplace(t *testing.T) {
	path, ch := watchTempStore(t)
	for i := 0; i < 2; i++ {
		tmp := path + ".tmp"
		writeFile(t, tmp, `{"services":[{"name":"app1"}]}`)
		if err := os.Rename(tmp, path); err != nil {
			t.Fatal(err)
		}
		expectReload(t, ch, "atomic replace")
		// The new file is watched: an in-place write is seen too.
		writeFile(t, path, `{"services":[]}`)
		expectReload(t, ch, "write after atomic replace")
	}
}

func TestWatchSurvivesRemoveAndRecreate(t *testing.T) {
	path, ch := watchTempStore(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	writeFile(t, path, `{"services":[{"name":"app1"}]}`)
	expectReload(t, ch, "remove and recreate")
	writeFile(t, path, `{"services":[]}`)
	expectReload(t, ch, "write after recreate")
}
