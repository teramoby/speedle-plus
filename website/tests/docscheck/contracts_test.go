package docscheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsrest"
	"gopkg.in/yaml.v3"
)

type schema map[string]interface{}
type response struct {
	Schema schema `yaml:"schema"`
}
type operation struct {
	Responses map[string]response `yaml:"responses"`
}
type spec struct {
	BasePath    string                          `yaml:"basePath"`
	Paths       map[string]map[string]operation `yaml:"paths"`
	Definitions map[string]schema               `yaml:"definitions"`
}

func root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}
func load(t *testing.T, name string) spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root(), "docs/hugo/static/swagger", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var s spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
func validate(s schema, v interface{}, defs map[string]schema, path string) error {
	if name, ok := s["$ref"].(string); ok {
		target, ok := defs[strings.TrimPrefix(name, "#/definitions/")]
		if !ok {
			return fmt.Errorf("%s: missing %s", path, name)
		}
		return validate(target, v, defs, path)
	}
	if v == nil {
		if nullable, _ := s["x-nullable"].(bool); nullable || s["type"] == nil {
			return nil
		}
		return fmt.Errorf("%s: null for %v", path, s["type"])
	}
	switch s["type"] {
	case "object":
		value, ok := v.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s: wanted object, got %T", path, v)
		}
		if required, ok := s["required"].([]interface{}); ok {
			for _, key := range required {
				if _, ok := value[key.(string)]; !ok {
					return fmt.Errorf("%s: missing %s", path, key)
				}
			}
		}
		properties, _ := s["properties"].(map[string]interface{})
		for key, field := range value {
			if property, ok := properties[key].(map[string]interface{}); ok {
				if err := validate(property, field, defs, path+"."+key); err != nil {
					return err
				}
			} else if additional, ok := s["additionalProperties"].(map[string]interface{}); ok {
				if err := validate(additional, field, defs, path+"."+key); err != nil {
					return err
				}
			}
		}
	case "array":
		value, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("%s: wanted array, got %T", path, v)
		}
		item, _ := s["items"].(map[string]interface{})
		for i, field := range value {
			if err := validate(item, field, defs, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%s: wanted string, got %T", path, v)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s: wanted bool, got %T", path, v)
		}
	case "integer", "number":
		n, ok := v.(float64)
		if !ok || s["type"] == "integer" && n != math.Trunc(n) {
			return fmt.Errorf("%s: wanted %v, got %v", path, s["type"], v)
		}
	}
	if values, ok := s["enum"].([]interface{}); ok {
		found := false
		for _, value := range values {
			found = found || reflect.DeepEqual(value, v)
		}
		if !found {
			return fmt.Errorf("%s: invalid enum %v", path, v)
		}
	}
	return nil
}
func compareRoutes(t *testing.T, s spec, router *mux.Router) {
	t.Helper()
	actual := map[string]bool{}
	if err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, err := route.GetPathTemplate()
		if err != nil {
			return err
		}
		methods, err := route.GetMethods()
		if err != nil {
			return err
		}
		for _, method := range methods {
			actual[method+" "+path] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, ops := range s.Paths {
		for method := range ops {
			documented[strings.ToUpper(method)+" "+s.BasePath+path] = true
		}
	}
	if !reflect.DeepEqual(actual, documented) {
		t.Fatalf("route drift: actual=%v documented=%v", actual, documented)
	}
}
func call(t *testing.T, server *httptest.Server, s spec, method, path, template string, body interface{}, status int) map[string]interface{} {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, server.URL+s.BasePath+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != status {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, res.StatusCode, status, raw)
	}
	op := s.Paths[template][strings.ToLower(method)]
	contract, ok := op.Responses[fmt.Sprint(status)]
	if !ok {
		t.Fatalf("undocumented status %d for %s %s", status, method, template)
	}
	if status == 204 {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if err := validate(contract.Schema, value, s.Definitions, method+" "+template); err != nil {
		t.Fatalf("%v\nresponse=%s", err, raw)
	}
	object, _ := value.(map[string]interface{})
	return object
}
func TestPublishedContractsAgainstNativeServices(t *testing.T) {
	for _, locale := range []string{"", ".zh"} {
		t.Run(locale, func(t *testing.T) {
			pm, ad := load(t, "policy-manage"+locale), load(t, "policy-check"+locale)
			services := []interface{}{}
			for _, name := range []string{"bookstore", "idd", "diagnosis"} {
				raw, err := os.ReadFile(filepath.Join(root(), "website/tests/examples", name+".json"))
				if err != nil {
					t.Fatal(err)
				}
				var service interface{}
				if err := json.Unmarshal(raw, &service); err != nil {
					t.Fatal(err)
				}
				services = append(services, service)
			}
			raw, _ := json.Marshal(map[string]interface{}{"services": services})
			loc := filepath.Join(t.TempDir(), "policies.json")
			if err := os.WriteFile(loc, raw, 0600); err != nil {
				t.Fatal(err)
			}
			ps, err := store.NewStore("file", map[string]interface{}{"FileLocation": loc})
			if err != nil {
				t.Fatal(err)
			}
			evaluator, err := eval.NewWithStore(&cfg.Config{}, ps)
			if err != nil {
				t.Fatal(err)
			}
			pRouter, err := pmsrest.NewRouter(ps)
			if err != nil {
				t.Fatal(err)
			}
			aRouter, err := adsrest.NewRouter(evaluator)
			if err != nil {
				t.Fatal(err)
			}
			compareRoutes(t, pm, pRouter)
			compareRoutes(t, ad, aRouter)
			pms := httptest.NewServer(pRouter)
			defer pms.Close()
			ads := httptest.NewServer(aRouter)
			defer ads.Close()
			request := map[string]interface{}{"subject": map[string]interface{}{"principals": []interface{}{map[string]interface{}{"type": "user", "name": "Alan"}}}, "serviceName": "onlineBookStore", "resource": "/books/HarryPotter", "action": "download", "attributes": []interface{}{map[string]interface{}{"name": "clearance", "type": "numeric", "value": 4}}}
			for _, path := range []string{"/is-allowed", "/all-granted-roles", "/all-granted-permissions", "/diagnose", "/discover"} {
				call(t, ads, ad, "POST", path, path, request, 200)
			}
			// Exercise all 27 PMS routes, including response shapes that used to drift.
			for _, path := range []string{"/discover-request", "/discover-request/onlineBookStore", "/discover-policy", "/discover-policy/onlineBookStore"} {
				template := strings.ReplaceAll(path, "onlineBookStore", "{serviceName}")
				call(t, pms, pm, "GET", path, template, nil, 200)
			}
			for _, path := range []string{"/discover-request/onlineBookStore", "/discover-request"} {
				template := strings.ReplaceAll(path, "onlineBookStore", "{serviceName}")
				call(t, pms, pm, "DELETE", path, template, nil, 204)
			}
			function := map[string]interface{}{"name": "docsFunction", "funcURL": "http://127.0.0.1:23456/func/isValid", "resultCachable": false}
			call(t, pms, pm, "POST", "/function", "/function", function, 201)
			call(t, pms, pm, "GET", "/function", "/function", nil, 200)
			call(t, pms, pm, "GET", "/function/docsFunction", "/function/{functionName}", nil, 200)
			call(t, pms, pm, "DELETE", "/function/docsFunction", "/function/{functionName}", nil, 204)
			call(t, pms, pm, "DELETE", "/function", "/function", nil, 204)
			call(t, pms, pm, "POST", "/service", "/service", map[string]interface{}{"name": "crud", "type": "application"}, 201)
			custom := call(t, pms, pm, "POST", "/service", "/service", map[string]interface{}{"name": "custom", "type": "custom-service"}, 201)
			if custom["type"] != "custom-service" {
				t.Fatalf("custom service type was not preserved: %v", custom)
			}
			call(t, pms, pm, "GET", "/service", "/service", nil, 200)
			call(t, pms, pm, "GET", "/service/crud", "/service/{serviceName}", nil, 200)
			for _, kind := range []string{"policy", "role-policy"} {
				body := map[string]interface{}{"name": "test", "effect": "grant", "principals": []interface{}{"user:alice"}, "roles": []interface{}{"reader"}}
				if kind == "policy" {
					body = map[string]interface{}{"name": "test", "effect": "grant", "principals": []interface{}{[]interface{}{"user:alice"}}, "permissions": []interface{}{map[string]interface{}{"resource": "book", "actions": []interface{}{"read"}}}}
				}
				template := "/service/{serviceName}/" + kind
				path := "/service/crud/" + kind
				created := call(t, pms, pm, "POST", path, template, body, 201)
				id, ok := created["id"].(string)
				if !ok || id == "" {
					t.Fatal("missing string ID", created)
				}
				idName := "policyID"
				if kind == "role-policy" {
					idName = "rolePolicyID"
				}
				single := template + "/{" + idName + "}"
				call(t, pms, pm, "GET", path, template, nil, 200)
				call(t, pms, pm, "GET", path+"/"+id, single, nil, 200)
				call(t, pms, pm, "DELETE", path+"/"+id, single, nil, 204)
				call(t, pms, pm, "DELETE", path, template, nil, 204)
			}
			call(t, pms, pm, "GET", "/policy-counts", "/policy-counts", nil, 200)
			call(t, pms, pm, "DELETE", "/service/crud", "/service/{serviceName}", nil, 204)
			call(t, pms, pm, "DELETE", "/service", "/service", nil, 204)
		})
	}
}
func TestAsserterResponseContract(t *testing.T) {
	for _, locale := range []string{"", ".zh"} {
		s := load(t, "asserter"+locale)
		result := assertion.AssertResponse{Principals: []*ads.Principal{{Type: "user", Name: "user1", IDD: "github"}}, Attributes: map[string]interface{}{"clearance": float64(4), "active": true}, ErrCode: 0}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var data interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		if err := validate(s.Definitions["Subject"], data, s.Definitions, "assertion response"); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-token") != "githubtoken" || r.Header.Get("x-idp") != "github" {
				t.Error("assertion headers missing")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(raw)
		}))
		client, err := assertion.NewAsserter(&assertion.AsserterConfig{Endpoint: server.URL}, nil)
		if err != nil {
			t.Fatal(err)
		}
		asserted, err := client.AssertToken("githubtoken", "github", "", nil)
		server.Close()
		if err != nil || asserted.Attributes["clearance"] != float64(4) {
			t.Fatalf("assertion response incompatible: %v %v", asserted, err)
		}
	}
}
func TestLocalizedSchemasHaveIdenticalContracts(t *testing.T) {
	var strip func(interface{}) interface{}
	strip = func(node interface{}) interface{} {
		switch node := node.(type) {
		case map[string]interface{}:
			result := map[string]interface{}{}
			for key, value := range node {
				if _, text := value.(string); text && (key == "description" || key == "summary" || key == "title") {
					continue
				}
				result[key] = strip(value)
			}
			return result
		case []interface{}:
			result := make([]interface{}, len(node))
			for i, value := range node {
				result[i] = strip(value)
			}
			return result
		default:
			return node
		}
	}
	for _, name := range []string{"policy-manage", "policy-check", "asserter"} {
		var forms []interface{}
		for _, suffix := range []string{"", ".zh"} {
			data, err := os.ReadFile(filepath.Join(root(), "docs/hugo/static/swagger", name+suffix+".yaml"))
			if err != nil {
				t.Fatal(err)
			}
			var form interface{}
			if err := yaml.Unmarshal(data, &form); err != nil {
				t.Fatal(err)
			}
			forms = append(forms, strip(form))
		}
		if !reflect.DeepEqual(forms[0], forms[1]) {
			t.Errorf("%s: translated API contract drift", name)
		}
	}
}
