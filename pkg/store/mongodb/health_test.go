package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/store"
)

func TestHealth(t *testing.T) {
	if !mongoAvailable {
		t.Skip("MongoDB not available")
	}

	s, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to create mongodb store:", err)
	}
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.(*Store).Health(ctx); err != nil {
		t.Errorf("Health check should succeed against MongoDB, got: %v", err)
	}
}
