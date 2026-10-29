//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package suid

import (
	"sync"
	"testing"
)

func TestSUIDLength(t *testing.T) {
	suid := New()
	suidStr := suid.String()
	t.Logf("SUID is %s.", suidStr)
	if len(suidStr) != 20 {
		t.Fatalf("SUID length is wrong, expected 20 but returned %d.", len(suidStr))
	}
}

func TestNoDuplicate(t *testing.T) {
	idmap := make(map[string]bool)
	// Test 1M times, test if there is any duplicated items
	for i := 0; i < 1000*1000; i++ {
		suid := New()
		str := suid.String()
		_, found := idmap[str]
		if found {
			t.Fatalf("SUID \"%s\" is duplicated.", str)
		}
		idmap[str] = true
	}
}

func TestConcurrentNew(t *testing.T) {
	const workers, perWorker = 8, 2000
	ids := make(chan string, workers*perWorker)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				ids <- New().String()
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]bool)
	for id := range ids {
		if seen[id] {
			t.Fatalf("SUID %q is duplicated", id)
		}
		seen[id] = true
	}
}

func TestParseSUID(t *testing.T) {
	id := New().String()
	cases := map[string]string{
		"":                    "",
		id:                    id,
		"policy/" + id + "/x": id,
		"short":               "short",
	}
	for src, want := range cases {
		if got := ParseSUID(src); got != want {
			t.Errorf("ParseSUID(%q) = %q, want %q", src, got, want)
		}
	}
}
