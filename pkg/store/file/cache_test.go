//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"fmt"
	"os"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/store"
)

// TestFileStoreCache verifies that the in-memory cache works correctly:
//  1. After writing policies, reads return the correct data.
//  2. Multiple reads return consistent results (cache hit).
//  3. After a subsequent write, reads return the updated data.
func TestFileStoreCache(t *testing.T) {
	// Use a dedicated file to avoid interfering with other tests.
	config := map[string]interface{}{
		"FileLocation": "./cache_test_ps.json",
	}
	// Clean up after the test.
	defer func() { _ = os.Remove("./cache_test_ps.json") }()
	defer func() { _ = os.Remove("./cache_test_ps.json.tmp") }()

	// Build a new store through the registered builder.
	s, err := store.NewStore("file", config)
	if err != nil {
		t.Fatal("fail to create file store:", err)
	}
	// Ensure Close() is called to release resources.
	defer func() { _ = s.Close() }()

	// ---- Step 1: Write initial policies (3 services) ----
	ps := &pms.PolicyStore{
		Services: []*pms.Service{
			{Name: "svc1"},
			{Name: "svc2"},
			{Name: "svc3"},
		},
	}
	if err := s.WritePolicyStore(ps); err != nil {
		t.Fatal("fail to write policy store:", err)
	}

	// ---- Step 2: Read back via public API ----
	got, err := s.GetServiceNames()
	if err != nil {
		t.Fatal("fail to get service names:", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 services after initial write, got %d", len(got))
	}

	// ---- Step 3: Read again (should hit cache) ----
	got2, err := s.GetServiceNames()
	if err != nil {
		t.Fatal("fail to get service names (second read):", err)
	}
	if len(got2) != 3 {
		t.Fatalf("expected 3 services on cache hit, got %d", len(got2))
	}

	// ---- Step 4: Write updated policies (6 services) ----
	ps2 := &pms.PolicyStore{
		Services: []*pms.Service{
			{Name: "svc1"},
			{Name: "svc2"},
			{Name: "svc3"},
			{Name: "svc4"},
			{Name: "svc5"},
			{Name: "svc6"},
		},
	}
	if err := s.WritePolicyStore(ps2); err != nil {
		t.Fatal("fail to write updated policy store:", err)
	}

	// ---- Step 5: Read again (must get updated data) ----
	got3, err := s.GetServiceNames()
	if err != nil {
		t.Fatal("fail to get service names after update:", err)
	}
	if len(got3) != 6 {
		t.Fatalf("expected 6 services after update, got %d", len(got3))
	}

	for i, svc := range got3 {
		expected := fmt.Sprintf("svc%d", i+1)
		if svc != expected {
			t.Errorf("expected service name %q at index %d, got %q", expected, i, svc)
		}
	}

	// ---- Step 6: Write empty policies (0 services) ----
	ps3 := &pms.PolicyStore{
		Services: []*pms.Service{},
	}
	if err := s.WritePolicyStore(ps3); err != nil {
		t.Fatal("fail to write empty policy store:", err)
	}

	// ---- Step 7: Read after clearing (must return empty) ----
	got4, err := s.GetServiceNames()
	if err != nil {
		t.Fatal("fail to get service names after clear:", err)
	}
	if len(got4) != 0 {
		t.Fatalf("expected 0 services after clear, got %d", len(got4))
	}
}
