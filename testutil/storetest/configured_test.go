package storetest

import "testing"

func TestFixtureBackendConfiguration(t *testing.T) {
	t.Setenv("STORE_TYPE", "")
	t.Setenv("SPEEDLE_TEST_STORE_PROPS", "")
	configuration, err := FixtureConfig("fixture.json")
	if err != nil || configuration.StoreType != "file" || configuration.StoreProps["FileLocation"] != "fixture.json" {
		t.Fatal(configuration, err)
	}
	for _, backend := range []string{"etcd", "mongodb", "mysql"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("STORE_TYPE", backend)
			t.Setenv("SPEEDLE_TEST_STORE_PROPS", `{"MongoURI":"mongodb://configured:27018","MongoDatabase":"speedle_test_custom","EtcdEndpoint":"127.0.0.1:12379","EtcdKeyPrefix":"/custom","MySQLDSN":"u:p@tcp(localhost)/speedle_test"}`)
			first, err := FixtureConfig("")
			if err != nil {
				t.Fatal(err)
			}
			second, err := FixtureConfig("")
			if err != nil {
				t.Fatal(err)
			}
			if first.StoreType != backend {
				t.Fatal("wrong backend")
			}
			switch backend {
			case "mongodb":
				if first.StoreProps["MongoURI"] != "mongodb://configured:27018" || first.StoreProps["MongoDatabase"] == second.StoreProps["MongoDatabase"] {
					t.Fatal("connection configuration or isolation lost")
				}
			case "etcd":
				if first.StoreProps["EtcdEndpoint"] != "127.0.0.1:12379" || first.StoreProps["EtcdKeyPrefix"] == second.StoreProps["EtcdKeyPrefix"] {
					t.Fatal("connection configuration or isolation lost")
				}
			case "mysql":
				if first.StoreProps["MySQLDSN"] != "u:p@tcp(localhost)/speedle_test" {
					t.Fatal("MySQL connection configuration lost")
				}
			}
		})
	}
}

func TestFixtureRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("STORE_TYPE", "mongodb")
	t.Setenv("SPEEDLE_MONGO_URI", "")
	for _, properties := range []string{"", "null", "{", `{"MongoURI":123}`, `{"MongoURI":"mongodb://localhost","MongoDatabase":"production"}`} {
		t.Setenv("SPEEDLE_TEST_STORE_PROPS", properties)
		if _, err := FixtureConfig(""); err == nil {
			t.Fatalf("accepted invalid properties %q", properties)
		}
	}
	t.Setenv("STORE_TYPE", "unknown")
	t.Setenv("SPEEDLE_TEST_STORE_PROPS", "")
	if _, err := FixtureConfig(""); err == nil {
		t.Fatal("unknown backend fell back to file")
	}
}
