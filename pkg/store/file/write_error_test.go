package file

import (
	"path/filepath"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestWriteStoreCreateErrors(t *testing.T) {
	missingParent := filepath.Join(t.TempDir(), "missing", "policies.json")
	s := &Store{FileLocation: missingParent}
	if err := s.WritePolicyStore(&pms.PolicyStore{}); errors.Code(err) != errors.StoreError {
		t.Fatalf("policy store create error = %v", err)
	}
	discovery := &discoverRequestStore{FileLocation: missingParent}
	if err := discovery.WriteDiscoverRequestStore(&StoreContent{}); errors.Code(err) != errors.StoreError {
		t.Fatalf("discovery store create error = %v", err)
	}
}
