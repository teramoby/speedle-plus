package docscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Config loading remains JSON. These existing JSON config samples are also valid
// YAML; decode their maps here to pin scalar types and case-sensitive keys.
func TestYAMLConfigSamples(t *testing.T) {
	for _, name := range []string{
		"pkg/cfg/config_file.json", "pkg/cfg/config_etcd.json",
		"pkg/svcs/pmsrest/config_file.json", "pkg/svcs/pmsrest/config_etcd.json",
		"pkg/svcs/pmsrest/config_mongodb.json", "pkg/store/mongodb/mongoStoreConfig.json",
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root(), name))
			if err != nil {
				t.Fatal(err)
			}
			var expected, actual map[string]any
			if err := json.Unmarshal(raw, &expected); err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(raw, &actual); err != nil {
				t.Fatal(err)
			}
			// JSON normalizes YAML integer scalars without changing their values.
			canonical, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(canonical, &actual); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(expected, actual) {
				t.Fatalf("config keys/values changed: got %#v, want %#v", actual, expected)
			}
		})
	}
}

func TestYAMLv3Semantics(t *testing.T) {
	type config struct {
		Defaults map[string]any `yaml:"defaults"`
		Store    map[string]any `yaml:"storeProps"`
		Text     string         `yaml:"condition"`
	}
	source := `defaults: &defaults
  enabled: true
  interval: 30
  label: on
  quoted: "001"
storeProps:
  <<: *defaults
  enabled: false
  empty: null
condition: |
  request.active == true
  && request.level > 2
`
	var actual config
	if err := yaml.Unmarshal([]byte(source), &actual); err != nil {
		t.Fatal(err)
	}
	defaults := map[string]any{"enabled": true, "interval": 30, "label": "on", "quoted": "001"}
	expected := config{Defaults: defaults, Store: map[string]any{"enabled": false, "interval": 30, "label": "on", "quoted": "001", "empty": nil}, Text: "request.active == true\n&& request.level > 2\n"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("YAML semantics changed: %#v", actual)
	}
	// yaml.v3 rejects duplicate keys and malformed input, while allowing unknown fields.
	for _, source := range []string{"storeProps: [", "storeProps: {}\nstoreProps: {}\n"} {
		if err := yaml.Unmarshal([]byte(source), &actual); err == nil {
			t.Fatalf("accepted invalid YAML %q", source)
		}
	}
	if err := yaml.Unmarshal([]byte("unknown: ignored\n"), &actual); err != nil {
		t.Fatal(err)
	}
}
