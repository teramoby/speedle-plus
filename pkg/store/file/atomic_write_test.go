// Licensed under the Universal Permissive License (UPL) Version 1.0.
package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func TestConcurrentManagersNeverReadPartialPolicyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.json")
	writer, reader := &Store{FileLocation: path}, &Store{FileLocation: path}
	makeSnapshot := func(generation int) *pms.PolicyStore {
		service := &pms.Service{Name: fmt.Sprint(generation)}
		for i := 0; i < 2000; i++ {
			service.Policies = append(service.Policies, &pms.Policy{ID: fmt.Sprint(i), Effect: pms.Grant, Metadata: map[string]string{"generation": service.Name, "padding": strings.Repeat("data", 128)}})
		}
		return &pms.PolicyStore{Services: []*pms.Service{service}}
	}
	if err := writer.WritePolicyStore(makeSnapshot(0)); err != nil {
		t.Fatal(err)
	}
	stop, done, started := make(chan struct{}), make(chan struct{}), make(chan struct{})
	errors := make(chan error, 1)
	var reads atomic.Int64
	go func() {
		defer close(done)
		close(started)
		for {
			select {
			case <-stop:
				return
			default:
			}
			snapshot, err := reader.ReadPolicyStore()
			if err != nil {
				errors <- err
				return
			}
			if len(snapshot.Services) != 1 || len(snapshot.Services[0].Policies) != 2000 {
				errors <- fmt.Errorf("partial snapshot")
				return
			}
			for _, policy := range snapshot.Services[0].Policies {
				if policy.Metadata["generation"] != snapshot.Services[0].Name {
					errors <- fmt.Errorf("mixed generations")
					return
				}
			}
			reads.Add(1)
		}
	}()
	<-started
	for generation := 1; generation <= 10; generation++ {
		if err := writer.WritePolicyStore(makeSnapshot(generation)); err != nil {
			close(stop)
			<-done
			t.Fatal(err)
		}
	}
	close(stop)
	<-done
	select {
	case err := <-errors:
		t.Fatalf("independent ADS reader observed an incomplete PMS write: %v", err)
	default:
	}
	if reads.Load() == 0 {
		t.Fatal("no concurrent complete read")
	}
}

func TestAtomicWritePreservesModeAndSymlink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.json"), filepath.Join(dir, "policy.json")
	if err := os.WriteFile(target, []byte(`{}`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.json", link); err != nil {
		t.Fatal(err)
	}
	s := &Store{FileLocation: link}
	if err := s.WritePolicyStore(&pms.PolicyStore{Services: []*pms.Service{{Name: "updated"}}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("replaced configured symlink: %v", err)
	}
	info, err = os.Stat(target)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("changed policy permissions: %v", err)
	}
	result, err := (&Store{FileLocation: target}).ReadPolicyStore()
	if err != nil || len(result.Services) != 1 || result.Services[0].Name != "updated" {
		t.Fatalf("symlink target was not updated: %v", err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".speedle-policy-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}
