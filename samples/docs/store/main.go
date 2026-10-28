package main

import (
	"fmt"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
	"os"
	"path/filepath"
)

// This example delegates to the file backend to demonstrate registration.
// A new backend must implement the complete PolicyStoreManager contract.
type Builder struct{ file.FileStoreBuilder }

func main() {
	dir, err := os.MkdirTemp("", "speedle-store-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store.Register("myfile", Builder{})
	policies, err := store.NewStore("myfile", map[string]interface{}{"FileLocation": filepath.Join(dir, "policies.json")})
	if err != nil {
		log.Fatal(err)
	}
	defer policies.Close()
	snapshot, err := policies.ReadPolicyStore()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Registered myfile store; services=%d\n", len(snapshot.Services))
}
