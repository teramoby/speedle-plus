package mongodb_test

import (
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/store/mongodb"
	"github.com/teramoby/speedle-plus/pkg/suid"
	"github.com/teramoby/speedle-plus/testutil/storetest"
	"os"
	"testing"
)

func TestMongoDBConformance(t *testing.T) {
	uri := os.Getenv("SPEEDLE_MONGO_URI")
	if uri == "" {
		if os.Getenv("SPEEDLE_REQUIRE_MONGO") == "1" {
			t.Fatal("SPEEDLE_MONGO_URI is required")
		}
		t.Skip("set SPEEDLE_MONGO_URI to a disposable replica set")
	}
	storetest.RunConformance(t, func(t *testing.T) pms.PolicyStoreManager {
		manager, err := (mongodb.MongoStoreBuilder{}).NewStore(map[string]interface{}{
			mongodb.MongoURIKey: uri, mongodb.MongoDatabaseNameKey: "speedle_test_contract_" + suid.New().String(),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = manager.(*mongodb.Store).Close() })
		return manager
	})
}
