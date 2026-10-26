package file

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/store"
)

func watchTestStore(t *testing.T, filename string) (*Store, pms.StorageChangeChannel) {
	t.Helper()
	manager, err := store.NewStore("file", map[string]interface{}{"FileLocation": filename})
	if err != nil {
		t.Fatal(err)
	}
	s := manager.(*Store)
	changes, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.StopWatch()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case _, ok := <-changes:
				if !ok {
					return
				}
			case <-timer.C:
				t.Error("file watcher did not stop")
				return
			}
		}
	})
	return s, changes
}

func awaitServices(t *testing.T, s *Store, changes pms.StorageChangeChannel, names ...string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-changes:
			if !ok {
				t.Fatal("watcher closed before reload")
			}
			if event.Type != pms.FULL_RELOAD {
				t.Fatalf("unexpected event %v", event.Type)
			}
			got, err := s.GetServiceNames()
			if err == nil && slices.Equal(got, names) {
				return
			}
		case <-timer.C:
			t.Fatalf("no reload with services %v", names)
		}
	}
}

func TestWatch(t *testing.T) {
	s, changes := watchTestStore(t, filepath.Join(t.TempDir(), "policies.json"))
	for _, name := range []string{"first", "second"} {
		if err := s.CreateService(&pms.Service{Name: name, Type: pms.TypeApplication}); err != nil {
			t.Fatal(err)
		}
		awaitServices(t, s, changes, name)
		if err := s.DeleteService(name); err != nil {
			t.Fatal(err)
		}
		awaitServices(t, s, changes)
	}
}

func TestWatchExternalReplacement(t *testing.T) {
	for _, extension := range []string{"json", "spdl"} {
		t.Run(extension, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "policies."+extension)
			content := func(name string) []byte {
				if extension == "spdl" {
					return []byte(fmt.Sprintf("[service.%s]\n[policy]\ngrant user alice read books\n", name))
				}
				return []byte(fmt.Sprintf(`{"services":[{"name":%q,"type":"application"}]}`, name))
			}
			if err := os.WriteFile(filename, content("initial"), 0600); err != nil {
				t.Fatal(err)
			}
			s, changes := watchTestStore(t, filename)
			if _, err := s.ReadPolicyStore(); err != nil {
				t.Fatal(err)
			} // Populate the cache before external edits.
			for _, name := range []string{"first", "second", "third"} {
				if err := os.WriteFile(filename+".tmp", content(name), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filename+".tmp", filename); err != nil {
					t.Fatal(err)
				}
				awaitServices(t, s, changes, name)
			}
			if err := os.WriteFile(filename, content("inplace"), 0600); err != nil {
				t.Fatal(err)
			}
			awaitServices(t, s, changes, "inplace")
		})
	}
}

func TestWatchStopWithUnreadEvent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "policies.json")
	s, _ := watchTestStore(t, filename)
	if _, err := s.ReadPolicyStore(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(`{"services":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		s.rwLock.RLock()
		invalidated := s.cache == nil
		s.rwLock.RUnlock()
		if invalidated {
			break
		}
		select {
		case <-deadline:
			t.Fatal("watcher never processed the write")
		case <-time.After(time.Millisecond):
		}
	}
	// Cleanup must stop the watcher even while its event has no consumer.
}

func TestWatchSymlink(t *testing.T) {
	targetDirectory := t.TempDir()
	target := filepath.Join(targetDirectory, "target.json")
	link := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(target, []byte(`{"services":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	s, changes := watchTestStore(t, link)
	if _, err := s.ReadPolicyStore(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"services":[{"name":"direct"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	awaitServices(t, s, changes, "direct")
	other := filepath.Join(targetDirectory, "other.json")
	if err := os.WriteFile(other, []byte(`{"services":[{"name":"retarget"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(link+".tmp", link); err != nil {
		t.Fatal(err)
	}
	awaitServices(t, s, changes, "retarget")
	if err := os.WriteFile(other, []byte(`{"services":[{"name":"after"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	awaitServices(t, s, changes, "after")
}

func TestWatchSymlinkedDirectory(t *testing.T) {
	directory := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}
	s, changes := watchTestStore(t, filepath.Join(alias, "policies.json"))
	if err := s.CreateService(&pms.Service{Name: "first"}); err != nil {
		t.Fatal(err)
	}
	awaitServices(t, s, changes, "first")
	if err := s.DeleteService("first"); err != nil {
		t.Fatal(err)
	}
	awaitServices(t, s, changes)
}
